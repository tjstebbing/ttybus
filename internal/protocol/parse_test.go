package protocol

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseTable(t *testing.T) {
	tests := []struct {
		in      string
		want    Line
		wantErr error
	}{
		{in: "", wantErr: ErrEmpty},
		{in: "   ", wantErr: ErrEmpty},
		{in: "HELLO", want: Line{Verb: VerbHello}},
		{
			in:   "HELLO name=yazi pid=4123 pane=%3 cwd=/code/foo",
			want: Line{Verb: VerbHello, Args: []string{"name=yazi", "pid=4123", "pane=%3", "cwd=/code/foo"}, Rest: "name=yazi pid=4123 pane=%3 cwd=/code/foo"},
		},
		{
			in:   "HELLO name=ttybus pid=1 id=cli-9",
			want: Line{Verb: VerbHello, Args: []string{"name=ttybus", "pid=1", "id=cli-9"}, Rest: "name=ttybus pid=1 id=cli-9"},
		},
		{in: "SUB files", want: Line{Verb: VerbSub, Args: []string{"files"}, Rest: "files"}},
		{in: "UNSUB files", want: Line{Verb: VerbUnsub, Args: []string{"files"}, Rest: "files"}},
		{in: "PUB files", want: Line{Verb: VerbPub, Args: []string{"files"}, Rest: "files"}},
		{in: "PUB files main.go", want: Line{Verb: VerbPub, Args: []string{"files", "main.go"}, Rest: "files main.go"}},
		{
			in:   `PUB files {"path":"main.go","line":12}`,
			want: Line{Verb: VerbPub, Args: []string{"files", `{"path":"main.go","line":12}`}, Rest: `files {"path":"main.go","line":12}`},
		},
		{in: "SEND yazi-1 files main.go", want: Line{Verb: VerbSend, Args: []string{"yazi-1", "files", "main.go"}, Rest: "yazi-1 files main.go"}},
		{in: "LIST", want: Line{Verb: VerbList}},
		{in: "OK", want: Line{Verb: VerbOK}},
		{in: "OK id=cli-9", want: Line{Verb: VerbOK, Args: []string{"id=cli-9"}, Rest: "id=cli-9"}},
		{in: "OK n=3", want: Line{Verb: VerbOK, Args: []string{"n=3"}, Rest: "n=3"}},
		{in: "MSG files main.go", want: Line{Verb: VerbMsg, Args: []string{"files", "main.go"}, Rest: "files main.go"}},
		{in: "ERR PEER no such id", want: Line{Verb: VerbErr, Args: []string{"PEER", "no", "such", "id"}, Rest: "PEER no such id"}},
		{
			in:   "PEER id=yazi-1 name=yazi pid=4123 pane=%3 cwd=/code/foo subs=files,build",
			want: Line{Verb: VerbPeer, Args: []string{"id=yazi-1", "name=yazi", "pid=4123", "pane=%3", "cwd=/code/foo", "subs=files,build"}, Rest: "id=yazi-1 name=yazi pid=4123 pane=%3 cwd=/code/foo subs=files,build"},
		},
		{in: "hello", wantErr: ErrLine},
		{in: "PUB", want: Line{Verb: VerbPub}},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := Parse(tt.in)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err=%v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Verb != tt.want.Verb || got.Rest != tt.want.Rest {
				t.Fatalf("verb/rest got %+v want %+v", got, tt.want)
			}
			if strings.Join(got.Args, "|") != strings.Join(tt.want.Args, "|") {
				t.Fatalf("args got %q want %q", got.Args, tt.want.Args)
			}
		})
	}
}

func TestAfterPayload(t *testing.T) {
	l, err := Parse("PUB files hello  world")
	if err != nil {
		t.Fatal(err)
	}
	if g := l.After(1); g != "hello  world" {
		t.Fatalf("payload %q", g)
	}
	l, _ = Parse("PUB files")
	if g := l.After(1); g != "" {
		t.Fatalf("empty payload %q", g)
	}
}

func TestParseHelloID(t *testing.T) {
	l, err := Parse("HELLO id=cli-9 name=ttybus")
	if err != nil {
		t.Fatal(err)
	}
	h, err := l.ParseHello()
	if err != nil {
		t.Fatal(err)
	}
	if h.ID != "cli-9" || h.Name != "ttybus" {
		t.Fatalf("%+v", h)
	}
	l, _ = Parse("HELLO id=bad!id")
	if _, err := l.ParseHello(); !errors.Is(err, ErrID) {
		t.Fatalf("expected illegal id, got %v", err)
	}
}

func TestValidChannel(t *testing.T) {
	ok := []string{"files", "build.ok", "editor/open", "a", "A1_2-3.z"}
	for _, s := range ok {
		if err := ValidChannel(s); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	bad := []string{"", "1files", "-x", "has space", "x" + strings.Repeat("a", 64), "foo$bar", "foo:bar"}
	for _, s := range bad {
		if err := ValidChannel(s); err == nil {
			t.Errorf("%q: accepted", s)
		}
	}
}

func TestFormatRoundTrip(t *testing.T) {
	lines := []string{
		FormatHello(Hello{Name: "yazi", PID: "1", ID: "a1"}),
		FormatSub("files"),
		FormatPub("files", "main.go"),
		FormatPub("files", ""),
		FormatMsg("files", "hello world"),
		FormatSend("yazi-1", "files", "x"),
		FormatOK("id=cli-9"),
		FormatErr("PEER", "no such id"),
		FormatList(),
		FormatPeer(Hello{ID: "yazi-1", Name: "yazi", Subs: []string{"files", "build"}}),
	}
	for _, s := range lines {
		l, err := Parse(s)
		if err != nil {
			t.Fatalf("parse %q: %v", s, err)
		}
		if !KnownVerb(l.Verb) {
			t.Fatalf("unknown verb in %q", s)
		}
	}
}

func TestTestdataGoldens(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "protocol", "hello.txt")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for i, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		l, err := Parse(line)
		if err != nil {
			t.Errorf("line %d %q: %v", i+1, line, err)
			continue
		}
		if !KnownVerb(l.Verb) {
			t.Errorf("line %d %q: unknown verb", i+1, line)
		}
	}
}

func TestReadWriteLine(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteLine(&buf, "HELLO name=x"); err != nil {
		t.Fatal(err)
	}
	if err := WriteLine(&buf, "PUB files hi"); err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(&buf)
	s, err := ReadLine(r)
	if err != nil || s != "HELLO name=x" {
		t.Fatalf("got %q %v", s, err)
	}
	s, err = ReadLine(r)
	if err != nil || s != "PUB files hi" {
		t.Fatalf("got %q %v", s, err)
	}
	if _, err := ReadLine(r); !errors.Is(err, io.EOF) {
		t.Fatalf("eof %v", err)
	}
}

func TestReadLineCRLF(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("HELLO\r\nSUB files\n"))
	s, err := ReadLine(r)
	if err != nil || s != "HELLO" {
		t.Fatalf("%q %v", s, err)
	}
	s, err = ReadLine(r)
	if err != nil || s != "SUB files" {
		t.Fatalf("%q %v", s, err)
	}
}

func TestReadLineTooLong(t *testing.T) {
	big := strings.Repeat("a", MaxLine) + "\n"
	r := bufio.NewReader(strings.NewReader(big))
	_, err := ReadLine(r)
	if !errors.Is(err, ErrLineLen) {
		t.Fatalf("err=%v", err)
	}
}

func TestParseOversize(t *testing.T) {
	_, err := Parse(strings.Repeat("A", MaxLine))
	if !errors.Is(err, ErrLineLen) {
		t.Fatalf("err=%v", err)
	}
}

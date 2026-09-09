package plumb

import (
	"strings"
	"testing"
)

func TestMatchPub(t *testing.T) {
	rules, err := ParseRules(strings.NewReader(`
# comment
match ^([^\s:]+):(\d+)$
  pub editor.open

match ^https?://
  exec open $0

match \.(go|rs|py)$
  pub editor.open
`))
	if err != nil {
		t.Fatal(err)
	}
	a, ok := Match(rules, "main.go:42")
	if !ok || a.Kind != "pub" || a.Channel != "editor.open" || a.Payload != "main.go:42" {
		t.Fatalf("%+v ok=%v", a, ok)
	}
	a, ok = Match(rules, "https://example.com/x")
	if !ok || a.Kind != "exec" {
		t.Fatalf("%+v", a)
	}
	if len(a.Argv) != 2 || a.Argv[0] != "open" || a.Argv[1] != "https://example.com/x" {
		t.Fatalf("%q", a.Argv)
	}
	a, ok = Match(rules, "foo.rs")
	if !ok || a.Kind != "pub" {
		t.Fatalf("%+v", a)
	}
	if _, ok := Match(rules, "nope"); ok {
		t.Fatal("expected no match")
	}
}

func TestParseError(t *testing.T) {
	_, err := ParseRules(strings.NewReader("  pub x\n"))
	if err == nil {
		t.Fatal("expected error")
	}
}

package daemon

import (
	"bufio"
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/tjstebbing/ttybus/internal/protocol"
)

type testPeer struct {
	c net.Conn
	r *bufio.Reader
}

func pair(t *testing.T, hub *Hub) *testPeer {
	t.Helper()
	a, b := net.Pipe()
	t.Cleanup(func() { _ = a.Close(); _ = b.Close() })
	go newConn(b, hub).serve()
	return &testPeer{c: a, r: bufio.NewReader(a)}
}

func (p *testPeer) send(t *testing.T, line string) {
	t.Helper()
	if err := protocol.WriteLine(p.c, line); err != nil {
		t.Fatal(err)
	}
}

func (p *testPeer) read(t *testing.T) protocol.Line {
	t.Helper()
	p.c.SetReadDeadline(time.Now().Add(2 * time.Second))
	s, err := protocol.ReadLine(p.r)
	if err != nil {
		t.Fatal(err)
	}
	l, err := protocol.Parse(s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return l
}

func (p *testPeer) readSkipMsg(t *testing.T) protocol.Line {
	t.Helper()
	for {
		l := p.read(t)
		if l.Verb != protocol.VerbMsg && l.Verb != protocol.VerbErr {
			return l
		}
	}
}

func (p *testPeer) hello(t *testing.T, line string) string {
	t.Helper()
	p.send(t, line)
	l := p.read(t)
	if l.Verb != protocol.VerbOK {
		t.Fatalf("hello: %s %s", l.Verb, l.Raw)
	}
	h, err := l.ParseHello()
	if err != nil {
		t.Fatal(err)
	}
	return h.ID
}

func TestHelloRequired(t *testing.T) {
	hub := newHub()
	p := pair(t, hub)
	p.send(t, "SUB files")
	l := p.read(t)
	if l.Verb != protocol.VerbErr || l.Token(0) != "HELLO" {
		t.Fatalf("got %s", l.Raw)
	}
}

func TestPubFanout(t *testing.T) {
	hub := newHub()
	a := pair(t, hub)
	b := pair(t, hub)
	c := pair(t, hub)
	a.hello(t, "HELLO name=a")
	b.hello(t, "HELLO name=b")
	c.hello(t, "HELLO name=c")
	a.send(t, "SUB files")
	if v := a.read(t); v.Verb != protocol.VerbOK {
		t.Fatal(v.Raw)
	}
	b.send(t, "SUB files")
	if v := b.read(t); v.Verb != protocol.VerbOK {
		t.Fatal(v.Raw)
	}
	c.send(t, "PUB files hello")
	ok := c.read(t)
	if ok.Verb != protocol.VerbOK || ok.Rest != "n=2" {
		t.Fatalf("pub reply %s", ok.Raw)
	}
	ma := a.read(t)
	mb := b.read(t)
	if ma.Raw != "MSG files hello" || mb.Raw != "MSG files hello" {
		t.Fatalf("msgs %q %q", ma.Raw, mb.Raw)
	}
}

func TestPubZeroSubs(t *testing.T) {
	hub := newHub()
	p := pair(t, hub)
	p.hello(t, "HELLO")
	p.send(t, "PUB files x")
	ok := p.read(t)
	if ok.Rest != "n=0" {
		t.Fatalf("%s", ok.Raw)
	}
}

func TestSelfSubscribe(t *testing.T) {
	hub := newHub()
	p := pair(t, hub)
	p.hello(t, "HELLO")
	p.send(t, "SUB files")
	p.read(t)
	p.send(t, "PUB files me")
	// MSG may arrive before or after OK
	sawOK, sawMSG := false, false
	for i := 0; i < 2; i++ {
		l := p.read(t)
		switch l.Verb {
		case protocol.VerbOK:
			sawOK = true
		case protocol.VerbMsg:
			if l.After(1) != "me" {
				t.Fatalf("%s", l.Raw)
			}
			sawMSG = true
		default:
			t.Fatalf("%s", l.Raw)
		}
	}
	if !sawOK || !sawMSG {
		t.Fatalf("ok=%v msg=%v", sawOK, sawMSG)
	}
}

func TestSendUnicast(t *testing.T) {
	hub := newHub()
	a := pair(t, hub)
	b := pair(t, hub)
	idA := a.hello(t, "HELLO id=peer-a")
	b.hello(t, "HELLO id=peer-b")
	if idA != "peer-a" {
		t.Fatal(idA)
	}
	b.send(t, "SEND peer-a files secret")
	ok := b.read(t)
	if ok.Verb != protocol.VerbOK {
		t.Fatal(ok.Raw)
	}
	msg := a.read(t)
	if msg.Raw != "MSG files secret" {
		t.Fatalf("%s", msg.Raw)
	}
}

func TestSendMissingPeer(t *testing.T) {
	hub := newHub()
	p := pair(t, hub)
	p.hello(t, "HELLO")
	p.send(t, "SEND nobody files x")
	l := p.read(t)
	if l.Verb != protocol.VerbErr || l.Token(0) != "PEER" {
		t.Fatalf("%s", l.Raw)
	}
}

func TestDuplicateID(t *testing.T) {
	hub := newHub()
	a := pair(t, hub)
	b := pair(t, hub)
	a.hello(t, "HELLO id=same")
	b.send(t, "HELLO id=same")
	l := b.read(t)
	if l.Verb != protocol.VerbErr || l.Token(0) != "ID" {
		t.Fatalf("%s", l.Raw)
	}
}

func TestDisconnectUnsub(t *testing.T) {
	hub := newHub()
	a := pair(t, hub)
	b := pair(t, hub)
	a.hello(t, "HELLO")
	b.hello(t, "HELLO")
	a.send(t, "SUB files")
	a.read(t)
	_ = a.c.Close()
	time.Sleep(50 * time.Millisecond)
	b.send(t, "PUB files gone")
	ok := b.read(t)
	if ok.Rest != "n=0" {
		t.Fatalf("want n=0 after disconnect, got %s", ok.Raw)
	}
}

func TestListPeers(t *testing.T) {
	hub := newHub()
	a := pair(t, hub)
	b := pair(t, hub)
	a.hello(t, "HELLO name=alpha id=a1")
	b.hello(t, "HELLO name=beta id=b1")
	a.send(t, "SUB files")
	a.read(t)
	a.send(t, "LIST")
	var peers int
	for {
		l := a.read(t)
		switch l.Verb {
		case protocol.VerbPeer:
			peers++
		case protocol.VerbOK:
			if peers != 2 {
				t.Fatalf("peers=%d ok=%s", peers, l.Raw)
			}
			return
		default:
			t.Fatalf("%s", l.Raw)
		}
	}
}

func TestIllegalChannel(t *testing.T) {
	hub := newHub()
	p := pair(t, hub)
	p.hello(t, "HELLO")
	p.send(t, "SUB 1bad")
	l := p.read(t)
	if l.Verb != protocol.VerbErr || l.Token(0) != "CHAN" {
		t.Fatalf("%s", l.Raw)
	}
}

func TestUnknownVerb(t *testing.T) {
	hub := newHub()
	p := pair(t, hub)
	p.hello(t, "HELLO")
	p.send(t, "FOO bar")
	l := p.read(t)
	if l.Verb != protocol.VerbErr || l.Token(0) != "VERB" {
		t.Fatalf("%s", l.Raw)
	}
}

func TestSlowClientDoesNotBlockPublisher(t *testing.T) {
	hub := newHub()
	fast := pair(t, hub)
	slow := pair(t, hub)
	pub := pair(t, hub)
	fast.hello(t, "HELLO name=fast")
	slow.hello(t, "HELLO name=slow")
	pub.hello(t, "HELLO name=pub")
	fast.send(t, "SUB files")
	fast.read(t)
	slow.send(t, "SUB files")
	slow.read(t)

	const n = outQueue + 50
	msgs := make(chan string, n+16)
	go func() {
		for {
			_ = fast.c.SetReadDeadline(time.Now().Add(4 * time.Second))
			s, err := protocol.ReadLine(fast.r)
			if err != nil {
				close(msgs)
				return
			}
			msgs <- s
		}
	}()

	// Flood while slow is not reading. Publisher must not block.
	deadline := time.Now().Add(3 * time.Second)
	for i := 0; i < n; i++ {
		pub.send(t, "PUB files x")
		pub.c.SetReadDeadline(deadline)
		l := pub.readSkipMsg(t)
		if l.Verb != protocol.VerbOK {
			t.Fatalf("pub %d: %s", i, l.Raw)
		}
	}

	got := 0
	timeout := time.After(3 * time.Second)
	for got < n {
		select {
		case s, ok := <-msgs:
			if !ok {
				t.Fatalf("fast closed after %d of %d", got, n)
			}
			if strings.HasPrefix(s, "MSG ") {
				got++
			}
		case <-timeout:
			t.Fatalf("fast got %d of %d", got, n)
		}
	}
}

func TestServeListen(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/bus.sock"
	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() { errc <- Serve(ctx, path) }()
	waitUnix(t, path)
	c, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	if err := protocol.WriteLine(c, "HELLO name=nc"); err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(c)
	s, err := protocol.ReadLine(r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(s, "OK id=") {
		t.Fatalf("%q", s)
	}
	_ = c.Close()
	cancel()
	select {
	case err := <-errc:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("serve did not exit")
	}
}

func waitUnix(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.Dial("unix", path)
		if err == nil {
			_ = c.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("socket %s not up", path)
}

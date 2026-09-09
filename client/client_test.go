package ttybus

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/tjstebbing/ttybus/internal/daemon"
)

func startBus(t *testing.T) string {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "bus.sock")
	ctx, cancel := context.WithCancel(context.Background())
	go daemon.Serve(ctx, sock)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.Dial("unix", sock)
		if err == nil {
			_ = c.Close()
			t.Cleanup(cancel)
			return sock
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("bus not up")
	return ""
}

func TestClientPubSubSend(t *testing.T) {
	sock := startBus(t)
	a, err := Dial(sock)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if err := a.Hello(Info{Name: "a", ID: "peer-a"}); err != nil {
		t.Fatal(err)
	}
	ch, err := a.Sub("files")
	if err != nil {
		t.Fatal(err)
	}

	b, err := Dial(sock)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if err := b.Hello(Info{Name: "b"}); err != nil {
		t.Fatal(err)
	}
	if err := b.Pub("files", "hello"); err != nil {
		t.Fatal(err)
	}
	select {
	case s := <-ch:
		if s != "hello" {
			t.Fatalf("%q", s)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no pub")
	}

	if err := b.Send("peer-a", "files", "unicast"); err != nil {
		t.Fatal(err)
	}
	select {
	case s := <-ch:
		if s != "unicast" {
			t.Fatalf("%q", s)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no send")
	}

	peers, err := a.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) < 2 {
		t.Fatalf("peers %d", len(peers))
	}
}

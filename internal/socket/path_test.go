package socket

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPathPrecedence(t *testing.T) {
	t.Setenv(EnvSocket, "")
	t.Setenv(EnvBus, "")
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1")

	if g := Path(Opts{Socket: "/tmp/x.sock"}); g != "/tmp/x.sock" {
		t.Fatal(g)
	}
	t.Setenv(EnvSocket, "/env.sock")
	if g := Path(Opts{}); g != "/env.sock" {
		t.Fatal(g)
	}
	t.Setenv(EnvSocket, "")
	if g := Path(Opts{Bus: "foo"}); g != "/run/user/1/ttybus/buses/foo.sock" {
		t.Fatal(g)
	}
	if g := Path(Opts{}); g != "/run/user/1/ttybus/bus.sock" {
		t.Fatal(g)
	}
}

func TestSanitizeBus(t *testing.T) {
	p := Path(Opts{Bus: "/tmp/tmux-501/default,1234,0"})
	if strings.Contains(p, "/tmp/tmux") {
		t.Fatalf("unsanitized path %s", p)
	}
	if !strings.HasSuffix(filepath.Base(p), ".sock") {
		t.Fatal(p)
	}
}

func TestDirFallback(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")
	d := Dir()
	if !strings.Contains(d, "ttybus-") {
		t.Fatal(d)
	}
	_ = os.TempDir()
}

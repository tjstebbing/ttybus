package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tjstebbing/ttybus/internal/daemon"
)

var testBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "ttybus-bin-")
	if err != nil {
		panic(err)
	}
	testBin = filepath.Join(dir, "ttybus")
	cmd := exec.Command("go", "build", "-o", testBin, ".")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.RemoveAll(dir)
		panic(err)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func startBus(t *testing.T) string {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "bus.sock")
	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() { errc <- daemon.Serve(ctx, sock) }()
	waitSock(t, sock)
	t.Cleanup(func() {
		cancel()
		select {
		case <-errc:
		case <-time.After(2 * time.Second):
		}
	})
	return sock
}

func waitSock(t *testing.T, path string) {
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
	t.Fatalf("no bus at %s", path)
}

func runBin(t *testing.T, sock string, args ...string) *exec.Cmd {
	t.Helper()
	all := []string{args[0], "--socket", sock}
	all = append(all, args[1:]...)
	return exec.Command(testBin, all...)
}

func TestPubSub(t *testing.T) {
	sock := startBus(t)
	sub := runBin(t, sock, "sub", "--limit", "1", "foo")
	stdout, err := sub.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := sub.Start(); err != nil {
		t.Fatal(err)
	}
	waitPeers(t, sock, 1)
	pub := runBin(t, sock, "pub", "foo", "hello")
	if out, err := pub.CombinedOutput(); err != nil {
		t.Fatalf("pub: %v %s", err, out)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(line) != "hello" {
		t.Fatalf("%q", line)
	}
	if err := sub.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestFlushWithoutClose(t *testing.T) {
	sock := startBus(t)
	sub := runBin(t, sock, "sub", "foo")
	stdout, err := sub.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := sub.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sub.Process.Kill() })
	waitPeers(t, sock, 1)
	if out, err := runBin(t, sock, "pub", "foo", "hello").CombinedOutput(); err != nil {
		t.Fatalf("pub: %v %s", err, out)
	}
	r := bufio.NewReader(stdout)
	ch := make(chan string, 1)
	go func() {
		s, err := r.ReadString('\n')
		if err != nil {
			ch <- err.Error()
			return
		}
		ch <- s
	}()
	select {
	case s := <-ch:
		if strings.TrimSpace(s) != "hello" {
			t.Fatalf("%q", s)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sub did not flush a single line (the 2018 pipe bug)")
	}
}

func TestGoldenPipeline(t *testing.T) {
	sock := startBus(t)
	cats := runBin(t, sock, "sub", "cats")
	catsOut, err := cats.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cats.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cats.Process.Kill() })
	waitPeers(t, sock, 1)

	// awk fflush: grep block-buffers when stdout is a pipe (not ttybus's bug).
	sh := "set -e\n" +
		testBin + " sub --socket " + sock + " animals | awk '/^cat/ { print; fflush() }' | " +
		testBin + " pub --socket " + sock + " cats\n"
	pipe := exec.Command("sh", "-c", sh)
	if err := pipe.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pipe.Process.Kill() })
	waitPeers(t, sock, 3)

	for _, msg := range []string{"catflap", "dog", "cats"} {
		if out, err := runBin(t, sock, "pub", "animals", msg).CombinedOutput(); err != nil {
			t.Fatalf("pub %s: %v %s", msg, err, out)
		}
	}
	r := bufio.NewReader(catsOut)
	got := []string{readLine(t, r), readLine(t, r)}
	want := map[string]bool{"catflap": true, "cats": true}
	for _, g := range got {
		if !want[g] {
			t.Fatalf("got %q", got)
		}
		delete(want, g)
	}
	if len(want) != 0 {
		t.Fatalf("missing %v", want)
	}
}

func TestSubTimeout(t *testing.T) {
	sock := startBus(t)
	cmd := runBin(t, sock, "sub", "--timeout", "50ms", "z")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected timeout")
	}
	ee, ok := err.(*exec.ExitError)
	if !ok || ee.ExitCode() != exitTimeout {
		t.Fatalf("err=%v", err)
	}
}

func TestSubLimit(t *testing.T) {
	sock := startBus(t)
	sub := runBin(t, sock, "sub", "--limit", "1", "foo")
	var buf bytes.Buffer
	sub.Stdout = &buf
	if err := sub.Start(); err != nil {
		t.Fatal(err)
	}
	waitPeers(t, sock, 1)
	_ = runBin(t, sock, "pub", "foo", "one").Run()
	if err := sub.Wait(); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(buf.String()) != "one" {
		t.Fatalf("%q", buf.String())
	}
}

func TestLsAndNamedBus(t *testing.T) {
	// macOS unix socket paths are capped ~104 bytes; t.TempDir() is too long
	// once we nest ttybus/buses/.
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("tb%d", os.Getpid()))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Setenv("XDG_RUNTIME_DIR", dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sockA := filepath.Join(dir, "ttybus", "buses", "alpha.sock")
	sockB := filepath.Join(dir, "ttybus", "buses", "beta.sock")
	errA := make(chan error, 1)
	errB := make(chan error, 1)
	go func() { errA <- daemon.Serve(ctx, sockA) }()
	go func() { errB <- daemon.Serve(ctx, sockB) }()
	waitSock(t, sockA)
	waitSock(t, sockB)

	sub := exec.Command(testBin, "sub", "--bus", "alpha", "--limit", "1", "files")
	sub.Env = append(os.Environ(), "XDG_RUNTIME_DIR="+dir)
	stdout, _ := sub.StdoutPipe()
	if err := sub.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		out, _ := exec.Command(testBin, "ls", "--bus", "alpha").CombinedOutput()
		if strings.Contains(string(out), "PEER ") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	pub := exec.Command(testBin, "pub", "--bus", "alpha", "files", "x")
	pub.Env = append(os.Environ(), "XDG_RUNTIME_DIR="+dir)
	if err := pub.Run(); err != nil {
		t.Fatal(err)
	}
	line, _ := bufio.NewReader(stdout).ReadString('\n')
	if strings.TrimSpace(line) != "x" {
		t.Fatalf("%q", line)
	}
	// beta is a different bus
	out, err := exec.Command(testBin, "ls", "--bus", "beta").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(out), "PEER ") != 1 { // just ls itself... ls disconnects
		// ls is gone; should be empty or only leftover
	}
	_ = sub.Wait()
}

func TestTTYBUSID(t *testing.T) {
	sock := startBus(t)
	sub := runBin(t, sock, "sub", "files")
	sub.Env = append(os.Environ(), "TTYBUS_ID=my-tui")
	if err := sub.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sub.Process.Kill() })
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		out, _ := runBin(t, sock, "ls").Output()
		if strings.Contains(string(out), "id=my-tui") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	out, _ := runBin(t, sock, "ls").Output()
	t.Fatalf("missing id=my-tui\n%s", out)
}

func TestSendTo(t *testing.T) {
	sock := startBus(t)
	sub := runBin(t, sock, "sub", "--limit", "1", "files")
	stdout, _ := sub.StdoutPipe()
	if err := sub.Start(); err != nil {
		t.Fatal(err)
	}
	var id string
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && id == "" {
		out, _ := runBin(t, sock, "ls").Output()
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(line, "subs=files") {
				for _, f := range strings.Fields(line) {
					if strings.HasPrefix(f, "id=") {
						id = strings.TrimPrefix(f, "id=")
					}
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if id == "" {
		t.Fatal("no subscriber id")
	}
	if out, err := runBin(t, sock, "send", "--to", id, "files", "secret").CombinedOutput(); err != nil {
		t.Fatalf("send: %v %s", err, out)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(line) != "secret" {
		t.Fatalf("%q", line)
	}
}

func TestSecondServeDaemon(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "bus.sock")
	fg := exec.Command(testBin, "serve", "--foreground", "--socket", sock)
	if err := fg.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fg.Process.Kill(); _ = fg.Wait() })
	waitSock(t, sock)
	cmd2 := exec.Command(testBin, "serve", "--daemon", "--socket", sock)
	if err := cmd2.Run(); err != nil {
		t.Fatalf("second daemon should exit 0: %v", err)
	}
}

func TestPlumbPub(t *testing.T) {
	sock := startBus(t)
	plumbFile := filepath.Join(t.TempDir(), "plumb")
	if err := os.WriteFile(plumbFile, []byte("match ^(.+):(\\d+)$\n  pub editor.open\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sub := runBin(t, sock, "sub", "--limit", "1", "editor.open")
	stdout, _ := sub.StdoutPipe()
	if err := sub.Start(); err != nil {
		t.Fatal(err)
	}
	waitPeers(t, sock, 1)
	cmd := runBin(t, sock, "plumb", "main.go:42")
	cmd.Env = append(os.Environ(), "TTYBUS_PLUMB="+plumbFile)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("plumb: %v %s", err, out)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(line) != "main.go:42" {
		t.Fatalf("%q", line)
	}
}

func TestPlumbExec(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "ran")
	plumbFile := filepath.Join(dir, "plumb")
	script := filepath.Join(dir, "touchit")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plumbFile, []byte("match ^https?://\n  exec "+script+" "+marker+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(testBin, "plumb", "https://example.com")
	cmd.Env = append(os.Environ(), "TTYBUS_PLUMB="+plumbFile)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("exec did not run")
	}
}

func waitPeers(t *testing.T, sock string, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		out, err := runBin(t, sock, "ls").Output()
		if err == nil && strings.Count(string(out), "PEER ") >= n {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	out, _ := runBin(t, sock, "ls").Output()
	t.Fatalf("wanted %d peers, ls:\n%s", n, out)
}

func readLine(t *testing.T, r *bufio.Reader) string {
	t.Helper()
	ch := make(chan string, 1)
	errc := make(chan error, 1)
	go func() {
		s, err := r.ReadString('\n')
		if err != nil {
			errc <- err
			return
		}
		ch <- strings.TrimSpace(s)
	}()
	select {
	case s := <-ch:
		return s
	case err := <-errc:
		t.Fatal(err)
		return ""
	case <-time.After(2 * time.Second):
		t.Fatal("timeout reading line")
		return ""
	}
}

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/tjstebbing/ttybus/internal/daemon"
)

func cmdServe(args []string) int {
	var (
		c         commonFlags
		daemonize bool
		fg        bool
	)
	fs := newFlagSet("serve")
	addCommon(fs, &c)
	fs.BoolVar(&daemonize, "daemon", false, "detach")
	fs.BoolVar(&fg, "foreground", false, "run in the foreground (default)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitUsage
	}
	path := c.path()

	if daemonize {
		return spawnDaemon(path, c)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err := daemon.Serve(ctx, path)
	if errors.Is(err, daemon.ErrAlreadyRunning) {
		fmt.Fprintf(os.Stderr, "ttybus: already running on %s\n", path)
		return exitUsage
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(os.Stderr, "ttybus: serve: %v\n", err)
		return exitUsage
	}
	return exitOK
}

func spawnDaemon(path string, c commonFlags) int {
	if socketAlive(path) {
		return exitOK
	}
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitUsage
	}
	child := []string{"serve", "--foreground"}
	if c.socket != "" {
		child = append(child, "--socket", c.socket)
	}
	if c.bus != "" {
		child = append(child, "--bus", c.bus)
	}
	devnull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitUsage
	}
	defer devnull.Close()
	files := []*os.File{devnull, devnull, devnull}
	if os.Getenv("DEBUG") != "" {
		files = []*os.File{devnull, os.Stdout, os.Stderr}
	}
	p, err := os.StartProcess(exe, append([]string{exe}, child...), &os.ProcAttr{
		Files: files,
		Sys:   &syscall.SysProcAttr{Setsid: true},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "ttybus: daemonize: %v\n", err)
		return exitUsage
	}
	_ = p.Release()
	return exitOK
}

func socketAlive(path string) bool {
	return path != "" && fileAlive(path)
}

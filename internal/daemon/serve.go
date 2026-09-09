package daemon

import (
	"context"
	"errors"
	"log"
	"net"
	"os"
	"sync"

	"github.com/tjstebbing/ttybus/internal/socket"
)

// ErrAlreadyRunning is returned when the socket is already served.
var ErrAlreadyRunning = socket.ErrBusy

// Serve listens on path until ctx is cancelled. It takes an exclusive
// lock, unlinks a stale socket, and removes the socket on return.
func Serve(ctx context.Context, path string) error {
	lock, err := socket.Acquire(path)
	if err != nil {
		if errors.Is(err, socket.ErrBusy) {
			if socket.Alive(path) {
				return ErrAlreadyRunning
			}
			return err
		}
		return err
	}
	defer lock.Close()

	if err := socket.UnlinkStale(path); err != nil {
		if errors.Is(err, socket.ErrBusy) {
			return ErrAlreadyRunning
		}
		return err
	}

	ln, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		return err
	}

	hub := newHub()
	var wg sync.WaitGroup
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		nc, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			if errors.Is(err, net.ErrClosed) {
				break
			}
			log.Printf("ttybus: accept: %v", err)
			continue
		}
		c := newConn(nc, hub)
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.serve()
		}()
	}

	hub.mu.Lock()
	conns := make([]*Conn, 0, len(hub.conns))
	for _, c := range hub.conns {
		conns = append(conns, c)
	}
	hub.mu.Unlock()
	for _, c := range conns {
		c.close()
	}
	wg.Wait()
	_ = ln.Close()
	_ = os.Remove(path)
	return ctx.Err()
}

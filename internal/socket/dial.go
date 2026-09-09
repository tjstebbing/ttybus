package socket

import (
	"errors"
	"net"
	"os"
	"syscall"
	"time"
)

// Dial connects to an existing bus socket. It does not start a daemon.
func Dial(path string) (net.Conn, error) {
	return net.Dial("unix", path)
}

// Alive reports whether a process is accepting connections on path.
func Alive(path string) bool {
	c, err := net.Dial("unix", path)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// UnlinkStale removes path if it exists and nothing is listening.
func UnlinkStale(path string) error {
	if Alive(path) {
		return ErrBusy
	}
	err := os.Remove(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Transient reports whether err is the kind auto-start should retry:
// missing socket or connection refused.
func Transient(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	if errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	var op *net.OpError
	if errors.As(err, &op) {
		return Transient(op.Err)
	}
	s := err.Error()
	return errors.Is(err, syscall.ECONNREFUSED) ||
		s == "connection refused" ||
		s == "no such file or directory"
}

// DialRetry dials path, waiting up to the usual auto-start window.
func DialRetry(path string, attempts int) (net.Conn, error) {
	if attempts <= 0 {
		attempts = 10
	}
	d := 20 * time.Millisecond
	var last error
	for i := 0; i < attempts; i++ {
		c, err := net.Dial("unix", path)
		if err == nil {
			return c, nil
		}
		last = err
		time.Sleep(d)
		d *= 2
		if d > 200*time.Millisecond {
			d = 200 * time.Millisecond
		}
	}
	return nil, last
}

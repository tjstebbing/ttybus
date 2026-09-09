//go:build unix

package socket

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

var ErrBusy = errors.New("bus already running")

// Lock is an exclusive flock held for the lifetime of a serving process.
type Lock struct {
	f *os.File
}

// Acquire takes an exclusive non-blocking lock next to sockPath.
func Acquire(sockPath string) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(sockPath), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(sockPath+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, ErrBusy
		}
		return nil, fmt.Errorf("flock: %w", err)
	}
	return &Lock{f: f}, nil
}

func (l *Lock) Close() error {
	if l == nil || l.f == nil {
		return nil
	}
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	err := l.f.Close()
	l.f = nil
	return err
}

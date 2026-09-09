package socket

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

const (
	EnvSocket = "TTYBUS_SOCKET"
	EnvBus    = "TTYBUS_BUS"
)

// Opts selects a bus socket. Precedence: Socket, TTYBUS_SOCKET, Bus /
// TTYBUS_BUS, then the default bus.sock.
type Opts struct {
	Socket string
	Bus    string
}

// Path resolves the unix socket path for a bus.
func Path(o Opts) string {
	if o.Socket != "" {
		return o.Socket
	}
	if s := os.Getenv(EnvSocket); s != "" {
		return s
	}
	bus := o.Bus
	if bus == "" {
		bus = os.Getenv(EnvBus)
	}
	base := Dir()
	if bus != "" {
		return filepath.Join(base, "buses", sanitizeBus(bus)+".sock")
	}
	return filepath.Join(base, "bus.sock")
}

// Dir is the per-user ttybus runtime directory.
func Dir() string {
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return filepath.Join(d, "ttybus")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("ttybus-%d", os.Getuid()))
}

func sanitizeBus(name string) string {
	var b strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	s := b.String()
	if s == "" {
		return "bus"
	}
	if len(s) > 64 {
		s = s[:64]
	}
	return s
}

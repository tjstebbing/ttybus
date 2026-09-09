package protocol

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// MaxLine is the maximum on-wire line length, including the newline.
const MaxLine = 64 * 1024

const (
	VerbHello = "HELLO"
	VerbSub   = "SUB"
	VerbUnsub = "UNSUB"
	VerbPub   = "PUB"
	VerbSend  = "SEND"
	VerbList  = "LIST"
	VerbMsg   = "MSG"
	VerbOK    = "OK"
	VerbErr   = "ERR"
	VerbPeer  = "PEER"
)

var (
	ErrEmpty   = errors.New("empty line")
	ErrLine    = errors.New("malformed line")
	ErrLineLen = errors.New("line too long")
	ErrChan    = errors.New("illegal channel")
	ErrID      = errors.New("illegal id")
	ErrVerb    = errors.New("unknown verb")
)

// Line is one protocol message. Args are the space-separated tokens
// after the verb. Rest is that same region unmodified (payloads keep
// interior spaces).
type Line struct {
	Verb string
	Args []string
	Rest string
	Raw  string
}

// Hello attributes from a HELLO, OK, or PEER line.
type Hello struct {
	Name  string
	PID   string
	Pane  string
	CWD   string
	ID    string
	Subs  []string
	Extra map[string]string
}

// Parse splits a single line (newline already stripped) into a Line.
func Parse(raw string) (Line, error) {
	if len(raw) > MaxLine-1 {
		return Line{}, ErrLineLen
	}
	raw = strings.TrimSuffix(raw, "\r")
	if strings.TrimSpace(raw) == "" {
		return Line{}, ErrEmpty
	}
	verb, rest, _ := strings.Cut(raw, " ")
	if verb == "" || !isVerb(verb) {
		return Line{}, fmt.Errorf("%w: %q", ErrLine, verb)
	}
	var args []string
	if rest != "" {
		args = strings.Split(rest, " ")
	}
	return Line{Verb: verb, Args: args, Rest: rest, Raw: raw}, nil
}

func isVerb(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

// Token returns Args[i] or "".
func (l Line) Token(i int) string {
	if i < 0 || i >= len(l.Args) {
		return ""
	}
	return l.Args[i]
}

// After returns Rest with the first n tokens removed (the remainder of
// the line, preserving spaces). n=1 on "PUB files hello  world" yields
// "hello  world".
func (l Line) After(n int) string {
	rest := l.Rest
	for i := 0; i < n; i++ {
		sp := strings.IndexByte(rest, ' ')
		if sp < 0 {
			return ""
		}
		rest = rest[sp+1:]
	}
	return rest
}

// KVs parses Args as key=value pairs. Used by HELLO, OK, PEER.
func (l Line) KVs() (map[string]string, error) {
	out := make(map[string]string, len(l.Args))
	for _, a := range l.Args {
		if a == "" {
			continue
		}
		k, v, ok := strings.Cut(a, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("%w: expected key=value, got %q", ErrLine, a)
		}
		out[k] = v
	}
	return out, nil
}

// ParseHello extracts HELLO/PEER/OK attributes.
func (l Line) ParseHello() (Hello, error) {
	kvs, err := l.KVs()
	if err != nil {
		return Hello{}, err
	}
	h := Hello{Extra: map[string]string{}}
	for k, v := range kvs {
		switch k {
		case "name":
			h.Name = v
		case "pid":
			h.PID = v
		case "pane":
			h.Pane = v
		case "cwd":
			h.CWD = v
		case "id":
			h.ID = v
		case "subs":
			if v != "" {
				h.Subs = strings.Split(v, ",")
			}
		default:
			h.Extra[k] = v
		}
	}
	if h.ID != "" {
		if err := ValidID(h.ID); err != nil {
			return Hello{}, err
		}
	}
	return h, nil
}

// ChannelArg is Args[0] validated as a channel name.
func (l Line) ChannelArg() (string, error) {
	ch := l.Token(0)
	if err := ValidChannel(ch); err != nil {
		return "", err
	}
	return ch, nil
}

// ValidChannel reports whether s is a v0 channel name.
func ValidChannel(s string) error {
	if s == "" || len(s) > 64 {
		return ErrChan
	}
	if !isAlpha(rune(s[0])) {
		return ErrChan
	}
	for _, r := range s {
		if isAlpha(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-' || r == '/' {
			continue
		}
		return ErrChan
	}
	return nil
}

// ValidID reports whether s is a v0 client id.
func ValidID(s string) error {
	if s == "" || len(s) > 64 {
		return ErrID
	}
	for _, r := range s {
		if isAlpha(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-' || r == ':' {
			continue
		}
		return ErrID
	}
	return nil
}

func isAlpha(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
}

// KnownVerb reports whether v is a protocol verb (not necessarily valid
// in the current direction).
func KnownVerb(v string) bool {
	switch v {
	case VerbHello, VerbSub, VerbUnsub, VerbPub, VerbSend, VerbList,
		VerbMsg, VerbOK, VerbErr, VerbPeer:
		return true
	}
	return false
}

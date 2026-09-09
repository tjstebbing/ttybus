package plumb

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Action is one matched plumbing rule.
type Action struct {
	Kind    string // "pub" or "exec"
	Channel string
	Payload string
	Argv    []string
}

// Rule is one match/action pair.
type Rule struct {
	RE      *regexp.Regexp
	Kind    string
	Channel string
	Argv    []string
}

// ParseRules reads a plumber file.
func ParseRules(r io.Reader) ([]Rule, error) {
	sc := bufio.NewScanner(r)
	var rules []Rule
	var cur *Rule
	lineno := 0
	flush := func() {
		if cur != nil {
			rules = append(rules, *cur)
			cur = nil
		}
	}
	for sc.Scan() {
		lineno++
		line := strings.TrimRight(sc.Text(), " \t")
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		indented := len(line) > 0 && (line[0] == ' ' || line[0] == '\t')
		if !indented {
			flush()
			if !strings.HasPrefix(trim, "match ") {
				return nil, fmt.Errorf("line %d: expected match", lineno)
			}
			pat := strings.TrimSpace(strings.TrimPrefix(trim, "match "))
			re, err := regexp.Compile(pat)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", lineno, err)
			}
			cur = &Rule{RE: re}
			continue
		}
		if cur == nil {
			return nil, fmt.Errorf("line %d: action without match", lineno)
		}
		fields := strings.Fields(trim)
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "pub":
			if len(fields) != 2 {
				return nil, fmt.Errorf("line %d: pub CHANNEL", lineno)
			}
			cur.Kind = "pub"
			cur.Channel = fields[1]
		case "exec":
			if len(fields) < 2 {
				return nil, fmt.Errorf("line %d: exec CMD", lineno)
			}
			cur.Kind = "exec"
			cur.Argv = fields[1:]
		default:
			return nil, fmt.Errorf("line %d: unknown action %q", lineno, fields[0])
		}
	}
	flush()
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return rules, nil
}

// Match returns the first matching action for text.
func Match(rules []Rule, text string) (Action, bool) {
	for _, r := range rules {
		m := r.RE.FindStringSubmatch(text)
		if m == nil {
			continue
		}
		expand := func(s string) string {
			out := s
			for i := len(m) - 1; i >= 1; i-- {
				out = strings.ReplaceAll(out, fmt.Sprintf("$%d", i), m[i])
			}
			return strings.ReplaceAll(out, "$0", text)
		}
		a := Action{Kind: r.Kind, Payload: text}
		if r.Kind == "pub" {
			a.Channel = expand(r.Channel)
		}
		if r.Kind == "exec" {
			argv := make([]string, len(r.Argv))
			for i, p := range r.Argv {
				argv[i] = expand(p)
			}
			a.Argv = argv
		}
		return a, true
	}
	return Action{}, false
}

// ConfigPath is $TTYBUS_PLUMB or $XDG_CONFIG_HOME/ttybus/plumb.
func ConfigPath() string {
	if p := os.Getenv("TTYBUS_PLUMB"); p != "" {
		return p
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "ttybus", "plumb")
}

// RunExec runs an exec action. Tests replace this.
var RunExec = func(argv []string) error {
	if len(argv) == 0 {
		return fmt.Errorf("empty exec")
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

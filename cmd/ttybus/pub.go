package main

import (
	"bufio"
	"fmt"
	"os"

	"github.com/tjstebbing/ttybus/internal/protocol"
)

func cmdPub(args []string) int {
	var (
		cf    commonFlags
		limit int
		log   bool
	)
	fs := newFlagSet("pub")
	addCommon(fs, &cf)
	fs.IntVar(&limit, "limit", 0, "max stdin lines")
	fs.BoolVar(&log, "log", false, "log protocol to stderr")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitUsage
	}
	rest := fs.Args()
	if len(rest) < 1 {
		fmt.Fprintln(os.Stderr, "ttybus pub: channel required")
		return exitUsage
	}
	ch := rest[0]
	if err := protocol.ValidChannel(ch); err != nil {
		fmt.Fprintf(os.Stderr, "ttybus: %v\n", err)
		return exitUsage
	}

	bc, code := dialBus(cf, true)
	if code != 0 {
		return code
	}
	defer bc.close()
	bc.log = log
	if err := bc.hello(); err != nil {
		fmt.Fprintf(os.Stderr, "ttybus: hello: %v\n", err)
		return exitUsage
	}

	if len(rest) > 1 {
		payload := joinArgs(rest[1:])
		if err := bc.write(protocol.FormatPub(ch, payload)); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return exitUsage
		}
		if err := bc.expectOK(); err != nil {
			fmt.Fprintf(os.Stderr, "ttybus: %v\n", err)
			return exitUsage
		}
		return exitOK
	}

	if stdinIsTTY() {
		fmt.Fprintln(os.Stderr, "ttybus pub: no message and stdin is a TTY")
		return exitUsage
	}
	sc := bufio.NewScanner(os.Stdin)
	n := 0
	for sc.Scan() {
		if err := bc.write(protocol.FormatPub(ch, sc.Text())); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return exitUsage
		}
		if err := bc.expectOK(); err != nil {
			fmt.Fprintf(os.Stderr, "ttybus: %v\n", err)
			return exitUsage
		}
		n++
		if limit > 0 && n >= limit {
			break
		}
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitUsage
	}
	return exitOK
}

func joinArgs(a []string) string {
	out := ""
	for i, s := range a {
		if i > 0 {
			out += " "
		}
		out += s
	}
	return out
}

func stdinIsTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/tjstebbing/ttybus/internal/protocol"
)

func cmdSub(args []string) int {
	var (
		cf      commonFlags
		limit   int
		timeout string
		log     bool
		raw     bool
	)
	fs := newFlagSet("sub")
	addCommon(fs, &cf)
	fs.IntVar(&limit, "limit", 0, "exit after N messages")
	fs.StringVar(&timeout, "timeout", "", "overall deadline (e.g. 2s)")
	fs.BoolVar(&log, "log", false, "log protocol to stderr")
	fs.BoolVar(&raw, "raw", false, "print full MSG lines")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitUsage
	}
	rest := fs.Args()
	if len(rest) != 1 {
		fmt.Fprintln(os.Stderr, "ttybus sub: channel required")
		return exitUsage
	}
	ch := rest[0]
	if err := protocol.ValidChannel(ch); err != nil {
		fmt.Fprintf(os.Stderr, "ttybus: %v\n", err)
		return exitUsage
	}
	d, err := parseDuration(timeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ttybus: timeout: %v\n", err)
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
	if err := bc.write(protocol.FormatSub(ch)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitUsage
	}
	if err := bc.expectOK(); err != nil {
		fmt.Fprintf(os.Stderr, "ttybus: %v\n", err)
		return exitUsage
	}

	var deadline time.Time
	if d > 0 {
		deadline = time.Now().Add(d)
	}
	out := bufio.NewWriter(os.Stdout)
	n := 0
	for {
		if !deadline.IsZero() {
			bc.nc.SetReadDeadline(deadline)
		}
		l, err := bc.read()
		if err != nil {
			if isTimeout(err) {
				return exitTimeout
			}
			if err == io.EOF {
				return exitOK
			}
			fmt.Fprintln(os.Stderr, err)
			return exitUsage
		}
		if l.Verb != protocol.VerbMsg {
			continue
		}
		if l.Token(0) != ch {
			continue
		}
		if raw {
			fmt.Fprintln(out, l.Raw)
		} else {
			fmt.Fprintln(out, l.After(1))
		}
		if err := out.Flush(); err != nil {
			return exitOK // SIGPIPE
		}
		n++
		if limit > 0 && n >= limit {
			return exitOK
		}
	}
}

func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	ne, ok := err.(interface{ Timeout() bool })
	return ok && ne.Timeout()
}

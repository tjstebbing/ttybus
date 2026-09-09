package main

import (
	"fmt"
	"os"

	"github.com/tjstebbing/ttybus/internal/protocol"
)

func cmdSend(args []string) int {
	var (
		cf commonFlags
		to string
	)
	fs := newFlagSet("send")
	addCommon(fs, &cf)
	fs.StringVar(&to, "to", "", "peer id")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitUsage
	}
	if to == "" {
		fmt.Fprintln(os.Stderr, "ttybus send: --to ID required")
		return exitUsage
	}
	rest := fs.Args()
	if len(rest) < 1 {
		fmt.Fprintln(os.Stderr, "ttybus send: channel required")
		return exitUsage
	}
	ch := rest[0]
	if err := protocol.ValidChannel(ch); err != nil {
		fmt.Fprintf(os.Stderr, "ttybus: %v\n", err)
		return exitUsage
	}
	payload := joinArgs(rest[1:])
	bc, code := dialBus(cf, true)
	if code != 0 {
		return code
	}
	defer bc.close()
	if err := bc.hello(); err != nil {
		fmt.Fprintf(os.Stderr, "ttybus: hello: %v\n", err)
		return exitUsage
	}
	if err := bc.write(protocol.FormatSend(to, ch, payload)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitUsage
	}
	if err := bc.expectOK(); err != nil {
		fmt.Fprintf(os.Stderr, "ttybus: %v\n", err)
		return exitUsage
	}
	return exitOK
}

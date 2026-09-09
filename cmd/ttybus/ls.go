package main

import (
	"fmt"
	"os"

	"github.com/tjstebbing/ttybus/internal/protocol"
)

func cmdLs(args []string) int {
	var cf commonFlags
	fs := newFlagSet("ls")
	addCommon(fs, &cf)
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitUsage
	}
	bc, code := dialBus(cf, true)
	if code != 0 {
		return code
	}
	defer bc.close()
	if err := bc.hello(); err != nil {
		fmt.Fprintf(os.Stderr, "ttybus: hello: %v\n", err)
		return exitUsage
	}
	if err := bc.write(protocol.FormatList()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitUsage
	}
	for {
		l, err := bc.read()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return exitUsage
		}
		switch l.Verb {
		case protocol.VerbPeer:
			fmt.Println(l.Raw)
		case protocol.VerbOK:
			return exitOK
		case protocol.VerbErr:
			fmt.Fprintf(os.Stderr, "ttybus: %s\n", l.Rest)
			return exitUsage
		}
	}
}

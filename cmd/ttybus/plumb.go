package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/tjstebbing/ttybus/internal/plumb"
	"github.com/tjstebbing/ttybus/internal/protocol"
)

func cmdPlumb(args []string) int {
	var cf commonFlags
	fs := newFlagSet("plumb")
	addCommon(fs, &cf)
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitUsage
	}
	text := strings.Join(fs.Args(), " ")
	if text == "" {
		sc := bufio.NewScanner(os.Stdin)
		if !sc.Scan() {
			fmt.Fprintln(os.Stderr, "ttybus plumb: no text")
			return exitUsage
		}
		text = sc.Text()
	}
	path := plumb.ConfigPath()
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ttybus plumb: %s: %v\n", path, err)
		return exitUsage
	}
	defer f.Close()
	rules, err := plumb.ParseRules(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ttybus plumb: %v\n", err)
		return exitUsage
	}
	act, ok := plumb.Match(rules, text)
	if !ok {
		fmt.Fprintf(os.Stderr, "ttybus plumb: no match for %q\n", text)
		return exitUsage
	}
	switch act.Kind {
	case "pub":
		bc, code := dialBus(cf, true)
		if code != 0 {
			return code
		}
		defer bc.close()
		if err := bc.hello(); err != nil {
			fmt.Fprintf(os.Stderr, "ttybus: hello: %v\n", err)
			return exitUsage
		}
		if err := bc.write(protocol.FormatPub(act.Channel, act.Payload)); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return exitUsage
		}
		if err := bc.expectOK(); err != nil {
			fmt.Fprintf(os.Stderr, "ttybus: %v\n", err)
			return exitUsage
		}
		return exitOK
	case "exec":
		if err := plumb.RunExec(act.Argv); err != nil {
			fmt.Fprintf(os.Stderr, "ttybus plumb: exec: %v\n", err)
			return exitUsage
		}
		return exitOK
	default:
		fmt.Fprintf(os.Stderr, "ttybus plumb: unknown action %q\n", act.Kind)
		return exitUsage
	}
}

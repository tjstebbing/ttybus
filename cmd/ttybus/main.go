// Command ttybus is the daemon and the Unix-filter CLI for the ttybus
// text message bus.
package main

import (
	"fmt"
	"io"
	"log"
	"os"
)

const (
	exitOK      = 0
	exitUsage   = 1
	exitConnect = 2
	exitTimeout = 124
)

// version is set at release with -ldflags "-X main.version=v1.2.3".
var version = "dev"

var usage = ` ________
|[][][]|_\_
| TTYBus   |
=-OO----OO-=

ttybus — a text bus for TUIs and CLIs

Usage:
  ttybus serve [--foreground] [--daemon] [--socket PATH] [--bus NAME]
  ttybus pub   CHANNEL [MESSAGE...] [--limit N] [--socket PATH] [--bus NAME]
  ttybus sub   CHANNEL [--limit N] [--timeout D] [--log] [--raw] [--socket PATH] [--bus NAME]
  ttybus ls    [--socket PATH] [--bus NAME]
  ttybus send  --to ID CHANNEL [MESSAGE...] [--socket PATH] [--bus NAME]
  ttybus plumb [TEXT] [--socket PATH] [--bus NAME]
  ttybus version

Socket: $XDG_RUNTIME_DIR/ttybus/bus.sock (or --socket / --bus).
See docs/PROTOCOL.md.
`

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprint(os.Stderr, usage)
		fmt.Fprintf(os.Stderr, "\n%s\n", version)
		return exitOK
	case "version", "-v", "--version":
		fmt.Println(version)
		return exitOK
	case "serve":
		return cmdServe(args[1:])
	case "pub":
		return cmdPub(args[1:])
	case "sub":
		return cmdSub(args[1:])
	case "ls":
		return cmdLs(args[1:])
	case "send":
		return cmdSend(args[1:])
	case "plumb":
		return cmdPlumb(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "ttybus: unknown command %q\n", args[0])
		fmt.Fprint(os.Stderr, usage)
		return exitUsage
	}
}

func init() {
	if os.Getenv("DEBUG") == "" {
		log.SetOutput(io.Discard)
	}
	log.SetFlags(0)
	log.SetPrefix("ttybus: ")
}

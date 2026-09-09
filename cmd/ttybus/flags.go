package main

import (
	"flag"
	"os"

	"github.com/tjstebbing/ttybus/internal/socket"
)

type commonFlags struct {
	socket string
	bus    string
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

func addCommon(fs *flag.FlagSet, c *commonFlags) {
	fs.StringVar(&c.socket, "socket", "", "unix socket path")
	fs.StringVar(&c.bus, "bus", "", "named bus")
}

func (c commonFlags) path() string {
	return socket.Path(socket.Opts{Socket: c.socket, Bus: c.bus})
}

// Package ttybus is a small Go helper for TUI authors.
//
// It speaks docs/PROTOCOL.md over a unix stream socket. It is not the
// spec: nc(1) is a supported client.
//
//	c, err := ttybus.Dial(ttybus.DefaultSocket())
//	_ = c.Hello(ttybus.Info{Name: "mytui"})
//	ch, _ := c.Sub("files")
//	_ = c.Pub("files", path)
package ttybus

package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/tjstebbing/ttybus/internal/protocol"
	"github.com/tjstebbing/ttybus/internal/socket"
)

type busConn struct {
	nc  net.Conn
	r   *bufio.Reader
	log bool
}

func dialBus(cf commonFlags, auto bool) (*busConn, int) {
	path := cf.path()
	c, err := net.Dial("unix", path)
	if err != nil && auto && socket.Transient(err) {
		if code := spawnDaemon(path, cf); code != exitOK {
			return nil, code
		}
		c, err = socket.DialRetry(path, 10)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "ttybus: connect %s: %v\n", path, err)
		return nil, exitConnect
	}
	return &busConn{nc: c, r: bufio.NewReaderSize(c, protocol.MaxLine)}, 0
}

func fileAlive(path string) bool {
	return socket.Alive(path)
}

func (c *busConn) close() { _ = c.nc.Close() }

func (c *busConn) write(line string) error {
	if c.log {
		fmt.Fprintln(os.Stderr, "->", line)
	}
	return protocol.WriteLine(c.nc, line)
}

func (c *busConn) read() (protocol.Line, error) {
	s, err := protocol.ReadLine(c.r)
	if err != nil {
		return protocol.Line{}, err
	}
	if c.log {
		fmt.Fprintln(os.Stderr, "<-", s)
	}
	return protocol.Parse(s)
}

func (c *busConn) hello() error {
	h := protocol.Hello{
		Name: "ttybus",
		PID:  strconv.Itoa(os.Getpid()),
		ID:   os.Getenv("TTYBUS_ID"),
	}
	if cwd, err := os.Getwd(); err == nil && !containsSpace(cwd) {
		h.CWD = cwd
	}
	if err := c.write(protocol.FormatHello(h)); err != nil {
		return err
	}
	l, err := c.readSkipMsg()
	if err != nil {
		return err
	}
	if l.Verb == protocol.VerbErr {
		return fmt.Errorf("%s", l.Rest)
	}
	if l.Verb != protocol.VerbOK {
		return fmt.Errorf("unexpected %s", l.Raw)
	}
	return nil
}

func (c *busConn) readSkipMsg() (protocol.Line, error) {
	for {
		l, err := c.read()
		if err != nil {
			return l, err
		}
		if errorsIsEmpty(l) {
			continue
		}
		if l.Verb == protocol.VerbMsg {
			continue
		}
		return l, nil
	}
}

func errorsIsEmpty(l protocol.Line) bool {
	return l.Verb == ""
}

func (c *busConn) expectOK() error {
	l, err := c.readSkipMsg()
	if err != nil {
		return err
	}
	if l.Verb == protocol.VerbErr {
		return fmt.Errorf("%s", l.Rest)
	}
	if l.Verb != protocol.VerbOK {
		return fmt.Errorf("unexpected %s", l.Raw)
	}
	return nil
}

func containsSpace(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' {
			return true
		}
	}
	return false
}

func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	return time.ParseDuration(s)
}

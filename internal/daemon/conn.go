package daemon

import (
	"bufio"
	"errors"
	"io"
	"log"
	"net"
	"strconv"
	"sync"

	"github.com/tjstebbing/ttybus/internal/protocol"
)

var errNoPeer = errors.New("no such peer")

// Conn is one connected client. The writer goroutine is the only
// goroutine that writes to the socket.
type Conn struct {
	nc     net.Conn
	hub    *Hub
	id     string
	hello  protocol.Hello
	subSet map[string]struct{}

	out        chan string
	writerDone chan struct{}
	closeOnce  sync.Once
}

func newConn(nc net.Conn, hub *Hub) *Conn {
	c := &Conn{
		nc:         nc,
		hub:        hub,
		subSet:     make(map[string]struct{}),
		out:        make(chan string, outQueue),
		writerDone: make(chan struct{}),
	}
	go c.writer()
	return c
}

func (c *Conn) writer() {
	defer close(c.writerDone)
	w := bufio.NewWriter(c.nc)
	for line := range c.out {
		if err := protocol.WriteLine(w, line); err != nil {
			return
		}
		if err := w.Flush(); err != nil {
			return
		}
	}
}

// enqueue is non-blocking. Callers may hold hub.mu.
func (c *Conn) enqueue(line, channel string) {
	select {
	case c.out <- line:
		return
	default:
	}
	// Queue full: drop `line` and try to tell the client.
	slow := protocol.FormatErr("SLOW", channel)
	select {
	case c.out <- slow:
	default:
	}
}

func (c *Conn) reply(line string) {
	select {
	case c.out <- line:
	default:
		log.Printf("ttybus: dropping reply for %s", c.id)
	}
}

func (c *Conn) close() {
	c.closeOnce.Do(func() {
		c.hub.unregister(c)
		// Unblock a writer stuck in Write before closing the queue.
		_ = c.nc.Close()
		close(c.out)
		<-c.writerDone
	})
}

func (c *Conn) serve() {
	defer c.close()
	r := bufio.NewReaderSize(c.nc, protocol.MaxLine)
	hellod := false
	for {
		s, err := protocol.ReadLine(r)
		if err != nil {
			if errors.Is(err, protocol.ErrLineLen) {
				c.reply(protocol.FormatErr("LINELEN", "line too long"))
				// drain writer a moment is unnecessary; close will drop
			}
			if err != io.EOF && !errors.Is(err, protocol.ErrLineLen) {
				log.Printf("ttybus: read: %v", err)
			}
			return
		}
		line, err := protocol.Parse(s)
		if errors.Is(err, protocol.ErrEmpty) {
			continue
		}
		if err != nil {
			c.reply(protocol.FormatErr("LINE", err.Error()))
			continue
		}
		if !hellod {
			if line.Verb != protocol.VerbHello {
				c.reply(protocol.FormatErr("HELLO", "HELLO required"))
				continue
			}
			h, err := line.ParseHello()
			if err != nil {
				code := "HELLO"
				if errors.Is(err, protocol.ErrID) {
					code = "ID"
				}
				c.reply(protocol.FormatErr(code, err.Error()))
				continue
			}
			c.hello = h
			id, err := c.hub.register(c, h.ID)
			if err != nil {
				c.reply(protocol.FormatErr("ID", "duplicate or illegal id"))
				continue
			}
			hellod = true
			c.reply(protocol.FormatOK("id=" + id))
			continue
		}
		c.dispatch(line)
	}
}

func (c *Conn) dispatch(line protocol.Line) {
	if !protocol.KnownVerb(line.Verb) {
		c.reply(protocol.FormatErr("VERB", line.Verb))
		return
	}
	switch line.Verb {
	case protocol.VerbSub:
		ch, err := line.ChannelArg()
		if err != nil {
			c.reply(protocol.FormatErr("CHAN", err.Error()))
			return
		}
		c.hub.sub(c, ch)
		c.reply(protocol.FormatOK())
	case protocol.VerbUnsub:
		ch, err := line.ChannelArg()
		if err != nil {
			c.reply(protocol.FormatErr("CHAN", err.Error()))
			return
		}
		c.hub.unsub(c, ch)
		c.reply(protocol.FormatOK())
	case protocol.VerbPub:
		ch, err := line.ChannelArg()
		if err != nil {
			c.reply(protocol.FormatErr("CHAN", err.Error()))
			return
		}
		n := c.hub.pub(ch, line.After(1))
		c.reply(protocol.FormatOK("n=" + strconv.Itoa(n)))
	case protocol.VerbSend:
		id := line.Token(0)
		if err := protocol.ValidID(id); err != nil {
			c.reply(protocol.FormatErr("ID", err.Error()))
			return
		}
		ch := line.Token(1)
		if err := protocol.ValidChannel(ch); err != nil {
			c.reply(protocol.FormatErr("CHAN", err.Error()))
			return
		}
		if err := c.hub.send(id, ch, line.After(2)); err != nil {
			c.reply(protocol.FormatErr("PEER", "no such id"))
			return
		}
		c.reply(protocol.FormatOK())
	case protocol.VerbList:
		peers := c.hub.list()
		for _, p := range peers {
			c.reply(protocol.FormatPeer(p))
		}
		c.reply(protocol.FormatOK("n=" + strconv.Itoa(len(peers))))
	case protocol.VerbHello:
		c.reply(protocol.FormatErr("HELLO", "already said hello"))
	default:
		c.reply(protocol.FormatErr("VERB", line.Verb))
	}
}

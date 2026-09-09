package ttybus

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"sync"

	"github.com/tjstebbing/ttybus/internal/protocol"
	"github.com/tjstebbing/ttybus/internal/socket"
)

// Info is the HELLO identity of this client.
type Info struct {
	Name string
	PID  int
	Pane string
	CWD  string
	ID   string
}

// Peer is one LIST entry.
type Peer struct {
	ID   string
	Name string
	PID  string
	Pane string
	CWD  string
	Subs []string
}

// Client is a ttybus connection. A background goroutine reads the socket.
type Client struct {
	nc net.Conn
	w  *bufio.Writer

	writeMu   sync.Mutex
	mu        sync.Mutex
	replies   chan protocol.Line
	subs      map[string]chan string
	closed    chan struct{}
	closeOnce sync.Once
}

// DefaultSocket is the default bus path for this user.
func DefaultSocket() string {
	return socket.Path(socket.Opts{})
}

// Dial connects to path. The caller must Hello before other methods.
func Dial(path string) (*Client, error) {
	if path == "" {
		path = DefaultSocket()
	}
	nc, err := net.Dial("unix", path)
	if err != nil {
		return nil, err
	}
	c := &Client{
		nc:      nc,
		w:       bufio.NewWriter(nc),
		replies: make(chan protocol.Line, 16),
		subs:    make(map[string]chan string),
		closed:  make(chan struct{}),
	}
	go c.readLoop()
	return c, nil
}

func (c *Client) readLoop() {
	r := bufio.NewReaderSize(c.nc, protocol.MaxLine)
	defer func() {
		c.Close()
		c.mu.Lock()
		for _, ch := range c.subs {
			close(ch)
		}
		c.subs = map[string]chan string{}
		c.mu.Unlock()
	}()
	for {
		s, err := protocol.ReadLine(r)
		if err != nil {
			return
		}
		l, err := protocol.Parse(s)
		if err != nil {
			continue
		}
		if l.Verb == protocol.VerbMsg {
			chName := l.Token(0)
			payload := l.After(1)
			c.mu.Lock()
			ch := c.subs[chName]
			c.mu.Unlock()
			if ch == nil {
				continue
			}
			select {
			case ch <- payload:
			default:
			}
			continue
		}
		select {
		case c.replies <- l:
		case <-c.closed:
			return
		}
	}
}

func (c *Client) rpc(line string) (protocol.Line, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := protocol.WriteLine(c.w, line); err != nil {
		return protocol.Line{}, err
	}
	if err := c.w.Flush(); err != nil {
		return protocol.Line{}, err
	}
	for {
		select {
		case <-c.closed:
			return protocol.Line{}, fmt.Errorf("closed")
		case l := <-c.replies:
			if l.Verb == protocol.VerbPeer {
				continue
			}
			if l.Verb == protocol.VerbErr {
				return l, fmt.Errorf("%s", l.Rest)
			}
			return l, nil
		}
	}
}

// Hello identifies this connection.
func (c *Client) Hello(info Info) error {
	h := protocol.Hello{
		Name: info.Name,
		Pane: info.Pane,
		CWD:  info.CWD,
		ID:   info.ID,
	}
	if info.PID != 0 {
		h.PID = fmt.Sprintf("%d", info.PID)
	} else {
		h.PID = fmt.Sprintf("%d", os.Getpid())
	}
	_, err := c.rpc(protocol.FormatHello(h))
	return err
}

// Sub subscribes to channel and returns a buffered payload stream.
func (c *Client) Sub(channel string) (<-chan string, error) {
	if err := protocol.ValidChannel(channel); err != nil {
		return nil, err
	}
	ch := make(chan string, 64)
	c.mu.Lock()
	c.subs[channel] = ch
	c.mu.Unlock()
	if _, err := c.rpc(protocol.FormatSub(channel)); err != nil {
		c.mu.Lock()
		delete(c.subs, channel)
		c.mu.Unlock()
		return nil, err
	}
	return ch, nil
}

// Pub publishes payload on channel.
func (c *Client) Pub(channel, payload string) error {
	_, err := c.rpc(protocol.FormatPub(channel, payload))
	return err
}

// Send unicasts payload to a peer id.
func (c *Client) Send(id, channel, payload string) error {
	_, err := c.rpc(protocol.FormatSend(id, channel, payload))
	return err
}

// List returns connected peers.
func (c *Client) List() ([]Peer, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := protocol.WriteLine(c.w, protocol.FormatList()); err != nil {
		return nil, err
	}
	if err := c.w.Flush(); err != nil {
		return nil, err
	}
	var out []Peer
	for {
		select {
		case <-c.closed:
			return nil, fmt.Errorf("closed")
		case l := <-c.replies:
			switch l.Verb {
			case protocol.VerbPeer:
				h, err := l.ParseHello()
				if err != nil {
					continue
				}
				out = append(out, Peer{
					ID: h.ID, Name: h.Name, PID: h.PID,
					Pane: h.Pane, CWD: h.CWD, Subs: h.Subs,
				})
			case protocol.VerbOK:
				return out, nil
			case protocol.VerbErr:
				return nil, fmt.Errorf("%s", l.Rest)
			}
		}
	}
}

// Close tears down the connection and unblocks readers.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		close(c.closed)
		_ = c.nc.Close()
	})
	return nil
}

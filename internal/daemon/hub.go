package daemon

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/tjstebbing/ttybus/internal/protocol"
)

const outQueue = 256

// Hub is the in-memory subscription router. It is safe for concurrent use.
type Hub struct {
	mu    sync.Mutex
	conns map[string]*Conn
	subs  map[string]map[string]*Conn
	seq   atomic.Uint64
}

func newHub() *Hub {
	return &Hub{
		conns: make(map[string]*Conn),
		subs:  make(map[string]map[string]*Conn),
	}
}

func (h *Hub) register(c *Conn, wantID string) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	id := wantID
	if id == "" {
		id = fmt.Sprintf("auto-%d", h.seq.Add(1))
	} else if err := protocol.ValidID(id); err != nil {
		return "", err
	}
	if _, ok := h.conns[id]; ok {
		return "", protocol.ErrID
	}
	c.id = id
	c.hello.ID = id
	h.conns[id] = c
	return id, nil
}

func (h *Hub) unregister(c *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if c.id == "" {
		return
	}
	delete(h.conns, c.id)
	for ch, m := range h.subs {
		delete(m, c.id)
		if len(m) == 0 {
			delete(h.subs, ch)
		}
	}
	c.id = ""
}

func (h *Hub) sub(c *Conn, channel string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	m := h.subs[channel]
	if m == nil {
		m = make(map[string]*Conn)
		h.subs[channel] = m
	}
	m[c.id] = c
	c.subSet[channel] = struct{}{}
}

func (h *Hub) unsub(c *Conn, channel string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if m := h.subs[channel]; m != nil {
		delete(m, c.id)
		if len(m) == 0 {
			delete(h.subs, channel)
		}
	}
	delete(c.subSet, channel)
}

func (h *Hub) pub(channel, payload string) int {
	msg := protocol.FormatMsg(channel, payload)
	h.mu.Lock()
	n := 0
	if m := h.subs[channel]; m != nil {
		n = len(m)
		for _, c := range m {
			c.enqueue(msg, channel)
		}
	}
	h.mu.Unlock()
	return n
}

func (h *Hub) send(id, channel, payload string) error {
	msg := protocol.FormatMsg(channel, payload)
	h.mu.Lock()
	defer h.mu.Unlock()
	c, ok := h.conns[id]
	if !ok {
		return errNoPeer
	}
	c.enqueue(msg, channel)
	return nil
}

func (h *Hub) list() []protocol.Hello {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]protocol.Hello, 0, len(h.conns))
	for _, c := range h.conns {
		hcopy := c.hello
		hcopy.ID = c.id
		hcopy.Subs = c.subNames()
		out = append(out, hcopy)
	}
	return out
}

func (c *Conn) subNames() []string {
	if len(c.subSet) == 0 {
		return nil
	}
	s := make([]string, 0, len(c.subSet))
	for ch := range c.subSet {
		s = append(s, ch)
	}
	return s
}

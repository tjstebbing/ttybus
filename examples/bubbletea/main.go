// A tiny TUI that subscribes to a ttybus channel and treats each payload
// as a Bubble Tea message. Type a line and press enter to PUB.
package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	ttybus "github.com/tjstebbing/ttybus/client"
)

type busMsg struct {
	payload string
}

type model struct {
	ch      <-chan string
	client  *ttybus.Client
	channel string
	lines   []string
	input   string
	err     error
}

func waitBus(ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		s, ok := <-ch
		if !ok {
			return closedMsg{}
		}
		return busMsg{payload: s}
	}
}

func (m model) Init() tea.Cmd { return waitBus(m.ch) }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyEnter:
			text := strings.TrimSpace(m.input)
			m.input = ""
			if text == "" {
				return m, nil
			}
			c := m.client
			ch := m.channel
			return m, func() tea.Msg {
				if err := c.Pub(ch, text); err != nil {
					return errMsg{err}
				}
				return nil
			}
		case tea.KeyBackspace:
			if m.input != "" {
				m.input = m.input[:len(m.input)-1]
			}
			return m, nil
		case tea.KeyRunes:
			m.input += string(msg.Runes)
			return m, nil
		}
	case busMsg:
		m.lines = append(m.lines, msg.payload)
		if len(m.lines) > 20 {
			m.lines = m.lines[len(m.lines)-20:]
		}
		return m, waitBus(m.ch)
	case errMsg:
		m.err = msg.error
		return m, nil
	case closedMsg:
		return m, tea.Quit
	}
	return m, nil
}

type errMsg struct{ error }

type closedMsg struct{}

func (m model) View() string {
	title := lipgloss.NewStyle().Bold(true).Render("ttybus  " + m.channel)
	body := strings.Join(m.lines, "\n")
	if body == "" {
		body = "(waiting for messages)"
	}
	prompt := "> " + m.input
	err := ""
	if m.err != nil {
		err = "\n" + m.err.Error()
	}
	return fmt.Sprintf("%s\n\n%s\n\n%s%s\n", title, body, prompt, err)
}

func main() {
	channel := "demo"
	if len(os.Args) > 1 {
		channel = os.Args[1]
	}
	path := os.Getenv("TTYBUS_SOCKET")
	c, err := ttybus.Dial(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ttybus: %v\n(start with: ttybus serve)\n", err)
		os.Exit(1)
	}
	defer c.Close()
	if err := c.Hello(ttybus.Info{Name: "bubbletea-demo"}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ch, err := c.Sub(channel)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	p := tea.NewProgram(model{ch: ch, client: c, channel: channel})
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

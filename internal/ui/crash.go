package ui

import (
	"fmt"
	"runtime/debug"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

type Crash struct {
	mu    sync.Mutex
	Value any
	Stack []byte
	Where string

	prog *tea.Program
}

func (c *Crash) Attach(p *tea.Program) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.prog = p
}

func (c *Crash) record(where string, r any) {
	stack := debug.Stack()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Value == nil {
		c.Value, c.Stack, c.Where = r, stack, where
	}
}

func (c *Crash) Crashed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Value != nil
}

func (c *Crash) Summary() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return strings.Join(strings.Fields(fmt.Sprintf("%v (in %s)", c.Value, c.Where)), " ")
}

type crashMsg struct{}

type guarded struct {
	inner tea.Model
	crash *Crash
}

func Guard(m tea.Model) (tea.Model, *Crash) {
	c := &Crash{}
	return guarded{inner: m, crash: c}, c
}

func (g guarded) Init() (cmd tea.Cmd) {
	defer func() {
		if r := recover(); r != nil {
			g.crash.record("init", r)
			cmd = tea.Quit
		}
	}()
	return g.wrap(g.inner.Init())
}

func (g guarded) Update(msg tea.Msg) (next tea.Model, cmd tea.Cmd) {
	if _, ok := msg.(crashMsg); ok || g.crash.Crashed() {
		return g, tea.Quit
	}
	defer func() {
		if r := recover(); r != nil {
			g.crash.record("update", r)
			next, cmd = g, tea.Quit
		}
	}()
	inner, c := g.inner.Update(msg)
	g.inner = inner
	return g, g.wrap(c)
}

func (g guarded) View() (out string) {
	if g.crash.Crashed() {
		return ""
	}
	defer func() {
		if r := recover(); r != nil {
			g.crash.record("view", r)
			out = ""
			g.crash.mu.Lock()
			p := g.crash.prog
			g.crash.mu.Unlock()
			if p != nil {

				go p.Quit()
			}
		}
	}()
	return g.inner.View()
}

func (g guarded) wrap(c tea.Cmd) tea.Cmd {
	if c == nil {
		return nil
	}
	return func() (msg tea.Msg) {
		defer func() {
			if r := recover(); r != nil {
				g.crash.record("command", r)
				msg = crashMsg{}
			}
		}()
		msg = c()
		if batch, ok := msg.(tea.BatchMsg); ok {
			out := make(tea.BatchMsg, len(batch))
			for i, b := range batch {
				out[i] = g.wrap(b)
			}
			return out
		}
		return msg
	}
}

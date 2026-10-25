package ui

import (
	"bytes"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type boom struct{ in string }

type goMsg struct{}

func (b boom) Init() tea.Cmd {
	if b.in == "init" {
		var s []int
		_ = s[-1+len(s)]
	}
	return func() tea.Msg { return goMsg{} }
}

func (b boom) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(goMsg); !ok {
		return b, nil
	}
	switch b.in {
	case "update":
		var s []int
		i := -1
		_ = s[i]
	case "command":
		return b, func() tea.Msg { var p *model; return p.status }
	case "batch":
		return b, tea.Batch(nil, func() tea.Msg { panic("in a batch") })
	case "view":
		b.in = "view!"
	case "none":
		return b, tea.Quit
	}
	return b, nil
}

func (b boom) View() string {
	if b.in == "view!" {
		panic("in view")
	}
	return "ok"
}

func TestGuardRecoversPanics(t *testing.T) {
	for _, where := range []string{"init", "update", "command", "batch", "view"} {
		t.Run(where, func(t *testing.T) {
			m, crash := Guard(boom{in: where})
			var out bytes.Buffer
			p := tea.NewProgram(m, tea.WithInput(nil), tea.WithOutput(&out), tea.WithoutSignals())
			crash.Attach(p)
			done := make(chan error, 1)
			var final tea.Model
			go func() { var err error; final, err = p.Run(); done <- err }()
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("Run: %v", err)
				}
			case <-time.After(5 * time.Second):
				p.Kill()
				t.Fatal("program did not quit after the panic")
			}
			if !crash.Crashed() || len(crash.Stack) == 0 {
				t.Fatal("no crash recorded")
			}
			want := map[string]string{"init": "init", "update": "update", "command": "command", "batch": "command", "view": "view"}[where]
			if s := crash.Summary(); !strings.HasSuffix(s, "(in "+want+")") || strings.Contains(s, "\n") {
				t.Fatalf("summary %q", s)
			}
			if s := Finish(final); s != (Session{}) {
				t.Fatalf("finish after a crash: %+v", s)
			}
		})
	}
}

func TestGuardTransparent(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	m.kubeconfig, m.context = "/kc", "kind-dev"
	g, crash := Guard(m)
	next, _ := g.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if crash.Crashed() || !strings.Contains(next.View(), "k2") {
		t.Fatal("guard changed a normal run")
	}
	if s := Finish(next); s.Kubeconfig != "/kc" || s.Context != "kind-dev" {
		t.Fatalf("finish %+v", s)
	}
	b, c2 := Guard(boom{in: "none"})
	if _, cmd := b.Update(goMsg{}); cmd == nil || c2.Crashed() {
		t.Fatal("quit lost")
	}
}

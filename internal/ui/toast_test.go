package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"k2/internal/shell"
	"k2/internal/theme"
)

func TestToastLifecycle(t *testing.T) {
	cfg := testEnv(t)
	cfg.UI.ToastSeconds = 1
	m := newTestModel(t, cfg, nil)
	first := m.notify(toneOK, "forward started")
	if first == nil || m.toast.text != "forward started" {
		t.Fatalf("toast %+v", m.toast)
	}
	old := m.toast.id
	m.notify(toneErr, "apply failed")
	if m.toast.text != "apply failed" || m.toast.tone != toneErr {
		t.Fatalf("toast %+v", m.toast)
	}

	next, _ := m.Update(toastExpiredMsg{id: old})
	m = next.(model)
	if m.toast.text != "apply failed" {
		t.Fatal("an old timer cleared the current toast")
	}
	next, _ = m.Update(toastExpiredMsg{id: m.toast.id})
	if m = next.(model); m.toast.text != "" {
		t.Fatal("toast not cleared by its own timer")
	}

	if msg := first(); msg != (toastExpiredMsg{id: old}) {
		t.Fatalf("timer produced %#v", msg)
	}
}

func TestToastsCanBeDisabled(t *testing.T) {
	cfg := testEnv(t)
	cfg.UI.Toasts = false
	m := newTestModel(t, cfg, nil)
	if cmd := m.notify(toneOK, "x"); cmd != nil || m.toast.text != "" {
		t.Fatal("toast shown although ui.toasts is false")
	}
}

func TestToastRenderedInBorder(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	defer useTheme(theme.Nord())
	for _, name := range theme.Names() {
		cfg := testEnv(t)
		cfg.Theme = name
		m := newTestModel(t, cfg, nil)
		m = press(t, m, "ctrl+a", "h")
		m.notify(toneOK, "secret decoded")
		out := m.View()
		lines := strings.Split(ansi.Strip(out), "\n")
		var bottoms []string
		for _, l := range lines {
			if strings.Contains(l, "╰") && strings.Contains(l, "╯") {
				bottoms = append(bottoms, l)
			}
		}
		joined := strings.Join(bottoms, "\n")
		if strings.Count(joined, "✓ secret decoded") != 1 {
			t.Fatalf("%s: toast not in exactly one bottom border:\n%s", name, joined)
		}
		styled := toneStyle(toneOK).Bold(true).Render(" ✓ secret decoded ")
		if !strings.Contains(out, styled) || !strings.Contains(ansi.Strip(out), "✓ secret decoded ━━╯") {
			t.Fatalf("%s: toast not right-aligned in the ok color", name)
		}
		m.notify(toneErr, "apply failed")
		if out := m.View(); !strings.Contains(out, "✗ apply failed") {
			t.Fatalf("%s: error toast missing", name)
		}
	}
}

func TestToastAfterDecodeExpires(t *testing.T) {
	cfg := testEnv(t)
	cfg.UI.ToastSeconds = 1
	f := &shell.Fake{Handler: func(shell.Request) shell.Result { return shell.OK(`{"data":{"k":"dg=="}}`) }}
	m := newTestModel(t, cfg, f)
	m.kubeconfig, m.context, m.kind, m.stage = "/kc", "c", "secrets", "res"
	m.content.SetItems([]list.Item{item{id: "db", title: "db"}})
	tm := start(t, m)
	send(t, tm, "x")
	waitFor(t, tm, "✓ secret decoded")
	time.Sleep(1500 * time.Millisecond)
	final := finish(t, tm)
	if final.toast.text != "" || final.stage != "view" {
		t.Fatalf("toast %q stage %q", final.toast.text, final.stage)
	}
}

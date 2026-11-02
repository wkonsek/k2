package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"k2/internal/config"
)

func helpAt(t *testing.T, m model, stage, kind string) model {
	t.Helper()
	m.stage, m.kind = stage, kind
	next, _ := m.openHelp()
	return next.(model)
}

func helpText(m model) string {
	lines, _, _ := m.helpLayout()
	return ansi.Strip(strings.Join(lines, "\n"))
}

func TestHelpSectionsPerView(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	m.width, m.height = 120, 35
	cases := []struct{ stage, kind, title, row string }{
		{"nav", "", "Home", "list pods"},
		{"workpods", "", "Pods", "port-forward"},
		{"res", "deployments", "Resources · deployments", "rollout restart"},
		{"res", "secrets", "Resources · secrets", "decode the secret"},
		{"logs", "", "Logs", "previous container"},
		{"mlogs", "", "Logs", "filter lines"},
		{"yaml", "", "Viewer", "watch for changes"},
		{"describe", "", "Viewer", "watch for changes"},
		{"kdir", "", "Kustomize", "scan for kustomizations"},
		{"kust", "", "Kustomize", "open folder in code"},
		{"fwds", "", "Forwards", "auto restart on / off"},
		{"fwds", "", "New forward", "next suggested port"},
		{"helm", "", "Helm", "computed values"},
		{"helmhist", "", "Helm history", "diff marked"},
		{"xray", "", "Owner tree", "expand / collapse all"},
		{"kcmd", "", "kubectl", "output format"},
		{"contexts", "", "Pick a context", "choose"},
		{"projects", "", "Pick a namespace", "choose"},
		{"pulse", "", "Viewer", "all namespaces"},
		{"weird", "", "Actions", "open"},
	}
	for _, c := range cases {
		h := helpAt(t, m, c.stage, c.kind)
		text := helpText(h)
		nav, sec := strings.Index(text, "Navigation"), strings.Index(text, c.title)
		if nav < 0 || sec < 0 || nav > sec || strings.Index(text, "\n") < nav {
			t.Errorf("%s: Navigation %d, %q %d:\n%s", c.stage, nav, c.title, sec, text)
		}
		if !strings.Contains(text, c.row) || !strings.Contains(text, "split right / down") || !strings.Contains(text, "ctrl+a % \"") {
			t.Errorf("%s: missing rows:\n%s", c.stage, text)
		}
		if !strings.HasPrefix(h.dlg.title, "Help · ") {
			t.Errorf("%s: title %q", c.stage, h.dlg.title)
		}
	}
	if text := helpText(helpAt(t, m, "res", "configmaps")); strings.Contains(text, "rollout restart") || strings.Contains(text, "decode") {
		t.Errorf("configmaps get deployment keys:\n%s", text)
	}
}

func TestHelpFitsTheScreen(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	for _, size := range [][2]int{{120, 35}, {80, 24}, {40, 12}, {30, 10}, {20, 8}} {
		for _, stage := range []string{"nav", "workpods", "kdir", "fwds", "logs", "helm"} {
			m.width, m.height = size[0], size[1]
			h := helpAt(t, m, stage, "")
			box, w, bh := h.helpBox()
			rows := strings.Split(box, "\n")
			if w > size[0] || bh > size[1] || len(rows) != bh {
				t.Fatalf("%v %s: box %dx%d, %d rows", size, stage, w, bh, len(rows))
			}
			for _, r := range rows {
				if ansi.StringWidth(r) != w {
					t.Fatalf("%v %s: row %d wide, box %d: %q", size, stage, ansi.StringWidth(r), w, ansi.Strip(r))
				}
			}
			view := strings.Split(h.View(), "\n")
			h.dlg = nil
			if len(view) != len(strings.Split(h.View(), "\n")) {
				t.Fatalf("%v %s: view has %d rows", size, stage, len(view))
			}
			for _, r := range view {
				if ansi.StringWidth(r) > size[0] {
					t.Fatalf("%v %s: view row %d wide", size, stage, ansi.StringWidth(r))
				}
			}
			if !strings.Contains(ansi.Strip(strings.Join(view, "\n")), "Help · ") && size[0] >= 30 {
				t.Fatalf("%v %s: no popup", size, stage)
			}
		}
	}
	m.width, m.height = 120, 35
	box, w, _ := helpAt(t, m, "kdir", "").helpBox()
	if text := ansi.Strip(box); !strings.Contains(text, "Kustomize") || strings.Contains(text, "more") || w > 120 {
		t.Fatalf("120x35 should show it all:\n%s", text)
	}
}

func TestHelpKeyOpensAndCloses(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	for _, k := range []string{"?", "esc", "q"} {
		m = press(t, m, "?")
		if m.dlg == nil || m.dlg.act != "help" || !strings.Contains(ansi.Strip(m.View()), "Help · home") {
			t.Fatal("? did not open the help")
		}
		m = press(t, m, "n", "K", "f", ":")
		if m.dlg == nil || m.stage != "nav" || m.cmdOn {
			t.Fatalf("keys leaked through the help: stage %q", m.stage)
		}
		if m = press(t, m, k); m.dlg != nil || m.quitting || m.stage != "nav" {
			t.Fatalf("%s did not close the help", k)
		}
	}
	m = press(t, m, "?")
	if m = press(t, m, "ctrl+c"); !m.quitting {
		t.Fatal("ctrl+c does not quit from the help")
	}
}

func TestHelpScrolls(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	m.width, m.height = 40, 12
	m = press(t, m, "?")
	lines, _, room := m.helpLayout()
	last := len(lines) - room
	if last < 5 {
		t.Fatalf("expected a long help at 40x12, got %d lines in %d rows", len(lines), room)
	}
	box := func() string { b, _, _ := m.helpBox(); return ansi.Strip(b) }
	if !strings.Contains(box(), "more") {
		t.Fatalf("no more indicator:\n%s", box())
	}
	first := box()
	if m = press(t, m, "j"); m.dlg.idx != 1 || box() == first {
		t.Fatalf("j did not scroll: %d", m.dlg.idx)
	}
	if m = press(t, m, "down", "k"); m.dlg.idx != 1 {
		t.Fatalf("down/k: %d", m.dlg.idx)
	}
	if m = press(t, m, "pgdown"); m.dlg.idx != min(1+room, last) {
		t.Fatalf("pgdown: %d", m.dlg.idx)
	}
	if m = press(t, m, "G"); m.dlg.idx != last || strings.Contains(box(), "more") {
		t.Fatalf("G: %d\n%s", m.dlg.idx, box())
	}
	if m = press(t, m, "j", "j"); m.dlg.idx != last {
		t.Fatalf("scrolled past the end: %d", m.dlg.idx)
	}
	if m = press(t, m, "pgup", "g"); m.dlg.idx != 0 || box() != first {
		t.Fatalf("g: %d", m.dlg.idx)
	}
	next, _ := m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	if m = next.(model); m.dlg.idx != 3 {
		t.Fatalf("wheel: %d", m.dlg.idx)
	}
}

func TestHelpKeyTypesIntoInputs(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	if c := press(t, m, ":", "?"); c.dlg != nil || c.cmd.Value() != "?" {
		t.Fatalf("command line: dlg %v value %q", c.dlg != nil, c.cmd.Value())
	}
	if s := press(t, m, "U", "?"); s.dlg != nil || s.stage != "kcmd" || s.sh.Value() != "?" {
		t.Fatalf("kubectl line: %q", s.sh.Value())
	}
	if r := press(t, m, "ctrl+a", ",", "?"); r.dlg == nil || r.dlg.act != "rename_tab" || !strings.HasSuffix(r.dlg.input.Value(), "?") {
		t.Fatal("rename did not take ?")
	}
	f := m
	f.stage = "workpods"
	f.content.SetItems([]list.Item{item{id: "shop/web", title: "web"}})
	f = press(t, f, "/", "?")
	if f.dlg != nil || f.content.FilterInput.Value() != "?" {
		t.Fatalf("filter: %q", f.content.FilterInput.Value())
	}
	p := m
	p.pfOn = true
	p.portInput.SetValue("")
	p.portInput.Focus()
	if p = press(t, p, "?"); p.dlg != nil || p.portInput.Value() != "?" {
		t.Fatalf("port input: %q", p.portInput.Value())
	}
}

func TestHelpKeyRebinds(t *testing.T) {
	cfg := testEnv(t)
	c, w := config.Parse([]byte("keys:\n  help: f1\n"))
	if len(w) != 0 {
		t.Fatal(w)
	}
	cfg.Keys = c.Keys
	m := newTestModel(t, cfg, nil)
	if m = press(t, m, "?"); m.dlg != nil {
		t.Fatal("? still opens the help")
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyF1})
	m = next.(model)
	if m.dlg == nil || m.dlg.act != "help" {
		t.Fatal("f1 did not open the help")
	}
	if text := helpText(m); !strings.Contains(text, "f1/esc") || strings.Contains(text, "?") {
		t.Fatalf("help does not show the new key:\n%s", text)
	}
	if !strings.Contains(ansi.Strip(m.legend()), "f1 help") {
		t.Fatalf("legend %q", ansi.Strip(m.legend()))
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyF1})
	if next.(model).dlg != nil {
		t.Fatal("f1 did not close the help")
	}
}

func TestHelpInEditorPane(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	m.stage, m.subject, m.logText = "view", "notes", "x\n"
	h := newEdHarness(t, m)
	startCat(t, h)
	h.typeText("a?b")
	h.pump(func() bool { return strings.Contains(h.screen(), "a?b") }, "cat to echo the ?")
	if h.m.dlg != nil {
		t.Fatal("? in the editor opened the help")
	}
	h.keys("ctrl+a", "?")
	if h.m.dlg == nil || h.m.dlg.act != "help" || !strings.Contains(helpText(h.m), "Shell / vim pane") {
		t.Fatal("ctrl+a ? did not open the help over the editor")
	}
	h.keys("esc")
	if h.m.dlg != nil || !h.m.ed.live() {
		t.Fatal("esc did not close the help back to the editor")
	}
	h.m.killEditors()
}

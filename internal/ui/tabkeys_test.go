package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/charmbracelet/x/ansi"

	"k2/internal/config"
)

func TestTmuxKeysAfterPrefix(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	m = press(t, m, "ctrl+a", `"`)
	if got := shape(m, m.tabs[0].root); got != "col(1,2)" {
		t.Fatalf(`ctrl+a ": %s`, got)
	}
	m = press(t, m, "ctrl+a", "%")
	if got := shape(m, m.tabs[0].root); got != "col(1,row(2,3))" {
		t.Fatalf("ctrl+a %%: %s", got)
	}

	before := m.tabPos(m.logID)
	m = press(t, m, "ctrl+a", "o")
	if m.tabPos(m.logID) != (before+1)%3 {
		t.Fatalf("ctrl+a o: %d after %d", m.tabPos(m.logID), before)
	}
	m = press(t, m, "ctrl+a", ";")
	if m.tabPos(m.logID) != before {
		t.Fatalf("ctrl+a ;: %d want %d", m.tabPos(m.logID), before)
	}

	m = press(t, m, "ctrl+a", "z")
	if !m.zoom {
		t.Fatal("ctrl+a z did not zoom")
	}
	m = press(t, m, "ctrl+a", "z")
	if m.zoom {
		t.Fatal("ctrl+a z did not unzoom")
	}

	m = press(t, m, "ctrl+a", "c")
	if len(m.tabs) != 2 || m.tab != 1 {
		t.Fatalf("ctrl+a c: tabs %d active %d", len(m.tabs), m.tab)
	}
	m = press(t, m, "ctrl+a", "n")
	if m.tab != 0 {
		t.Fatalf("ctrl+a n: tab %d", m.tab)
	}
	m = press(t, m, "ctrl+a", "p")
	if m.tab != 1 {
		t.Fatalf("ctrl+a p: tab %d", m.tab)
	}
	m = press(t, m, "ctrl+a", "&")
	if m.dlg == nil || !strings.Contains(m.asking(), "close tab") {
		t.Fatalf("ctrl+a &: no popup (%q)", m.asking())
	}
	m = press(t, m, "y")
	if len(m.tabs) != 1 {
		t.Fatalf("ctrl+a & y: tabs %d", len(m.tabs))
	}

	m = press(t, m, "ctrl+a", "t", "ctrl+a", "w", "y")
	if len(m.tabs) != 1 {
		t.Fatalf("ctrl+a t / w: tabs %d", len(m.tabs))
	}
	k := m.cfg.Keys
	if k.PrefixedAction("c") != "new_tab" || !k.Context.Has("c") || k.DirectAction("c") != "" {
		t.Fatalf("c: after prefix %q, context %v", k.PrefixedAction("c"), k.Context)
	}
}

func TestCtrlAIsLineStartInInputs(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	m = press(t, m, ":", "x", "y", "ctrl+a", "z")
	if m.prefixOn || m.cmd.Value() != "zxy" {
		t.Fatalf("prefix %v value %q", m.prefixOn, m.cmd.Value())
	}

	k := config.DefaultKeys()
	if k.Prefix.Label() != "ctrl+a" {
		t.Fatalf("prefix %v", k.Prefix)
	}
}

func TestRenameTab(t *testing.T) {
	m := tabbed(t)
	m = press(t, m, "ctrl+a", "1")
	if got := m.tabLabel(0); got != "1 kind-dev" {
		t.Fatalf("auto label %q", got)
	}
	m = press(t, m, "ctrl+a", ",")
	if m.dlg == nil || m.dlg.act != "rename_tab" || m.dlg.input.Value() != "kind-dev" {
		t.Fatalf("popup %+v", m.dlg)
	}
	if !strings.Contains(ansi.Strip(m.View()), "rename tab 1") {
		t.Fatal("popup not drawn")
	}
	m = press(t, m, "ctrl+u", "k", "a", "f", "k", "a", "-", "d", "e", "b", "u", "g", "enter")
	if m.dlg != nil || m.tabs[0].name != "kafka-debug" || m.tabLabel(0) != "1 kafka-debug" {
		t.Fatalf("renamed: dlg %v name %q label %q", m.dlg != nil, m.tabs[0].name, m.tabLabel(0))
	}
	if !strings.Contains(ansi.Strip(m.tabBar(120)), "1 kafka-debug") {
		t.Fatalf("tab bar %q", ansi.Strip(m.tabBar(120)))
	}

	m = press(t, m, "ctrl+a", ",", "ctrl+u", "x", "esc")
	if m.dlg != nil || m.tabs[0].name != "kafka-debug" {
		t.Fatalf("esc: name %q", m.tabs[0].name)
	}

	m = press(t, m, "ctrl+a", "v")
	m.context = "other"
	m = press(t, m, "ctrl+a", "x", "y")
	if m.tabs[0].name != "kafka-debug" || len(m.tabIDs()) != 1 {
		t.Fatalf("after split/close: name %q panes %d", m.tabs[0].name, len(m.tabIDs()))
	}

	m = press(t, m, "ctrl+a", ",", "ctrl+u", "enter")
	if m.tabs[0].name != "" || m.tabLabel(0) != "1 "+m.tabAutoName(0) {
		t.Fatalf("reset: name %q label %q", m.tabs[0].name, m.tabLabel(0))
	}

	m = press(t, m, "ctrl+a", ",", "enter")
	if m.tabs[0].name != "" {
		t.Fatalf("unchanged auto name stored: %q", m.tabs[0].name)
	}
}

func TestTabNamesPersist(t *testing.T) {
	cfg := testEnv(t)
	m := newTestModel(t, cfg, nil)
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	m = press(t, m, "ctrl+a", "t", "ctrl+a", ",", "ctrl+u", "l", "o", "g", "s", "enter")
	m.save()
	again := newTestModel(t, cfg, nil)
	if len(again.tabs) != 2 || again.tabs[1].name != "logs" || again.tabs[0].name != "" {
		t.Fatalf("restored tabs %d names %q %q", len(again.tabs), again.tabs[0].name, again.tabs[1].name)
	}
	if got := cleanTabName("  a\tb\x1b[31m  c  "); got != "a b c" {
		t.Fatalf("clean %q", got)
	}
	if got := cleanTabName(strings.Repeat("x", 99)); len(got) != maxTabName {
		t.Fatalf("not capped: %d", len(got))
	}
}

func TestTabBarCutsNamesKeepsNumbers(t *testing.T) {
	m := tabbed(t)
	for i := range m.tabs {
		m.tabs[i].name = "a-very-long-custom-tab-name-" + string(rune('a'+i))
	}
	for _, w := range []int{40, 50, 60} {
		bar := ansi.Strip(m.tabBar(w))
		if ansi.StringWidth(bar) > w {
			t.Fatalf("width %d: bar %d wide", w, ansi.StringWidth(bar))
		}
		for i := range m.tabs {
			if !strings.Contains(bar, " "+string(rune('1'+i))+" a-") {
				t.Fatalf("width %d: tab %d number lost: %q", w, i+1, bar)
			}
		}
	}
}

func TestStartNoticeToast(t *testing.T) {
	cfg := testEnv(t)
	m := initialModel(Options{Config: &cfg, Notice: "created ~/.k2/config.yaml (all options commented)"})
	m.width, m.height = 120, 40
	cmd := m.Init()
	var found bool
	for _, msg := range drain(cmd) {
		if n, ok := msg.(noticeMsg); ok {
			next, _ := m.Update(n)
			m = next.(model)
			found = true
		}
	}
	if !found || m.toast.text != "created ~/.k2/config.yaml (all options commented)" || !strings.Contains(m.status, "created ~/.k2") {
		t.Fatalf("toast %q status %q", m.toast.text, m.status)
	}
}

func drain(cmd tea.Cmd) []tea.Msg { return drainWait(cmd, 200*time.Millisecond) }

func drainWait(cmd tea.Cmd, wait time.Duration) []tea.Msg {
	if cmd == nil {
		return nil
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		if b, ok := msg.(tea.BatchMsg); ok {
			var out []tea.Msg
			for _, c := range b {
				out = append(out, drainWait(c, wait)...)
			}
			return out
		}
		return []tea.Msg{msg}
	case <-time.After(wait):
		return nil
	}
}

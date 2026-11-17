package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"k2/internal/shell"
)

func TestPaneTitleComposition(t *testing.T) {
	frame := dimStyle
	for _, c := range []struct {
		t    paneTitle
		want string
	}{
		{paneTitle{num: 1, ctx: "kind-kindest", view: "all/pods"}, "1 kind-kindest · all/pods"},
		{paneTitle{num: 2, ctx: "ctx", config: "dev-eu-1", view: "shop/pods"}, "2 dev-eu-1:ctx · shop/pods"},
		{paneTitle{num: 3, hot: true, ctx: "prod", view: "home"}, "3 ☠ prod · home"},
		{paneTitle{num: 1, view: "home"}, "1 home"},
	} {
		if got := ansi.Strip(c.t.render(80, frame)); got != c.want {
			t.Errorf("title %q, want %q", got, c.want)
		}
	}
}

func TestPaneTitleTruncationOrder(t *testing.T) {
	pt := paneTitle{num: 1, ctx: "kind-kindest", config: "dev-eu-1", view: "shop/deployments"}
	for _, c := range []struct {
		width int
		want  string
	}{
		{60, "1 dev-eu-1:kind-kindest · shop/deployments"},
		{33, "1 kind-kindest · shop/deployments"},
		{25, "1 kind-kindest · shop/de…"},
		{19, "1 kind-kindest · s…"},
		{18, "1 kind-kindest"},
		{10, "1 kind-ki…"},
		{1, "1"},
	} {
		got := ansi.Strip(pt.render(c.width, dimStyle))
		if got != c.want {
			t.Errorf("width %d: %q, want %q", c.width, got, c.want)
		}
		if ansi.StringWidth(got) > c.width {
			t.Errorf("width %d: %q is too wide", c.width, got)
		}
	}
}

func TestShortKubeconfig(t *testing.T) {
	home, _ := os.UserHomeDir()
	for kc, want := range map[string]string{
		"":                                     "",
		filepath.Join(home, ".kube", "config"): "",
		"/home/x/.kube/dev-eu-1":               "dev-eu-1",
		"/configs/dev-eu-1.yaml":               "dev-eu-1",
		"/configs/.hidden":                     ".hidden",
	} {
		if got := shortKubeconfig(kc); got != want {
			t.Errorf("shortKubeconfig(%q) = %q, want %q", kc, got, want)
		}
	}
}

func TestPaneTitleEverywhere(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	home, _ := os.UserHomeDir()
	m.kubeconfig, m.context = filepath.Join(home, ".kube", "config"), "kind-kindest"
	m = press(t, m, "ctrl+a", "h")
	m.context = "kind-other"
	view := ansi.Strip(m.View())
	for _, want := range []string{"╭ 1 kind-kindest · home", "╭ 2 kind-other · home"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	m = press(t, m, "Z")
	if view = ansi.Strip(m.View()); !m.zoom || !strings.Contains(view, "╭ 2 kind-other · home") {
		t.Errorf("fullscreen title:\n%s", view)
	}
}

func TestPickWatchBadge(t *testing.T) {
	if got := pickWatchBadge(80, 20, 2); got != "⟳ watch 2s" {
		t.Fatalf("wide: %q", got)
	}
	if got := pickWatchBadge(80, 20, 5); got != "⟳ watch 5s" {
		t.Fatalf("configured: %q", got)
	}
	if got := pickWatchBadge(30, 18, 2); got != "⟳ 2s" {
		t.Fatalf("mid-width: %q", got)
	}
	if got := pickWatchBadge(24, 18, 2); got != "⟳" && got != "" {
		t.Fatalf("narrow: %q", got)
	}
	if got := pickWatchBadge(18, 16, 2); got != "" {
		t.Fatalf("too tight should be empty unless ⟳ fits: %q", got)
	}
}

func TestWatchBadgeInPaneTitle(t *testing.T) {
	text := "apiVersion: v1\nkind: Pod\nmetadata:\n  name: web-1\n"
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		switch {
		case len(r.Args) > 1 && r.Args[0] == "get" && r.Args[1] == "pods":
			return shell.OK(podsJSON)
		case len(r.Args) >= 5 && r.Args[0] == "get" && r.Args[3] == "-o" && r.Args[4] == "yaml":
			return shell.OK(text)
		case len(r.Args) > 0 && r.Args[0] == "describe":
			return shell.OK("Name: web-1\nStatus: Pending\n")
		}
		return shell.Fail(1, "not in this test")
	}}
	m := newTestModel(t, testEnv(t), f)
	m.kubeconfig, m.context = "/kc", "kind-dev"
	n, c := m.openPods()
	m = settle(t, n.(model), c)
	m = pressSettle(t, m, "d")
	if m.stage != "describe" {
		t.Fatalf("stage %q", m.stage)
	}
	view := ansi.Strip(m.View())
	if strings.Contains(view, "⟳") || strings.Contains(view, "watch 2s") {
		t.Fatalf("badge while off:\n%s", view)
	}
	m = pressSettle(t, m, "r")
	if !m.vs.watchOn {
		t.Fatal("watch not on")
	}
	view = ansi.Strip(m.View())
	if !strings.Contains(view, "⟳ watch 2s") {
		t.Fatalf("missing badge:\n%s", view)
	}
	m.cfg.UI.WatchEvery = 5
	view = ansi.Strip(m.View())
	if !strings.Contains(view, "⟳ watch 5s") || strings.Contains(view, "⟳ watch 2s") {
		t.Fatalf("interval not from config:\n%s", view)
	}
	m = press(t, m, "r")
	view = ansi.Strip(m.View())
	if m.vs.watchOn || strings.Contains(view, "⟳") {
		t.Fatalf("badge after off: on %v\n%s", m.vs.watchOn, view)
	}
}

func TestBoxTopKeepsLeftTitleWithoutBadge(t *testing.T) {
	got := ansi.Strip(boxTop(roundGlyphs, dimStyle, 40, "1 kind-dev · shop/web-config", ""))
	want := "╭ 1 kind-dev · shop/web-config ──────────╮"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	got = ansi.Strip(boxTop(roundGlyphs, dimStyle, 50, "1 kind-dev · shop/web-config", "⟳ watch 2s"))
	if !strings.HasPrefix(got, "╭ 1 kind-dev · shop/web-config") || !strings.HasSuffix(got, "⟳ watch 2s ╮") {
		t.Fatalf("with badge: %q", got)
	}
	if ansi.StringWidth(got) != 52 {
		t.Fatalf("width %d for %q", ansi.StringWidth(got), got)
	}
	got = ansi.Strip(boxTop(roundGlyphs, dimStyle, 40, "1 kind-dev · shop/web-config", "⟳ 2s"))
	if !strings.HasSuffix(got, "⟳ 2s ╮") || !strings.Contains(got, "kind-dev") {
		t.Fatalf("short badge: %q", got)
	}
	if ansi.StringWidth(got) != 42 {
		t.Fatalf("short width %d for %q", ansi.StringWidth(got), got)
	}
}

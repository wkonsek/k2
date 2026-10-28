package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
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

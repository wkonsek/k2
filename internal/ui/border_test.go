package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func normalizeBorders(s string) string {
	r := strings.NewReplacer("┏", "╭", "┓", "╮", "┗", "╰", "┛", "╯", "━", "─", "┃", "│",
		"╔", "╭", "╗", "╮", "╚", "╰", "╝", "╯", "═", "─", "║", "│")
	return r.Replace(s)
}

func borderScreen(t *testing.T, style string, w, h int, keys ...string) string {
	t.Helper()
	cfg := testEnv(t)
	cfg.UI.ActiveBorder = style
	m := newTestModel(t, cfg, nil)
	m.kubeconfig, m.context = "/kc", "kind-dev"
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m = next.(model)
	m = press(t, m, keys...)
	return ansi.Strip(m.View())
}

func TestActiveBorderKeepsLayout(t *testing.T) {
	layouts := [][]string{nil, {"ctrl+n"}, {"ctrl+v"}, {"ctrl+n", "ctrl+v"}, {"ctrl+v", "Z"}}
	for _, size := range [][2]int{{60, 20}, {100, 30}, {160, 48}} {
		for _, keys := range layouts {
			base := borderScreen(t, "normal", size[0], size[1], keys...)
			for _, style := range []string{"thick", "double"} {
				got := borderScreen(t, style, size[0], size[1], keys...)
				if normalizeBorders(got) != base {
					t.Fatalf("%s %v %v changes the layout:\n%s\n---\n%s", style, size, keys, got, base)
				}
				corner := map[string]string{"thick": "┏", "double": "╔"}[style]
				if n := strings.Count(got, corner); n != 1 {
					t.Fatalf("%s %v %v: %d focused corners\n%s", style, size, keys, n, got)
				}
				for i, line := range strings.Split(got, "\n") {
					if ansi.StringWidth(line) != ansi.StringWidth(strings.Split(base, "\n")[i]) {
						t.Fatalf("line %d width differs", i)
					}
				}
			}
			if strings.ContainsAny(base, "┏╔") {
				t.Fatal("normal must keep the rounded border")
			}
		}
	}
}

func TestActiveBorderProdStaysRed(t *testing.T) {
	trueColor(t)
	cfg := testEnv(t)
	cfg.UI.ActiveBorder = "thick"
	cfg.Prod = []string{"prod-eu"}
	m := newTestModel(t, cfg, nil)
	m.kubeconfig, m.context = "/kc", "prod-eu"
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	view := next.(model).View()
	if colorBefore(view, "┏") != sgr(th.Danger) || !strings.Contains(ansi.Strip(view), skull) {
		t.Fatalf("prod border not red: %q", colorBefore(view, "┏"))
	}
}

func TestDimInactive(t *testing.T) {
	trueColor(t)
	for _, dim := range []bool{false, true} {
		cfg := testEnv(t)
		cfg.UI.DimInactive = dim
		m := newTestModel(t, cfg, nil)
		m.kubeconfig, m.context = "/kc", "kind-dev"
		next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
		m = press(t, next.(model), "ctrl+a", "v")
		views := strings.Split(m.View(), "\n")
		top := views[m.tabBarRows()+1]
		unfocused := top[:strings.Index(top, "2 ")]
		want := sgr(th.Accent)
		if dim {
			want = sgr(th.Muted)
		}
		if got := colorBefore(unfocused, "kind-dev"); !sameColor(got, want) {
			t.Fatalf("dim=%v: unfocused context color %q want %q", dim, got, want)
		}
		focused := top[strings.Index(top, "2 "):]
		if !sameColor(colorBefore(focused, "kind-dev"), sgr(th.Accent)) {
			t.Fatalf("dim=%v: focused context changed", dim)
		}
		if colorBefore(unfocused, "╭") != sgr(th.Muted) {
			t.Fatal("unfocused border should be muted")
		}
	}

	cfg := testEnv(t)
	cfg.UI.DimInactive = true
	cfg.Prod = []string{"prod-eu"}
	m := newTestModel(t, cfg, nil)
	m.kubeconfig, m.context = "/kc", "prod-eu"
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = press(t, next.(model), "ctrl+a", "v")
	top := strings.Split(m.View(), "\n")[m.tabBarRows()+1]
	unfocused := top[:strings.LastIndex(top, "2 ")]
	if !sameColor(colorBefore(unfocused, "prod-eu"), sgr(th.Danger)) || colorBefore(unfocused, "╭") != sgr(th.Danger) {
		t.Fatal("prod pane must stay red")
	}
}

func sameColor(got, want string) bool {
	return got != "" && strings.HasSuffix(got, strings.TrimPrefix(want, "\x1b["))
}

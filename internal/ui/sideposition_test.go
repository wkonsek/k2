package ui

import (
	"fmt"
	"strings"
	"testing"

	"k2/internal/forward"

	"github.com/charmbracelet/x/ansi"
)

func placedModel(t *testing.T, pos string, w, h int) model {
	t.Helper()
	cfg := testEnv(t)
	cfg.UI.SidebarPosition = pos
	m := newTestModel(t, cfg, nil)
	m.width, m.height = w, h
	m.kubeconfig, m.context, m.namespace = "/home/u/.kube/config", "kind-dev", "shop"
	m.forwards = []forward.Forward{{Local: "8080", Remote: "80", Pod: "web-0", Namespace: "shop"}}
	m.refit()
	return m
}

func TestSidebarPositionLayoutFills(t *testing.T) {
	for _, pos := range []string{"left", "right", "top", "bottom"} {
		for _, sz := range [][2]int{{120, 35}, {80, 24}, {200, 60}, {70, 20}} {
			m := placedModel(t, pos, sz[0], sz[1])
			sideW, mainW, panelH := m.layout()
			across := mainW + 2
			if sideW > 0 {
				across += sideW + 2
			}
			if across != sz[0] {
				t.Errorf("%s %v: width %d", pos, sz, across)
			}
			if down := panelH + 2 + 1 + m.tabBarRows() + m.stripH(); down != sz[1] {
				t.Errorf("%s %v: height %d", pos, sz, down)
			}
			if (pos == "top" || pos == "bottom") != (sideW == 0 && m.stripH() > 0) {
				t.Errorf("%s %v: side %d strip %d", pos, sz, sideW, m.stripH())
			}
			lines := strings.Split(m.View(), "\n")
			if len(lines) != sz[1] {
				t.Errorf("%s %v: %d lines", pos, sz, len(lines))
			}
			for i, l := range lines {
				if ansi.StringWidth(l) > sz[0] {
					t.Errorf("%s %v: line %d is %d wide", pos, sz, i, ansi.StringWidth(l))
				}
			}
			if plain := ansi.Strip(m.View()); !strings.Contains(plain, "kind-dev") || !strings.Contains(plain, "8080 → 80") {
				t.Errorf("%s %v: sidebar content missing\n%s", pos, sz, plain)
			}
		}
	}
}

func TestSidebarPositionSmallTerminals(t *testing.T) {
	for _, pos := range []string{"left", "right", "top", "bottom"} {
		for _, sz := range [][2]int{{1, 1}, {10, 5}, {30, 10}, {50, 14}, {300, 8}, {20, 80}} {
			m := placedModel(t, pos, sz[0], sz[1])
			for _, l := range strings.Split(m.View(), "\n") {
				if ansi.StringWidth(l) > sz[0] {
					t.Fatalf("%s %v: line %q", pos, sz, ansi.Strip(l))
				}
			}
			if _, _, panelH := m.layout(); panelH < 8 {
				t.Fatalf("%s %v: panel %d", pos, sz, panelH)
			}
		}
	}
}

func TestPaneOriginFollowsSidebar(t *testing.T) {
	for _, pos := range []string{"left", "right", "top", "bottom"} {
		m := placedModel(t, pos, 120, 35)
		x, y := m.mainOrigin()
		lines := strings.Split(ansi.Strip(m.View()), "\n")
		if got := []rune(lines[m.tabBarRows()+y])[x]; got != '╭' {
			t.Errorf("%s: origin %d,%d is %q", pos, x, y, got)
		}
		px, py, _ := m.popupRect()
		if px <= x || py <= y {
			t.Errorf("%s: popup at %d,%d outside the panes at %d,%d", pos, px, py, x, y)
		}
		if pos == "right" && x != 0 {
			t.Errorf("right: panes start at %d", x)
		}
	}
}

func TestStripCapsForwards(t *testing.T) {
	m := placedModel(t, "bottom", 140, 40)
	for i := 0; i < 9; i++ {
		m.forwards = append(m.forwards, forward.Forward{Local: fmt.Sprint(9000 + i), Remote: "80", Pod: fmt.Sprintf("p-%d", i)})
	}
	strip := ansi.Strip(m.sideStrip(140, m.stripH()))
	if !strings.Contains(strip, "+7") || strings.Contains(strip, "9003") {
		t.Fatalf("strip:\n%s", strip)
	}
	m.cfg.UI.ForwardDockRows = 1
	strip = ansi.Strip(m.sideStrip(140, m.stripH()))
	if !strings.Contains(strip, "8080 → 80") || !strings.Contains(strip, "+9") {
		t.Fatalf("one row:\n%s", strip)
	}
	if lines := strings.Split(strip, "\n"); len(lines) != m.stripH() {
		t.Fatalf("%d lines, want %d", len(lines), m.stripH())
	}
}

func TestZoomHidesStrip(t *testing.T) {
	for _, pos := range []string{"top", "bottom", "right"} {
		m := placedModel(t, pos, 120, 35)
		m.zoom = true
		if sideW, mainW, _ := m.layout(); sideW != 0 || mainW != 118 || m.stripH() != 0 {
			t.Errorf("%s: %d %d %d", pos, sideW, mainW, m.stripH())
		}
		if x, y := m.mainOrigin(); x != 0 || y != 0 {
			t.Errorf("%s: origin %d,%d", pos, x, y)
		}
		if strings.Contains(ansi.Strip(m.View()), "Kubeconfig") {
			t.Errorf("%s: zoom still shows the sidebar", pos)
		}
	}
}

func TestRightSidebarHugsTheEdge(t *testing.T) {
	m := placedModel(t, "right", 120, 35)
	sideW, _, panelH := m.layout()
	w := sideW - 2
	lines := strings.Split(ansi.Strip(m.sidebarBody(w, panelH-2)), "\n")
	ends := func(suffix string) bool {
		for _, l := range lines {
			if strings.HasSuffix(l, suffix) && ansi.StringWidth(l) == w {
				return true
			}
		}
		return false
	}
	for _, want := range []string{wordmark[0], tagline, "kind-dev", "shop", "Kubeconfig  C", "Forwards  f", "forwards", "8080 → 80 ■", "shop/web-0  "} {
		if !ends(want) {
			t.Errorf("%q is not flush right:\n%s", want, strings.Join(lines, "\n"))
		}
	}
	for i, l := range lines {
		if ansi.StringWidth(l) > w {
			t.Errorf("line %d is %d wide", i, ansi.StringWidth(l))
		}
	}
	left := placedModel(t, "left", 120, 35)
	if plain := ansi.Strip(left.sidebarBody(w, panelH-2)); !strings.Contains(plain, "\nC  Kubeconfig\n") || !strings.Contains(plain, "\n■ 8080 → 80\n") {
		t.Fatalf("left sidebar changed:\n%s", plain)
	}
	for _, l := range strings.Split(ansi.Strip(m.sidebarBody(12, 40)), "\n") {
		if ansi.StringWidth(l) > 12 {
			t.Fatalf("narrow line %q", l)
		}
		if strings.Contains(l, "Kubeconf") && !strings.HasSuffix(l, " C") {
			t.Fatalf("narrow menu row lost its key: %q", l)
		}
	}
}

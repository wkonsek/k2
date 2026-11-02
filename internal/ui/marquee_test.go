package ui

import (
	"strings"
	"testing"
	"unicode/utf8"

	"k2/internal/forward"

	"github.com/charmbracelet/x/ansi"
)

func TestMarqueeStaticWhenItFits(t *testing.T) {
	for frame := 0; frame < 50; frame++ {
		if got := marquee("shop/web-0", 12, frame); got != "shop/web-0" {
			t.Fatalf("frame %d: %q", frame, got)
		}
		if got := marquee("exactly-ten", 11, frame); got != "exactly-ten" {
			t.Fatalf("frame %d: %q", frame, got)
		}
	}
	if marquee("abc", 0, 3) != "" {
		t.Fatal("zero width should be empty")
	}
}

func TestMarqueePausesThenScrolls(t *testing.T) {
	s := "shop/web-kddvhhmp-5xwnk"
	for frame := 0; frame < marqueePause; frame++ {
		if got := marquee(s, 10, frame); got != "shop/web-k" {
			t.Fatalf("frame %d should hold the start: %q", frame, got)
		}
	}
	if got := marquee(s, 10, marqueePause+1); got != "hop/web-kd" {
		t.Fatalf("one step in: %q", got)
	}
	if got := marquee(s, 10, marqueePause+3); got != "p/web-kddv" {
		t.Fatalf("three steps in: %q", got)
	}
	end := marqueePause + len(s) - 5
	if got := marquee(s, 10, end); got != "5xwnk    s" {
		t.Fatalf("gap before the repeat: %q", got)
	}
}

func TestMarqueeLoops(t *testing.T) {
	s := "monitoring/prometheus-k8s-0"
	period := len(s) + marqueeGap + marqueePause
	for frame := 0; frame < period; frame++ {
		if a, b := marquee(s, 12, frame), marquee(s, 12, frame+period); a != b {
			t.Fatalf("frame %d: %q then %q", frame, a, b)
		}
	}
	if marquee(s, 12, period-1) == marquee(s, 12, 0) {
		t.Fatal("last frame before the loop should not be the start")
	}
	if marquee(s, 12, period) != marquee(s, 12, 0) {
		t.Fatal("a loop should come back to the start")
	}
}

func TestMarqueeNeverExceedsWidth(t *testing.T) {
	for _, s := range []string{"shop/web-kddvhhmp-5xwnk", "a/b-c-d-e-f-g-h-i-j-k-l", "8080 → 8080"} {
		for width := 1; width < 30; width++ {
			for frame := 0; frame < 80; frame++ {
				if got := ansi.StringWidth(marquee(s, width, frame)); got > width || (got != width && ansi.StringWidth(s) > width) {
					t.Fatalf("%q width %d frame %d: %d", s, width, frame, got)
				}
			}
		}
	}
}

func TestMarqueeKeepsWideCharsWhole(t *testing.T) {
	s := "zespół/ポッド名前-é́-日本"
	runes := map[rune]bool{' ': true}
	for _, r := range s {
		runes[r] = true
	}
	for _, width := range []int{3, 5, 7, 8, 11} {
		for frame := 0; frame < 120; frame++ {
			got := marquee(s, width, frame)
			if !utf8.ValidString(got) || ansi.StringWidth(got) != width {
				t.Fatalf("width %d frame %d: %q is %d wide", width, frame, got, ansi.StringWidth(got))
			}
			for _, r := range got {
				if !runes[r] {
					t.Fatalf("width %d frame %d: stray %q in %q", width, frame, r, got)
				}
			}
		}
	}
}

func longForwardModel(t *testing.T, pos string, w, h int) model {
	t.Helper()
	m := placedModel(t, pos, w, h)
	m.cfg.UI.SidebarWidth = 18
	m.forwards = []forward.Forward{
		{Local: "18080", Remote: "8080", Pod: "web-kddvhhmp-5xwnk-with-a-very-long-suffix", Namespace: "shop-production"},
		{Local: "5432", Remote: "5432", Pod: "db-0", Namespace: "shop"},
	}
	m.refit()
	return m
}

func TestNarrowSidebarScrollsLongForwards(t *testing.T) {
	for _, pos := range []string{"left", "right", "top", "bottom"} {
		for _, sz := range [][2]int{{100, 30}, {60, 20}, {40, 16}} {
			m := longForwardModel(t, pos, sz[0], sz[1])
			if sideW, _, _ := m.layout(); pos == "left" || pos == "right" {
				if sideW != 18 {
					t.Fatalf("%s %v: sidebar %d", pos, sz, sideW)
				}
			}
			seen := map[string]bool{}
			for frame := 0; frame < 60; frame++ {
				m.ticker = frame
				view := m.View()
				for i, l := range strings.Split(view, "\n") {
					if ansi.StringWidth(l) > sz[0] {
						t.Fatalf("%s %v frame %d: line %d is %d wide: %q", pos, sz, frame, i, ansi.StringWidth(l), ansi.Strip(l))
					}
				}
				seen[ansi.Strip(view)] = true
			}
			if !m.fwdOverflow() {
				continue
			}
			if len(seen) < 2 {
				t.Errorf("%s %v: the long forward never moved", pos, sz)
			}
		}
	}
	for _, pos := range []string{"left", "right"} {
		m := longForwardModel(t, pos, 100, 30)
		if !m.fwdOverflow() {
			t.Fatalf("%s: a long forward should overflow", pos)
		}
		plain := ansi.Strip(m.View())
		if !strings.Contains(plain, "shop-production/w") || strings.Contains(plain, "shop-production/web-kddvhhmp…") {
			t.Fatalf("%s: start of the name should show without a cut\n%s", pos, plain)
		}
	}
}

func TestRightSidebarTickerFillsTheWidth(t *testing.T) {
	m := longForwardModel(t, "right", 100, 30)
	m.ticker = marqueePause + 5
	inner := m.dockRoom(18 - 2)
	dock := strings.Split(ansi.Strip(m.fwdDock(inner+2)), "\n")
	if len(dock) != 5 {
		t.Fatalf("dock rows %q", dock)
	}
	if w := ansi.StringWidth(dock[2]); w != inner+2 {
		t.Fatalf("scrolling line is %d wide, want %d: %q", w, inner+2, dock[2])
	}
	if !strings.HasSuffix(dock[2], "  ") || !strings.HasSuffix(dock[4], "shop/db-0  ") {
		t.Fatalf("right side should keep the gutter: %q", dock)
	}
}

func TestForwardTickerOffCuts(t *testing.T) {
	for _, off := range []string{"animations", "forward_scroll"} {
		for _, pos := range []string{"left", "right", "top", "bottom"} {
			m := longForwardModel(t, pos, 100, 30)
			if off == "animations" {
				m.cfg.UI.Animations = false
			} else {
				m.cfg.UI.ForwardScroll = false
			}
			if m.fwdOverflow() || m.marqueeStart() != nil {
				t.Fatalf("%s off, %s: should not tick", off, pos)
			}
			m.ticker = 0
			first := m.View()
			m.ticker = marqueePause + 7
			if m.View() != first {
				t.Fatalf("%s off, %s: the view moved", off, pos)
			}
			if !strings.Contains(ansi.Strip(first), "…") {
				t.Fatalf("%s off, %s: no cut\n%s", off, pos, ansi.Strip(first))
			}
			for _, l := range strings.Split(first, "\n") {
				if ansi.StringWidth(l) > 100 {
					t.Fatalf("%s off, %s: %q", off, pos, ansi.Strip(l))
				}
			}
		}
	}
}

func TestForwardTickerLifecycle(t *testing.T) {
	m := longForwardModel(t, "left", 100, 30)
	if m.marqueeStart() == nil {
		t.Fatal("an overflowing forward should start the ticker")
	}
	if m.marqueeStart() != nil {
		t.Fatal("a running ticker should not start twice")
	}
	if m.marqueeStep(marqueeMsg{m.tickerSeq - 1}) != nil {
		t.Fatal("a stale tick should be dropped")
	}
	before := m.ticker
	if m.marqueeStep(marqueeMsg{m.tickerSeq}) == nil || m.ticker != before+1 {
		t.Fatal("a tick should advance and schedule the next one")
	}
	m.forwards = m.forwards[1:]
	if m.marqueeStep(marqueeMsg{m.tickerSeq}) != nil || m.ticker != 0 {
		t.Fatal("the ticker should stop once nothing overflows")
	}
	short := placedModel(t, "left", 100, 30)
	if short.fwdOverflow() || short.marqueeStart() != nil {
		t.Fatal("short forwards should not tick")
	}
}

func TestNarrowSidebarRestFits(t *testing.T) {
	for _, pos := range []string{"left", "right"} {
		m := longForwardModel(t, pos, 100, 40)
		sideW, _, _ := m.layout()
		body := m.sidebarBody(sideW-2, 36)
		for _, l := range strings.Split(body, "\n") {
			if ansi.StringWidth(l) > sideW-2 {
				t.Fatalf("%s: %q", pos, ansi.Strip(l))
			}
		}
		plain := ansi.Strip(body)
		for _, want := range []string{"kind-dev", "forwards", "18080 → 8080"} {
			if !strings.Contains(plain, want) {
				t.Fatalf("%s: missing %q\n%s", pos, want, plain)
			}
		}
	}
}

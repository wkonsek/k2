package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"k2/internal/theme"
)

func TestThemeBackgroundPaintsEverything(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	defer useTheme(theme.Nord())
	for _, name := range theme.Names() {
		cfg := testEnv(t)
		cfg.Theme = name
		cfg.UI.Background, cfg.UI.BackgroundSet = "theme", true
		m := newTestModel(t, cfg, podCluster())
		n, c := m.openPods()
		m = settle(t, n.(model), c)
		m = pressSettle(t, m, "ctrl+a", "v", "ctrl+a", "t")
		if !m.paintBg {
			t.Fatalf("%s: background not painted", name)
		}
		on := baseSeq()
		if !strings.Contains(on, "48;2;") {
			t.Fatalf("%s: base sequence %q has no background", name, on)
		}
		lines := strings.Split(m.View(), "\n")
		if len(lines) != m.height {
			t.Errorf("%s: %d lines on a %d-row terminal", name, len(lines), m.height)
		}
		for i, l := range lines {
			if w := ansi.StringWidth(l); w != m.width {
				t.Errorf("%s: line %d is %d wide, want %d", name, i, w, m.width)
			}
			if !strings.HasPrefix(l, on) {
				t.Errorf("%s: line %d does not start painted", name, i)
			}
			body := strings.TrimSuffix(l, "\x1b[0m")
			if strings.Count(body, "\x1b[0m") != strings.Count(body, "\x1b[0m"+on) {
				t.Errorf("%s: line %d has a reset to the terminal background", name, i)
			}
		}
	}
}

func TestTransparentBackgroundByDefault(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer useTheme(theme.Nord())
	m := newTestModel(t, testEnv(t), podCluster())
	n, c := m.openPods()
	m = settle(t, n.(model), c)
	bg := lipgloss.NewStyle().Background(th.Bg).Render("x")
	bg = bg[:strings.Index(bg, "x")]
	if m.paintBg || strings.Contains(m.View(), bg) {
		t.Fatal("transparent background painted the theme background")
	}
	lipgloss.SetColorProfile(termenv.Ascii)
	if got := paintBackground("a\nb", 3, 2); got != "a\nb" {
		t.Fatalf("painted without colors: %q", got)
	}
}

func TestLightThemeAutoPaint(t *testing.T) {
	defer useTheme(theme.Nord())
	cfg := testEnv(t)
	cfg.Theme = "solarized-light"
	m := newTestModel(t, cfg, nil)
	if !m.paintBg || strings.Contains(m.status, "light theme") {
		t.Fatalf("light theme: paint=%v status=%q", m.paintBg, m.status)
	}
	cfg.UI.Background, cfg.UI.BackgroundSet = "transparent", true
	m = newTestModel(t, cfg, nil)
	if m.paintBg || !strings.Contains(m.status, "solarized-light: light theme on a transparent background (ui.background: theme paints it)") {
		t.Fatalf("explicit transparent: paint=%v status=%q", m.paintBg, m.status)
	}
	cfg.Theme = "dracula"
	m = newTestModel(t, cfg, nil)
	if m.paintBg || strings.Contains(m.status, "light theme") {
		t.Fatalf("dark theme: paint=%v status=%q", m.paintBg, m.status)
	}
}

func TestSelectedRowColors(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	defer useTheme(theme.Nord())
	for _, name := range theme.Names() {
		p, _ := theme.Lookup(name)
		useTheme(p)
		fg, bg := p.SelectionColors()
		if theme.Contrast(fg, bg) < 4.5 {
			t.Errorf("%s: selection contrast %.2f", name, theme.Contrast(fg, bg))
		}
		want := lipgloss.NewStyle().Foreground(fg).Background(bg).Bold(true).Render("x")
		if got := selectedItemStyle.Render("x"); got != want {
			t.Errorf("%s: selected row %q, want %q", name, got, want)
		}
	}
	if fg, bg := theme.SolarizedLight().SelectionColors(); fg != "#073642" || bg != "#eee8d5" {
		t.Fatalf("solarized-light selection %s on %s", fg, bg)
	}
}

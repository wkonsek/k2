package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"k2/internal/config"
	"k2/internal/theme"
)

func sidebarPlain(m model, w, h int) []string {
	return strings.Split(ansi.Strip(m.sidebarBody(w, h)), "\n")
}

func logoLine(i int) string {
	return mountain[i] + logoGap + wordmark[i]
}

func wordmarkModel(t *testing.T) model {
	m := newTestModel(t, testEnv(t), nil)
	m.cfg.UI.Logo = "wordmark"
	return m
}

func TestSidebarDefaultLogoAndTagline(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	lines := sidebarPlain(m, 26, 60)
	if lines[0] != logoLine(0) || lines[1] != logoLine(1) || lines[2] != "kubectl, elevated" || lines[3] != "" {
		t.Fatalf("head:\n%s", strings.Join(lines[:4], "\n"))
	}
}

func TestMountainArt(t *testing.T) {
	if len(mountain) != len(wordmark) || len(mountainMask) != len(mountain) {
		t.Fatalf("mountain has %d rows, wordmark %d", len(mountain), len(wordmark))
	}
	w := ansi.StringWidth(mountain[0])
	if w%2 != 1 {
		t.Fatalf("mountain width %d is even", w)
	}
	for i, row := range mountain {
		r := []rune(row)
		if len(r) != w || len([]rune(mountainMask[i])) != w {
			t.Fatalf("row %d: %q is not %d wide", i, row, w)
		}
		for j := range r {
			if mirror(r[j]) != r[len(r)-1-j] {
				t.Fatalf("row %d is not symmetric: %q", i, row)
			}
		}
	}
	if []rune(mountain[0])[w/2] != '█' || strings.TrimSpace(mountain[1]) != mountain[1] {
		t.Fatalf("peak is off the axis:\n%s", strings.Join(mountain, "\n"))
	}
}

func mirror(r rune) rune {
	pairs := map[rune]rune{'▟': '▙', '▙': '▟', '▗': '▖', '▖': '▗', '▛': '▜', '▜': '▛', '▘': '▝', '▝': '▘'}
	if m, ok := pairs[r]; ok {
		return m
	}
	return r
}

func TestSidebarPicksWordmark(t *testing.T) {
	lines := sidebarPlain(wordmarkModel(t), 26, 60)
	if lines[0] != wordmark[0] || lines[1] != wordmark[1] || lines[2] != tagline || strings.HasPrefix(lines[0], " ") {
		t.Fatalf("wordmark:\n%s", strings.Join(lines[:4], "\n"))
	}
}

func TestMountainWidths(t *testing.T) {
	for _, w := range []int{26, 20, 16, 15} {
		lines := sidebarPlain(newTestModel(t, testEnv(t), nil), w, 60)
		if lines[0] != logoLine(0) || lines[1] != logoLine(1) {
			t.Fatalf("%d:\n%s", w, strings.Join(lines[:3], "\n"))
		}
	}
	for _, w := range []int{14, 12, 10} {
		lines := sidebarPlain(newTestModel(t, testEnv(t), nil), w, 60)
		if lines[0] != wordmark[0] || lines[1] != wordmark[1] {
			t.Fatalf("%d should drop the mountain:\n%s", w, strings.Join(lines[:3], "\n"))
		}
	}
}

func TestRightSidebarMirrorsLogo(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	m.cfg.UI.SidebarPosition = "right"
	for _, w := range []int{26, 20, 16} {
		lines := sidebarPlain(m, w, 60)
		for i := range wordmark {
			want := wordmark[i] + logoGap + mountain[i]
			if strings.TrimLeft(lines[i], " ") != want || ansi.StringWidth(lines[i]) != w {
				t.Fatalf("%d row %d: %q", w, i, lines[i])
			}
		}
	}
}

func TestStripShowsMountain(t *testing.T) {
	for _, pos := range []string{"top", "bottom"} {
		m := newTestModel(t, testEnv(t), nil)
		m.cfg.UI.SidebarPosition = pos
		lines := strings.Split(ansi.Strip(m.stripBody(130, stripRows)), "\n")
		if !strings.HasPrefix(lines[0], logoLine(0)) || !strings.HasPrefix(lines[1], logoLine(1)) || !strings.HasPrefix(lines[2], tagline) {
			t.Fatalf("%s:\n%s", pos, strings.Join(lines, "\n"))
		}
		two := strings.Split(ansi.Strip(m.stripBody(130, 2)), "\n")
		if len(two) != 2 || !strings.HasPrefix(two[0], "k2") {
			t.Fatalf("%s two rows:\n%s", pos, strings.Join(two, "\n"))
		}
	}
}

func TestTaglineWraps(t *testing.T) {
	got := taglineRows(tagline, 12)
	if len(got) != 2 || strings.Join(got, " ") != tagline {
		t.Fatalf("%q", got)
	}
	for _, r := range got {
		if ansi.StringWidth(r) > 12 {
			t.Fatalf("too wide: %q", r)
		}
	}
	lines := sidebarPlain(newTestModel(t, testEnv(t), nil), 12, 60)
	if !strings.HasPrefix(lines[0], wordmark[0]) || !strings.Contains(strings.Join(lines[:4], "\n"), "kubectl,") {
		t.Fatalf("narrow tagline:\n%s", strings.Join(lines[:5], "\n"))
	}
}

func TestSidebarDegrades(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	h := 8
	for !strings.Contains(ansi.Strip(m.sidebarBody(26, h)), "Kustomize") {
		h++
	}
	short := ansi.Strip(m.sidebarBody(26, h))
	if strings.Contains(short, wordmark[0]) || strings.Contains(short, mountain[1]) || !strings.Contains(short, "Kustomize") {
		t.Fatalf("short:\n%s", short)
	}
	if tall := ansi.Strip(m.sidebarBody(26, 40)); !strings.HasPrefix(tall, logoLine(0)+"\n") {
		t.Fatalf("tall lost the logo:\n%s", tall)
	}
}

func TestBrandIsLeftAligned(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	defer useTheme(theme.Nord())
	for _, name := range []string{"nord", "matrix", "solarized-light"} {
		pal, _ := theme.Lookup(name)
		useTheme(pal)
		if want := lipgloss.NewStyle().Foreground(pal.OK).Bold(true).Render("k2"); logoTitle("green").Render("k2") != want {
			t.Fatalf("%s: k2 is not the OK green", name)
		}
		for _, logo := range []string{"mountain", "wordmark"} {
			for _, inner := range []int{12, 22, 23} {
				cfg := config.Default().UI
				cfg.Logo = logo
				rows := brandRows(cfg, inner, true)
				plain := make([]string, len(rows))
				for i, r := range rows {
					plain[i] = ansi.Strip(r)
					if w := ansi.StringWidth(plain[i]); w > inner {
						t.Fatalf("%s %s %d: %q", name, logo, inner, plain[i])
					}
				}
				want := wordmark
				if logo == "mountain" && inner >= 15 {
					want = []string{logoLine(0), logoLine(1)}
				}
				if plain[0] != want[0] || plain[1] != want[1] || strings.HasPrefix(plain[1], " ") {
					t.Fatalf("%s %s %d: %q %q", name, logo, inner, plain[0], plain[1])
				}
				if strings.Join(plain[2:], " ") != tagline || strings.HasPrefix(plain[2], " ") {
					t.Fatalf("%s: tagline %q", name, plain[2:])
				}
			}
		}
	}
}

func TestMountainColors(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	rows := brandRows(config.Default().UI, 26, true)
	snow := lipgloss.NewStyle().Foreground(th.Bright).Render("▄█▄")
	body := lipgloss.NewStyle().Foreground(th.Accent).Render("█████")
	base := dimStyle.Render("▄")
	if !strings.Contains(rows[0], snow) || !strings.Contains(rows[1], body) || !strings.HasPrefix(rows[1], base) {
		t.Fatalf("colors: %q %q", rows[0], rows[1])
	}
}

func TestLogoTitleAccent(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	cfg := config.Default().UI
	cfg.LogoTitleColor = "accent"
	want := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Render(wordmark[0])
	if rows := brandRows(cfg, 26, true); !strings.HasSuffix(rows[0], want) {
		t.Fatalf("wordmark not in the accent: %q", rows[0])
	}
	cfg.Logo = "wordmark"
	if rows := brandRows(cfg, 26, true); rows[0] != want {
		t.Fatalf("wordmark not in the accent: %q", rows[0])
	}
}

func TestSmallBrandWhenShort(t *testing.T) {
	rows := brandRows(config.Default().UI, 22, false)
	if ansi.Strip(rows[0]) != "k2" || strings.HasPrefix(ansi.Strip(rows[0]), " ") || ansi.Strip(rows[1]) != tagline {
		t.Fatalf("%q", rows)
	}
}

func TestLogoFallbackCountsBlankRow(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	h := 8
	for !strings.Contains(ansi.Strip(m.sidebarBody(26, h)), wordmark[0]) {
		h++
		if h > 40 {
			t.Fatal("logo never fit")
		}
	}
	tall := ansi.Strip(m.sidebarBody(26, h))
	short := ansi.Strip(m.sidebarBody(26, h-1))
	for _, out := range []string{tall, short} {
		for _, want := range []string{"Kubeconfig", "Kustomize", "Forwards"} {
			if !strings.Contains(out, want) {
				t.Fatalf("lost %q:\n%s", want, out)
			}
		}
	}
	if !strings.HasPrefix(tall, logoLine(0)) || strings.Contains(short, wordmark[0]) || !strings.Contains(short, "k2") {
		t.Fatalf("short should fall back to k2:\n%s", short)
	}
}

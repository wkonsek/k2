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

func TestSidebarDefaultLogoAndTagline(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	lines := sidebarPlain(m, 26, 60)
	if lines[0] != wordmark[0] || lines[1] != wordmark[1] || lines[2] != "kubectl, elevated" || lines[3] != "" || strings.HasPrefix(lines[0], " ") {
		t.Fatalf("head:\n%s", strings.Join(lines[:4], "\n"))
	}
}

func TestSidebarPicksWordmark(t *testing.T) {
	lines := sidebarPlain(newTestModel(t, testEnv(t), nil), 26, 60)
	if lines[0] != wordmark[0] || lines[1] != wordmark[1] || lines[2] != tagline {
		t.Fatalf("wordmark:\n%s", strings.Join(lines[:4], "\n"))
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
	if strings.Contains(short, "▟") || !strings.Contains(short, "Kustomize") {
		t.Fatalf("short:\n%s", short)
	}
	if tall := ansi.Strip(m.sidebarBody(26, 40)); !strings.HasPrefix(tall, wordmark[0]+"\n") || strings.Contains(tall, "▟") {
		t.Fatalf("tall lost the wordmark:\n%s", tall)
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
		for _, inner := range []int{12, 22, 23} {
			cfg := config.Default().UI
			rows := brandRows(cfg, inner, true)
			plain := make([]string, len(rows))
			for i, r := range rows {
				plain[i] = ansi.Strip(r)
				if w := ansi.StringWidth(plain[i]); w > inner || strings.HasPrefix(plain[i], " ") {
					t.Fatalf("%s %d: %q", name, inner, plain[i])
				}
			}
			if plain[0] != wordmark[0] || plain[1] != wordmark[1] || strings.HasPrefix(plain[0], " ") {
				t.Fatalf("%s: %q %q", name, plain[0], plain[1])
			}
			if strings.Join(plain[2:], " ") != tagline {
				t.Fatalf("%s: tagline %q", name, plain[2:])
			}
		}
	}
}

func TestLogoTitleAccent(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	cfg := config.Default().UI
	cfg.LogoTitleColor = "accent"
	rows := brandRows(cfg, 26, true)
	want := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Render(wordmark[0])
	if rows[0] != want {
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
			t.Fatal("wordmark never fit")
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
	if strings.Contains(short, wordmark[0]) || !strings.Contains(short, "k2") {
		t.Fatalf("short should fall back to k2:\n%s", short)
	}
}

func mountainModel(t *testing.T, pos string) model {
	m := newTestModel(t, testEnv(t), nil)
	m.cfg.UI.Logo = "mountain"
	m.cfg.UI.SidebarPosition = pos
	return m
}

func mirrorGlyph(r rune) rune {
	pairs := map[rune]rune{'▟': '▙', '▙': '▟', '▗': '▖', '▖': '▗', '▛': '▜', '▜': '▛', '▘': '▝', '▝': '▘'}
	if m, ok := pairs[r]; ok {
		return m
	}
	return r
}

func TestMountainShape(t *testing.T) {
	if len(mountain) != 5 || len(mountainMask) != len(mountain) || mountainIndent() != 0 {
		t.Fatalf("%d rows, %d masks, indent %d", len(mountain), len(mountainMask), mountainIndent())
	}
	w := mountainWidth()
	if w%2 != 1 {
		t.Fatalf("width %d has no middle column", w)
	}
	for i, row := range mountain {
		r, mk := []rune(row), []rune(mountainMask[i])
		if len(r) != len(mk) {
			t.Fatalf("row %d: mask %q for %q", i, mountainMask[i], row)
		}
		lead := len(r) - len([]rune(strings.TrimLeft(row, " ")))
		full := []rune(padPlain(row, w))
		for j := range full {
			if mirrorGlyph(full[j]) != full[w-1-j] {
				t.Fatalf("row %d is not symmetric: %q", i, row)
			}
		}
		for j, g := range r {
			if (g == ' ') != (mk[j] == ' ') {
				t.Fatalf("row %d col %d: glyph %q mask %q", i, j, g, mk[j])
			}
		}
		if i == 0 && (lead != w/2 || strings.TrimSpace(row) != "▄") {
			t.Fatalf("peak %q is not on the middle column %d", row, w/2)
		}
	}
	if []rune(mountain[1])[w/2] != '█' || strings.TrimSpace(mountain[len(mountain)-1]) != mountain[len(mountain)-1] {
		t.Fatalf("body is off the axis:\n%s", strings.Join(mountain, "\n"))
	}
}

func TestMountainColors(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	defer useTheme(theme.Nord())
	for _, name := range []string{"nord", "monokai", "solarized-light"} {
		pal, _ := theme.Lookup(name)
		useTheme(pal)
		open := func(st lipgloss.Style) string {
			s := st.Render("x")
			return s[:strings.Index(s, "x")]
		}
		snow, body, base := open(lipgloss.NewStyle().Foreground(th.Bright)), open(lipgloss.NewStyle().Foreground(th.Accent)), open(dimStyle)
		for i := range mountain {
			line := paintMountain(i)
			for c, o := range map[rune]string{'S': snow, 'A': body, 'B': base} {
				if strings.ContainsRune(mountainMask[i], c) != strings.Contains(line, o) {
					t.Fatalf("%s row %d: color %c in %q", name, i, c, line)
				}
			}
		}
	}
}

func TestMountainSidebar(t *testing.T) {
	for _, w := range []int{26, 20, 16, 13} {
		lines := sidebarPlain(mountainModel(t, "left"), w, 60)
		block := mountainWidth()
		if w >= len(tagline) {
			block = len(tagline)
		}
		pad := (block - mountainWidth()) / 2
		for i, row := range mountain {
			if lines[i] != strings.Repeat(" ", pad)+row {
				t.Fatalf("%d row %d: %q", w, i, lines[i])
			}
		}
		n := len(mountain)
		if lines[n] != "" || strings.TrimSpace(lines[n+1]) != "k2" || !strings.HasPrefix(strings.TrimSpace(lines[n+2]), "kubectl,") {
			t.Fatalf("%d head:\n%s", w, strings.Join(lines[:n+4], "\n"))
		}
		axis := pad*2 + mountainWidth()
		if k := strings.Index(lines[n+1], "k2"); k*2+2 != axis-1 {
			t.Fatalf("%d: k2 at %d is off the axis %d", w, k, axis)
		}
	}
}

func TestMountainFallsBack(t *testing.T) {
	lines := sidebarPlain(mountainModel(t, "left"), 12, 60)
	if lines[0] != wordmark[0] || lines[1] != wordmark[1] {
		t.Fatalf("narrow:\n%s", strings.Join(lines[:4], "\n"))
	}
	m := mountainModel(t, "left")
	h := 8
	for !strings.Contains(ansi.Strip(m.sidebarBody(26, h)), mountain[len(mountain)-1]) {
		h++
		if h > 60 {
			t.Fatal("mountain never fit")
		}
	}
	short := ansi.Strip(m.sidebarBody(26, h-1))
	if strings.Contains(short, "▟") || !strings.Contains(short, wordmark[0]) {
		t.Fatalf("short should show the wordmark:\n%s", short)
	}
}

func TestMountainRightIsMirrored(t *testing.T) {
	for _, w := range []int{26, 20, 16} {
		lines := sidebarPlain(mountainModel(t, "right"), w, 60)
		block := mountainWidth()
		if w >= len(tagline) {
			block = len(tagline)
		}
		pad := (block - mountainWidth()) / 2
		for i, row := range mountain {
			want := strings.Repeat(" ", w-block+pad) + padPlain(row, block-pad)
			if lines[i] != want {
				t.Fatalf("%d row %d: %q want %q", w, i, lines[i], want)
			}
		}
		if tag := lines[len(mountain)+2]; ansi.StringWidth(tag) != w || !strings.Contains(tag, "kubectl,") {
			t.Fatalf("%d tagline: %q", w, tag)
		}
	}
}

func TestMountainStripUsesWordmark(t *testing.T) {
	for _, pos := range []string{"top", "bottom"} {
		m := mountainModel(t, pos)
		lines := strings.Split(ansi.Strip(m.stripBody(130, stripRows)), "\n")
		if !strings.HasPrefix(lines[0], wordmark[0]) || !strings.HasPrefix(lines[1], wordmark[1]) || !strings.HasPrefix(lines[2], tagline) || strings.Contains(strings.Join(lines, "\n"), "▟") {
			t.Fatalf("%s:\n%s", pos, strings.Join(lines, "\n"))
		}
	}
}

func TestDefaultIsWordmark(t *testing.T) {
	if config.Default().UI.Logo != "wordmark" {
		t.Fatalf("default %q", config.Default().UI.Logo)
	}
	if out := ansi.Strip(newTestModel(t, testEnv(t), nil).sidebarBody(26, 60)); strings.Contains(out, "▟") {
		t.Fatalf("default drew the mountain:\n%s", out)
	}
}

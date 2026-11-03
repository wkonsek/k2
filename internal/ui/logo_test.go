package ui

import (
	"fmt"
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

func TestMountainShape(t *testing.T) {
	if len(mountain) != 3 || len(mountainMask) != len(mountain) {
		t.Fatalf("%d rows, %d masks", len(mountain), len(mountainMask))
	}
	w := ansi.StringWidth(mountain[0])
	if w != 9 {
		t.Fatalf("width %d", w)
	}
	px := make([][]bool, 2*len(mountain))
	for i, row := range mountain {
		r, mk := []rune(row), []rune(mountainMask[i])
		if len(r) != w || len(mk) != w {
			t.Fatalf("row %d: %q / %q", i, row, mountainMask[i])
		}
		px[2*i], px[2*i+1] = make([]bool, w), make([]bool, w)
		for j, g := range r {
			if (g == ' ') != (mk[j] == ' ') || r[w-1-j] != g {
				t.Fatalf("row %d col %d: glyph %q mask %q, not symmetric", i, j, g, mk[j])
			}
			px[2*i][j] = g == '█' || g == '▀'
			px[2*i+1][j] = g == '█' || g == '▄'
		}
	}
	var widths []int
	for _, line := range px {
		n, first := 0, -1
		for j, on := range line {
			if on {
				n++
				if first < 0 {
					first = j
				}
			}
		}
		if n > 0 {
			if first*2+n != w {
				t.Fatalf("pixel row %v is off the axis", line)
			}
			widths = append(widths, n)
		}
	}
	if fmt.Sprint(widths) != "[1 3 5 7 9]" {
		t.Fatalf("pixel rows %v", widths)
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

func TestMountainSidebarIsLeftAligned(t *testing.T) {
	for _, w := range []int{26, 20, 16, 10} {
		lines := sidebarPlain(mountainModel(t, "left"), w, 60)
		for i, row := range mountain {
			if lines[i] != strings.TrimRight(row, " ") {
				t.Fatalf("%d row %d: %q", w, i, lines[i])
			}
		}
		n := len(mountain)
		if lines[n] != "k2" || !strings.HasPrefix(lines[n+1], "kubectl,") {
			t.Fatalf("%d head:\n%s", w, strings.Join(lines[:n+3], "\n"))
		}
	}
}

func TestMountainFallsBack(t *testing.T) {
	m := mountainModel(t, "left")
	h := 8
	for !strings.Contains(ansi.Strip(m.sidebarBody(26, h)), mountain[2]) {
		h++
		if h > 60 {
			t.Fatal("mountain never fit")
		}
	}
	short := ansi.Strip(m.sidebarBody(26, h-1))
	if strings.Contains(short, mountain[2]) || !strings.Contains(short, wordmark[0]) {
		t.Fatalf("short should show the wordmark:\n%s", short)
	}
}

func TestMountainRightIsFlush(t *testing.T) {
	for _, w := range []int{26, 20, 16} {
		lines := sidebarPlain(mountainModel(t, "right"), w, 60)
		for i, row := range mountain {
			if want := strings.Repeat(" ", w-len([]rune(row))) + row; lines[i] != want {
				t.Fatalf("%d row %d: %q want %q", w, i, lines[i], want)
			}
		}
		n := len(mountain)
		if !strings.HasSuffix(lines[n], " k2") || ansi.StringWidth(lines[n]) != w || ansi.StringWidth(lines[n+1]) != w {
			t.Fatalf("%d: %q %q", w, lines[n], lines[n+1])
		}
	}
}

func TestMountainStrip(t *testing.T) {
	for _, pos := range []string{"top", "bottom"} {
		m := mountainModel(t, pos)
		lines := strings.Split(ansi.Strip(m.stripBody(130, stripRows)), "\n")
		for i, row := range mountain {
			if !strings.HasPrefix(lines[i], strings.TrimRight(row, " ")) {
				t.Fatalf("%s:\n%s", pos, strings.Join(lines, "\n"))
			}
		}
		if !strings.HasPrefix(lines[3], "k2 ") || !strings.HasPrefix(lines[4], tagline) {
			t.Fatalf("%s:\n%s", pos, strings.Join(lines, "\n"))
		}
		short := strings.Split(ansi.Strip(m.stripBody(130, 4)), "\n")
		if !strings.HasPrefix(short[0], wordmark[0]) || !strings.HasPrefix(short[2], tagline) {
			t.Fatalf("%s four rows:\n%s", pos, strings.Join(short, "\n"))
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

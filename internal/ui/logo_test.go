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

func TestLogoCountMatchesConfig(t *testing.T) {
	if len(logos) != len(config.LogoStyles) {
		t.Fatalf("%d logos, config allows %d", len(logos), len(config.LogoStyles))
	}
}

func TestLogoShapes(t *testing.T) {
	wantRows := []int{4, 3}
	for i, l := range logos {
		if len(l.rows) != wantRows[i] || len(l.masks) != len(l.rows) {
			t.Fatalf("logo %d: %d rows, %d masks", i+1, len(l.rows), len(l.masks))
		}
		for r, row := range l.rows {
			if len([]rune(l.masks[r])) > len([]rune(row)) {
				t.Fatalf("logo %d row %d: mask %q longer than %q", i+1, r, l.masks[r], row)
			}

			mk := []rune(l.masks[r])
			for c, g := range []rune(row) {
				m := ' '
				if c < len(mk) {
					m = mk[c]
				}
				if (g == ' ') != (m == ' ') {
					t.Fatalf("logo %d row %d col %d: glyph %q mask %q", i+1, r, c, g, m)
				}
			}
		}

		indent := 1 << 30
		for _, row := range l.rows {
			if row != strings.TrimRight(row, " ") {
				t.Fatalf("logo %d row %q has trailing spaces", i+1, row)
			}
			indent = min(indent, len(row)-len(strings.TrimLeft(row, " ")))
		}
		if indent != 0 {
			t.Fatalf("logo %d keeps %d cells of shared indentation", i+1, indent)
		}
		for _, inner := range []int{20, 26, l.width()} {
			out, _ := l.render(inner)

			p0 := ansi.Strip(out[0])
			pad := len(p0) - len(strings.TrimLeft(p0, " ")) - (len(l.rows[0]) - len(strings.TrimLeft(l.rows[0], " ")))
			for r, line := range out {
				plain := ansi.Strip(line)
				if plain != strings.Repeat(" ", pad)+l.rows[r] {
					t.Fatalf("logo %d inner %d row %d: %q (block pad %d)", i+1, inner, r, plain, pad)
				}
				if ansi.StringWidth(plain) > inner {
					t.Fatalf("logo %d inner %d row %d wider than the sidebar", i+1, inner, r)
				}
			}
		}
	}
}

func TestLogoColors(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	defer useTheme(theme.Nord())
	for _, name := range theme.Names() {
		pal, _ := theme.Lookup(name)
		useTheme(pal)
		for i, l := range logos {
			for r, line := range first(l.render(l.width())) {
				for _, c := range "SAB" {
					st, _ := maskStyle(c)
					want := st.Render("x")
					open := want[:strings.Index(want, "x")]
					has := strings.ContainsRune(l.masks[r], c)
					if has != strings.Contains(line, open) {
						t.Fatalf("%s logo %d row %d: color %c present=%v in %q", name, i+1, r, c, has, line)
					}
				}
			}
		}
	}
}

func sidebarPlain(m model, w, h int) []string {
	return strings.Split(ansi.Strip(m.sidebarBody(w, h)), "\n")
}

func TestSidebarDefaultLogoAndTagline(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	lines := sidebarPlain(m, 26, 60)
	for r, row := range logos[0].rows {
		if strings.TrimLeft(lines[r], " ") != strings.TrimLeft(row, " ") {
			t.Fatalf("row %d: %q", r, lines[r])
		}
	}
	if lines[4] != "" || strings.TrimSpace(lines[5]) != "k2" || strings.TrimSpace(lines[6]) != "kubectl, elevated" {
		t.Fatalf("head:\n%s", strings.Join(lines[:7], "\n"))
	}
}

func TestSidebarPicksLogoAndCustomTagline(t *testing.T) {
	cfg := testEnv(t)
	cfg.UI.Logo, cfg.UI.Tagline = "simple", "prod cockpit"
	m := newTestModel(t, cfg, nil)
	lines := sidebarPlain(m, 26, 60)
	if strings.TrimSpace(lines[0]) != "/\\" || !strings.Contains(lines[2], `/  \/    \/  \`) {
		t.Fatalf("logo 3 not drawn:\n%s", strings.Join(lines[:5], "\n"))
	}
	if lines[3] != "" || strings.TrimSpace(lines[4]) != "k2" || strings.TrimSpace(lines[5]) != "prod cockpit" {
		t.Fatalf("custom tagline:\n%s", strings.Join(lines[:6], "\n"))
	}
	cfg.UI.Tagline = ""
	lines = sidebarPlain(newTestModel(t, cfg, nil), 26, 60)
	if strings.TrimSpace(lines[4]) != "k2" || strings.TrimSpace(lines[5]) != "" {
		t.Fatalf("empty tagline still drawn:\n%s", strings.Join(lines[:6], "\n"))
	}
}

func TestTaglineWraps(t *testing.T) {
	got := taglineRows("clusters from the summit", 18)
	if len(got) != 2 || got[0] != "clusters from" || got[1] != "the summit" {
		t.Fatalf("two lines: %q", got)
	}
	long := "a very long custom tagline that will need several lines here"
	rows := taglineRows(long, 14)
	if strings.Join(rows, " ") != long {
		t.Fatalf("cut: %q", rows)
	}
	for _, r := range rows {
		if ansi.StringWidth(r) > 14 {
			t.Fatalf("too wide: %q", r)
		}
	}
	cfg := testEnv(t)
	cfg.UI.Tagline = "clusters from the summit"
	lines := sidebarPlain(newTestModel(t, cfg, nil), 18, 60)
	i := 0
	for strings.TrimSpace(lines[i]) != "k2" {
		i++
	}
	if strings.TrimSpace(lines[i+1]) != "clusters from" || strings.TrimSpace(lines[i+2]) != "the summit" {
		t.Fatalf("wrapped tagline:\n%s", strings.Join(lines[i:i+3], "\n"))
	}
}

func TestSidebarDegrades(t *testing.T) {
	cfg := testEnv(t)
	cfg.UI.Logo = "simple"
	m := newTestModel(t, cfg, nil)
	narrow := ansi.Strip(m.sidebarBody(12, 60))
	if strings.Contains(narrow, `/\`) || !strings.Contains(narrow, "k2") {
		t.Fatalf("narrow:\n%s", narrow)
	}
	cfg.UI.Logo = "default"
	m = newTestModel(t, cfg, nil)
	h := 8
	for !strings.Contains(ansi.Strip(m.sidebarBody(26, h)), "Kustomize") {
		h++
	}
	short := ansi.Strip(m.sidebarBody(26, h))
	if strings.Contains(short, "▟") || !strings.Contains(short, "k2") || !strings.Contains(short, "Kustomize") {
		t.Fatalf("short:\n%s", short)
	}
	if tall := ansi.Strip(m.sidebarBody(26, 40)); !strings.Contains(tall, "▟") {
		t.Fatalf("tall lost the logo:\n%s", tall)
	}
}

func first(rows []string, _ int) []string { return rows }

func axisOf(line string) int {
	trimmed := strings.TrimLeft(strings.TrimRight(line, " "), " ")
	if trimmed == "" {
		return -1
	}
	lead := ansi.StringWidth(line) - ansi.StringWidth(strings.TrimLeft(line, " "))
	return 2*lead + ansi.StringWidth(trimmed)
}

func TestHeadAxisAlignment(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	defer useTheme(theme.Nord())
	for _, name := range []string{"nord", "matrix", "solarized-light"} {
		pal, _ := theme.Lookup(name)
		useTheme(pal)
		green := logoTitle("green").Render("k2")
		if want := lipgloss.NewStyle().Foreground(pal.OK).Bold(true).Render("k2"); logoTitle("green").Render("k2") != want {
			t.Fatalf("%s: k2 is not the OK green", name)
		}
		for ni, name := range config.LogoStyles {
			l := logos[ni]
			for ti, tl := range []string{config.DefaultTagline, "prod cockpit"} {
				for _, inner := range []int{22, 23, 30, 31} {
					cfg := config.Default().UI
					cfg.Logo, cfg.Tagline = name, tl
					rows := head(cfg, inner, true)
					tag := fmt.Sprintf("%s logo %s tagline %d inner %d", name, cfg.Logo, ti+1, inner)
					plain := make([]string, len(rows))
					for i, r := range rows {
						plain[i] = ansi.Strip(r)
						if w := ansi.StringWidth(plain[i]); w > inner {
							t.Fatalf("%s: row %d is %d wide", tag, i, w)
						}
					}

					peak := axisOf(plain[0])
					k := len(l.rows) + 1
					if plain[k-1] != "" {
						t.Fatalf("%s: no blank row between logo and k2: %q", tag, plain[k-1])
					}
					if strings.TrimSpace(plain[k]) != "k2" || !strings.Contains(rows[k], green) {
						t.Fatalf("%s: no green k2 in %q", tag, rows[k])
					}

					if d := peak - inner; d < -1 || d > 1 {
						t.Fatalf("%s: axis %d/2 is off the middle %d/2", tag, peak, inner)
					}
					got := strings.Join(plain[k+1:], " ")
					if strings.Join(strings.Fields(got), " ") != tl {
						t.Fatalf("%s: tagline %q cut or changed", tag, got)
					}
					for i := k; i < len(plain); i++ {
						if d := axisOf(plain[i]) - peak; d < -1 || d > 1 {
							t.Fatalf("%s: row %d %q axis %d/2 vs peak %d/2\n%s", tag, i, plain[i], axisOf(plain[i]), peak, strings.Join(plain, "\n"))
						}
					}
				}
			}
		}
	}
}

func TestLogoTitleAccent(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	cfg := config.Default().UI
	cfg.LogoTitleColor = "accent"
	rows := head(cfg, 26, true)
	want := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Render("k2")
	if !strings.Contains(rows[len(logos[0].rows)+1], want) {
		t.Fatalf("k2 not in the accent: %q", rows[len(logos[3].rows)+1])
	}
}

func TestHeadAxisWithoutLogo(t *testing.T) {
	for _, inner := range []int{10, 11, 22, 23} {
		rows := head(config.Default().UI, inner, false)
		for _, r := range rows {
			if d := axisOf(ansi.Strip(r)) - inner; d < -1 || d > 1 {
				t.Fatalf("inner %d: %q off the middle", inner, ansi.Strip(r))
			}
		}
	}
}

func TestLogoFallbackCountsBlankRow(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	h := 8
	for !strings.Contains(ansi.Strip(m.sidebarBody(26, h)), "▟") {
		h++
	}
	for _, hh := range []int{h, h - 1} {
		out := ansi.Strip(m.sidebarBody(26, hh))
		for _, want := range []string{"k2", "Kubeconfig", "Kustomize", "Forwards", "forwards"} {
			if !strings.Contains(out, want) {
				t.Fatalf("height %d lost %q:\n%s", hh, want, out)
			}
		}
		if hh == h-1 && strings.Contains(out, "▟") {
			t.Fatalf("height %d still draws the logo:\n%s", hh, out)
		}
	}
}

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

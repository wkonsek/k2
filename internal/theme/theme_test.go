package theme

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestNordIsTheOriginalPalette(t *testing.T) {
	p := Nord()
	want := Palette{Name: "nord", Bg: "#2e3440", Muted: "#7b88a1", Text: "#d8dee9", Accent: "#88c0d0",
		Accent2: "#81a1c1", Accent3: "#5e81ac", Danger: "#bf616a", OK: "#a3be8c", Warn: "#ebcb8b", Bright: "#eceff4"}
	if p != want {
		t.Fatalf("nord changed: %+v", p)
	}
}

var allThemes = []string{"nord", "dracula", "catppuccin", "gruvbox", "tokyo-night",
	"kanagawa", "rose-pine", "everforest", "one-dark", "solarized-dark", "material", "matrix", "solarized-light",
	"night-owl", "ayu", "monokai", "github-dark", "flexoki", "oxocarbon", "poimandres", "horizon", "zenburn", "catppuccin-latte"}

func TestLookupEveryTheme(t *testing.T) {
	names := Names()
	if len(names) != len(allThemes) || names[0] != "nord" {
		t.Fatalf("names %v", names)
	}
	for i, n := range allThemes {
		if names[i] != n {
			t.Fatalf("names[%d] = %q, want %q", i, names[i], n)
		}
		p, ok := Lookup(n)
		if !ok || p.Name != n {
			t.Fatalf("Lookup(%q) = %+v %v", n, p, ok)
		}
		if Describe[n] == "" {
			t.Errorf("%s has no description", n)
		}
		if p.Light != (n == "solarized-light" || n == "catppuccin-latte") {
			t.Errorf("%s: Light = %v", n, p.Light)
		}

		for role, c := range map[string]string{"bg": string(p.Bg), "muted": string(p.Muted), "text": string(p.Text),
			"accent": string(p.Accent), "accent2": string(p.Accent2), "accent3": string(p.Accent3),
			"danger": string(p.Danger), "ok": string(p.OK), "warn": string(p.Warn), "bright": string(p.Bright)} {
			if len(c) != 7 || c[0] != '#' {
				t.Errorf("%s.%s = %q", n, role, c)
			}
		}
		if p.Danger == p.Accent || p.Danger == p.Muted {
			t.Errorf("%s: danger color not distinct", n)
		}

		if p.Warn == p.OK || p.Warn == p.Danger || p.Warn == p.Muted {
			t.Errorf("%s: warn color not distinct", n)
		}
	}
	if _, ok := Lookup("solarized"); ok {
		t.Fatal("unknown theme found")
	}
}

func TestThemeContrast(t *testing.T) {
	const term = "#1e1e2e"
	for _, n := range Names() {
		p, _ := Lookup(n)
		fg, bg := p.SelectionColors()
		if c := Contrast(fg, bg); c < 4.5 {
			t.Errorf("%s: selected row contrast %.2f < 4.5", n, c)
		}
		if c := Contrast(p.Muted, p.Bg); c < 3 {
			t.Errorf("%s: muted on bg %.2f < 3", n, c)
		}
		if c := Contrast(p.Muted, term); c < 3 {
			t.Errorf("%s: muted on %s %.2f < 3", n, term, c)
		}
		if c := Contrast(p.Text, p.Bg); c < 4.5 {
			t.Errorf("%s: text on bg %.2f < 4.5", n, c)
		}

		if h := Hue(p.Danger); h > 15 && h < 335 {
			t.Errorf("%s: danger %s hue %.0f is not red", n, p.Danger, h)
		}
		if r, g, b := rgb(p.Danger); r <= g || r <= b {
			t.Errorf("%s: danger %s is not red-dominant", n, p.Danger)
		}
	}
}

func TestSignatureAccents(t *testing.T) {
	want := map[string][2]string{
		"dracula":     {"#bd93f9", "#ff79c6"},
		"tokyo-night": {"#7aa2f7", "#ff9e64"},
		"catppuccin":  {"#cba6f7", ""},
		"gruvbox":     {"#fabd2f", ""},
	}
	for n, w := range want {
		p, _ := Lookup(n)
		if string(p.Accent) != w[0] {
			t.Errorf("%s accent %s, want %s", n, p.Accent, w[0])
		}
		if w[1] != "" && string(p.Accent2) != w[1] && string(p.Accent3) != w[1] {
			t.Errorf("%s lacks %s", n, w[1])
		}
	}
	if g, _ := Lookup("gruvbox"); g.Warn != "#fe8019" {
		t.Errorf("gruvbox warn %s, want orange", g.Warn)
	}
	seen := map[string]string{}
	for _, n := range Names() {
		p, _ := Lookup(n)
		if o, dup := seen[string(p.Accent)]; dup {
			t.Errorf("%s and %s share accent %s", n, o, p.Accent)
		}
		seen[string(p.Accent)] = n
	}
}

func TestContrastAndHue(t *testing.T) {
	if c := Contrast("#000000", "#ffffff"); c < 20.9 || c > 21.1 {
		t.Fatalf("black/white %.2f", c)
	}
	if c := Contrast("#123456", "#123456"); c != 1 {
		t.Fatalf("same color %.2f", c)
	}
	for c, h := range map[string]float64{"#ff0000": 0, "#00ff00": 120, "#0000ff": 240} {
		if got := Hue(lipgloss.Color(c)); got != h {
			t.Errorf("Hue(%s) = %.0f", c, got)
		}
	}
}

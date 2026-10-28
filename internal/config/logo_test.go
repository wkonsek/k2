package config

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestLogoTitleColorSetting(t *testing.T) {
	if Default().UI.LogoTitleColor != "green" {
		t.Fatalf("default %q", Default().UI.LogoTitleColor)
	}
	if c, w := Parse([]byte("ui:\n  logo_title_color: Accent\n")); len(w) != 0 || c.UI.LogoTitleColor != "accent" {
		t.Fatalf("accent: %q %v", c.UI.LogoTitleColor, w)
	}
	c, w := Parse([]byte("ui:\n  logo_title_color: pink\n"))
	if c.UI.LogoTitleColor != "green" || len(w) != 1 || !strings.Contains(w[0], "ui.logo_title_color") {
		t.Fatalf("pink: %q %v", c.UI.LogoTitleColor, w)
	}
}

func TestCustomTheme(t *testing.T) {
	in := `
theme: custom
colors:
  bg: "#111111"
  muted: "#888888"
  text: "#eeeeee"
  accent: "#88c0d0"
  accent2: "#81a1c1"
  accent3: "#5e81ac"
  danger: "#bf616a"
  ok: "#a3be8c"
  warn: "#ebcb8b"
  bright: "#ffffff"
  sel_bg: "#222222"
  sel_fg: "#eeeeee"
  light: false
`
	c, w := Parse([]byte(in))
	if len(w) != 0 || c.Theme != "custom" || c.Custom.Name != "custom" || c.Custom.Bg != lipgloss.Color("#111111") || c.Custom.Light {
		t.Fatalf("custom %+v %v", c.Custom, w)
	}
	if got := c.PaletteNamed("custom"); got.Accent != "#88c0d0" {
		t.Fatalf("palette %+v", got)
	}
	c, w = Parse([]byte("theme: custom\ncolors:\n  bg: \"#111111\"\n"))
	if c.Theme != "nord" || len(w) != 1 || !strings.Contains(w[0], "needs") {
		t.Fatalf("incomplete: theme %s %v", c.Theme, w)
	}
	c, w = Parse([]byte("theme: nord\ncolors:\n  bg: \"#111111\"\n"))
	if c.Theme != "nord" || c.Custom.Bg != "" || len(w) != 1 || !strings.Contains(w[0], "ignored") {
		t.Fatalf("ignored: %+v %v", c.Custom, w)
	}
}

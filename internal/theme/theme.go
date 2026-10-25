package theme

import (
	"math"
	"strconv"

	"github.com/charmbracelet/lipgloss"
)

type Palette struct {
	Name string

	Bg lipgloss.Color

	Muted lipgloss.Color

	Text lipgloss.Color

	Accent lipgloss.Color

	Accent2 lipgloss.Color

	Accent3 lipgloss.Color

	Danger lipgloss.Color

	OK lipgloss.Color

	Warn lipgloss.Color

	Bright lipgloss.Color

	SelBg, SelFg lipgloss.Color

	Light bool
}

func (p Palette) SelectionColors() (fg, bg lipgloss.Color) {
	fg, bg = p.Bg, p.Accent
	if p.SelFg != "" {
		fg = p.SelFg
	}
	if p.SelBg != "" {
		bg = p.SelBg
	}
	return fg, bg
}

type Styles struct {
	Title    lipgloss.Style
	Item     lipgloss.Style
	Selected lipgloss.Style
	Dim      lipgloss.Style
	Key      lipgloss.Style
	OK       lipgloss.Style
	Warn     lipgloss.Style
	Err      lipgloss.Style
}

func (p Palette) Styles() Styles {
	selFg, selBg := p.SelectionColors()
	return Styles{
		Title:    lipgloss.NewStyle().Foreground(p.Accent).Bold(true),
		Item:     lipgloss.NewStyle().Foreground(p.Text),
		Selected: lipgloss.NewStyle().Foreground(selFg).Background(selBg).Bold(true),
		Dim:      lipgloss.NewStyle().Foreground(p.Muted),
		Key:      lipgloss.NewStyle().Foreground(p.Accent).Bold(true),
		OK:       lipgloss.NewStyle().Foreground(p.OK),
		Warn:     lipgloss.NewStyle().Foreground(p.Warn),
		Err:      lipgloss.NewStyle().Foreground(p.Danger),
	}
}

func Nord() Palette {
	return Palette{
		Name:    "nord",
		Bg:      lipgloss.Color("#2e3440"),
		Muted:   lipgloss.Color("#7b88a1"),
		Text:    lipgloss.Color("#d8dee9"),
		Accent:  lipgloss.Color("#88c0d0"),
		Accent2: lipgloss.Color("#81a1c1"),
		Accent3: lipgloss.Color("#5e81ac"),
		Danger:  lipgloss.Color("#bf616a"),
		OK:      lipgloss.Color("#a3be8c"),
		Warn:    lipgloss.Color("#ebcb8b"),
		Bright:  lipgloss.Color("#eceff4"),
	}
}

var registry = []func() Palette{
	Nord, Dracula, Catppuccin, Gruvbox, TokyoNight,
	Kanagawa, RosePine, Everforest, OneDark, SolarizedDark, Material, Matrix,
	SolarizedLight,
}

var Describe = map[string]string{
	"nord":            "arctic blue-gray with frost cyan accents (default)",
	"dracula":         "purple and pink on dark gray",
	"catppuccin":      "Mocha: pastel mauve on deep blue-gray",
	"gruvbox":         "retro warm yellow and orange on brown-gray",
	"tokyo-night":     "Night: blue and orange on deep navy",
	"kanagawa":        "Wave: muted ink blue and violet, after Hokusai",
	"rose-pine":       "Main: soft rose and iris on dark purple",
	"everforest":      "Dark medium: calm forest green on gray-green",
	"one-dark":        "Atom One Dark: blue and purple on charcoal",
	"solarized-dark":  "Solarized dark: cyan and blue on deep teal",
	"material":        "Material Oceanic: teal on blue-gray",
	"matrix":          "digital rain: phosphor green on near black",
	"solarized-light": "Solarized light: blue on cream (light theme)",
}

func Names() []string {
	out := make([]string, len(registry))
	for i, f := range registry {
		out[i] = f().Name
	}
	return out
}

func Lookup(name string) (Palette, bool) {
	for _, f := range registry {
		if p := f(); p.Name == name {
			return p, true
		}
	}
	return Palette{}, false
}

func Dracula() Palette {
	return Palette{
		Name: "dracula", Bg: "#282a36", Muted: "#6272a4", Text: "#f8f8f2",
		Accent: "#bd93f9", Accent2: "#ff79c6", Accent3: "#ffb86c",
		Danger: "#ff5555", OK: "#50fa7b", Warn: "#f1fa8c", Bright: "#ffffff",
	}
}

func Catppuccin() Palette {
	return Palette{
		Name: "catppuccin", Bg: "#1e1e2e", Muted: "#7f849c", Text: "#cdd6f4",
		Accent: "#cba6f7", Accent2: "#89b4fa", Accent3: "#fab387",
		Danger: "#f38ba8", OK: "#a6e3a1", Warn: "#f9e2af", Bright: "#f5e0dc",
	}
}

func Gruvbox() Palette {
	return Palette{
		Name: "gruvbox", Bg: "#282828", Muted: "#928374", Text: "#ebdbb2",
		Accent: "#fabd2f", Accent2: "#8ec07c", Accent3: "#d3869b",
		Danger: "#fb4934", OK: "#b8bb26", Warn: "#fe8019", Bright: "#fbf1c7",
	}
}

func TokyoNight() Palette {
	return Palette{
		Name: "tokyo-night", Bg: "#1a1b26", Muted: "#737aa2", Text: "#c0caf5",
		Accent: "#7aa2f7", Accent2: "#bb9af7", Accent3: "#ff9e64",
		Danger: "#f7768e", OK: "#9ece6a", Warn: "#e0af68", Bright: "#ffffff",
	}
}

func Kanagawa() Palette {
	return Palette{
		Name: "kanagawa", Bg: "#1f1f28", Muted: "#727169", Text: "#dcd7ba",
		Accent: "#7e9cd8", Accent2: "#957fb8", Accent3: "#d27e99",
		Danger: "#e82424", OK: "#98bb6c", Warn: "#ff9e3b", Bright: "#c8c093",
	}
}

func RosePine() Palette {
	return Palette{
		Name: "rose-pine", Bg: "#191724", Muted: "#908caa", Text: "#e0def4",
		Accent: "#ebbcba", Accent2: "#c4a7e7", Accent3: "#31748f",
		Danger: "#eb6f92", OK: "#9ccfd8", Warn: "#f6c177", Bright: "#e0def4",
	}
}

func Everforest() Palette {
	return Palette{
		Name: "everforest", Bg: "#2d353b", Muted: "#859289", Text: "#d3c6aa",
		Accent: "#a7c080", Accent2: "#83c092", Accent3: "#d699b6",
		Danger: "#e67e80", OK: "#a7c080", Warn: "#dbbc7f", Bright: "#d3c6aa",
	}
}

func OneDark() Palette {
	return Palette{
		Name: "one-dark", Bg: "#282c34", Muted: "#828997", Text: "#abb2bf",
		Accent: "#61afef", Accent2: "#c678dd", Accent3: "#d19a66",
		Danger: "#e06c75", OK: "#98c379", Warn: "#e5c07b", Bright: "#ffffff",
	}
}

func SolarizedDark() Palette {
	return Palette{
		Name: "solarized-dark", Bg: "#002b36", Muted: "#657b83", Text: "#839496",
		Accent: "#2aa198", Accent2: "#268bd2", Accent3: "#d33682",
		Danger: "#dc322f", OK: "#859900", Warn: "#b58900", Bright: "#93a1a1",
	}
}

func Material() Palette {
	return Palette{
		Name: "material", Bg: "#263238", Muted: "#607d8b", Text: "#b0bec5",
		Accent: "#80cbc4", Accent2: "#82aaff", Accent3: "#f78c6c",
		Danger: "#ff5370", OK: "#c3e88d", Warn: "#ffcb6b", Bright: "#ffffff",
	}
}

func Matrix() Palette {
	return Palette{
		Name: "matrix", Bg: "#0d0208", Muted: "#008f11", Text: "#7dff9a",
		Accent: "#00ff41", Accent2: "#39ff14", Accent3: "#00b32c",
		Danger: "#ff3b3b", OK: "#39ff14", Warn: "#ffb000", Bright: "#d4ffd4",
		SelBg: "#003b00", SelFg: "#00ff41",
	}
}

func SolarizedLight() Palette {
	return Palette{
		Name: "solarized-light", Light: true, Bg: "#fdf6e3", Muted: "#657b83", Text: "#586e75",
		Accent: "#268bd2", Accent2: "#6c71c4", Accent3: "#d33682",
		Danger: "#dc322f", OK: "#859900", Warn: "#b58900", Bright: "#073642",
		SelBg: "#eee8d5", SelFg: "#073642",
	}
}

func Contrast(a, b lipgloss.Color) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func luminance(c lipgloss.Color) float64 {
	r, g, b := rgb(c)
	lin := func(v float64) float64 {
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

func Hue(c lipgloss.Color) float64 {
	r, g, b := rgb(c)
	hi, lo := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	d := hi - lo
	if d == 0 {
		return 0
	}
	var h float64
	switch hi {
	case r:
		h = math.Mod((g-b)/d, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return h
}

func rgb(c lipgloss.Color) (r, g, b float64) {
	s := string(c)
	if len(s) != 7 || s[0] != '#' {
		return 0, 0, 0
	}
	v, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return 0, 0, 0
	}
	return float64(v>>16&0xff) / 255, float64(v>>8&0xff) / 255, float64(v&0xff) / 255
}

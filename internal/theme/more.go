package theme

func init() {
	registry = append(registry,
		NightOwl, Ayu, Monokai, GitHubDark, Flexoki,
		Oxocarbon, Poimandres, Horizon, Zenburn, CatppuccinLatte,
		CatppuccinFrappe, CatppuccinMacchiato, RosePineMoon, RosePineDawn, GruvboxLight,
		TokyoNightStorm, TokyoNightDay, Moonlight, Synthwave84, Palenight,
		Sonokai, Vesper, EverforestLight,
	)
	extra := map[string]string{
		"night-owl":            "Night Owl: blue and gold on deep navy",
		"ayu":                  "Ayu Mirage: warm yellow on blue-gray",
		"monokai":              "Monokai: green and pink on olive black",
		"github-dark":          "GitHub Dark: blue links on near black",
		"flexoki":              "Flexoki: warm paper ink, dark",
		"oxocarbon":            "IBM Oxocarbon: cool gray and cyan",
		"poimandres":           "Poimandres: ice blue on blue-black",
		"horizon":              "Horizon: coral and peach on indigo",
		"zenburn":              "Zenburn: low contrast cream on gray",
		"catppuccin-latte":     "Catppuccin Latte: mauve on warm paper (light)",
		"catppuccin-frappe":    "Catppuccin Frappé: mauve on muted blue-gray",
		"catppuccin-macchiato": "Catppuccin Macchiato: mauve on slate blue",
		"rose-pine-moon":       "Rosé Pine Moon: rose and iris on dusky violet",
		"rose-pine-dawn":       "Rosé Pine Dawn: iris and pine on parchment (light)",
		"gruvbox-light":        "Gruvbox light: retro yellow and orange on cream (light)",
		"tokyo-night-storm":    "Tokyo Night Storm: cyan and orange on stormy navy",
		"tokyo-night-day":      "Tokyo Night Day: blue ink on pale gray (light)",
		"moonlight":            "Moonlight II: sky blue and lilac on midnight",
		"synthwave84":          "Synthwave '84: neon pink and cyan on dusk purple",
		"palenight":            "Material Palenight: purple and blue on dusky indigo",
		"sonokai":              "Sonokai: Monokai pro tones, blue on charcoal",
		"vesper":               "Vesper: peach and mint on black",
		"everforest-light":     "Everforest light medium: blue and green on soft paper (light)",
	}
	for k, v := range extra {
		Describe[k] = v
	}
}

func NightOwl() Palette {
	return Palette{
		Name: "night-owl", Bg: "#011627", Muted: "#89a4bb", Text: "#d6deeb",
		Accent: "#82aaff", Accent2: "#c792ea", Accent3: "#ffcb8b",
		Danger: "#ef5350", OK: "#addb67", Warn: "#f78c6c", Bright: "#ffffff",
	}
}

func Ayu() Palette {
	return Palette{
		Name: "ayu", Bg: "#1f2430", Muted: "#8a9199", Text: "#cbccc6",
		Accent: "#ffcc66", Accent2: "#73d0ff", Accent3: "#f28779",
		Danger: "#ff3333", OK: "#bae67e", Warn: "#ffd580", Bright: "#ffffff",
	}
}

func Monokai() Palette {
	return Palette{
		Name: "monokai", Bg: "#272822", Muted: "#8f908a", Text: "#f8f8f2",
		Accent: "#a6e22e", Accent2: "#66d9ef", Accent3: "#ae81ff",
		Danger: "#f92672", OK: "#a6e22e", Warn: "#e6db74", Bright: "#ffffff",
	}
}

func GitHubDark() Palette {
	return Palette{
		Name: "github-dark", Bg: "#0d1117", Muted: "#8b949e", Text: "#e6edf3",
		Accent: "#58a6ff", Accent2: "#bc8cff", Accent3: "#ffa657",
		Danger: "#f85149", OK: "#3fb950", Warn: "#d29922", Bright: "#ffffff",
	}
}

func Flexoki() Palette {
	return Palette{
		Name: "flexoki", Bg: "#100f0f", Muted: "#878580", Text: "#cecdc3",
		Accent: "#3aa99f", Accent2: "#4385be", Accent3: "#ce5d97",
		Danger: "#d14d41", OK: "#879a39", Warn: "#d0a215", Bright: "#fffcf0",
	}
}

func Oxocarbon() Palette {
	return Palette{
		Name: "oxocarbon", Bg: "#161616", Muted: "#8d8d8d", Text: "#f2f4f8",
		Accent: "#33b1ff", Accent2: "#be95ff", Accent3: "#ff7eb6",
		Danger: "#fa4d56", OK: "#42be65", Warn: "#f1c21b", Bright: "#ffffff",
	}
}

func Poimandres() Palette {
	return Palette{
		Name: "poimandres", Bg: "#1b1e28", Muted: "#8b90a7", Text: "#e4f0fb",
		Accent: "#add7ff", Accent2: "#5de4c7", Accent3: "#91b4d5",
		Danger: "#e24b6a", OK: "#5de4c7", Warn: "#fffac2", Bright: "#ffffff",
	}
}

func Horizon() Palette {
	return Palette{
		Name: "horizon", Bg: "#1c1e26", Muted: "#8b8da3", Text: "#d5d8da",
		Accent: "#e95678", Accent2: "#fab795", Accent3: "#25b2bc",
		Danger: "#ff4d4d", OK: "#29d398", Warn: "#f0c674", Bright: "#ffffff",
	}
}

func Zenburn() Palette {
	return Palette{
		Name: "zenburn", Bg: "#3f3f3f", Muted: "#9fafaf", Text: "#dcdccc",
		Accent: "#f0dfaf", Accent2: "#8cd0d3", Accent3: "#dc8cc3",
		Danger: "#cc9393", OK: "#7f9f7f", Warn: "#dfaf8f", Bright: "#ffffef",
	}
}

func CatppuccinLatte() Palette {
	return Palette{
		Name: "catppuccin-latte", Light: true, Bg: "#eff1f5", Muted: "#6c6f85", Text: "#4c4f69",
		Accent: "#8839ef", Accent2: "#1e66f5", Accent3: "#fe640b",
		Danger: "#d20f39", OK: "#40a02b", Warn: "#df8e1d", Bright: "#4c4f69",
		SelBg: "#ccd0da", SelFg: "#4c4f69",
	}
}

func CatppuccinFrappe() Palette {
	return Palette{
		Name: "catppuccin-frappe", Bg: "#303446", Muted: "#838ba7", Text: "#c6d0f5",
		Accent: "#ca9ee6", Accent2: "#8caaee", Accent3: "#ef9f76",
		Danger: "#e78284", OK: "#a6d189", Warn: "#e5c890", Bright: "#f2d5cf",
	}
}

func CatppuccinMacchiato() Palette {
	return Palette{
		Name: "catppuccin-macchiato", Bg: "#24273a", Muted: "#8087a2", Text: "#cad3f5",
		Accent: "#c6a0f6", Accent2: "#8aadf4", Accent3: "#f5a97f",
		Danger: "#ed8796", OK: "#a6da95", Warn: "#eed49f", Bright: "#f4dbd6",
	}
}

func RosePineMoon() Palette {
	return Palette{
		Name: "rose-pine-moon", Bg: "#232136", Muted: "#908caa", Text: "#e0def4",
		Accent: "#ea9a97", Accent2: "#c4a7e7", Accent3: "#3e8fb0",
		Danger: "#eb6f92", OK: "#9ccfd8", Warn: "#f6c177", Bright: "#e0def4",
	}
}

func RosePineDawn() Palette {
	return Palette{
		Name: "rose-pine-dawn", Light: true, Bg: "#faf4ed", Muted: "#797593", Text: "#464261",
		Accent: "#907aa9", Accent2: "#286983", Accent3: "#d7827e",
		Danger: "#b4637a", OK: "#56949f", Warn: "#ea9d34", Bright: "#464261",
		SelBg: "#dfdad9", SelFg: "#464261",
	}
}

func GruvboxLight() Palette {
	return Palette{
		Name: "gruvbox-light", Light: true, Bg: "#fbf1c7", Muted: "#7c6f64", Text: "#3c3836",
		Accent: "#b57614", Accent2: "#427b58", Accent3: "#8f3f71",
		Danger: "#9d0006", OK: "#79740e", Warn: "#af3a03", Bright: "#282828",
		SelBg: "#ebdbb2", SelFg: "#282828",
	}
}

func TokyoNightStorm() Palette {
	return Palette{
		Name: "tokyo-night-storm", Bg: "#24283b", Muted: "#737aa2", Text: "#c0caf5",
		Accent: "#7dcfff", Accent2: "#bb9af7", Accent3: "#ff9e64",
		Danger: "#f7768e", OK: "#9ece6a", Warn: "#e0af68", Bright: "#ffffff",
	}
}

func TokyoNightDay() Palette {
	return Palette{
		Name: "tokyo-night-day", Light: true, Bg: "#e1e2e7", Muted: "#68709a", Text: "#3760bf",
		Accent: "#2e7de9", Accent2: "#9854f1", Accent3: "#b15c00",
		Danger: "#f52a65", OK: "#587539", Warn: "#8c6c3e", Bright: "#3760bf",
		SelBg: "#3760bf", SelFg: "#e1e2e7",
	}
}

func Moonlight() Palette {
	return Palette{
		Name: "moonlight", Bg: "#222436", Muted: "#828bb8", Text: "#c8d3f5",
		Accent: "#7cafff", Accent2: "#c4a2ff", Accent3: "#ff966c",
		Danger: "#ff757f", OK: "#c3e88d", Warn: "#ffc777", Bright: "#ffffff",
	}
}

func Synthwave84() Palette {
	return Palette{
		Name: "synthwave84", Bg: "#262335", Muted: "#848bbd", Text: "#ffffff",
		Accent: "#ff7edb", Accent2: "#36f9f6", Accent3: "#f97e72",
		Danger: "#fe4450", OK: "#72f1b8", Warn: "#fede5d", Bright: "#ffffff",
	}
}

func Palenight() Palette {
	return Palette{
		Name: "palenight", Bg: "#292d3e", Muted: "#717cb4", Text: "#a6accd",
		Accent: "#c792ea", Accent2: "#82aaff", Accent3: "#f78c6c",
		Danger: "#ff5370", OK: "#c3e88d", Warn: "#ffcb6b", Bright: "#ffffff",
	}
}

func Sonokai() Palette {
	return Palette{
		Name: "sonokai", Bg: "#2c2e34", Muted: "#7f8490", Text: "#e2e2e3",
		Accent: "#76cce0", Accent2: "#b39df3", Accent3: "#f39660",
		Danger: "#fc5d7c", OK: "#9ed072", Warn: "#e7c664", Bright: "#ffffff",
	}
}

func Vesper() Palette {
	return Palette{
		Name: "vesper", Bg: "#101010", Muted: "#a0a0a0", Text: "#ffffff",
		Accent: "#ffc799", Accent2: "#99ffe4", Accent3: "#ffcfa8",
		Danger: "#ff8080", OK: "#99ffe4", Warn: "#ffc799", Bright: "#ffffff",
	}
}

func EverforestLight() Palette {
	return Palette{
		Name: "everforest-light", Light: true, Bg: "#fdf6e3", Muted: "#829181", Text: "#5c6a72",
		Accent: "#3a94c5", Accent2: "#8da101", Accent3: "#df69ba",
		Danger: "#f85552", OK: "#8da101", Warn: "#dfa000", Bright: "#5c6a72",
		SelBg: "#eaedc8", SelFg: "#5c6a72",
	}
}

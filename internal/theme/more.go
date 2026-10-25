package theme

func init() {
	registry = append(registry,
		NightOwl, Ayu, Monokai, GitHubDark, Flexoki,
		Oxocarbon, Poimandres, Horizon, Zenburn, CatppuccinLatte,
	)
	extra := map[string]string{
		"night-owl":        "Night Owl: blue and gold on deep navy",
		"ayu":              "Ayu Mirage: warm yellow on blue-gray",
		"monokai":          "Monokai: green and pink on olive black",
		"github-dark":      "GitHub Dark: blue links on near black",
		"flexoki":          "Flexoki: warm paper ink, dark",
		"oxocarbon":        "IBM Oxocarbon: cool gray and cyan",
		"poimandres":       "Poimandres: ice blue on blue-black",
		"horizon":          "Horizon: coral and peach on indigo",
		"zenburn":          "Zenburn: low contrast cream on gray",
		"catppuccin-latte": "Catppuccin Latte: mauve on warm paper (light)",
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

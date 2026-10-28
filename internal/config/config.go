package config

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/charmbracelet/lipgloss"

	"k2/internal/theme"
)

var OutputFormats = []string{"default", "wide", "yaml", "json", "name"}

type PortForward struct {
	AutoRestart bool

	RestartBackoff int

	MaxRestarts int

	HealthEvery int
}

const MaxBackoff = 30

type Logs struct {
	Tail     int
	MaxLines int

	MultiMaxLines int

	MultiEvery int

	MultiMaxPods int

	MultiAllContainers bool
}

type Helm struct {
	Binary string
}

type Favorite struct {
	Name       string
	Kubeconfig string
	Context    string
	Namespace  string
	View       string
}

var LogoStyles = []string{"default", "simple"}

const DefaultTagline = "kubectl, elevated"

type UI struct {
	Logo string

	Tagline string

	LogoTitleColor     string
	SidebarWidth       int
	SidebarNarrowWidth int
	RefreshEvery       int
	ForwardDockRows    int
	Animations         bool
	Toasts             bool
	ToastSeconds       int
	Tabbar             string

	EditorInline bool

	ActiveBorder string

	DimInactive bool

	ClusterInfo bool

	PodMetrics   bool
	MetricsEvery int

	MetricsBars              bool
	MetricsWarn, MetricsCrit int

	TokenBar             bool
	TokenWarn, TokenCrit int

	Clipboard string

	Wrap bool

	CopyModeHint bool

	Background string

	BackgroundSet bool
}

var BorderStyles = []string{"normal", "thick", "double"}

var ClipboardModes = []string{"auto", "wl-copy", "xclip", "xsel", "osc52"}

var LogoTitleColors = []string{"green", "accent"}

var TabbarModes = []string{"auto", "always", "never"}

var BackgroundModes = []string{"transparent", "theme"}

func (u UI) PaintBackground(light bool) bool {
	if u.Background == "theme" {
		return true
	}
	return light && !u.BackgroundSet
}

type Config struct {
	Theme         string
	MaxPanes      int
	StateDir      string
	KubectlOutput string
	Prod          []string
	Aliases       map[string]string

	ContextThemes []ContextTheme
	Logs          Logs
	Helm          Helm
	Favorites     []Favorite
	PortForward   PortForward
	UI            UI
	Keys          Keys
	Custom        theme.Palette
}

func Default() Config {
	return Config{
		Theme:         "nord",
		MaxPanes:      4,
		StateDir:      DefaultDir(),
		KubectlOutput: "default",
		Aliases:       map[string]string{},
		Logs:          Logs{Tail: 80, MaxLines: 400, MultiMaxLines: 5000, MultiEvery: 2, MultiMaxPods: 20, MultiAllContainers: true},
		Helm:          Helm{Binary: "helm"},
		PortForward:   PortForward{AutoRestart: false, RestartBackoff: 1, MaxRestarts: 10, HealthEvery: 5},
		UI:            UI{Logo: "default", Tagline: DefaultTagline, LogoTitleColor: "green", SidebarWidth: 28, SidebarNarrowWidth: 22, RefreshEvery: 3, ForwardDockRows: 12, Animations: true, Toasts: true, ToastSeconds: 3, Tabbar: "auto", EditorInline: true, ActiveBorder: "normal", Background: "transparent", ClusterInfo: true, PodMetrics: true, MetricsEvery: 15, MetricsBars: true, MetricsWarn: 70, MetricsCrit: 90, TokenBar: true, TokenWarn: 25, TokenCrit: 10, Clipboard: "auto", Wrap: false, CopyModeHint: true},
		Keys:          DefaultKeys(),
	}
}

func (c Config) PaletteNamed(name string) theme.Palette {
	if name == "custom" && c.Custom.Name == "custom" {
		return c.Custom
	}
	if p, ok := theme.Lookup(name); ok {
		return p
	}
	return theme.Nord()
}

func (c Config) OutputIndex() int {
	for i, f := range OutputFormats {
		if f == c.KubectlOutput {
			return i
		}
	}
	return 0
}

func (c Config) IsProd(ctx string) bool {
	for _, p := range c.Prod {
		if p != "" && p == ctx {
			return true
		}
	}
	return false
}

func Load() (Config, []string) {
	b, err := os.ReadFile(Path())
	if err != nil {
		b, err = os.ReadFile(legacyPath())
	}
	if err != nil {
		return Default(), nil
	}
	return Parse(b)
}

func LoadFile(path string) (Config, []string) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return Default(), []string{"config: " + err.Error()}
	}
	return Parse(b)
}

func Parse(data []byte) (Config, []string) {
	cfg := Default()
	p := parser{}
	var colorsSeen bool
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		p.warn("%s", strings.TrimPrefix(err.Error(), "yaml: "))
		return cfg, p.warnings
	}
	if len(root.Content) == 0 {
		return cfg, nil
	}
	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		p.warn("top level must be a mapping of settings")
		return cfg, p.warnings
	}
	for i := 0; i+1 < len(doc.Content); i += 2 {
		key, val := doc.Content[i].Value, doc.Content[i+1]
		switch key {
		case "theme":
			if s, ok := p.str(key, val); ok {
				name := strings.ToLower(s)
				if name == "custom" {
					cfg.Theme = name
				} else if _, found := theme.Lookup(name); found {
					cfg.Theme = name
				} else {
					p.warn("theme: unknown theme %q (using %s; have %s)", s, cfg.Theme, themeList())
				}
			}
		case "colors":
			colorsSeen = p.colors(val, &cfg.Custom)
		case "max_panes":
			p.intIn(key, val, 1, 8, &cfg.MaxPanes)
		case "state_dir":
			if s, ok := p.str(key, val); ok {
				if s == "" {
					p.warn("state_dir: empty (using %s)", cfg.StateDir)
				} else {
					cfg.StateDir = ExpandHome(s)
				}
			}
		case "kubectl_output":
			if s, ok := p.str(key, val); ok {
				if contains(OutputFormats, s) {
					cfg.KubectlOutput = s
				} else {
					p.warn("kubectl_output: %q is not one of %s (using %s)", s, strings.Join(OutputFormats, ", "), cfg.KubectlOutput)
				}
			}
		case "prod":
			cfg.Prod = p.strList(key, val)
		case "aliases":
			p.aliases(val, cfg.Aliases)
		case "logs":
			p.logs(val, &cfg.Logs)
		case "helm":
			p.helm(val, &cfg.Helm)
		case "favorites":
			cfg.Favorites = p.favorites(val)
		case "portforward":
			p.portForward(val, &cfg.PortForward)
		case "ui":
			p.uiSection(val, &cfg.UI, &cfg.ContextThemes)
		case "keys":
			p.keys(val, &cfg.Keys)
		case "profiles":
			p.warn("profiles: files in the profiles directory, not in the config (ignored)")
		default:
			p.warn("unknown setting %q (ignored)", key)
		}
	}
	p.finishCustom(&cfg, colorsSeen)
	return cfg, p.warnings
}

type parser struct{ warnings []string }

func (p *parser) warn(format string, args ...any) {
	p.warnings = append(p.warnings, "config: "+fmt.Sprintf(format, args...))
}

func (p *parser) str(key string, n *yaml.Node) (string, bool) {
	if n.Kind != yaml.ScalarNode {
		p.warn("%s: expected a single value (line %d, using default)", key, n.Line)
		return "", false
	}
	return strings.TrimSpace(n.Value), true
}

func (p *parser) intIn(key string, n *yaml.Node, lo, hi int, dst *int) {
	var v int
	if n.Kind != yaml.ScalarNode || n.Decode(&v) != nil {
		p.warn("%s: %q is not a whole number (using %d)", key, n.Value, *dst)
		return
	}
	if v < lo || v > hi {
		p.warn("%s: %d is out of range %d..%d (using %d)", key, v, lo, hi, *dst)
		return
	}
	*dst = v
}

func (p *parser) strList(key string, n *yaml.Node) []string {
	switch n.Kind {
	case yaml.ScalarNode:
		if s := strings.TrimSpace(n.Value); s != "" {
			return []string{s}
		}
		return nil
	case yaml.SequenceNode:
		var out []string
		for _, it := range n.Content {
			if it.Kind != yaml.ScalarNode || strings.TrimSpace(it.Value) == "" {
				p.warn("%s: skipped an entry that is not a plain name (line %d)", key, it.Line)
				continue
			}
			out = append(out, strings.TrimSpace(it.Value))
		}
		return out
	default:
		p.warn("%s: expected a list of names (line %d, ignored)", key, n.Line)
		return nil
	}
}

func (p *parser) section(name string, n *yaml.Node, fields map[string]*int, lo, hi int) {
	if n.Kind != yaml.MappingNode {
		p.warn("%s: expected a mapping (line %d, using defaults)", name, n.Line)
		return
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k := n.Content[i].Value
		dst, ok := fields[k]
		if !ok {
			p.warn("%s: unknown setting %q (have %s)", name, k, strings.Join(sortedKeys(fields), ", "))
			continue
		}
		p.intIn(name+"."+k, n.Content[i+1], lo, hi, dst)
	}
}

func (p *parser) logs(n *yaml.Node, l *Logs) {
	if n.Kind != yaml.MappingNode {
		p.warn("logs: expected a mapping (line %d, using defaults)", n.Line)
		return
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i].Value, n.Content[i+1]
		switch k {
		case "tail":
			p.intIn("logs."+k, v, 1, 100000, &l.Tail)
		case "max_lines":
			p.intIn("logs."+k, v, 1, 100000, &l.MaxLines)
		case "multi_max_lines":
			p.intIn("logs."+k, v, 100, 200000, &l.MultiMaxLines)
		case "multi_every":
			p.intIn("logs."+k, v, 1, 60, &l.MultiEvery)
		case "multi_max_pods":
			p.intIn("logs."+k, v, 1, 200, &l.MultiMaxPods)
		case "multi_all_containers":
			p.boolean("logs."+k, v, &l.MultiAllContainers)
		default:
			p.warn("logs: unknown setting %q (have max_lines, multi_all_containers, multi_every, multi_max_lines, multi_max_pods, tail)", k)
		}
	}
}

func (p *parser) helm(n *yaml.Node, h *Helm) {
	if n.Kind != yaml.MappingNode {
		p.warn("helm: expected a mapping (line %d, using defaults)", n.Line)
		return
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i].Value, n.Content[i+1]
		switch k {
		case "binary":
			if v.Tag == "!!null" {
				h.Binary = ""
			} else if s, ok := p.str("helm."+k, v); ok {
				h.Binary = s
			}
		default:
			p.warn("helm: unknown setting %q (have binary)", k)
		}
	}
}

func (p *parser) favorites(n *yaml.Node) []Favorite {
	if n.Tag == "!!null" || (n.Kind == yaml.ScalarNode && strings.TrimSpace(n.Value) == "") {
		return nil
	}
	if n.Kind != yaml.SequenceNode {
		p.warn("favorites: expected a list like `- {context: kind-dev, namespace: shop}` (line %d, ignored)", n.Line)
		return nil
	}
	var out []Favorite
	for _, it := range n.Content {
		if it.Kind != yaml.MappingNode {
			p.warn("favorites: skipped an entry that is not a mapping (line %d)", it.Line)
			continue
		}
		var f Favorite
		ok := true
		for i := 0; i+1 < len(it.Content); i += 2 {
			k, v := it.Content[i].Value, it.Content[i+1]
			s, good := p.str("favorites."+k, v)
			if !good {
				ok = false
				break
			}
			switch k {
			case "name":
				f.Name = s
			case "kubeconfig":
				f.Kubeconfig = ExpandHome(s)
			case "context":
				f.Context = s
			case "namespace":
				f.Namespace = s
			case "view":
				f.View = strings.TrimPrefix(s, ":")
			default:
				p.warn("favorites: unknown field %q (line %d; have context, kubeconfig, name, namespace, view)", k, v.Line)
			}
		}
		if !ok {
			continue
		}
		if f.Context == "" {
			p.warn("favorites: an entry without context was skipped (line %d)", it.Line)
			continue
		}
		out = append(out, f)
	}
	return out
}

func (p *parser) portForward(n *yaml.Node, pf *PortForward) {
	if n.Kind != yaml.MappingNode {
		p.warn("portforward: expected a mapping (line %d, using defaults)", n.Line)
		return
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i].Value, n.Content[i+1]
		switch k {
		case "auto_restart":
			p.boolean("portforward."+k, v, &pf.AutoRestart)
		case "restart_backoff":
			p.intIn("portforward."+k, v, 1, MaxBackoff, &pf.RestartBackoff)
		case "max_restarts":
			p.intIn("portforward."+k, v, 0, 1000, &pf.MaxRestarts)
		case "health_every":
			p.intIn("portforward."+k, v, 0, 300, &pf.HealthEvery)
		default:
			p.warn("portforward: unknown setting %q (have auto_restart, health_every, max_restarts, restart_backoff)", k)
		}
	}
}

func (p *parser) uiSection(n *yaml.Node, ui *UI, themes *[]ContextTheme) {
	ints := map[string]*int{
		"sidebar_width":        &ui.SidebarWidth,
		"sidebar_narrow_width": &ui.SidebarNarrowWidth,
		"refresh_every":        &ui.RefreshEvery,
		"forward_dock_rows":    &ui.ForwardDockRows,
	}
	bools := map[string]*bool{"animations": &ui.Animations, "toasts": &ui.Toasts, "editor_inline": &ui.EditorInline, "dim_inactive": &ui.DimInactive, "cluster_info": &ui.ClusterInfo, "pod_metrics": &ui.PodMetrics, "wrap": &ui.Wrap, "metrics_bars": &ui.MetricsBars, "copy_mode_hint": &ui.CopyModeHint, "token_bar": &ui.TokenBar}
	def := ui.MetricsWarn
	defCrit := ui.MetricsCrit
	defer func() {
		if ui.MetricsWarn >= ui.MetricsCrit {
			p.warn("ui.metrics_warn (%d) must be below ui.metrics_crit (%d) (using %d and %d)", ui.MetricsWarn, ui.MetricsCrit, def, defCrit)
			ui.MetricsWarn, ui.MetricsCrit = def, defCrit
		}
	}()

	tokWarn, tokCrit := ui.TokenWarn, ui.TokenCrit
	defer func() {
		if ui.TokenCrit >= ui.TokenWarn {
			p.warn("ui.token_crit (%d) must be below ui.token_warn (%d) (using %d and %d)", ui.TokenCrit, ui.TokenWarn, tokCrit, tokWarn)
			ui.TokenWarn, ui.TokenCrit = tokWarn, tokCrit
		}
	}()
	if n.Kind != yaml.MappingNode {
		p.warn("ui: expected a mapping (line %d, using defaults)", n.Line)
		return
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i].Value, n.Content[i+1]
		switch {
		case k == "logo_title_color":
			if s, ok := p.str("ui."+k, v); ok {
				if s = strings.ToLower(s); contains(LogoTitleColors, s) {
					ui.LogoTitleColor = s
				} else {
					p.warn("ui.logo_title_color: %q is not one of %s (using %s)", v.Value, strings.Join(LogoTitleColors, ", "), ui.LogoTitleColor)
				}
			}
		case k == "tabbar":
			if s, ok := p.str("ui."+k, v); ok {
				if s = strings.ToLower(s); contains(TabbarModes, s) {
					ui.Tabbar = s
				} else {
					p.warn("ui.tabbar: %q is not one of %s (using %s)", v.Value, strings.Join(TabbarModes, ", "), ui.Tabbar)
				}
			}
		case k == "active_border":
			if s, ok := p.str("ui."+k, v); ok {
				if s = strings.ToLower(s); contains(BorderStyles, s) {
					ui.ActiveBorder = s
				} else {
					p.warn("ui.active_border: %q is not one of %s (using %s)", v.Value, strings.Join(BorderStyles, ", "), ui.ActiveBorder)
				}
			}
		case k == "clipboard":
			if s, ok := p.str("ui."+k, v); ok {
				if s = strings.ToLower(s); contains(ClipboardModes, s) {
					ui.Clipboard = s
				} else {
					p.warn("ui.clipboard: %q is not one of %s (using %s)", v.Value, strings.Join(ClipboardModes, ", "), ui.Clipboard)
				}
			}
		case k == "background":
			if s, ok := p.str("ui."+k, v); ok {
				if s = strings.ToLower(s); contains(BackgroundModes, s) {
					ui.Background, ui.BackgroundSet = s, true
				} else {
					p.warn("ui.background: %q is not one of %s (using %s)", v.Value, strings.Join(BackgroundModes, ", "), ui.Background)
				}
			}
		case k == "logo":
			if s, ok := p.str("ui."+k, v); ok {
				if s = strings.ToLower(s); contains(LogoStyles, s) {
					ui.Logo = s
				} else {
					p.warn("ui.logo: %q is not one of %s (using %s)", v.Value, strings.Join(LogoStyles, ", "), ui.Logo)
				}
			}
		case k == "tagline":
			p.tagline(v, &ui.Tagline)
		case k == "toast_seconds":
			p.intIn("ui."+k, v, 1, 30, &ui.ToastSeconds)
		case k == "metrics_every":
			p.intIn("ui."+k, v, 5, 600, &ui.MetricsEvery)
		case k == "metrics_warn":
			p.intIn("ui."+k, v, 1, 99, &ui.MetricsWarn)
		case k == "metrics_crit":
			p.intIn("ui."+k, v, 1, 99, &ui.MetricsCrit)
		case k == "context_themes":
			p.contextThemes(v, themes)
		case k == "token_warn":
			p.intIn("ui."+k, v, 1, 99, &ui.TokenWarn)
		case k == "token_crit":
			p.intIn("ui."+k, v, 1, 99, &ui.TokenCrit)
		case ints[k] != nil:
			p.intIn("ui."+k, v, 1, 200, ints[k])
		case bools[k] != nil:
			p.boolean("ui."+k, v, bools[k])
		default:
			names := append(sortedKeys(ints), "active_border", "animations", "background", "clipboard", "cluster_info", "context_themes", "copy_mode_hint", "dim_inactive", "editor_inline", "logo", "logo_title_color", "metrics_bars", "metrics_crit", "metrics_every", "metrics_warn", "pod_metrics", "tabbar", "tagline", "toast_seconds", "toasts", "token_bar", "token_crit", "token_warn", "wrap")
			sort.Strings(names)
			p.warn("ui: unknown setting %q (have %s)", k, strings.Join(names, ", "))
		}
	}
}

func (p *parser) tagline(n *yaml.Node, dst *string) {
	if n.Kind != yaml.ScalarNode {
		p.warn("ui.tagline: expected a text (line %d, using %q)", n.Line, *dst)
		return
	}
	if n.Tag == "!!null" {
		*dst = ""
		return
	}
	*dst = strings.TrimSpace(n.Value)
}

func themeList() string {
	return strings.Join(append(theme.Names(), "custom"), ", ")
}

func (p *parser) colors(n *yaml.Node, dst *theme.Palette) bool {
	if n.Kind != yaml.MappingNode {
		p.warn("colors: expected a mapping (line %d)", n.Line)
		return false
	}
	fields := map[string]*lipgloss.Color{
		"bg": &dst.Bg, "muted": &dst.Muted, "text": &dst.Text,
		"accent": &dst.Accent, "accent2": &dst.Accent2, "accent3": &dst.Accent3,
		"danger": &dst.Danger, "ok": &dst.OK, "warn": &dst.Warn, "bright": &dst.Bright,
		"sel_bg": &dst.SelBg, "sel_fg": &dst.SelFg,
	}
	saw := false
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i].Value, n.Content[i+1]
		if k == "light" {
			p.boolean("colors.light", v, &dst.Light)
			saw = true
			continue
		}
		f, ok := fields[k]
		if !ok {
			p.warn("colors: unknown field %q (have bg, muted, text, accent, accent2, accent3, danger, ok, warn, bright, sel_bg, sel_fg, light)", k)
			continue
		}
		s, ok := p.str("colors."+k, v)
		if !ok {
			continue
		}
		if len(s) != 7 || s[0] != '#' || !hexColor(s[1:]) {
			p.warn("colors.%s: %q is not a #rrggbb color", k, s)
			continue
		}
		*f = lipgloss.Color(strings.ToLower(s))
		saw = true
	}
	return saw
}

func hexColor(s string) bool {
	if len(s) != 6 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

func (p *parser) finishCustom(cfg *Config, colorsSeen bool) {
	if cfg.Theme != "custom" {
		if colorsSeen {
			p.warn("colors: ignored unless theme is custom")
		}
		cfg.Custom = theme.Palette{}
		return
	}
	missing := missingColors(cfg.Custom)
	if len(missing) > 0 {
		p.warn("colors: custom theme needs %s (using nord)", strings.Join(missing, ", "))
		cfg.Theme = "nord"
		cfg.Custom = theme.Palette{}
		return
	}
	cfg.Custom.Name = "custom"
}

func missingColors(p theme.Palette) []string {
	need := []struct {
		name string
		c    lipgloss.Color
	}{
		{"bg", p.Bg}, {"muted", p.Muted}, {"text", p.Text},
		{"accent", p.Accent}, {"accent2", p.Accent2}, {"accent3", p.Accent3},
		{"danger", p.Danger}, {"ok", p.OK}, {"warn", p.Warn}, {"bright", p.Bright},
	}
	var missing []string
	for _, n := range need {
		if n.c == "" {
			missing = append(missing, n.name)
		}
	}
	return missing
}

func (p *parser) boolean(key string, n *yaml.Node, dst *bool) {
	if n.Kind == yaml.ScalarNode {
		switch strings.ToLower(strings.TrimSpace(n.Value)) {
		case "true", "yes", "on":
			*dst = true
			return
		case "false", "no", "off":
			*dst = false
			return
		}
	}
	p.warn("%s: %q is not true or false (using %t)", key, n.Value, *dst)
}

func (p *parser) aliases(n *yaml.Node, dst map[string]string) {
	if n.Kind != yaml.MappingNode {
		p.warn("aliases: expected a mapping like `dp: deployments` (line %d, ignored)", n.Line)
		return
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k := strings.ToLower(strings.TrimSpace(n.Content[i].Value))
		v := n.Content[i+1]
		if k == "" || v.Kind != yaml.ScalarNode || strings.TrimSpace(v.Value) == "" {
			p.warn("aliases: %q needs a resource name (line %d, ignored)", k, v.Line)
			continue
		}
		dst[k] = strings.ToLower(strings.TrimSpace(v.Value))
	}
}

func (p *parser) keys(n *yaml.Node, k *Keys) {
	if n.Kind != yaml.MappingNode {
		p.warn("keys: expected a mapping like `delete: ctrl+d` (line %d, using defaults)", n.Line)
		return
	}
	fields := map[string]keyField{}
	for _, f := range keyFields {
		fields[f.name] = f
	}
	var changed []string
	defer func() { resolveConflicts(k, changed, p.warn) }()
	for i := 0; i+1 < len(n.Content); i += 2 {
		name, v := n.Content[i].Value, n.Content[i+1]
		if alias, ok := keyAliases[name]; ok {
			name = alias
		}
		f, ok := fields[name]
		if !ok {
			p.warn("keys: unknown action %q (ignored)", name)
			continue
		}
		dst := f.ptr(k)
		var raw []*yaml.Node
		switch v.Kind {
		case yaml.ScalarNode:
			raw = []*yaml.Node{v}
		case yaml.SequenceNode:
			raw = v.Content
		}

		if v.Kind == yaml.SequenceNode && len(raw) == 0 && optionalKeys[name] {
			*dst = nil
			continue
		}
		var b Binding
		bad := len(raw) == 0
		for _, r := range raw {
			if r.Kind != yaml.ScalarNode {
				bad = true
				break
			}
			val := r.Value
			direct := false
			if rest, ok := cutDirect(val); ok {
				if !PrefixedKeys[name] {
					p.warn("keys.%s: %q: only pane and tab keys take \"direct\" (using %s)", name, val, strings.Join(*dst, ", "))
					bad = false
					b = nil
					break
				}
				val, direct = rest, true
			}
			key, err := ParseKey(val)
			if err == nil && name == "prefix" {
				if why := ValidPrefix(key); why != "" {
					err = fmt.Errorf("%s cannot be the prefix: %s", key, why)
				}
			}
			if err != nil {
				p.warn("keys.%s: %v (using %s)", name, err, strings.Join(*dst, ", "))
				bad = false
				b = nil
				break
			}
			if direct || legacyDirect(name, key) {
				key = DirectMark + key
			}
			b = append(b, key)
		}
		if bad {
			p.warn("keys.%s: expected a key or a list of keys (line %d, using %s)", name, v.Line, strings.Join(*dst, ", "))
			continue
		}
		if b != nil {
			*dst = b
			changed = append(changed, name)
		}
	}
}

func cutDirect(s string) (string, bool) {
	t := strings.TrimSpace(s)
	if len(t) > len(DirectMark) && strings.EqualFold(t[:len(DirectMark)], DirectMark) {
		return strings.TrimSpace(t[len(DirectMark):]), true
	}
	return s, false
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]*int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

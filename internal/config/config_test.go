package config

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestDefaultsMatchOriginal(t *testing.T) {
	c := Default()
	if c.Theme != "nord" || c.MaxPanes != 4 || c.KubectlOutput != "default" {
		t.Fatalf("unexpected defaults: %+v", c)
	}
	if c.Logs.Tail != 80 || c.Logs.MaxLines != 400 {
		t.Fatalf("log defaults: %+v", c.Logs)
	}
	if c.UI != (UI{LogoTitleColor: "green", SidebarWidth: 28, SidebarNarrowWidth: 22, RefreshEvery: 3, ForwardDockRows: 12, Animations: true, Toasts: true, ToastSeconds: 3, Tabbar: "auto", EditorInline: true, ActiveBorder: "thick", Background: "transparent", ClusterInfo: true, PodMetrics: true, MetricsEvery: 15, MetricsBars: true, MetricsWarn: 70, MetricsCrit: 90, TokenBar: true, TokenWarn: 25, TokenCrit: 10, Clipboard: "auto", Wrap: false, CopyModeHint: true}) {
		t.Fatalf("ui defaults: %+v", c.UI)
	}
	if c.StateDir != DefaultDir() || len(c.Prod) != 0 || len(c.Aliases) != 0 {
		t.Fatalf("state/prod/aliases defaults: %+v", c)
	}
	k := c.Keys
	want := map[string]Binding{
		"kubeconfig": {"C"}, "context": {"c"}, "namespace": {"n"}, "kustomize": {"K"},
		"kubectl": {"U"}, "forwards": {"f"}, "port_forward": {"ctrl+f", "F"}, "describe": {"d"}, "edit": {"e"},
		"kustomize_delete": {"d"}, "page_down": {"pgdown", "}"}, "page_up": {"pgup", "{"},
		"decode": {"x"}, "apply": {"a"}, "diff": {"D"}, "delete_all": {"D"},

		"prefix": {"ctrl+a"},

		"split_down": {"h", `"`}, "split_right": {"v", "%"}, "close_pane": {"x"},

		"pane_left": {"direct ctrl+left", "left"}, "pane_right": {"direct ctrl+right", "right"}, "pane_up": {"direct ctrl+up", "up"}, "pane_down": {"direct ctrl+down", "down"},

		"next_pane": {"o"}, "prev_pane": nil, "last_pane": {";"},
		"new_tab": {"t", "c"}, "close_tab": {"w", "&"}, "rename_tab": {","}, "zoom": {"z"},
		"next_tab": {"direct ]", "n", "]"}, "prev_tab": {"direct [", "p", "["},
		"wrap":  {"w"},
		"tab_1": {"1"}, "tab_5": {"5"}, "tab_9": {"9"},
		"fullscreen": {"Z"}, "quit": {"Q", "ctrl+c"}, "back": {"q", "esc", "H"},
		"delete": {"ctrl+d"}, "output": {"ctrl+o"}, "jq": {"ctrl+j"}, "command": {":"},
		"yes": {"y", "Y"}, "no": {"n", "N", "esc", "q"}, "vim": {"v"}, "complete": {"tab"},
	}
	for _, f := range keyFields {
		if w, ok := want[f.name]; ok && !reflect.DeepEqual(*f.ptr(&k), w) {
			t.Errorf("keys.%s = %v, want %v", f.name, *f.ptr(&k), w)
		}
	}
}

func TestEveryKeyHasADefault(t *testing.T) {
	k := DefaultKeys()
	for _, f := range keyFields {
		if len(*f.ptr(&k)) == 0 && !optionalKeys[f.name] {
			t.Errorf("keys.%s has no default", f.name)
		}
	}
}

func TestEmptyAndMissingFileGiveDefaults(t *testing.T) {
	for _, in := range []string{"", "# only a comment\n"} {
		c, w := Parse([]byte(in))
		if len(w) != 0 || !reflect.DeepEqual(c, Default()) {
			t.Fatalf("Parse(%q) = %+v, %v", in, c, w)
		}
	}
	c, w := LoadFile(filepath.Join(t.TempDir(), "nope.yaml"))
	if len(w) != 0 || !reflect.DeepEqual(c, Default()) {
		t.Fatalf("missing file: %+v %v", c, w)
	}
}

func TestUserExample(t *testing.T) {
	in := `
theme: nord
keys:
  kubeconfig: c
  context: C
  namespace: n
  kustomize: K
  kubectl: U
  forwards: f
  forward: N
  edit: e
  decode: x
  delete: ctrl+d
prod:
  - prod-eu-1
  - prod
  - kind-prod
`
	c, w := Parse([]byte(in))
	if len(w) != 0 {
		t.Fatalf("warnings: %v", w)
	}
	if !reflect.DeepEqual(c.Prod, []string{"prod-eu-1", "prod", "kind-prod"}) {
		t.Fatalf("prod = %v", c.Prod)
	}

	want := DefaultKeys()
	want.PortForward = Binding{"N"}

	want.Kubeconfig, want.Context = Binding{"c"}, Binding{"C"}
	if !reflect.DeepEqual(c.Keys, want) {
		t.Fatalf("keys changed: %+v", c.Keys)
	}
	if !c.IsProd("kind-prod") || c.IsProd("kind-dev") {
		t.Fatal("IsProd")
	}
}

func TestOverrides(t *testing.T) {
	in := `
theme: Dracula
max_panes: 6
state_dir: ~/k2state
kubectl_output: wide
aliases:
  DP: Deployments
  sub: subscriptions
logs: {tail: 10, max_lines: 50}
ui: {sidebar_width: 30, refresh_every: 5}
keys:
  delete: Ctrl+Y
  back: [esc, backspace]
  next_pane: shift+tab
  quit: shift+q
`
	c, w := Parse([]byte(in))
	if len(w) != 0 {
		t.Fatalf("warnings: %v", w)
	}
	home, _ := os.UserHomeDir()
	if c.Theme != "dracula" || c.MaxPanes != 6 || c.StateDir != filepath.Join(home, "k2state") {
		t.Fatalf("got %+v", c)
	}
	if c.KubectlOutput != "wide" || c.OutputIndex() != 1 {
		t.Fatalf("output %q", c.KubectlOutput)
	}
	if c.Aliases["dp"] != "deployments" || c.Aliases["sub"] != "subscriptions" {
		t.Fatalf("aliases %v", c.Aliases)
	}
	if c.Logs.Tail != 10 || c.Logs.MaxLines != 50 || c.UI.SidebarWidth != 30 || c.UI.RefreshEvery != 5 || c.UI.SidebarNarrowWidth != 22 {
		t.Fatalf("logs/ui %+v %+v", c.Logs, c.UI)
	}
	if !reflect.DeepEqual(c.Keys.Delete, Binding{"ctrl+y"}) ||
		!reflect.DeepEqual(c.Keys.Back, Binding{"esc", "backspace"}) ||
		!reflect.DeepEqual(c.Keys.NextPane, Binding{"shift+tab"}) ||
		!reflect.DeepEqual(c.Keys.Quit, Binding{"Q"}) {
		t.Fatalf("keys %+v", c.Keys)
	}
	if c.Keys.Edit.Label() != "e" {
		t.Fatal("untouched key changed")
	}
}

func TestBadEntriesFallBack(t *testing.T) {
	in := `
theme: solarized
max_panes: 99
kubectl_output: table
colour: blue
logs: {tail: lots, depth: 3}
ui: nope
prod: {a: b}
aliases: [x]
keys:
  delete: hyper+d
  edit: [e, {x: 1}]
  fly: f
  context: ""
  namespace: m
`
	c, w := Parse([]byte(in))
	d := Default()
	if c.Theme != d.Theme || c.MaxPanes != d.MaxPanes || c.KubectlOutput != d.KubectlOutput ||
		c.Logs != d.Logs || c.UI != d.UI || len(c.Prod) != 0 || len(c.Aliases) != 0 {
		t.Fatalf("values not defaulted: %+v", c)
	}
	if !reflect.DeepEqual(c.Keys.Delete, d.Keys.Delete) || !reflect.DeepEqual(c.Keys.Edit, d.Keys.Edit) || !reflect.DeepEqual(c.Keys.Context, d.Keys.Context) {
		t.Fatalf("bad keys not defaulted: %+v", c.Keys)
	}
	if c.Keys.Namespace.Label() != "m" {
		t.Fatal("good key next to bad ones was not applied")
	}
	joined := strings.Join(w, "\n")
	for _, frag := range []string{
		`theme: unknown theme "solarized"`, "max_panes: 99 is out of range", "kubectl_output:",
		`unknown setting "colour"`, "logs.tail:", `logs: unknown setting "depth"`, "ui: expected a mapping",
		"prod: expected a list", "aliases: expected a mapping", `keys.delete: unknown key "hyper+d" (using ctrl+d)`,
		"keys.edit:", `keys: unknown action "fly"`, "keys.context:",
	} {
		if !strings.Contains(joined, frag) {
			t.Errorf("missing warning %q in:\n%s", frag, joined)
		}
	}
	for _, line := range w {
		if !strings.HasPrefix(line, "config: ") {
			t.Errorf("warning without prefix: %q", line)
		}
	}
}

func TestSyntaxErrorKeepsDefaults(t *testing.T) {
	c, w := Parse([]byte("theme: [nord\nkeys: :"))
	if len(w) != 1 || !strings.HasPrefix(w[0], "config: ") {
		t.Fatalf("warnings %v", w)
	}
	if !reflect.DeepEqual(c, Default()) {
		t.Fatal("not defaults")
	}
	_, w = Parse([]byte("- just\n- a list\n"))
	if len(w) != 1 || !strings.Contains(w[0], "top level") {
		t.Fatalf("warnings %v", w)
	}
}

func TestParseKey(t *testing.T) {
	good := map[string]string{
		"c": "c", "C": "C", ":": ":", "/": "/", "ü": "ü",
		"ctrl+d": "ctrl+d", "Ctrl+D": "ctrl+d", "CTRL+n": "ctrl+n",
		"ctrl+up": "ctrl+up", "ctrl+Right": "ctrl+right", "ctrl+pgdown": "ctrl+pgdown",
		"ctrl+i": "tab", "ctrl+m": "enter", "ctrl+[": "esc",
		"shift+o": "O", "Shift+O": "O", "shift+tab": "shift+tab",
		"alt+x": "alt+x", "alt+X": "alt+X", "alt+enter": "alt+enter",
		"enter": "enter", "Return": "enter", "escape": "esc", "esc": "esc",
		"tab": "tab", "space": " ", " ": " ", "pageup": "pgup", "pgdown": "pgdown",
		"up": "up", "f1": "f1", "F12": "f12", "backspace": "backspace",
	}
	for in, want := range good {
		got, err := ParseKey(in)
		if err != nil || got != want {
			t.Errorf("ParseKey(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "ctrl+", "ctrl+1", "hyper+a", "f0", "f21", "shift+pgup", "blah", "ctrl+ü"} {
		if got, err := ParseKey(in); err == nil {
			t.Errorf("ParseKey(%q) = %q, want error", in, got)
		}
	}
}

func TestBindingHelpers(t *testing.T) {
	b := Binding{"ctrl+d", "x"}
	if !b.Has("x") || b.Has("ctrl+x") || b.Label() != "ctrl+d" || b.Short() != "^d" {
		t.Fatal("binding helpers")
	}
	if (Binding{}).Label() != "" {
		t.Fatal("empty label")
	}
}

func TestHomePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cases := map[string]string{
		home:                         "~",
		home + "/.kube/config":       "~/.kube/config",
		home + "x/file":              home + "x/file",
		"/etc/kubernetes/admin.conf": "/etc/kubernetes/admin.conf",
		"":                           "",
	}
	for in, want := range cases {
		if got := HomePath(in); got != want {
			t.Errorf("HomePath(%q) = %q, want %q", in, got, want)
		}
	}
	if ExpandHome("~/a") != filepath.Join(home, "a") || ExpandHome("/x") != "/x" {
		t.Fatal("ExpandHome")
	}
}

func TestLoadPrefersConfigYAML(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".k2")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "config"), []byte("theme: gruvbox\n"), 0o644)
	if c, _ := Load(); c.Theme != "gruvbox" {
		t.Fatalf("legacy file not read: %s", c.Theme)
	}
	_ = os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("theme: dracula\n"), 0o644)
	if c, _ := Load(); c.Theme != "dracula" {
		t.Fatalf("config.yaml not preferred: %s", c.Theme)
	}
}

func TestUISwitches(t *testing.T) {
	c, w := Parse([]byte("ui:\n  animations: false\n  toasts: off\n  toast_seconds: 5\n"))
	if len(w) != 0 || c.UI.Animations || c.UI.Toasts || c.UI.ToastSeconds != 5 {
		t.Fatalf("ui %+v warnings %v", c.UI, w)
	}
	c, w = Parse([]byte("ui:\n  animations: maybe\n  toasts: [x]\n  toast_seconds: 99\n  sparkles: true\n  sidebar_width: 30\n"))
	if !c.UI.Animations || !c.UI.Toasts || c.UI.ToastSeconds != 3 || c.UI.SidebarWidth != 30 {
		t.Fatalf("bad values must keep defaults: %+v", c.UI)
	}
	want := []string{
		`config: ui.animations: "maybe" is not true or false (using true)`,
		`config: ui.toasts: "" is not true or false (using true)`,
		`config: ui.toast_seconds: 99 is out of range 1..30 (using 3)`,
		`config: ui: unknown setting "sparkles" (have active_border, animations, background, clipboard, cluster_info, context_themes, copy_mode_hint, dim_inactive, editor_inline, forward_dock_rows, logo_title_color, metrics_bars, metrics_crit, metrics_every, metrics_warn, pod_metrics, refresh_every, sidebar_narrow_width, sidebar_width, tabbar, toast_seconds, toasts, token_bar, token_crit, token_warn, wrap)`,
	}
	if strings.Join(w, "\n") != strings.Join(want, "\n") {
		t.Fatalf("warnings:\n%s", strings.Join(w, "\n"))
	}
}

func TestDefaultKeysHaveNoConflicts(t *testing.T) {
	k := DefaultKeys()
	for _, f := range keyFields {
		direct, after := uses(&k, f.name)
		for _, key := range direct {
			if other := conflict(&k, f.name, key, false); other != "" {
				t.Errorf("default keys.%s %q clashes with %s", f.name, key, other)
			}
		}
		for _, key := range after {
			if other := conflict(&k, f.name, key, true); other != "" {
				t.Errorf("default keys.%s prefix+%q clashes with %s", f.name, key, other)
			}
		}
	}
}

func TestKeyConflictsKeepDefaults(t *testing.T) {
	in := `
keys:
  new_pane: j
  split_right: t
  next_tab: x
  new_tab: T
  prev_tab: "{"
  tab_3: n
  edit: direct x
  close_pane: [x, direct ctrl+d]
`
	c, w := Parse([]byte(in))
	k := c.Keys

	if !reflect.DeepEqual(k.SplitDown, Binding{"j"}) || !reflect.DeepEqual(k.NewTab, Binding{"T"}) || !reflect.DeepEqual(k.PrevTab, Binding{"{"}) {
		t.Fatalf("good remaps lost: %+v", k)
	}

	if !reflect.DeepEqual(k.SplitRight, Binding{"t"}) || !reflect.DeepEqual(k.NextTab, Binding{"direct ]", "n", "]"}) ||
		!reflect.DeepEqual(k.Tab[2], Binding{"3"}) || !reflect.DeepEqual(k.Edit, Binding{"e"}) || !reflect.DeepEqual(k.ClosePane, Binding{"x"}) {
		t.Fatalf("colliding remaps not reset: split_right %v next_tab %v tab_3 %v edit %v close %v", k.SplitRight, k.NextTab, k.Tab[2], k.Edit, k.ClosePane)
	}
	want := []string{
		`config: keys.edit: "direct x": only pane and tab keys take "direct" (using e)`,
		"config: keys.next_tab: ctrl+a x is already used by close_pane (using direct ], n, ])",
		"config: keys.tab_3: ctrl+a n is already used by next_tab (using 3)",
		"config: keys.close_pane: ctrl+d is already used by delete (using x)",
	}
	if !reflect.DeepEqual(w, want) {
		t.Fatalf("warnings:\n%s", strings.Join(w, "\n"))
	}
}

func TestTabbarSetting(t *testing.T) {
	for in, want := range map[string]string{"always": "always", "Never": "never", "auto": "auto"} {
		c, w := Parse([]byte("ui: {tabbar: " + in + "}"))
		if c.UI.Tabbar != want || len(w) != 0 {
			t.Errorf("%s: %q %v", in, c.UI.Tabbar, w)
		}
	}
	c, w := Parse([]byte("ui: {tabbar: sometimes}"))
	if c.UI.Tabbar != "auto" || len(w) != 1 || !strings.Contains(w[0], `"sometimes" is not one of auto, always, never`) {
		t.Fatalf("%q %v", c.UI.Tabbar, w)
	}
}

func TestBackgroundSetting(t *testing.T) {
	c, w := Parse(nil)
	if c.UI.Background != "transparent" || c.UI.BackgroundSet || len(w) != 0 {
		t.Fatalf("default %+v %v", c.UI, w)
	}

	if c.UI.PaintBackground(false) || !c.UI.PaintBackground(true) {
		t.Fatal("default painting rule")
	}
	c, w = Parse([]byte("ui: {background: Theme}"))
	if c.UI.Background != "theme" || !c.UI.BackgroundSet || len(w) != 0 || !c.UI.PaintBackground(false) || !c.UI.PaintBackground(true) {
		t.Fatalf("theme %+v %v", c.UI, w)
	}

	c, w = Parse([]byte("ui: {background: transparent}"))
	if c.UI.Background != "transparent" || !c.UI.BackgroundSet || len(w) != 0 || c.UI.PaintBackground(true) {
		t.Fatalf("transparent %+v %v", c.UI, w)
	}

	c, w = Parse([]byte("ui: {background: black}"))
	if c.UI.Background != "transparent" || c.UI.BackgroundSet || len(w) != 1 ||
		!strings.Contains(w[0], `ui.background: "black" is not one of transparent, theme (using transparent)`) {
		t.Fatalf("bad %+v %v", c.UI, w)
	}
}

func TestScreenKeyConflicts(t *testing.T) {
	cases := []struct {
		yaml, warn string
		check      func(Keys) bool
	}{
		{"keys: {describe: e}", "keys.describe: e is already used by edit (using d)", func(k Keys) bool { return k.Describe.Has("d") }},
		{"keys: {port_forward: ctrl+d}", "keys.port_forward: ctrl+d is already used by delete (using ctrl+f, F)", func(k Keys) bool { return k.PortForward.Has("ctrl+f") && k.PortForward.Has("F") }},
		{"keys: {page_down: ctrl+f}", "keys.page_down: ctrl+f is already used by port_forward (using pgdown, })", func(k Keys) bool { return k.PageDown.Has("pgdown") && !k.PageDown.Has("ctrl+f") }},
		{"keys: {kustomize_delete: a}", "keys.kustomize_delete: a is already used by apply (using d)", func(k Keys) bool { return k.KustomizeDelete.Has("d") }},
		{"keys: {describe: f}", "keys.describe: f is already used by forwards (using d)", func(k Keys) bool { return k.Describe.Has("d") }},

		{"keys: {shell: d}", "keys.shell: d is already used by describe (using s)", func(k Keys) bool { return k.Shell.Has("s") }},
		{"keys: {previous: v}", "keys.previous: v is already used by vim (using p)", func(k Keys) bool { return k.Previous.Has("p") }},
		{"keys: {copy_name: e}", "keys.copy_name: e is already used by edit (using w)", func(k Keys) bool { return k.CopyName.Has("w") }},
		{"keys: {sort_next: c}", "keys.sort_next: c is already used by context (using >)", func(k Keys) bool { return k.SortNext.Has(">") }},
		{"keys: {kustomize_path: s}", "keys.kustomize_path: s is already used by scan (using p)", func(k Keys) bool { return k.KustomizePath.Has("p") }},
		{"keys: {approve: ctrl+a}", "keys.approve: ctrl+a is already used by prefix (using a)", func(k Keys) bool { return k.Approve.Has("a") }},

		{"keys: {approve: O, next_pane: [o, direct O]}", "keys.approve: O is already used by next_pane (using a)", func(k Keys) bool { return k.Approve.Has("a") && k.NextPane.Direct("O") }},
		{"keys: {wrap: v}", "keys.wrap: v is already used by vim (using w)", func(k Keys) bool { return k.Wrap.Has("w") }},
	}
	for _, c := range cases {
		cfg, w := Parse([]byte(c.yaml))
		if len(w) != 1 || !strings.Contains(w[0], c.warn) || !c.check(cfg.Keys) {
			t.Errorf("%s: %v %+v", c.yaml, w, cfg.Keys)
		}
	}

	for _, y := range []string{"keys: {describe: v}", "keys: {kustomize_delete: x}", "keys: {port_forward: D}", "keys: {forward: N}",
		"keys: {previous: x}", "keys: {scan: S}"} {
		if _, w := Parse([]byte(y)); len(w) != 0 {
			t.Errorf("%s: %v", y, w)
		}
	}
	if !sameScreen("describe", "page_down") || sameScreen("describe", "kustomize_delete") || sameScreen("apply", "delete_all") {
		t.Fatal("screen groups")
	}
}

func TestConfigExampleIsDefaults(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "config.example.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	setting := regexp.MustCompile(`^\s*[a-z_0-9]+:(\s|$)`)
	var lines []string
	uncommented := 0
	for _, l := range strings.Split(string(data), "\n") {
		if rest, ok := strings.CutPrefix(l, "# "); ok && setting.MatchString(rest) {
			l = rest
			uncommented++
		}
		lines = append(lines, l)
	}
	if uncommented < 60 {
		t.Fatalf("only %d settings found to uncomment", uncommented)
	}
	c, w := Parse([]byte(strings.Join(lines, "\n")))
	if len(w) != 0 {
		t.Fatalf("warnings:\n%s", strings.Join(w, "\n"))
	}
	if !reflect.DeepEqual(c.Prod, []string{"prod-eu-1", "prod", "kind-prod"}) || c.Aliases["deps"] != "deployments" {
		t.Fatalf("prod %v aliases %v", c.Prod, c.Aliases)
	}
	def := Default()
	c.Prod, c.Aliases = def.Prod, def.Aliases
	c.UI.BackgroundSet = false
	for _, f := range keyFields {
		if a, b := *f.ptr(&c.Keys), *f.ptr(&def.Keys); !reflect.DeepEqual(a, b) {
			t.Errorf("keys.%s: example %v, default %v", f.name, a, b)
		}
	}
	if !reflect.DeepEqual(c, def) {
		t.Fatalf("example differs from defaults:\n%+v\n%+v", c, def)
	}

	for _, name := range append(KeyNames(), "editor_inline", "background", "tabbar", "toast_seconds", "active_border", "dim_inactive", "cluster_info", "pod_metrics", "metrics_every", "clipboard", "wrap") {
		if !regexp.MustCompile(`(?m)^#\s+` + name + `:`).Match(data) {
			t.Errorf("config.example.yaml lacks %s", name)
		}
	}
}

func TestPrefixConflicts(t *testing.T) {
	def := Binding{"ctrl+a"}
	if !reflect.DeepEqual(DefaultKeys().Prefix, def) {
		t.Fatalf("default prefix %v", DefaultKeys().Prefix)
	}
	for in, want := range map[string]string{
		"keys:\n  prefix: F\n":        "keys.prefix: F cannot be the prefix: F is a printable character (using ctrl+a)",
		"keys:\n  editor_prefix: F\n": "keys.prefix: F cannot be the prefix: F is a printable character",
		"keys:\n  prefix: enter\n":    "enter is needed by the screens",
		"keys:\n  prefix: ctrl+d\n":   "keys.prefix: ctrl+d is already used by delete (using ctrl+a)",
		"keys:\n  prefix: ctrl+c\n":   "already used by quit",
	} {
		c, w := Parse([]byte(in))
		if !reflect.DeepEqual(c.Keys.Prefix, def) || len(w) != 1 || !strings.Contains(w[0], want) {
			t.Errorf("%q: prefix %v warnings %v", in, c.Keys.Prefix, w)
		}
	}

	c, w := Parse([]byte("keys:\n  scroll_down: ctrl+a\n"))
	if !reflect.DeepEqual(c.Keys.ScrollDown, Binding{"j", "down"}) || len(w) != 1 || !strings.Contains(w[0], "already used by prefix") {
		t.Fatalf("scroll_down %v warnings %v", c.Keys.ScrollDown, w)
	}

	c, w = Parse([]byte("keys:\n  next_pane: ctrl+a\n"))
	if !reflect.DeepEqual(c.Keys.NextPane, Binding{"o"}) || len(w) != 1 || !strings.Contains(w[0], "keys.next_pane: ctrl+a ctrl+a is already used by prefix") {
		t.Fatalf("next_pane %v warnings %v", c.Keys.NextPane, w)
	}
	c, w = Parse([]byte("keys:\n  next_pane: v\n"))
	if !reflect.DeepEqual(c.Keys.NextPane, Binding{"o"}) || len(w) != 1 || !strings.Contains(w[0], "ctrl+a v is already used by split_right") {
		t.Fatalf("next_pane %v warnings %v", c.Keys.NextPane, w)
	}

	for in, want := range map[string]string{"Ctrl+\\": "ctrl+\\", "ctrl+k": "ctrl+k", "ctrl+space": "ctrl+@", "alt+a": "alt+a", "f12": "f12"} {
		for _, name := range []string{"prefix", "editor_prefix"} {
			c, w := Parse([]byte("keys:\n  " + name + ": \"" + strings.ReplaceAll(in, "\\", "\\\\") + "\"\n"))
			if !reflect.DeepEqual(c.Keys.Prefix, Binding{want}) || len(w) != 0 {
				t.Errorf("%s: %s: prefix %v warnings %v", name, in, c.Keys.Prefix, w)
			}
		}
	}

	k := DefaultKeys()
	for _, f := range keyFields {
		if f.name != "prefix" && f.ptr(&k).Has("ctrl+a") {
			t.Errorf("keys.%s defaults to the prefix ctrl+a", f.name)
		}
	}
}

func TestDirectPaneKeys(t *testing.T) {
	c, w := Parse([]byte("keys:\n  next_pane: [o, \"direct O\"]\n  split_right: [v, Direct ctrl+v]\n  tab_1: [\"1\", direct alt+1]\n"))
	if len(w) != 0 {
		t.Fatalf("warnings %v", w)
	}
	k := c.Keys
	if !k.NextPane.After("o") || k.NextPane.After("O") || !k.NextPane.Direct("O") || k.NextPane.Direct("o") {
		t.Fatalf("next_pane %v", k.NextPane)
	}
	if k.DirectAction("ctrl+v") != "split_right" || k.DirectAction("alt+1") != "tab_1" || k.PrefixedAction("v") != "split_right" || k.PrefixedAction("x") != "close_pane" {
		t.Fatalf("actions %v %v", k.SplitRight, k.Tab[0])
	}
	if got := k.NextPane.PaneLabel(k.Prefix); got != "ctrl+a o" {
		t.Fatalf("label %q", got)
	}

	c, w = Parse([]byte("keys:\n  next_pane: [o, direct d]\n"))
	if !reflect.DeepEqual(c.Keys.NextPane, Binding{"o"}) || len(w) != 1 || !strings.Contains(w[0], "keys.next_pane: d is already used by") {
		t.Fatalf("direct d: %v %v", c.Keys.NextPane, w)
	}

	c, w = Parse([]byte("keys:\n  describe: direct D\n"))
	if !reflect.DeepEqual(c.Keys.Describe, Binding{"d"}) || len(w) != 1 || !strings.Contains(w[0], "only pane and tab keys") {
		t.Fatalf("describe: %v %v", c.Keys.Describe, w)
	}
}

func TestPodMetricsSettings(t *testing.T) {
	d := Default().UI
	if !d.PodMetrics || d.MetricsEvery != 15 {
		t.Fatalf("defaults %v %d", d.PodMetrics, d.MetricsEvery)
	}
	c, w := Parse([]byte("ui: {pod_metrics: false, metrics_every: 30}"))
	if len(w) != 0 || c.UI.PodMetrics || c.UI.MetricsEvery != 30 {
		t.Fatalf("%+v %v", c.UI, w)
	}
	c, w = Parse([]byte("ui: {metrics_every: 1}"))
	if len(w) != 1 || c.UI.MetricsEvery != 15 {
		t.Fatalf("out of range: %d %v", c.UI.MetricsEvery, w)
	}
}

func TestClusterInfoSwitch(t *testing.T) {
	if !Default().UI.ClusterInfo {
		t.Fatal("cluster_info should default to true")
	}
	c, w := Parse([]byte("ui: {cluster_info: false}"))
	if len(w) != 0 || c.UI.ClusterInfo {
		t.Fatalf("cluster_info %v warnings %v", c.UI.ClusterInfo, w)
	}
}

func TestActiveBorderAndDimInactive(t *testing.T) {
	d := Default()
	if d.UI.ActiveBorder != "thick" || d.UI.DimInactive {
		t.Fatalf("defaults %q %v", d.UI.ActiveBorder, d.UI.DimInactive)
	}
	for _, in := range []string{"normal", "thick", "DOUBLE"} {
		c, w := Parse([]byte("ui: {active_border: " + in + ", dim_inactive: true}"))
		if len(w) != 0 || c.UI.ActiveBorder != strings.ToLower(in) || !c.UI.DimInactive {
			t.Errorf("%s: %+v %v", in, c.UI, w)
		}
	}
	c, w := Parse([]byte("ui: {active_border: dotted}"))
	if c.UI.ActiveBorder != "thick" || len(w) != 1 || !strings.Contains(w[0], `ui.active_border: "dotted" is not one of normal, thick, double (using thick)`) {
		t.Fatalf("bad value: %q %v", c.UI.ActiveBorder, w)
	}
}

func TestClipboardSetting(t *testing.T) {
	if d := Default(); d.UI.Clipboard != "auto" {
		t.Fatalf("default %q", d.UI.Clipboard)
	}
	for _, in := range []string{"auto", "wl-copy", "XClip", "xsel", "OSC52"} {
		c, w := Parse([]byte("ui: {clipboard: " + in + "}"))
		if len(w) != 0 || c.UI.Clipboard != strings.ToLower(in) {
			t.Fatalf("%s: %q %v", in, c.UI.Clipboard, w)
		}
	}
	c, w := Parse([]byte("ui: {clipboard: pbcopy}"))
	if c.UI.Clipboard != "auto" || len(w) != 1 || !strings.Contains(w[0], `ui.clipboard: "pbcopy" is not one of auto, wl-copy, xclip, xsel, osc52 (using auto)`) {
		t.Fatalf("bad value: %q %v", c.UI.Clipboard, w)
	}
}

func TestOldPaneKeysHonored(t *testing.T) {
	in := `
keys:
  split_right: [v, direct ctrl+v]
  new_pane: [h, direct ctrl+n]
  next_pane: [o, direct O]
  close_pane: [x, direct ctrl+x]
  new_tab: [t, c]
  close_tab: [w, direct ctrl+w]
  next_tab: [n, "]", "direct ]"]
  prev_tab: [p, "[", "direct ["]
  tab_1: ["1", direct alt+1]
  tab_9: ["9", direct alt+9]
`
	c, w := Parse([]byte(in))
	if len(w) != 0 {
		t.Fatalf("warnings: %v", w)
	}
	k := c.Keys
	for name, old := range OldPaneKeys {
		if old == nil || (strings.HasPrefix(name, "tab_") && name != "tab_1" && name != "tab_9") {
			continue
		}
		if name == "new_tab" {
			b := *fieldByName(name).ptr(&k)
			if b.Direct("ctrl+t") || !b.After("t") {
				t.Errorf("keys.new_tab = %v, ctrl+t is keys.theme", b)
			}
			continue
		}
		b := *fieldByName(name).ptr(&k)
		if !b.Direct(old[0]) || !b.After(DefaultKeys().PrefixedFor(name)) {
			t.Errorf("keys.%s = %v, want the old %v direct and the default after the prefix", name, b, old)
		}
	}

	if len(k.PrevPane) != 0 || !k.LastPane.After(";") || len(k.LastPane) != 1 {
		t.Fatalf("prev/last %v %v", k.PrevPane, k.LastPane)
	}

	for in, want := range map[string]string{"alt+v": "alt+v", "Alt+H": "alt+H", "alt+]": "alt+]", "alt+[": "alt+[", "alt+o": "alt+o"} {
		if got, err := ParseKey(in); err != nil || got != want {
			t.Errorf("ParseKey(%q) = %q %v, want %q", in, got, err, want)
		}
	}

	c, w = Parse([]byte("keys:\n  new_tab: [t, direct ctrl+t]\n"))
	if !c.Keys.Theme.Has("ctrl+t") || !reflect.DeepEqual(c.Keys.NewTab, DefaultKeys().NewTab) || len(w) != 1 || !strings.Contains(w[0], "theme") {
		t.Fatalf("new_tab stole ctrl+t: %v warnings %v", c.Keys.NewTab, w)
	}
}

func TestMetricsSettings(t *testing.T) {
	c, w := Parse(nil)
	if !c.UI.MetricsBars || c.UI.MetricsWarn != 70 || c.UI.MetricsCrit != 90 || c.UI.MetricsEvery != 15 || len(w) != 0 {
		t.Fatalf("defaults %+v %v", c.UI, w)
	}
	c, w = Parse([]byte("ui: {metrics_warn: 50, metrics_crit: 80, metrics_bars: false, metrics_every: 5}"))
	if c.UI.MetricsBars || c.UI.MetricsWarn != 50 || c.UI.MetricsCrit != 80 || c.UI.MetricsEvery != 5 || len(w) != 0 {
		t.Fatalf("set %+v %v", c.UI, w)
	}
	cases := map[string]string{
		"ui: {metrics_warn: 0}":                    "ui.metrics_warn: 0 is out of range 1..99 (using 70)",
		"ui: {metrics_crit: 100}":                  "ui.metrics_crit: 100 is out of range 1..99 (using 90)",
		"ui: {metrics_warn: 95}":                   "ui.metrics_warn (95) must be below ui.metrics_crit (90) (using 70 and 90)",
		"ui: {metrics_warn: 60, metrics_crit: 60}": "ui.metrics_warn (60) must be below ui.metrics_crit (60) (using 70 and 90)",
		"ui: {metrics_every: 2}":                   "ui.metrics_every: 2 is out of range 5..600 (using 15)",
		"ui: {metrics_every: 601}":                 "ui.metrics_every: 601 is out of range 5..600 (using 15)",
	}
	for in, want := range cases {
		c, w := Parse([]byte(in))
		if len(w) != 1 || !strings.Contains(w[0], want) || c.UI.MetricsWarn != 70 || c.UI.MetricsCrit != 90 || c.UI.MetricsEvery != 15 {
			t.Errorf("%s: %+v %v", in, c.UI, w)
		}
	}
}

func TestCheatsheetMatchesKeymap(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "CHEATSHEET.md"))
	if err != nil {
		t.Fatal(err)
	}
	var rows []string
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "|") {
			rows = append(rows, line)
		}
	}
	k := DefaultKeys()
	prefix := k.Prefix.Label()
	for _, f := range keyFields {
		ref := "`keys." + f.name + "`"
		var want []string
		for _, key := range *f.ptr(&k) {
			if PrefixedKeys[f.name] {
				if d, ok := strings.CutPrefix(key, DirectMark); ok {
					want = append(want, "`"+d+"`")
				} else {
					want = append(want, "`"+prefix+" "+key+"`")
				}
				continue
			}
			want = append(want, "`"+key+"`")
		}
		found := false
		for _, r := range rows {
			if !strings.Contains(r, ref) {
				continue
			}
			ok := true
			for _, w := range want {
				ok = ok && strings.Contains(r, w)
			}
			if ok {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("CHEATSHEET.md: no row with %s and its defaults %v", ref, want)
		}
	}
	names := map[string]bool{}
	for _, f := range keyFields {
		names[f.name] = true
	}
	for old := range keyAliases {
		names[old] = true
	}
	for _, m := range regexp.MustCompile("`keys\\.([a-z_0-9]+)`").FindAllStringSubmatch(string(raw), -1) {
		if !names[m[1]] {
			t.Errorf("CHEATSHEET.md names keys.%s, which does not exist", m[1])
		}
	}

	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil || !strings.Contains(string(readme), "(CHEATSHEET.md)") {
		t.Error("README.md does not link CHEATSHEET.md")
	}
}

func TestPaneMoveKeys(t *testing.T) {
	d := DefaultKeys()
	if d.PrefixedAction("left") != "pane_left" || d.PrefixedAction("down") != "pane_down" || d.DirectAction("ctrl+up") != "pane_up" {
		t.Fatalf("defaults: %v %v", d.PaneLeft, d.PaneDown)
	}
	c, w := Parse([]byte("keys:\n  pane_left: ctrl+left\n  pane_right: [alt+l, l]\n  pane_up: [\"direct ctrl+up\", u]\n"))
	if len(w) != 0 {
		t.Fatalf("warnings: %v", w)
	}
	k := c.Keys
	if !k.PaneLeft.Direct("ctrl+left") || k.PaneLeft.After("left") {
		t.Fatalf("legacy pane_left = %v", k.PaneLeft)
	}
	if !k.PaneRight.Direct("alt+l") || !k.PaneRight.After("l") || k.PrefixedAction("l") != "pane_right" {
		t.Fatalf("pane_right = %v", k.PaneRight)
	}
	if !k.PaneUp.Direct("ctrl+up") || !k.PaneUp.After("u") {
		t.Fatalf("pane_up = %v", k.PaneUp)
	}

	c, w = Parse([]byte("keys:\n  pane_left: [h]\n"))
	if len(w) != 1 || !strings.Contains(w[0], "keys.pane_left: ctrl+a h is already used by split_down") || !c.Keys.PaneLeft.After("left") {
		t.Fatalf("clash: %v %v", w, c.Keys.PaneLeft)
	}
}

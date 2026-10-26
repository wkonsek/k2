package config

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type Binding []string

func (b Binding) Has(key string) bool {
	for _, k := range b {
		if k == key {
			return true
		}
	}
	return false
}

func (b Binding) Label() string {
	if len(b) == 0 {
		return ""
	}
	return b[0]
}

func (b Binding) Short() string { return short(b.Label()) }

func short(k string) string { return strings.ReplaceAll(k, "ctrl+", "^") }

const DirectMark = "direct "

func (b Binding) After(key string) bool {
	for _, k := range b {
		if k == key && !strings.HasPrefix(k, DirectMark) {
			return true
		}
	}
	return false
}

func (b Binding) Direct(key string) bool {
	for _, k := range b {
		if strings.HasPrefix(k, DirectMark) && k[len(DirectMark):] == key {
			return true
		}
	}
	return false
}

func (b Binding) PaneLabel(prefix Binding) string {
	if len(b) == 0 {
		return ""
	}
	if k, ok := strings.CutPrefix(b[0], DirectMark); ok {
		return k
	}
	return prefix.Label() + " " + b[0]
}

func (b Binding) PaneShort(prefix Binding) string { return short(b.PaneLabel(prefix)) }

type Keys struct {
	Kubeconfig, Context, Namespace, Kustomize, Kubectl, Forwards Binding
	PortForward                                                  Binding
	Describe                                                     Binding
	Edit                                                         Binding
	Decode                                                       Binding
	Delete                                                       Binding
	KustomizeDelete                                              Binding
	Apply                                                        Binding
	Diff                                                         Binding
	DeleteAll                                                    Binding
	PfRestart                                                    Binding
	PfAuto                                                       Binding
	Scan                                                         Binding
	Vim                                                          Binding
	Wrap                                                         Binding
	CopyMode                                                     Binding
	LogMark                                                      Binding
	CopySelect, CopyYank, CopyAll                                Binding
	Shell                                                        Binding
	Previous                                                     Binding
	Scale                                                        Binding
	SetImage                                                     Binding
	Restart                                                      Binding
	Trigger                                                      Binding
	Label, Annotate                                              Binding
	CopyName, CopyRef                                            Binding
	SortNext, SortPrev, SortReverse                              Binding
	Approve                                                      Binding
	KustomizePath                                                Binding
	Command                                                      Binding
	SplitDown                                                    Binding
	SplitRight                                                   Binding
	ClosePane                                                    Binding
	PaneLeft, PaneRight, PaneUp, PaneDown                        Binding
	PrevPane                                                     Binding
	NextPane                                                     Binding
	LastPane                                                     Binding
	NewTab                                                       Binding
	CloseTab                                                     Binding
	RenameTab                                                    Binding
	NextTab, PrevTab                                             Binding
	Tab                                                          [9]Binding
	Fullscreen                                                   Binding
	Zoom                                                         Binding
	LastError                                                    Binding
	Quit                                                         Binding
	Prefix                                                       Binding
	Back                                                         Binding
	Left                                                         Binding
	Select                                                       Binding
	Submit                                                       Binding
	Cancel                                                       Binding
	ShellBack                                                    Binding
	ScrollDown                                                   Binding
	ScrollUp                                                     Binding
	PageDown                                                     Binding
	PageUp                                                       Binding
	Top                                                          Binding
	Bottom                                                       Binding
	Output                                                       Binding
	JQ                                                           Binding
	Complete                                                     Binding
	Yes                                                          Binding
	No                                                           Binding

	Xray          Binding
	MultiLogs     Binding
	Favorites     Binding
	Theme         Binding
	XrayToggle    Binding
	Search        Binding
	HelmMark      Binding
	HelmDiff      Binding
	XrayExpand    Binding
	XrayCollapse  Binding
	XrayFilter    Binding
	Yaml          Binding
	HelmValues    Binding
	HelmComputed  Binding
	HelmManifest  Binding
	HelmNotes     Binding
	HelmHistory   Binding
	HelmRollback  Binding
	AllNamespaces Binding
	LogFilter     Binding
	FavAdd        Binding
	FavRemove     Binding
	FavUp         Binding
	FavDown       Binding
}

func DefaultKeys() Keys {
	return Keys{
		Kubeconfig: Binding{"C"}, Context: Binding{"c"}, Namespace: Binding{"n"},
		Kustomize: Binding{"K"}, Kubectl: Binding{"U"}, Forwards: Binding{"f"},
		PortForward: Binding{"ctrl+f"}, Describe: Binding{"d"}, Edit: Binding{"e"}, Decode: Binding{"x"},
		Delete: Binding{"ctrl+d"}, KustomizeDelete: Binding{"d"}, Apply: Binding{"a"}, Diff: Binding{"D"},
		DeleteAll: Binding{"D"}, PfRestart: Binding{"r"}, PfAuto: Binding{"a"}, Scan: Binding{"s"}, Vim: Binding{"v"}, Wrap: Binding{"w"},
		CopyMode: Binding{"y"}, LogMark: Binding{"m"}, CopySelect: Binding{"v"}, CopyYank: Binding{"y"}, CopyAll: Binding{"Y"},
		Shell: Binding{"s"}, Previous: Binding{"p"}, Scale: Binding{"S"}, SetImage: Binding{"i"}, Restart: Binding{"r"}, Trigger: Binding{"t"},
		Label: Binding{"L"}, Annotate: Binding{"A"}, CopyName: Binding{"w"}, CopyRef: Binding{"W"},
		SortNext: Binding{">"}, SortPrev: Binding{"<"}, SortReverse: Binding{"="}, Approve: Binding{"a"}, KustomizePath: Binding{"p"},

		Command: Binding{":"}, SplitDown: Binding{"h", `"`}, SplitRight: Binding{"v", "%"}, ClosePane: Binding{"x"},

		PaneLeft: Binding{DirectMark + "ctrl+left", "left"}, PaneRight: Binding{DirectMark + "ctrl+right", "right"},
		PaneUp: Binding{DirectMark + "ctrl+up", "up"}, PaneDown: Binding{DirectMark + "ctrl+down", "down"},
		PrevPane: nil, NextPane: Binding{"o"}, LastPane: Binding{";"},
		NewTab: Binding{"t", "c"}, CloseTab: Binding{"w", "&"}, RenameTab: Binding{","},
		NextTab: Binding{DirectMark + "]", "n", "]"}, PrevTab: Binding{DirectMark + "[", "p", "["},
		Tab:        [9]Binding{{"1"}, {"2"}, {"3"}, {"4"}, {"5"}, {"6"}, {"7"}, {"8"}, {"9"}},
		Fullscreen: Binding{"F"}, Zoom: Binding{"z"}, LastError: Binding{"!"}, Quit: Binding{"Q", "ctrl+c"}, Prefix: Binding{"ctrl+a"},
		Back: Binding{"q", "esc", "H"}, Left: Binding{"left", "h"},
		Select: Binding{"enter", "right", "l"}, Submit: Binding{"enter"},
		Cancel: Binding{"esc"}, ShellBack: Binding{"esc", "q"},
		ScrollDown: Binding{"j", "down"}, ScrollUp: Binding{"k", "up"},
		PageDown: Binding{"pgdown", "}"}, PageUp: Binding{"pgup", "{"},
		Top: Binding{"g"}, Bottom: Binding{"G"},
		Output: Binding{"ctrl+o"}, JQ: Binding{"ctrl+j"}, Complete: Binding{"tab"},
		Yes: Binding{"y", "Y"}, No: Binding{"n", "N", "esc", "q"},

		Xray: Binding{"X"}, MultiLogs: Binding{"M"}, Favorites: Binding{"b"}, Theme: Binding{"ctrl+t"},
		XrayToggle: Binding{" "}, Yaml: Binding{"y"},
		HelmMark: Binding{" "}, HelmDiff: Binding{"D"}, Search: Binding{"/"},
		XrayExpand: Binding{"+"}, XrayCollapse: Binding{"-"}, XrayFilter: Binding{"/"},
		HelmValues: Binding{"v"}, HelmComputed: Binding{"V"}, HelmManifest: Binding{"M"}, HelmNotes: Binding{"N"},
		HelmHistory: Binding{"t"}, HelmRollback: Binding{"r"}, AllNamespaces: Binding{"0"},
		LogFilter: Binding{"/"},
		FavAdd:    Binding{"a"}, FavRemove: Binding{"x"}, FavUp: Binding{"K"}, FavDown: Binding{"J"},
	}
}

type keyField struct {
	name string
	ptr  func(*Keys) *Binding
}

var keyFields = []keyField{
	{"kubeconfig", func(k *Keys) *Binding { return &k.Kubeconfig }},
	{"context", func(k *Keys) *Binding { return &k.Context }},
	{"namespace", func(k *Keys) *Binding { return &k.Namespace }},
	{"kustomize", func(k *Keys) *Binding { return &k.Kustomize }},
	{"kubectl", func(k *Keys) *Binding { return &k.Kubectl }},
	{"forwards", func(k *Keys) *Binding { return &k.Forwards }},
	{"port_forward", func(k *Keys) *Binding { return &k.PortForward }},
	{"describe", func(k *Keys) *Binding { return &k.Describe }},
	{"edit", func(k *Keys) *Binding { return &k.Edit }},
	{"decode", func(k *Keys) *Binding { return &k.Decode }},
	{"delete", func(k *Keys) *Binding { return &k.Delete }},
	{"kustomize_delete", func(k *Keys) *Binding { return &k.KustomizeDelete }},
	{"apply", func(k *Keys) *Binding { return &k.Apply }},
	{"diff", func(k *Keys) *Binding { return &k.Diff }},
	{"delete_all", func(k *Keys) *Binding { return &k.DeleteAll }},
	{"pf_restart", func(k *Keys) *Binding { return &k.PfRestart }},
	{"pf_auto", func(k *Keys) *Binding { return &k.PfAuto }},
	{"scan", func(k *Keys) *Binding { return &k.Scan }},
	{"vim", func(k *Keys) *Binding { return &k.Vim }},
	{"wrap", func(k *Keys) *Binding { return &k.Wrap }},
	{"copy_mode", func(k *Keys) *Binding { return &k.CopyMode }},
	{"log_mark", func(k *Keys) *Binding { return &k.LogMark }},
	{"copy_select", func(k *Keys) *Binding { return &k.CopySelect }},
	{"copy_yank", func(k *Keys) *Binding { return &k.CopyYank }},
	{"copy_all", func(k *Keys) *Binding { return &k.CopyAll }},
	{"shell", func(k *Keys) *Binding { return &k.Shell }},
	{"previous", func(k *Keys) *Binding { return &k.Previous }},
	{"scale", func(k *Keys) *Binding { return &k.Scale }},
	{"set_image", func(k *Keys) *Binding { return &k.SetImage }},
	{"restart", func(k *Keys) *Binding { return &k.Restart }},
	{"trigger", func(k *Keys) *Binding { return &k.Trigger }},
	{"label", func(k *Keys) *Binding { return &k.Label }},
	{"annotate", func(k *Keys) *Binding { return &k.Annotate }},
	{"copy_name", func(k *Keys) *Binding { return &k.CopyName }},
	{"copy_ref", func(k *Keys) *Binding { return &k.CopyRef }},
	{"sort_next", func(k *Keys) *Binding { return &k.SortNext }},
	{"sort_prev", func(k *Keys) *Binding { return &k.SortPrev }},
	{"sort_reverse", func(k *Keys) *Binding { return &k.SortReverse }},
	{"approve", func(k *Keys) *Binding { return &k.Approve }},
	{"kustomize_path", func(k *Keys) *Binding { return &k.KustomizePath }},
	{"command", func(k *Keys) *Binding { return &k.Command }},
	{"split_down", func(k *Keys) *Binding { return &k.SplitDown }},
	{"split_right", func(k *Keys) *Binding { return &k.SplitRight }},
	{"close_pane", func(k *Keys) *Binding { return &k.ClosePane }},
	{"pane_left", func(k *Keys) *Binding { return &k.PaneLeft }},
	{"pane_right", func(k *Keys) *Binding { return &k.PaneRight }},
	{"pane_up", func(k *Keys) *Binding { return &k.PaneUp }},
	{"pane_down", func(k *Keys) *Binding { return &k.PaneDown }},
	{"prev_pane", func(k *Keys) *Binding { return &k.PrevPane }},
	{"next_pane", func(k *Keys) *Binding { return &k.NextPane }},
	{"last_pane", func(k *Keys) *Binding { return &k.LastPane }},
	{"new_tab", func(k *Keys) *Binding { return &k.NewTab }},
	{"close_tab", func(k *Keys) *Binding { return &k.CloseTab }},
	{"rename_tab", func(k *Keys) *Binding { return &k.RenameTab }},
	{"next_tab", func(k *Keys) *Binding { return &k.NextTab }},
	{"prev_tab", func(k *Keys) *Binding { return &k.PrevTab }},
	{"tab_1", func(k *Keys) *Binding { return &k.Tab[0] }},
	{"tab_2", func(k *Keys) *Binding { return &k.Tab[1] }},
	{"tab_3", func(k *Keys) *Binding { return &k.Tab[2] }},
	{"tab_4", func(k *Keys) *Binding { return &k.Tab[3] }},
	{"tab_5", func(k *Keys) *Binding { return &k.Tab[4] }},
	{"tab_6", func(k *Keys) *Binding { return &k.Tab[5] }},
	{"tab_7", func(k *Keys) *Binding { return &k.Tab[6] }},
	{"tab_8", func(k *Keys) *Binding { return &k.Tab[7] }},
	{"tab_9", func(k *Keys) *Binding { return &k.Tab[8] }},
	{"fullscreen", func(k *Keys) *Binding { return &k.Fullscreen }},
	{"zoom", func(k *Keys) *Binding { return &k.Zoom }},
	{"last_error", func(k *Keys) *Binding { return &k.LastError }},
	{"quit", func(k *Keys) *Binding { return &k.Quit }},
	{"prefix", func(k *Keys) *Binding { return &k.Prefix }},
	{"back", func(k *Keys) *Binding { return &k.Back }},
	{"left", func(k *Keys) *Binding { return &k.Left }},
	{"select", func(k *Keys) *Binding { return &k.Select }},
	{"submit", func(k *Keys) *Binding { return &k.Submit }},
	{"cancel", func(k *Keys) *Binding { return &k.Cancel }},
	{"shell_back", func(k *Keys) *Binding { return &k.ShellBack }},
	{"scroll_down", func(k *Keys) *Binding { return &k.ScrollDown }},
	{"scroll_up", func(k *Keys) *Binding { return &k.ScrollUp }},
	{"page_down", func(k *Keys) *Binding { return &k.PageDown }},
	{"page_up", func(k *Keys) *Binding { return &k.PageUp }},
	{"top", func(k *Keys) *Binding { return &k.Top }},
	{"bottom", func(k *Keys) *Binding { return &k.Bottom }},
	{"output", func(k *Keys) *Binding { return &k.Output }},
	{"jq", func(k *Keys) *Binding { return &k.JQ }},
	{"complete", func(k *Keys) *Binding { return &k.Complete }},
	{"yes", func(k *Keys) *Binding { return &k.Yes }},
	{"no", func(k *Keys) *Binding { return &k.No }},
	{"xray", func(k *Keys) *Binding { return &k.Xray }},
	{"multi_logs", func(k *Keys) *Binding { return &k.MultiLogs }},
	{"favorites", func(k *Keys) *Binding { return &k.Favorites }},
	{"theme", func(k *Keys) *Binding { return &k.Theme }},
	{"xray_toggle", func(k *Keys) *Binding { return &k.XrayToggle }},
	{"search", func(k *Keys) *Binding { return &k.Search }},
	{"helm_mark", func(k *Keys) *Binding { return &k.HelmMark }},
	{"helm_diff", func(k *Keys) *Binding { return &k.HelmDiff }},
	{"xray_expand_all", func(k *Keys) *Binding { return &k.XrayExpand }},
	{"xray_collapse_all", func(k *Keys) *Binding { return &k.XrayCollapse }},
	{"xray_filter", func(k *Keys) *Binding { return &k.XrayFilter }},
	{"yaml", func(k *Keys) *Binding { return &k.Yaml }},
	{"helm_values", func(k *Keys) *Binding { return &k.HelmValues }},
	{"helm_computed", func(k *Keys) *Binding { return &k.HelmComputed }},
	{"helm_manifest", func(k *Keys) *Binding { return &k.HelmManifest }},
	{"helm_notes", func(k *Keys) *Binding { return &k.HelmNotes }},
	{"helm_history", func(k *Keys) *Binding { return &k.HelmHistory }},
	{"helm_rollback", func(k *Keys) *Binding { return &k.HelmRollback }},
	{"all_namespaces", func(k *Keys) *Binding { return &k.AllNamespaces }},
	{"log_filter", func(k *Keys) *Binding { return &k.LogFilter }},
	{"fav_add", func(k *Keys) *Binding { return &k.FavAdd }},
	{"fav_remove", func(k *Keys) *Binding { return &k.FavRemove }},
	{"fav_up", func(k *Keys) *Binding { return &k.FavUp }},
	{"fav_down", func(k *Keys) *Binding { return &k.FavDown }},
}

var keyAliases = map[string]string{"new_pane": "split_down", "forward": "port_forward", "editor_prefix": "prefix"}

var PrefixedKeys = map[string]bool{
	"pane_left": true, "pane_right": true, "pane_up": true, "pane_down": true,
	"split_down": true, "split_right": true, "close_pane": true, "prev_pane": true, "next_pane": true, "last_pane": true,
	"zoom": true, "rename_tab": true, "new_tab": true, "close_tab": true, "next_tab": true, "prev_tab": true,
	"tab_1": true, "tab_2": true, "tab_3": true, "tab_4": true, "tab_5": true, "tab_6": true, "tab_7": true, "tab_8": true, "tab_9": true,
}

var MoveKeys = map[string]bool{"pane_left": true, "pane_right": true, "pane_up": true, "pane_down": true}

func legacyDirect(name, key string) bool {
	return MoveKeys[name] && len(key) > 1 && strings.Contains(key, "+")
}

func (k *Keys) PrefixedAction(key string) string {
	for _, f := range keyFields {
		if PrefixedKeys[f.name] && f.ptr(k).After(key) {
			return f.name
		}
	}
	return ""
}

func (k *Keys) DirectAction(key string) string {
	for _, f := range keyFields {
		if PrefixedKeys[f.name] && f.ptr(k).Direct(key) {
			return f.name
		}
	}
	return ""
}

func ValidPrefix(key string) string {
	switch {
	case strings.HasPrefix(key, "ctrl+"), strings.HasPrefix(key, "alt+"), isFKey(key):
		return ""
	case utf8.RuneCountInString(key) == 1:
		return key + " is a printable character"
	}
	return key + " is needed by the screens"
}

var optionalKeys = map[string]bool{"prev_pane": true}

var OldPaneKeys = map[string]Binding{
	"split_right": {"ctrl+v"}, "split_down": {"ctrl+n"}, "next_pane": {"O"}, "prev_pane": nil,
	"close_pane": {"ctrl+x"}, "new_tab": {"ctrl+t"}, "close_tab": {"ctrl+w"},
	"tab_1": {"alt+1"}, "tab_2": {"alt+2"}, "tab_3": {"alt+3"}, "tab_4": {"alt+4"}, "tab_5": {"alt+5"},
	"tab_6": {"alt+6"}, "tab_7": {"alt+7"}, "tab_8": {"alt+8"}, "tab_9": {"alt+9"},
}

var globalKeys = map[string]bool{
	"quit": true, "prefix": true, "fullscreen": true, "last_error": true, "theme": true, "split_down": true, "split_right": true, "close_pane": true,
	"pane_left": true, "pane_right": true, "pane_up": true, "pane_down": true, "prev_pane": true, "next_pane": true, "last_pane": true,
	"zoom": true, "rename_tab": true, "new_tab": true, "close_tab": true, "next_tab": true, "prev_tab": true,
	"tab_1": true, "tab_2": true, "tab_3": true, "tab_4": true, "tab_5": true, "tab_6": true, "tab_7": true, "tab_8": true, "tab_9": true,
}

var screenKeys = func() map[string][]string {
	common := []string{"kubeconfig", "context", "namespace", "kustomize", "kubectl", "forwards", "favorites", "command", "back", "left", "select"}
	move := []string{"scroll_down", "scroll_up", "page_down", "page_up", "top", "bottom"}
	with := func(extra ...string) []string {
		return append(append(append([]string(nil), common...), move...), extra...)
	}
	return map[string][]string{
		"tables": with("describe", "port_forward", "delete", "edit", "decode",
			"shell", "scale", "set_image", "restart", "trigger", "label", "annotate", "copy_name", "copy_ref",
			"sort_next", "sort_prev", "sort_reverse", "approve", "xray", "multi_logs", "all_namespaces"),

		"xray": with("describe", "yaml", "xray_toggle", "xray_expand_all", "xray_collapse_all", "xray_filter", "multi_logs", "copy_name", "copy_ref", "all_namespaces"),

		"helm": with("helm_values", "helm_computed", "helm_manifest", "helm_notes", "helm_history", "helm_rollback",
			"delete", "all_namespaces", "copy_name", "copy_ref", "sort_next", "sort_prev", "sort_reverse"),

		"helmhist": with("helm_rollback", "helm_mark", "helm_diff", "helm_values", "helm_computed", "helm_manifest", "helm_notes", "copy_name"),

		"mlogs": with("vim", "wrap", "copy_mode", "log_filter", "all_namespaces"),

		"favpicker": {"submit", "cancel", "scroll_down", "scroll_up", "fav_add", "fav_remove", "fav_up", "fav_down"},
		"kustomize": with("kustomize_delete", "apply", "diff", "edit", "scan", "kustomize_path"),
		"forwards":  with("delete", "delete_all", "pf_restart", "pf_auto"),

		"viewer": with("vim", "previous", "wrap", "copy_mode", "search", "all_namespaces"),

		"copy": {"copy_select", "copy_yank", "copy_all", "wrap", "back", "scroll_down", "scroll_up", "page_down", "page_up", "top", "bottom"},

		"popup": {"submit", "cancel", "complete", "scroll_down", "scroll_up"},
	}
}()

func sameScreen(a, b string) bool {
	for _, names := range screenKeys {
		hasA, hasB := false, false
		for _, n := range names {
			hasA = hasA || n == a
			hasB = hasB || n == b
		}
		if hasA && hasB {
			return true
		}
	}
	return false
}

func uses(k *Keys, name string) (direct, after []string) {
	b := *fieldByName(name).ptr(k)
	if !PrefixedKeys[name] {
		return b, nil
	}
	for _, key := range b {
		if d, ok := strings.CutPrefix(key, DirectMark); ok {
			direct = append(direct, d)
		} else {
			after = append(after, key)
		}
	}
	return direct, after
}

func conflict(k *Keys, name, key string, after bool) string {
	for _, f := range keyFields {
		if f.name == name {
			continue
		}
		direct, afterKeys := uses(k, f.name)
		if after {
			if contains(afterKeys, key) || (f.name == "prefix" && contains(direct, key)) {
				return f.name
			}
			continue
		}
		if !(globalKeys[name] || globalKeys[f.name] || sameScreen(name, f.name)) {
			continue
		}
		if contains(direct, key) {
			return f.name
		}
	}
	return ""
}

func resolveConflicts(k *Keys, changed []string, warn func(string, ...any)) {
	def := DefaultKeys()
	for _, name := range changed {
		f := fieldByName(name)
		direct, after := uses(k, name)
		check := func(key string, isAfter bool) bool {
			other := conflict(k, name, key, isAfter)
			if other == "" {
				return false
			}
			*f.ptr(k) = *f.ptr(&def)
			shown := key
			if isAfter {
				shown = k.Prefix.Label() + " " + key
			}
			warn("keys.%s: %s is already used by %s (using %s)", name, shown, other, labelOr(*f.ptr(k), "none"))
			return true
		}
		bad := false
		for _, key := range direct {
			if bad = check(key, false); bad {
				break
			}
		}
		for _, key := range after {
			if bad || check(key, true) {
				break
			}
		}
	}
}

func fieldByName(name string) keyField {
	for _, f := range keyFields {
		if f.name == name {
			return f
		}
	}
	panic("config: unknown key field " + name)
}

func labelOr(b Binding, alt string) string {
	if len(b) == 0 {
		return alt
	}
	return strings.Join(b, ", ")
}

func KeyNames() []string {
	out := make([]string, len(keyFields))
	for i, f := range keyFields {
		out[i] = f.name
	}
	return out
}

var namedKeys = map[string]bool{
	"enter": true, "esc": true, "tab": true, "backspace": true, "delete": true,
	"insert": true, "up": true, "down": true, "left": true, "right": true,
	"home": true, "end": true, "pgup": true, "pgdown": true, " ": true,
}

var aliasKeys = map[string]string{
	"escape": "esc", "return": "enter", "space": " ", "del": "delete",
	"pageup": "pgup", "pagedown": "pgdown", "ins": "insert",
}

func ParseKey(s string) (string, error) {
	raw := s
	s = strings.TrimSpace(s)
	if s == "" {
		if raw == " " {
			return " ", nil
		}
		return "", fmt.Errorf("empty key")
	}
	if utf8.RuneCountInString(s) == 1 {
		return s, nil
	}
	lower := strings.ToLower(s)
	if a, ok := aliasKeys[lower]; ok {
		return a, nil
	}
	if namedKeys[lower] || isFKey(lower) {
		return lower, nil
	}
	mod, rest, ok := strings.Cut(lower, "+")
	if !ok || rest == "" {
		return "", fmt.Errorf("unknown key %q", raw)
	}

	restOrig := s[len(mod)+1:]
	switch mod {
	case "shift":
		if utf8.RuneCountInString(restOrig) == 1 {
			return strings.ToUpper(restOrig), nil
		}
		switch rest {
		case "tab", "up", "down", "left", "right", "home", "end":
			return "shift+" + rest, nil
		}
	case "ctrl":
		if len(rest) == 1 && rest[0] >= 'a' && rest[0] <= 'z' {
			switch rest {
			case "i":
				return "tab", nil
			case "m":
				return "enter", nil
			}
			return "ctrl+" + rest, nil
		}
		switch rest {
		case "space":
			return "ctrl+@", nil
		case "up", "down", "left", "right", "home", "end", "pgup", "pgdown",
			"@", "\\", "]", "^", "_", "shift+up", "shift+down", "shift+left", "shift+right":
			return "ctrl+" + rest, nil
		case "[":
			return "esc", nil
		}
	case "alt":
		inner, err := ParseKey(restOrig)
		if err == nil {
			return "alt+" + inner, nil
		}
	}
	return "", fmt.Errorf("unknown key %q", raw)
}

func isFKey(s string) bool {
	if len(s) < 2 || s[0] != 'f' {
		return false
	}
	n := 0
	for _, c := range s[1:] {
		if c < '0' || c > '9' {
			return false
		}
		n = n*10 + int(c-'0')
	}
	return n >= 1 && n <= 20
}

func (k Keys) PrefixedAll(name string) string {
	_, after := uses(&k, name)
	return strings.Join(after, "/")
}

func (k Keys) PrefixedFor(name string) string {
	_, after := uses(&k, name)
	if len(after) == 0 {
		return ""
	}
	return after[0]
}

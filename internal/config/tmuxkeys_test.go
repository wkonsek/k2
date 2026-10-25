package config

import (
	"reflect"
	"testing"
)

func TestTmuxKeysInYAML(t *testing.T) {
	in := `
keys:
  split_down: [h, '"']
  split_right: ["v", "%"]
  close_tab: [w, "&"]
  rename_tab: ","
  last_pane: ';'
  new_tab: [t, c]
  zoom: z
  prev_pane: []
`
	c, w := Parse([]byte(in))
	if len(w) != 0 {
		t.Fatalf("warnings %v", w)
	}
	k := c.Keys
	for name, want := range map[string]Binding{
		"split_down": {"h", `"`}, "split_right": {"v", "%"}, "close_tab": {"w", "&"},
		"rename_tab": {","}, "last_pane": {";"}, "new_tab": {"t", "c"}, "zoom": {"z"},
	} {
		if got := *fieldByName(name).ptr(&k); !reflect.DeepEqual(got, want) {
			t.Errorf("keys.%s = %v, want %v", name, got, want)
		}
	}
	if len(k.PrevPane) != 0 {
		t.Errorf("prev_pane %v", k.PrevPane)
	}
	if k.PrefixedAction(`"`) != "split_down" || k.PrefixedAction("%") != "split_right" || k.PrefixedAction(",") != "rename_tab" {
		t.Errorf("actions %q %q %q", k.PrefixedAction(`"`), k.PrefixedAction("%"), k.PrefixedAction(","))
	}

	c, w = Parse([]byte("keys:\n  prev_pane: P\n"))
	if len(w) != 0 || !c.Keys.PrevPane.After("P") {
		t.Fatalf("prev_pane P: %v %v", c.Keys.PrevPane, w)
	}

	c, w = Parse([]byte("keys:\n  next_pane: []\n"))
	if len(w) != 1 || !reflect.DeepEqual(c.Keys.NextPane, Binding{"o"}) {
		t.Fatalf("next_pane []: %v %v", c.Keys.NextPane, w)
	}

	d := DefaultKeys()
	if d.PrefixedAction("c") != "new_tab" || !d.Context.Has("c") || d.DirectAction("c") != "" {
		t.Fatalf("c: %q %v", d.PrefixedAction("c"), d.Context)
	}
	if _, w := Parse([]byte("keys: {}\n")); len(w) != 0 {
		t.Fatalf("defaults warn: %v", w)
	}
}

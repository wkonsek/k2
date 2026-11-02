package config

import (
	"strings"
	"testing"
)

func TestEditorOpenFolder(t *testing.T) {
	if d := Default(); d.Editor.OpenFolder != "code" || !d.Keys.OpenFolder.Has(".") {
		t.Fatalf("defaults %q %v", d.Editor.OpenFolder, d.Keys.OpenFolder)
	}
	c, w := Parse([]byte("editor:\n  open_folder: code-insiders -n\nkeys:\n  open_folder: O\n"))
	if len(w) != 0 || c.Editor.OpenFolder != "code-insiders -n" || !c.Keys.OpenFolder.Has("O") || c.Keys.OpenFolder.Has(".") {
		t.Fatalf("%+v %v %v", c.Editor, c.Keys.OpenFolder, w)
	}
	for in, want := range map[string]string{
		"editor:\n  open_folder: \"\"\n": "editor.open_folder: empty (using code)",
		"editor:\n  folder: code\n":      `editor: unknown setting "folder" (have open_folder)`,
		"editor: code\n":                 "editor: expected a mapping",
		"keys:\n  open_folder: s\n":      "keys.open_folder: s is already used by scan (using .)",
	} {
		c, w := Parse([]byte(in))
		if len(w) != 1 || !strings.Contains(w[0], want) || c.Editor.OpenFolder != "code" || !c.Keys.OpenFolder.Has(".") {
			t.Errorf("%q: %v %+v %v", in, w, c.Editor, c.Keys.OpenFolder)
		}
	}
}

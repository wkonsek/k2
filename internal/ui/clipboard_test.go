package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeClip(t *testing.T, tools map[string]int) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")
	for name, code := range tools {
		script := "#!/bin/sh\n" +
			"in=$(cat)\n" +
			"echo \"" + name + " $*|$in\" >> '" + log + "'\n"
		if code != 0 {
			script += "echo \"" + name + ": Can't open display\" >&2\nexit 1\n"
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	for _, name := range []string{"wl-copy", "xclip", "xsel"} {
		if _, ok := tools[name]; ok {
			continue
		}
		for _, sys := range []string{"/bin", "/usr/bin"} {
			if _, err := os.Stat(filepath.Join(sys, name)); err == nil {
				t.Skipf("a real %s is installed in %s; the fake PATH cannot hide it", name, sys)
			}
		}
	}
	t.Setenv("PATH", dir+":/usr/bin:/bin")
	return log
}

func calledTools(t *testing.T, log string) []string {
	t.Helper()
	data, err := os.ReadFile(log)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func testClipEnv(osc *[]string) clipEnv {
	env := realClipEnv()
	env.writeTTY = func(s string) error { *osc = append(*osc, s); return nil }
	return env
}

func TestClipboardAutoOrder(t *testing.T) {
	all := map[string]int{"wl-copy": 0, "xclip": 0, "xsel": 0}
	cases := []struct {
		name         string
		tools        map[string]int
		wayland, x11 string
		wantMethod   string
		wantCalls    []string
	}{
		{"wayland first", all, "wayland-0", ":0", "wl-copy", []string{"wl-copy |kafka-client"}},
		{"x11 only", all, "", ":0", "xclip", []string{"xclip -selection clipboard|kafka-client"}},
		{"no xclip", map[string]int{"wl-copy": 0, "xsel": 0}, "", ":0", "xsel", []string{"xsel --clipboard --input|kafka-client"}},
		{"wl-copy fails, xclip works", map[string]int{"wl-copy": 1, "xclip": 0}, "wayland-0", ":0", "xclip",
			[]string{"wl-copy |kafka-client", "xclip -selection clipboard|kafka-client"}},
		{"no display at all", all, "", "", "osc52", nil},
		{"wayland without wl-copy, no DISPLAY", map[string]int{"xclip": 0}, "wayland-0", "", "osc52", nil},
		{"nothing installed", map[string]int{}, "wayland-0", ":0", "osc52", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			log := fakeClip(t, c.tools)
			t.Setenv("WAYLAND_DISPLAY", c.wayland)
			t.Setenv("DISPLAY", c.x11)
			t.Setenv("TMUX", "")
			var osc []string
			method, err := copyToClipboard(testClipEnv(&osc), "auto", "kafka-client")
			if err != nil || method != c.wantMethod {
				t.Fatalf("method %q err %v, want %q", method, err, c.wantMethod)
			}
			if got := calledTools(t, log); strings.Join(got, "\n") != strings.Join(c.wantCalls, "\n") {
				t.Fatalf("calls %q, want %q", got, c.wantCalls)
			}
			if (method == "osc52") != (len(osc) == 1) {
				t.Fatalf("osc writes %q for method %s", osc, method)
			}
		})
	}
}

func TestClipboardExplicitMode(t *testing.T) {
	log := fakeClip(t, map[string]int{"wl-copy": 0, "xclip": 1})
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	t.Setenv("DISPLAY", ":0")
	var osc []string
	env := testClipEnv(&osc)
	if m, err := copyToClipboard(env, "xclip", "a"); err == nil || m != "xclip" || !strings.Contains(err.Error(), "Can't open display") {
		t.Fatalf("xclip failure: %q %v", m, err)
	}
	if m, err := copyToClipboard(env, "xsel", "a"); err == nil || !strings.Contains(err.Error(), "xsel is not installed") {
		t.Fatalf("missing xsel: %q %v", m, err)
	}
	if m, err := copyToClipboard(env, "wl-copy", "b"); err != nil || m != "wl-copy" {
		t.Fatalf("wl-copy: %q %v", m, err)
	}
	if len(osc) != 0 {
		t.Fatalf("explicit tools must not fall back to OSC 52: %q", osc)
	}
	if got := calledTools(t, log); strings.Join(got, ",") != "xclip -selection clipboard|a,wl-copy |b" {
		t.Fatalf("calls %q", got)
	}

	if m, err := copyToClipboard(env, "osc52", "c"); err != nil || m != "osc52" || len(osc) != 1 {
		t.Fatalf("osc52: %q %v %q", m, err, osc)
	}
}

func TestClipboardTmuxPassthrough(t *testing.T) {
	fakeClip(t, map[string]int{})
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", "")
	t.Setenv("TMUX", "/tmp/tmux-1000/default,123,0")
	var osc []string
	if _, err := copyToClipboard(testClipEnv(&osc), "auto", "x"); err != nil {
		t.Fatal(err)
	}
	want := "\x1bPtmux;\x1b\x1b]52;c;eA==\a\x1b\\"
	if len(osc) != 1 || osc[0] != want {
		t.Fatalf("got %q want %q", osc, want)
	}
}

func TestClipboardToolForksDoesNotHang(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\ncat >/dev/null\n(sleep 30) &\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "xclip"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := runClipTool(filepath.Join(dir, "xclip"), nil, "x"); err != nil {
		t.Fatal(err)
	}
}

func TestCopyToastNamesMethod(t *testing.T) {
	old := clipboardRun
	t.Cleanup(func() { clipboardRun = old })
	var gotMode string
	clipboardRun = func(mode, text string) (string, error) { gotMode = mode; return "wl-copy", nil }
	m := pressSettle(t, selectRow(t, actionModel(t, actionCluster()), "db-0"), "w")
	if gotMode != "auto" || m.toast.text != "copied db-0 (wl-copy)" || m.toast.tone != toneOK {
		t.Fatalf("mode %q toast %q", gotMode, m.toast.text)
	}
	clipboardRun = func(mode, text string) (string, error) { return "xclip", os.ErrNotExist }
	m = pressSettle(t, m, "w")
	if !strings.HasPrefix(m.toast.text, "copy failed: ") || m.toast.tone != toneErr {
		t.Fatalf("failure toast %q", m.toast.text)
	}
}

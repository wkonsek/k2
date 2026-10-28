package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"k2/internal/shell"
)

type edHarness struct {
	t    *testing.T
	m    model
	msgs chan tea.Msg
}

func newEdHarness(t *testing.T, m model) *edHarness {
	return &edHarness{t: t, m: m, msgs: make(chan tea.Msg, 256)}
}

func (h *edHarness) run(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	go func() {
		switch msg := cmd().(type) {
		case nil, spinner.TickMsg, toastExpiredMsg, tickMsg:
		case tea.BatchMsg:
			for _, c := range msg {
				h.run(c)
			}
		default:
			h.msgs <- msg
		}
	}()
}

func (h *edHarness) send(msg tea.Msg) {
	next, cmd := h.m.Update(msg)
	h.m = next.(model)
	h.run(cmd)
}

func (h *edHarness) keys(ks ...string) {
	for _, k := range ks {
		h.send(key(h.t, k))
	}
}

func (h *edHarness) typeText(s string) {
	for _, r := range s {
		h.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func (h *edHarness) pump(cond func() bool, what string) {
	h.t.Helper()
	deadline := time.After(8 * time.Second)
	for !cond() {
		select {
		case msg := <-h.msgs:
			h.send(msg)
		case <-deadline:
			h.t.Fatalf("timeout waiting for %s; view:\n%s", what, ansi.Strip(h.m.View()))
		}
	}
}

func (h *edHarness) screen() string {
	if h.m.ed == nil || h.m.ed.t == nil {
		return ""
	}
	return h.m.ed.t.Plain()
}

func TestShellScrollback(t *testing.T) {
	bin := t.TempDir()
	script := "#!/bin/sh\ni=1\nwhile [ \"$i\" -le 80 ]; do printf 'line-%02d\\n' \"$i\"; i=$((i+1)); done\nread x\n"
	if err := os.WriteFile(filepath.Join(bin, "kubectl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	h := newEdHarness(t, selectRow(t, actionModel(t, actionCluster()), "db-0"))
	h.m.width, h.m.height = 80, 16
	h.keys("s")
	if !h.m.ed.live() {
		t.Fatalf("no shell: %s", h.m.status)
	}
	h.pump(func() bool { return strings.Contains(h.m.ed.t.Plain(), "line-80") }, "output")
	live := ansi.Strip(h.m.View())
	if strings.Contains(live, "line-01") {
		t.Fatalf("first line still visible:\n%s", live)
	}
	h.keys("pgup")
	if h.m.ed.scroll == 0 {
		t.Fatal("pgup stayed on the live screen")
	}
	scrolled := ansi.Strip(h.m.View())
	if scrolled == live || !strings.Contains(scrolled, "line-") {
		t.Fatalf("scrollback view:\n%s", scrolled)
	}
	before := h.m.ed.scroll
	h.send(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	if h.m.ed.scroll >= before {
		t.Fatalf("wheel %d -> %d", before, h.m.ed.scroll)
	}
	h.keys("esc")
	if h.m.ed.scroll != 0 || !strings.Contains(ansi.Strip(h.m.View()), "line-80") {
		t.Fatalf("esc did not return: scroll %d\n%s", h.m.ed.scroll, ansi.Strip(h.m.View()))
	}
	h.keys("Z")
	full := ansi.Strip(h.m.View())
	if strings.Contains(full, "╭") || strings.Contains(full, "│") {
		t.Fatalf("fullscreen shell still has a frame:\n%s", full)
	}
	row := -1
	for i, l := range strings.Split(full, "\n") {
		if strings.Contains(l, "line-80") {
			row = i
			break
		}
	}
	if row < 0 {
		t.Fatalf("line-80 not on screen:\n%s", full)
	}
	var got string
	old := clipboardRun
	t.Cleanup(func() { clipboardRun = old })
	clipboardRun = func(mode, text string) (string, error) { got = text; return "test", nil }
	h.send(tea.MouseMsg{X: 0, Y: row, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	h.send(tea.MouseMsg{X: 6, Y: row, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
	h.pump(func() bool { return strings.HasPrefix(h.m.status, "copied") }, "copy")
	if got != "line-80" {
		t.Fatalf("drag copied %q", got)
	}
	h.keys("ctrl+a", "y")
	if h.m.copy == nil || !strings.Contains(h.m.View(), "COPY") {
		t.Fatalf("copy mode:\n%s", ansi.Strip(h.m.View()))
	}
	if !strings.Contains(strings.Join(h.m.shellYank, "\n"), "line-01") {
		t.Fatal("yank buffer lost the start of the output")
	}
}

func needVim(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("vim"); err != nil {
		t.Skip("vim not installed")
	}
}

func startCat(t *testing.T, h *edHarness) {
	t.Helper()
	next, cmd := h.m.runEditor("vim", "vim notes", "notes", "", func() *exec.Cmd { return exec.Command("cat") }, nil)
	h.m = next.(model)
	h.run(cmd)
	if !h.m.ed.live() {
		t.Fatalf("no editor: %s", h.m.status)
	}
}

func TestInlineVimFromViewer(t *testing.T) {
	needVim(t)
	m := newTestModel(t, testEnv(t), nil)
	m.stage, m.subject, m.logText = "view", "notes", "hello from k2\nsecond line\n"
	h := newEdHarness(t, m)
	h.keys("v")
	h.pump(func() bool { return strings.Contains(h.screen(), "hello from k2") }, "vim to show the text")
	w, ht := h.m.ed.t.Size()
	if ew, eh := h.m.editorSize(h.m.logID); w != ew || ht != eh {
		t.Fatalf("editor %dx%d, pane text area %dx%d", w, ht, ew, eh)
	}
	path := h.m.ed.cleanup
	view := ansi.Strip(h.m.View())
	if !strings.Contains(view, "· vim · ctrl+a k2") || !strings.Contains(view, "vim · ctrl+a k2") {
		t.Fatalf("no vim hint:\n%s", view)
	}

	time.Sleep(50 * time.Millisecond)
	h.typeText(":q")
	h.keys("enter")
	h.pump(func() bool { return h.m.ed == nil }, "vim to exit")
	if h.m.stage != "view" {
		t.Fatalf("stage %q", h.m.stage)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("temp file %s left behind", path)
	}
}

func TestKubectlEditInlineKeepsError(t *testing.T) {
	needVim(t)
	bin := t.TempDir()
	script := "#!/bin/sh\nf=$(mktemp)\necho 'kind: Pod' > \"$f\"\n$KUBE_EDITOR \"$f\"\nrm -f \"$f\"\necho \"edit args: $*\"\necho 'error: pods \"web-1\" is invalid'\nexit 1\n"
	if err := os.WriteFile(filepath.Join(bin, "kubectl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	m := podsModel(t, podCluster())
	h := newEdHarness(t, m)
	h.keys("e")
	h.pump(func() bool { return strings.Contains(h.screen(), "kind: Pod") }, "vim via KUBE_EDITOR")
	time.Sleep(50 * time.Millisecond)
	h.typeText(":q")
	h.keys("enter")
	h.pump(func() bool { return h.m.ed != nil && h.m.ed.done }, "kubectl to fail")
	view := ansi.Strip(h.m.View())
	for _, want := range []string{`error: pods "web-1" is invalid`, "edit args: edit pod web-1 --kubeconfig /kc --context kind-dev -n shop", "exit 1 · q closes", "edit pod/web-1 · exit 1"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	h.keys("j")
	if h.m.ed == nil {
		t.Fatal("j closed the error screen")
	}
	h.keys("esc")
	if h.m.ed != nil || h.m.stage != "workpods" {
		t.Fatalf("ed %v stage %q", h.m.ed, h.m.stage)
	}
}

func TestEditorKeyRouting(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	h := newEdHarness(t, m)
	h.keys("ctrl+a", "v")
	startCat(t, h)
	h.typeText("qjF:O")
	h.pump(func() bool { return strings.Contains(h.screen(), "qjF:O") }, "cat to echo")
	if h.m.stage != "nav" || h.m.zoom || h.m.cmdOn || h.m.tabPos(h.m.logID) != 1 {
		t.Fatalf("keys leaked to k2: stage %q zoom %v cmd %v", h.m.stage, h.m.zoom, h.m.cmdOn)
	}

	w0, _ := h.m.ed.t.Size()
	h.keys("ctrl+a", "z")
	if w1, _ := h.m.ed.t.Size(); !h.m.zoom || w1 <= w0 {
		t.Fatalf("zoom %v width %d → %d", h.m.zoom, w0, w1)
	}
	h.keys("ctrl+a", "z")

	h.keys("ctrl+a", "j")
	if !strings.HasPrefix(h.m.status, "ctrl+a j: not a pane or tab key") {
		t.Fatalf("status %q", h.m.status)
	}

	h.keys("ctrl+left")
	if h.m.tabPos(h.m.logID) != 0 || h.m.ed != nil || !h.m.panes[1].ed.live() {
		t.Fatal("ctrl+left did not leave the editor pane")
	}
	if v := ansi.Strip(h.m.View()); !strings.Contains(v, "qjF:O") {
		t.Fatalf("unfocused editor not drawn:\n%s", v)
	}
	h.keys("ctrl+right")

	h.keys("ctrl+\\", "ctrl+\\")
	h.pump(func() bool { return h.m.ed == nil }, "cat to get ctrl+\\")
}

func TestEditorCloseAndQuitKill(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	h := newEdHarness(t, m)
	h.keys("ctrl+a", "v")
	startCat(t, h)
	term := h.m.ed.t
	h.keys("ctrl+a", "x")
	if h.m.dlg == nil || !h.m.dlg.danger || !strings.Contains(strings.Join(h.m.dlg.details, "\n"), "vim runs in this pane and is killed") {
		t.Fatalf("no close popup naming vim: %+v", h.m.dlg)
	}
	h.keys("y")
	if len(h.m.panes) != 1 {
		t.Fatalf("panes %d", len(h.m.panes))
	}
	waitExited(t, term)

	startCat(t, h)
	term = h.m.ed.t
	h.keys("ctrl+a", "t")
	h.keys("ctrl+a", "[")
	if h.m.tab != 0 || !h.m.ed.live() {
		t.Fatal("editor lost on tab switch")
	}
	h.keys("ctrl+a", "Q")
	if !h.m.quitting {
		t.Fatal("prefix Q did not quit")
	}
	waitExited(t, term)
}

func TestCloseTabWithEditorAsks(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	h := newEdHarness(t, m)
	h.keys("ctrl+a", "t")
	startCat(t, h)
	term := h.m.ed.t
	h.keys("ctrl+a", "w")
	if h.m.dlg == nil || !h.m.dlg.danger || !strings.Contains(strings.Join(h.m.dlg.details, "\n"), "vim or a shell runs in 1 pane and is killed") {
		t.Fatalf("no close popup naming the editor: %+v", h.m.dlg)
	}
	h.keys("y")
	if len(h.m.tabs) != 1 {
		t.Fatalf("tabs %d", len(h.m.tabs))
	}
	waitExited(t, term)
}

func waitExited(t *testing.T, term interface{ Exited() bool }) {
	t.Helper()
	for i := 0; i < 100 && !term.Exited(); i++ {
		time.Sleep(30 * time.Millisecond)
	}
	if !term.Exited() {
		t.Fatal("editor still running")
	}
}

func TestEditorFallbacks(t *testing.T) {
	cfg := testEnv(t)
	cfg.UI.EditorInline = false
	m := newTestModel(t, cfg, nil)
	next, cmd := m.runEditor("vim", "vim x", "x", "", func() *exec.Cmd { return exec.Command("cat") }, func(error) tea.Msg { return vimDoneMsg{} })
	if next.(model).ed != nil || cmd == nil {
		t.Fatal("editor_inline false must use ExecProcess")
	}
	m = newTestModel(t, testEnv(t), nil)
	next, cmd = m.runEditor("vim", "vim x", "x", "", func() *exec.Cmd { return exec.Command("/nonexistent/vim") }, func(error) tea.Msg { return vimDoneMsg{} })
	if got := next.(model); got.ed != nil || cmd == nil || !strings.HasPrefix(got.status, "inline editor unavailable") {
		t.Fatalf("ed %v status %q", got.ed, got.status)
	}
}

func TestEditorPrefixRemapped(t *testing.T) {
	for _, p := range []string{"ctrl+g", "ctrl+k", "ctrl+\\"} {
		cfg := testEnv(t)
		cfg.Keys.Prefix = []string{p}
		m := newTestModel(t, cfg, &shell.Fake{Handler: func(shell.Request) shell.Result { return shell.Fail(1, "") }})
		h := newEdHarness(t, m)
		startCat(t, h)
		h.keys("ctrl+a")
		if h.m.prefixOn {
			t.Fatalf("%s: ctrl+a started a prefix", p)
		}
		h.keys(p, "z")
		if !h.m.zoom || !strings.Contains(ansi.Strip(h.m.View()), "vim · "+p+" k2") {
			t.Fatalf("%s: zoom %v", p, h.m.zoom)
		}
		h.keys(p, "z", p, "v")
		if len(h.m.tabIDs()) != 2 {
			t.Fatalf("%s v did not split", p)
		}
		h.keys("ctrl+left")
		if p == "ctrl+\\" {
			h.keys(p, p)
			h.pump(func() bool { return h.m.ed == nil }, "cat to get ctrl+\\")
		}
		h.keys(p, "Q")
	}
}

func TestKustomizeAndEditSuccessInline(t *testing.T) {
	needVim(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "kustomization.yaml"), []byte("resources:\n- web.yaml\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newTestModel(t, testEnv(t), nil)
	m.stage = "kdir"
	m.content.SetItems([]list.Item{item{id: dir, title: "kustomization.yaml", desc: "kust-file"}})
	h := newEdHarness(t, m)
	h.keys("enter")
	h.pump(func() bool { return strings.Contains(h.screen(), "- web.yaml") }, "vim on the kustomization")
	if !strings.Contains(ansi.Strip(h.m.View()), "vim kustomization.yaml · vim") {
		t.Fatalf("title:\n%s", ansi.Strip(h.m.View()))
	}
	time.Sleep(50 * time.Millisecond)
	h.typeText(":q")
	h.keys("enter")
	h.pump(func() bool { return h.m.ed == nil }, "vim exit")
	if h.m.stage != "kdir" {
		t.Fatalf("stage %q", h.m.stage)
	}

	bin := t.TempDir()
	script := "#!/bin/sh\nf=$(mktemp)\necho 'kind: Pod' > \"$f\"\n$KUBE_EDITOR \"$f\"\nrm -f \"$f\"\necho 'pod/web-1 edited'\n"
	if err := os.WriteFile(filepath.Join(bin, "kubectl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	h = newEdHarness(t, podsModel(t, podCluster()))
	h.keys("e")
	h.pump(func() bool { return strings.Contains(h.screen(), "kind: Pod") }, "vim via KUBE_EDITOR")
	time.Sleep(50 * time.Millisecond)
	h.typeText(":q")
	h.keys("enter")
	h.pump(func() bool { return h.m.ed == nil && h.m.status == "done" }, "edit to finish")
	if h.m.toast.text != "edited web-1" {
		t.Fatalf("toast %+v", h.m.toast)
	}
}

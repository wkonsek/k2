package ui

import (
	"strings"
	"testing"

	"k2/internal/config"
	"k2/internal/state"

	"github.com/charmbracelet/x/ansi"
)

func TestSaveTypedCommandWithoutRunning(t *testing.T) {
	m := appModel(t, appCluster(t))
	m = pressSettle(t, m, ":")
	m = typeText(t, m, "logs app=web -c main")
	next, cmd := m.Update(key(t, "ctrl+s"))
	m = settle(t, next.(model), cmd)
	if len(m.cmdFavs) != 1 || m.cmdFavs[0].Command != "logs app=web -c main" {
		t.Fatalf("saved %+v", m.cmdFavs)
	}
	if !m.cmdOn || m.cmd.Value() != "logs app=web -c main" {
		t.Fatalf("the line should stay open: on %v %q", m.cmdOn, m.cmd.Value())
	}
	if m.stage == "mlogs" {
		t.Fatal("saving must not run the command")
	}
	if m.toast.text != "saved favorite: logs app=web -c main" {
		t.Fatalf("toast %q", m.toast.text)
	}
	if st := state.Load(m.cfg.StateDir); len(st.CommandFavorites) != 1 || st.CommandFavorites[0].Command != "logs app=web -c main" {
		t.Fatalf("not persisted: %+v", st.CommandFavorites)
	}

	next, cmd = m.Update(key(t, "ctrl+s"))
	m = settle(t, next.(model), cmd)
	if len(m.cmdFavs) != 1 || !strings.HasPrefix(m.toast.text, "already a favorite") {
		t.Fatalf("duplicate: %+v %q", m.cmdFavs, m.toast.text)
	}

	m.cmd.SetValue("   ")
	next, cmd = m.Update(key(t, "ctrl+s"))
	m = settle(t, next.(model), cmd)
	if len(m.cmdFavs) != 1 || !strings.HasPrefix(m.toast.text, "nothing to save") {
		t.Fatalf("empty: %+v %q", m.cmdFavs, m.toast.text)
	}
}

func TestCmdFavoritesRunByEnterAndNumber(t *testing.T) {
	m := appModel(t, appCluster(t))
	m.cmdFavs = []state.CommandFavorite{{Name: "pulse", Command: "pulse"}, {Command: "helm"}}
	m = pressSettle(t, m, "B")
	if m.dlg == nil || m.dlg.act != "cmdfavs" || len(m.dlg.options) != 2 {
		t.Fatalf("popup %+v", m.dlg)
	}
	view := ansi.Strip(m.View())
	for _, want := range []string{"★ commands", "1 pulse", "2 helm", ":pulse"} {
		if !strings.Contains(view, want) {
			t.Fatalf("popup lacks %q:\n%s", want, view)
		}
	}
	m = pressSettle(t, m, "enter")
	if m.dlg != nil || m.stage != "pulse" {
		t.Fatalf("enter: dlg %v stage %s", m.dlg != nil, m.stage)
	}
	m = pressSettle(t, m, "B", "2")
	if m.dlg != nil || m.stage != "helm" {
		t.Fatalf("2: dlg %v stage %s", m.dlg != nil, m.stage)
	}
}

func TestCmdFavoritesCommandWords(t *testing.T) {
	m := appModel(t, appCluster(t))
	m = open(t, m, "cmds add pulse")
	if len(m.cmdFavs) != 1 || m.cmdFavs[0].Command != "pulse" || m.stage == "pulse" {
		t.Fatalf("cmds add: %+v stage %s", m.cmdFavs, m.stage)
	}
	m = open(t, m, "cmds")
	if m.dlg == nil || m.dlg.act != "cmdfavs" {
		t.Fatal(":cmds")
	}
	m = pressSettle(t, m, "esc")
	m = open(t, m, "cmds 1")
	if m.stage != "pulse" {
		t.Fatalf(":cmds 1 stage %s", m.stage)
	}
}

func TestCmdFavoritesAddLastCommand(t *testing.T) {
	m := appModel(t, appCluster(t))
	m = pressSettle(t, m, "B", "a")
	if len(m.cmdFavs) != 0 || m.dlg == nil || !strings.Contains(m.dlg.msg, "nothing to add") {
		t.Fatalf("no last command: %+v %q", m.cmdFavs, m.dlg.msg)
	}
	m = pressSettle(t, m, "esc")
	m = pressSettle(t, m, ":")
	m = typeText(t, m, "pulse")
	m = pressSettle(t, m, "enter", "B", "a")
	if len(m.cmdFavs) != 1 || m.cmdFavs[0].Command != "pulse" {
		t.Fatalf("add last: %+v", m.cmdFavs)
	}
	m = pressSettle(t, m, "a")
	if len(m.cmdFavs) != 1 || m.dlg.msg != "already saved" {
		t.Fatalf("duplicate: %+v %q", m.cmdFavs, m.dlg.msg)
	}
	m = pressSettle(t, m, "esc", ":")
	m = typeText(t, m, "cmds")
	m = pressSettle(t, m, "enter", "esc")
	if m.lastCmd != "pulse" {
		t.Fatalf("the cmds word must not become the last command: %q", m.lastCmd)
	}
}

func TestCmdFavoritesEditRenameDeleteMove(t *testing.T) {
	cfg := testEnv(t)
	cfg.CommandFavs = []config.CommandFavorite{{Name: "fixed", Command: "pulse"}}
	m := newTestModel(t, cfg, appCluster(t))
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	m.cmdFavs = []state.CommandFavorite{{Command: "helm"}, {Command: "logs app=a"}}
	m = pressSettle(t, m, "B")
	if len(m.dlg.options) != 3 || !strings.Contains(m.dlg.notes[0], "config") {
		t.Fatalf("entries %v %v", m.dlg.options, m.dlg.notes)
	}

	m = pressSettle(t, m, "e")
	if m.dlg.hasIn || !strings.Contains(m.dlg.msg, "config.yaml") {
		t.Fatalf("config entries are read-only: %+v", m.dlg)
	}

	m.dlg.idx = 1
	m = pressSettle(t, m, "e")
	if !m.dlg.hasIn || m.dlg.input.Value() != "helm" {
		t.Fatalf("edit field %+v", m.dlg)
	}
	m = pressSettle(t, m, "backspace", "backspace", "backspace", "backspace")
	m = typeText(t, m, "logs app=b -c main")
	m = pressSettle(t, m, "enter")
	if m.dlg.hasIn || m.cmdFavs[0].Command != "logs app=b -c main" {
		t.Fatalf("edit: %+v hasIn %v", m.cmdFavs, m.dlg.hasIn)
	}

	m = pressSettle(t, m, "e")
	m = pressSettle(t, m, "esc")
	if m.dlg == nil || m.dlg.hasIn || m.cmdFavs[0].Command != "logs app=b -c main" {
		t.Fatalf("esc keeps the text: %+v", m.cmdFavs)
	}

	m = pressSettle(t, m, "n")
	if !m.dlg.hasIn || m.dlg.input.Value() != "" {
		t.Fatalf("rename field %+v", m.dlg)
	}
	m = typeText(t, m, "b logs")
	m = pressSettle(t, m, "enter")
	if m.cmdFavs[0].Name != "b logs" || !strings.Contains(ansi.Strip(m.View()), "2 b logs") {
		t.Fatalf("rename: %+v", m.cmdFavs)
	}
	m = pressSettle(t, m, "n", "ctrl+u", "enter")
	if m.cmdFavs[0].Name != "" {
		t.Fatalf("clear label: %+v", m.cmdFavs)
	}

	m = pressSettle(t, m, "J")
	if m.cmdFavs[1].Command != "logs app=b -c main" || m.dlg.idx != 2 {
		t.Fatalf("move: %+v idx %d", m.cmdFavs, m.dlg.idx)
	}
	st := state.Load(cfg.StateDir)
	if len(st.CommandFavorites) != 2 || st.CommandFavorites[1].Command != "logs app=b -c main" {
		t.Fatalf("persisted %+v", st.CommandFavorites)
	}

	m = pressSettle(t, m, "x")
	if len(m.cmdFavs) != 1 || m.cmdFavs[0].Command != "logs app=a" {
		t.Fatalf("remove: %+v", m.cmdFavs)
	}
	m.dlg.idx = 0
	m = pressSettle(t, m, "x")
	if len(m.cmdFavs) != 1 || !strings.Contains(m.dlg.msg, "config.yaml") {
		t.Fatalf("config entries stay: %+v %q", m.cmdFavs, m.dlg.msg)
	}
}

func TestCmdFavoritesPersistAcrossRestart(t *testing.T) {
	cfg := testEnv(t)
	m := newTestModel(t, cfg, nil)
	m = open(t, m, "cmds add logs app=web -c main")
	m = pressSettle(t, m, "B", "n")
	m = typeText(t, m, "web")
	m = pressSettle(t, m, "enter")

	again := newTestModel(t, cfg, nil)
	if len(again.cmdFavs) != 1 || again.cmdFavs[0] != (state.CommandFavorite{Name: "web", Command: "logs app=web -c main"}) {
		t.Fatalf("after restart %+v", again.cmdFavs)
	}
}

func TestCmdFavoritesReadOnlyBlocksLikeTyping(t *testing.T) {
	m := appModel(t, appCluster(t))
	m.cmdFavs = []state.CommandFavorite{{Command: "helm install web bitnami/nginx"}}
	m.readOnly = true
	typed := open(t, m, "helm install web bitnami/nginx")
	m = pressSettle(t, m, "B", "enter")
	if m.dlg != nil || m.status != typed.status || !strings.HasPrefix(m.status, "read-only") {
		t.Fatalf("favorite %q, typed %q", m.status, typed.status)
	}
}

func TestCmdFavoritesFromConfigFile(t *testing.T) {
	c, w := config.Parse([]byte("favorite_commands:\n  - {name: web logs, command: \":logs app=web\"}\n  - pulse\n  - {name: nothing}\n  - {command: helm, color: red}\n"))
	if len(c.CommandFavs) != 3 || c.CommandFavs[0] != (config.CommandFavorite{Name: "web logs", Command: "logs app=web"}) || c.CommandFavs[1].Command != "pulse" || c.CommandFavs[2].Command != "helm" {
		t.Fatalf("parsed %+v", c.CommandFavs)
	}
	if len(w) != 2 {
		t.Fatalf("warnings %v", w)
	}
	m := appModel(t, appCluster(t))
	m.cfg.CommandFavs = c.CommandFavs
	m = pressSettle(t, m, "B", "2")
	if m.stage != "pulse" {
		t.Fatalf("stage %s", m.stage)
	}
}

func TestCmdFavoritesKeysInTextFields(t *testing.T) {
	m := appModel(t, appCluster(t))
	m = pressSettle(t, m, ":")
	m = typeText(t, m, "B")
	if m.dlg != nil || m.cmd.Value() != "B" {
		t.Fatalf("B in the command line: %v %q", m.dlg != nil, m.cmd.Value())
	}
}

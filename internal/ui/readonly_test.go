package ui

import (
	"strings"
	"testing"

	"k2/internal/forward"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestReadOnlyToggleAndBadge(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	if m.readOnly {
		t.Fatal("starts off")
	}
	next, _ := m.toggleReadOnly()
	m = next.(model)
	if !m.readOnly {
		t.Fatal("should be on")
	}
	lines := sidebarPlain(m, 26, 60)
	joined := strings.Join(lines[:5], "\n")
	plain := ansi.Strip(joined)
	if !strings.Contains(plain, "READ-ONLY") || !strings.Contains(plain, lockIcon) {
		t.Fatalf("read-only badge missing:\n%s", joined)
	}
	next, _ = m.toggleReadOnly()
	m = next.(model)
	body := ansi.Strip(m.sidebarBody(26, 60))
	if m.readOnly || strings.Contains(body, "READ-ONLY") || strings.Contains(body, lockIcon) {
		t.Fatal("badge should hide when off")
	}
}

func TestReadOnlyBlocksMutations(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	next, _ := m.openList("workpods", "pods")
	m = next.(model)
	m.readOnly = true
	if next, _, blocked := m.readOnlyBlock("delete"); !blocked {
		t.Fatal("expected block")
	} else if !strings.Contains(next.(model).status, "read-only") {
		t.Fatalf("status %q", next.(model).status)
	}
	m.readOnly = true
	got, cmd := m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	_ = cmd
	gm := got.(model)
	if gm.ed != nil && gm.ed.live() {
		t.Fatal("shell must not start in read-only")
	}
}

func TestReadOnlyMountainStripKeepsBadge(t *testing.T) {
	m := mountainModel(t, "bottom")
	m.readOnly = true
	lines := strings.Split(ansi.Strip(m.stripBody(130, stripRows)), "\n")
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "READ-ONLY") || !strings.Contains(joined, lockIcon) {
		t.Fatalf("strip badge missing:\n%s", joined)
	}
	if len(lines) > stripRows {
		t.Fatalf("strip grew to %d rows", len(lines))
	}
}

func TestKustomizeKeysDefaults(t *testing.T) {
	k := newTestModel(t, testEnv(t), nil).cfg.Keys
	if !k.Apply.Has("A") || k.Apply.Has("a") {
		t.Fatalf("apply %v", k.Apply)
	}
	if !k.KustomizeDelete.Has("ctrl+d") || k.KustomizeDelete.Has("d") {
		t.Fatalf("kust delete %v", k.KustomizeDelete)
	}
}

func TestReadOnlyBlocksApplyAndDeleteKeys(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	m.kubeconfig, m.context = "/kc", "kind-dev"
	_ = m.showKList("/k", []string{"/k/overlays/dev"})
	m.readOnly = true
	if got := press(t, m, "A"); got.asking() != "" || !strings.Contains(got.status, "read-only") {
		t.Fatalf("apply: ask %q status %q", got.asking(), got.status)
	}
	if got := press(t, m, "ctrl+d"); got.asking() != "" || !strings.Contains(got.status, "read-only") {
		t.Fatalf("delete: ask %q status %q", got.asking(), got.status)
	}
	m.readOnly = false
	if got := press(t, m, "A"); got.asking() == "" {
		t.Fatal("apply should work again")
	}
}

func TestReadOnlyBlocksKustomizeEditAndFolder(t *testing.T) {
	root := kustTree(t)
	l := &launches{}
	m := browseModel(t, root)
	m.launch = l.fake
	selectTitle(t, &m, "deploy.yaml")
	m.readOnly = true
	if got := press(t, m, "e"); (got.ed != nil && got.ed.live()) || !strings.Contains(got.status, "read-only") {
		t.Fatalf("edit: status %q live=%v", got.status, got.ed != nil && got.ed.live())
	}
	if got := press(t, m, "."); len(l.dirs) != 0 || !strings.Contains(got.status, "read-only") {
		t.Fatalf("folder: launched %v status %q", l.dirs, got.status)
	}
}

func TestReadOnlyBlocksScaleEditExecKubectlAndConfirm(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	m.stage, m.kind = "res", "deployments"
	m.content.SetItems([]list.Item{item{id: "web", title: "web", cells: []string{"web", "1/1", "1", "1", "1d"}, heads: []string{"NAME", "READY", "UP-TO-DATE", "AVAILABLE", "AGE"}, extra: map[string]string{"replicas": "1"}}})
	m.readOnly = true

	if got := press(t, m, "S"); got.dlg != nil || !strings.Contains(got.status, "read-only") {
		t.Fatalf("scale: dlg=%v status %q", got.dlg != nil, got.status)
	}
	if got := press(t, m, "e"); (got.ed != nil && got.ed.live()) || !strings.Contains(got.status, "read-only") {
		t.Fatalf("edit: status %q", got.status)
	}

	m.stage, m.kind = "workpods", "pods"
	m.content.SetItems([]list.Item{item{id: "web-0", title: "web-0", cells: []string{"web-0", "1/1", "Running", "0", "1d"}, heads: []string{"NAME", "READY", "STATUS", "RESTARTS", "AGE"}, extra: map[string]string{"containers": "app"}}})
	if got := press(t, m, "s"); (got.ed != nil && got.ed.live()) || !strings.Contains(got.status, "read-only") {
		t.Fatalf("shell: status %q", got.status)
	}

	m.stage = "kcmd"
	m.sh.SetValue("get pods")
	m.sh.Focus()
	got, _ := m.updateShell(key(t, "enter"))
	gm := got.(model)
	if gm.stage == "kout" || !strings.Contains(gm.status, "read-only") {
		t.Fatalf("kubectl submit: stage %q status %q", gm.stage, gm.status)
	}

	m = newTestModel(t, testEnv(t), nil)
	m = m.confirmDialog("↻ rollout restart", "restart?", "deploy", "shop", "web", "restarting…", nil)
	m.readOnly = true
	got2 := press(t, m, "y")
	if !strings.Contains(got2.status, "read-only") {
		t.Fatalf("confirm yes: status %q ask %q", got2.status, got2.asking())
	}
}

func TestReadOnlyBlocksForwardDeleteAndBulk(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	m.kubeconfig, m.context = "/kc", "kind-dev"
	m.stage = "fwds"
	m.forwards = []forward.Forward{forwardFor(1, "8080", "80", "pod", "web")}
	m.content.SetItems(m.forwardItems())
	m.readOnly = true
	if got := press(t, m, "ctrl+d"); got.asking() != "" || !strings.Contains(got.status, "read-only") {
		t.Fatalf("fwd delete: ask %q status %q", got.asking(), got.status)
	}
	if got := press(t, m, "D"); got.asking() != "" || !strings.Contains(got.status, "read-only") {
		t.Fatalf("fwd delete all: ask %q status %q", got.asking(), got.status)
	}
}

func TestReadOnlyKeyIgnoredInFilter(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	m.kubeconfig, m.context = "/kc", "kind-dev"
	_ = m.showKList("/k", []string{"/k/overlays/dev"})
	m = press(t, m, "/")
	if !m.listFiltering() {
		t.Fatal("expected filter")
	}
	got := press(t, m, "R")
	if got.readOnly {
		t.Fatal("R in filter must type, not toggle read-only")
	}
}

func TestReadOnlyAllowsBrowse(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	m.kubeconfig, m.context = "/kc", "kind-dev"
	m.readOnly = true
	got := press(t, m, "c")
	if strings.Contains(got.status, "read-only") {
		t.Fatalf("context browse blocked: %q", got.status)
	}
}

func TestReadOnlyBadgeNarrow(t *testing.T) {
	cfg := testEnv(t).UI
	cfg.Logo = "wordmark"
	rows := brandRowsLocked(cfg, 12, true, true)
	joined := strings.Join(mapStrip(rows), "\n")
	if !strings.Contains(joined, "READ-ONLY") {
		t.Fatalf("narrow badge:\n%s", joined)
	}
	rows = brandRowsLocked(cfg, 40, true, true)
	joined = strings.Join(mapStrip(rows), "\n")
	if !strings.Contains(joined, lockIcon) || !strings.Contains(joined, "READ-ONLY") {
		t.Fatalf("wide badge:\n%s", joined)
	}
	off := brandRowsLocked(cfg, 40, true, false)
	if strings.Contains(strings.Join(mapStrip(off), "\n"), "READ-ONLY") {
		t.Fatal("badge when off")
	}
}

func mapStrip(rows []string) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = ansi.Strip(r)
	}
	return out
}

func TestReadOnlyLegendLabel(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	m.readOnly = true
	if !strings.Contains(ansi.Strip(m.legend()), "READ-ONLY") {
		t.Fatalf("legend %q", ansi.Strip(m.legend()))
	}
}

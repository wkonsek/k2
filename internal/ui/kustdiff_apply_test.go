package ui

import (
	"path/filepath"
	"strings"
	"testing"

	"k2/internal/shell"

	"github.com/charmbracelet/x/ansi"
)

func kustBrowse(t *testing.T, dir string, f *shell.Fake) model {
	t.Helper()
	m := newTestModel(t, testEnv(t), f)
	m.kubeconfig, m.context = "/kc", "kind-dev"
	m.cfg.UI.EditorInline = false
	m.cfg.UI.Toasts = true
	_ = m.showKDir(dir)
	if m.stage != "kdir" {
		t.Fatalf("stage %q", m.stage)
	}
	return m
}

func openKustDiff(t *testing.T, m model) model {
	t.Helper()
	next, cmd := m.Update(key(t, "D"))
	m = settle(t, next.(model), cmd)
	if m.stage != "kout" || m.subject != "diff" || m.kfile == "" {
		t.Fatalf("diff: stage %q subject %q kfile %q", m.stage, m.subject, m.kfile)
	}
	return m
}

func TestApplyFromKustomizeDiff(t *testing.T) {
	root := kustTree(t)
	f := kustCluster()
	m := kustBrowse(t, root, f)
	selectTitle(t, &m, "kustomization.yaml")
	m = openKustDiff(t, m)
	if m.kback != "kdir" {
		t.Fatalf("kback %q", m.kback)
	}
	diffText := m.logText
	if !strings.Contains(diffText, "Deployment shop/web") && !strings.Contains(diffText, "replicas") {
		t.Fatalf("expected diff body:\n%s", diffText)
	}

	fromList := kustBrowse(t, root, kustCluster())
	selectTitle(t, &fromList, "kustomization.yaml")
	fromList = press(t, fromList, "A")
	got := press(t, m, "A")
	if got.asking() != "⇪ kustomize apply" {
		t.Fatalf("confirm title %q", got.asking())
	}
	view := ansi.Strip(got.View())
	wantQ := "apply . on kind-dev?"
	if !strings.Contains(view, wantQ) || !strings.Contains(view, "y / n") || !strings.Contains(view, "default no") {
		t.Fatalf("confirm view missing prompt:\n%s", view)
	}
	if !strings.Contains(view, filepath.Clean(root)) && !strings.Contains(view, root) {
		t.Fatalf("confirm missing path:\n%s", view)
	}
	if fromList.asking() != got.asking() {
		t.Fatalf("list title %q vs diff title %q", fromList.asking(), got.asking())
	}
	listView := ansi.Strip(fromList.View())
	if !strings.Contains(listView, wantQ) {
		t.Fatalf("list confirm question missing:\n%s", listView)
	}

	cancelled := press(t, got, "n")
	if cancelled.dlg != nil || cancelled.stage != "kout" || cancelled.subject != "diff" || cancelled.logText != diffText {
		t.Fatalf("cancel n: dlg=%v stage %q subject %q log changed=%v status %q", cancelled.dlg != nil, cancelled.stage, cancelled.subject, cancelled.logText != diffText, cancelled.status)
	}
	if cancelled.status != "cancelled" {
		t.Fatalf("cancel status %q", cancelled.status)
	}

	again := press(t, cancelled, "A")
	escaped := press(t, again, "esc")
	if escaped.dlg != nil || escaped.stage != "kout" || escaped.subject != "diff" || escaped.logText != diffText {
		t.Fatalf("cancel esc: dlg=%v stage %q subject %q", escaped.dlg != nil, escaped.stage, escaped.subject)
	}

	next, cmd := escaped.Update(key(t, "A"))
	m = next.(model)
	if m.asking() != "⇪ kustomize apply" {
		t.Fatalf("reopen confirm %q", m.asking())
	}
	next, cmd = m.Update(key(t, "y"))
	m = settle(t, next.(model), cmd)
	if m.stage != "kout" || m.subject != "apply" {
		t.Fatalf("after apply: stage %q subject %q", m.stage, m.subject)
	}
	if m.kback != "kdir" {
		t.Fatalf("kback overwritten: %q", m.kback)
	}
	if calls := mutations(f); len(calls) == 0 {
		t.Fatal("apply was not sent")
	} else {
		joined := strings.Join(calls, " ")
		if !strings.Contains(joined, "apply") {
			t.Fatalf("mutations %q", joined)
		}
	}
	view = ansi.Strip(m.View())
	if !strings.Contains(view, "exit 0") || (!strings.Contains(m.logText, "created") && !strings.Contains(view, "applied")) {
		t.Fatalf("apply result missing:\n%s", view)
	}
	if strings.Contains(ansi.Strip(m.legend()), "A apply") {
		t.Fatalf("apply result should not advertise A apply: %q", ansi.Strip(m.legend()))
	}
	m = press(t, m, "q")
	if m.stage != "kdir" {
		t.Fatalf("back after apply: stage %q", m.stage)
	}
}

func TestApplyFromKustomizeDiffPlainYAML(t *testing.T) {
	root := kustTree(t)
	m := kustBrowse(t, root, kustCluster())
	selectTitle(t, &m, "deploy.yaml")
	m = openKustDiff(t, m)
	got := press(t, m, "A")
	if got.asking() != "⇪ kubectl apply" {
		t.Fatalf("title %q", got.asking())
	}
	view := ansi.Strip(got.View())
	if !strings.Contains(view, "apply deploy.yaml on kind-dev?") {
		t.Fatalf("question missing:\n%s", view)
	}
}

func TestApplyFromKustomizeDiffReadOnly(t *testing.T) {
	root := kustTree(t)
	m := kustBrowse(t, root, kustCluster())
	selectTitle(t, &m, "kustomization.yaml")
	m = openKustDiff(t, m)
	diffText := m.logText
	m.readOnly = true
	got := press(t, m, "A")
	if got.asking() != "" || got.dlg != nil {
		t.Fatalf("dialog opened: %q", got.asking())
	}
	if got.status != "read-only: apply blocked" {
		t.Fatalf("status %q", got.status)
	}
	if got.stage != "kout" || got.subject != "diff" || got.logText != diffText {
		t.Fatalf("diff changed: stage %q subject %q", got.stage, got.subject)
	}
}

func TestApplyFromKustomizeDiffProduction(t *testing.T) {
	root := kustTree(t)
	m := kustBrowse(t, root, kustCluster())
	m.cfg.Prod = []string{"kind-dev"}
	selectTitle(t, &m, "kustomization.yaml")
	m = openKustDiff(t, m)
	got := press(t, m, "A")
	view := ansi.Strip(got.View())
	if !got.dlg.danger || !strings.Contains(view, skull) || !strings.Contains(view, "production") {
		t.Fatalf("prod safety missing:\n%s", view)
	}
}

func TestApplyFromKustomizeDiffFooterAndHelp(t *testing.T) {
	root := kustTree(t)
	m := kustBrowse(t, root, kustCluster())
	selectTitle(t, &m, "kustomization.yaml")
	if strings.Contains(ansi.Strip(m.legend()), "A apply") {
		// list should show it
	} else {
		t.Fatalf("list should show A apply: %q", ansi.Strip(m.legend()))
	}
	m = openKustDiff(t, m)
	leg := ansi.Strip(m.legend())
	if !strings.Contains(leg, "A apply") {
		t.Fatalf("diff footer missing A apply: %q", leg)
	}
	h := m
	next, _ := h.openHelp()
	h = next.(model)
	text := helpText(h)
	if !strings.Contains(text, "apply the same kustomization") {
		t.Fatalf("diff help missing apply:\n%s", text)
	}

	m.subject = "apply"
	if strings.Contains(ansi.Strip(m.legend()), "A apply") {
		t.Fatalf("apply result advertised A apply: %q", ansi.Strip(m.legend()))
	}
	next, _ = m.openHelp()
	if strings.Contains(helpText(next.(model)), "apply the same kustomization") {
		t.Fatalf("apply help still has apply:\n%s", helpText(next.(model)))
	}

	m.subject, m.kfile = "kubectl", ""
	if strings.Contains(ansi.Strip(m.legend()), "A apply") {
		t.Fatalf("kubectl kout advertised A apply")
	}
	got := press(t, m, "A")
	if got.asking() != "" {
		t.Fatalf("A on kubectl kout opened %q", got.asking())
	}
}

func TestApplyFromKustomizeListUnchanged(t *testing.T) {
	root := kustTree(t)
	f := kustCluster()
	m := kustBrowse(t, root, f)
	selectTitle(t, &m, "kustomization.yaml")
	got := press(t, m, "A")
	if got.asking() != "⇪ kustomize apply" || got.stage != "kdir" {
		t.Fatalf("list apply: stage %q title %q", got.stage, got.asking())
	}
	next, cmd := got.Update(key(t, "y"))
	m = settle(t, next.(model), cmd)
	if m.stage != "kout" || m.subject != "apply" || m.kback != "kdir" {
		t.Fatalf("list apply result: stage %q subject %q kback %q", m.stage, m.subject, m.kback)
	}
}

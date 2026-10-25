package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestKustomizeDeleteIsD(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	m.kubeconfig, m.context = "/kc", "kind-dev"
	_ = m.showKList("/k", []string{"/k/overlays/dev"})
	if m.stage != "kust" {
		t.Fatalf("stage %q", m.stage)
	}
	if got := press(t, m, "ctrl+d"); got.asking() != "" || got.stage != "kust" {
		t.Fatalf("ctrl+d: confirm %q stage %q", got.asking(), got.stage)
	}
	m = press(t, m, "d")
	if m.asking() != "✗ kustomize delete" || !strings.Contains(ansi.Strip(m.View()), "delete overlays/dev on kind-dev?  y / n") {
		t.Fatalf("d: confirm %q view:\n%s", m.asking(), ansi.Strip(m.View()))
	}
	if got := press(t, m, "n"); got.asking() != "" || got.stage != "kust" {
		t.Fatalf("n: confirm %q stage %q", got.asking(), got.stage)
	}
	if got := press(t, m, "y"); got.stage != "kout" || got.subject != "delete" {
		t.Fatalf("y: stage %q subject %q", got.stage, got.subject)
	}
	if got := press(t, m, "esc"); got.asking() != "" {
		t.Fatalf("esc did not cancel: %q", got.asking())
	}
}

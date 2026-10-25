package ui

import (
	"strings"
	"testing"
)

func TestSidebarNeverFocused(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	if m.stage != "nav" {
		t.Fatalf("start stage %q", m.stage)
	}
	at := m.nav.Index()
	m = press(t, m, "j", "j", "down", "k")
	if m.nav.Index() != at {
		t.Fatalf("j/k moved the sidebar cursor %d → %d", at, m.nav.Index())
	}

	m = press(t, m, "ctrl+a", "h")
	seen := map[string]bool{}
	for i := 0; i < 4; i++ {
		m = press(t, m, "ctrl+a", "o")
		seen[m.logID] = true
		m = press(t, m, "ctrl+left", "ctrl+up")
		seen[m.logID] = true
	}
	if len(seen) != 2 {
		t.Fatalf("focus visited %d places, want the 2 panes", len(seen))
	}

	for cmd, stage := range map[string]string{
		"kubeconfig": "kube", "kc": "kube", "context": "ctx", "ctx": "ctx",
		"namespace": "proj", "ns": "proj", "kustomize": "kust", "k": "kust",
		"kubectl": "sh", "forwards": "fwds", "fwd": "fwds", "pf": "fwds", "f": "fwds",
	} {
		one := newTestModel(t, testEnv(t), nil)
		one.kubeconfig, one.context = "/kc", "kind-dev"
		one = press(t, one, append([]string{":"}, append(strings.Split(cmd, ""), "enter")...)...)
		if got := one.navTarget(); got != stage {
			t.Errorf(":%s opened %q (stage %q), want %q", cmd, got, one.stage, stage)
		}
	}
}

func (m model) navTarget() string {
	if sel := m.nav.SelectedItem(); sel != nil {
		return sel.(navItem).id
	}
	return ""
}

func TestForwardCommandOpensPods(t *testing.T) {
	for _, cmd := range []string{"forward", "p"} {
		m := v4Model(t, v4Cluster())
		m = pressSettle(t, m, append([]string{":"}, append(strings.Split(cmd, ""), "enter")...)...)
		if m.stage != "workpods" || strings.HasPrefix(m.status, "unknown") {
			t.Errorf(":%s: stage %q status %q", cmd, m.stage, m.status)
		}
	}
}

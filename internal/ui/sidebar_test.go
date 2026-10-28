package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k2/internal/forward"

	"github.com/charmbracelet/x/ansi"
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
		m := actionModel(t, actionCluster())
		m = pressSettle(t, m, append([]string{":"}, append(strings.Split(cmd, ""), "enter")...)...)
		if m.stage != "workpods" || strings.HasPrefix(m.status, "unknown") {
			t.Errorf(":%s: stage %q status %q", cmd, m.stage, m.status)
		}
	}
}

func TestSidebarIdentity(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	m.kubeconfig = filepath.Join(home, ".kube", "dppreprod1")
	m.context = "kind-preprod"
	m.namespace = "dp-preprod3"
	m.forwards = []forward.Forward{
		{Namespace: "shop", Pod: "web", Local: "8080", Remote: "80", PID: os.Getpid()},
		{Namespace: "shop", Pod: "db", Local: "9090", Remote: "90", PID: -1, Reason: "pod gone"},
	}
	plain := ansi.Strip(m.sidebarBody(26, 40))
	if strings.Contains(plain, "active ·") || strings.Contains(plain, "⇄") || strings.Contains(plain, "▟") || strings.HasPrefix(plain, " ") {
		t.Fatalf("old chrome:\n%s", plain)
	}
	if !strings.HasPrefix(plain, wordmark[0]+"\n"+wordmark[1]+"\nkubectl, elevated\n") {
		t.Fatalf("wordmark:\n%s", plain)
	}
	ctx := strings.Index(plain, "kind-preprod")
	path := strings.Index(plain, "~/.kube/dppreprod1")
	ns := strings.Index(plain, "dp-preprod3")
	menu := strings.Index(plain, "Kubeconfig")
	ver := strings.Index(plain, "k8s server")
	head := strings.Index(plain, "\nforwards\n")
	fwd := strings.Index(plain, "8080 → 80")
	if ctx < 0 || path < ctx || ns < path || menu < ns || ver < menu || head < ver || fwd < head {
		t.Fatalf("order ctx %d path %d ns %d menu %d ver %d head %d fwd %d\n%s", ctx, path, ns, menu, ver, head, fwd, plain)
	}
	styled := m.sidebarBody(26, 40)
	if !strings.Contains(styled, okStyle.Render(markSquare)) || !strings.Contains(styled, errStyle.Render(markSquare)) {
		t.Fatal("alive dot or dead dot missing")
	}
	if !strings.Contains(styled, keyStyle.Render("C")+itemStyle.Render("  Kubeconfig")) {
		t.Fatal("shortcut has a background or the wrong gap")
	}
	if !strings.Contains(plain, "shop/web") || strings.Contains(plain, "\ncontext\n") || strings.Contains(plain, "\nnamespace\n") {
		t.Fatalf("labels:\n%s", plain)
	}
	empty := newTestModel(t, testEnv(t), nil)
	if strings.Contains(ansi.Strip(empty.sidebarBody(26, 40)), "\nforwards\n") {
		t.Fatal("empty dock still has a heading")
	}
}

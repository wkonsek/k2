package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"k2/internal/shell"
)

func fakeKubectl(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	script := `#!/bin/sh
case "$1" in
kustomize) printf 'kind: Namespace\n---\nkind: Deployment\n---\nkind: Service\n---\nkind: ConfigMap\n' ;;
delete)
  echo 'namespace "shop" deleted'; sleep 0.15
  echo 'deployment.apps "web" deleted from shop namespace'; sleep 0.15
  echo 'Error from server (NotFound): configmaps "cfg" not found' >&2; sleep 0.15
  echo 'service "web" deleted from shop namespace'
  sleep 0.8
  exit 1 ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "kubectl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestKustomizeDeleteProgress(t *testing.T) {
	fakeKubectl(t)
	cfg := testEnv(t)
	m := initialModel(Options{Runner: shell.Exec{}, Config: &cfg})
	m.width, m.height = 120, 40
	m.kubeconfig, m.context, m.kroot = "/kc", "kind-dev", "/k"
	m.subject, m.stage = "delete", "kout"
	m.startLoad("deleting", false)

	type step struct {
		text string
		at   time.Time
	}
	var steps []step
	start := time.Now()
	msg := m.kustomizeCmd("delete", "/k/app")()
	for {
		if out, ok := msg.(kOutMsg); ok {
			steps = append(steps, step{"finished", time.Now()})
			nm, _ := m.Update(out)
			m = nm.(model)
			break
		}
		nm, next := m.Update(msg)
		m = nm.(model)
		steps = append(steps, step{strings.SplitN(loadText(m.load), " · ", 2)[0], time.Now()})
		if strings.HasPrefix(loadText(m.load), "waiting") && !strings.Contains(ansi.Strip(m.View()), "waiting for 3 to terminate…") {
			t.Fatalf("waiting phase not shown:\n%s", ansi.Strip(m.View()))
		}
		msg = next()
	}
	var texts []string
	for _, s := range steps {
		texts = append(texts, s.text)
	}
	want := "deleting 0/4|deleting 1/4|deleting 2/4|deleting 3/4|deleting 4/4|waiting for 3 to terminate… 0.0s|finished"
	got := strings.Join(texts, "|")

	if len(texts) != 7 || strings.Join(texts[:5], "|") != strings.Join(strings.Split(want, "|")[:5], "|") || !strings.HasPrefix(texts[5], "waiting for 3 to terminate…") || texts[6] != "finished" {
		t.Fatalf("steps %q", got)
	}

	if first, last := steps[1].at.Sub(start), steps[6].at.Sub(steps[5].at); first > 500*time.Millisecond || last < 500*time.Millisecond {
		t.Fatalf("first progress after %v, waiting phase lasted %v", first, last)
	}
	view := ansi.Strip(m.View())
	for _, want := range []string{
		`− deployment.apps "web" deleted from shop namespace`,
		`− namespace "shop" deleted`,
		"│ − 3 deleted   ✗ 1 error ",
		"│ ✗ delete failed — see error above",
		"── exit 1 · failed ──",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q in\n%s", want, view)
		}
	}
}

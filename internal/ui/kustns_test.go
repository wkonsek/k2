package ui

import (
	"strings"
	"sync"
	"testing"

	"k2/internal/shell"

	"github.com/charmbracelet/x/ansi"
)

func missingNSFake(ns string, createErr string) *shell.Fake {
	var mu sync.Mutex
	created := map[string]bool{}
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		switch {
		case len(r.Args) >= 3 && r.Args[0] == "create" && r.Args[1] == "namespace":
			name := r.Args[2]
			if createErr != "" {
				return shell.Fail(1, createErr)
			}
			mu.Lock()
			created[name] = true
			mu.Unlock()
			return shell.OK("namespace/" + name + " created")
		case len(r.Args) > 0 && r.Args[0] == "kustomize":
			return shell.OK("kind: Namespace\n---\nkind: Deployment\n---\nkind: Service\n")
		case len(r.Args) > 0 && r.Args[0] == "apply":
			mu.Lock()
			ok := created[ns]
			mu.Unlock()
			if !ok {
				return shell.Fail(1, `Error from server (NotFound): namespaces "`+ns+`" not found`)
			}
			return shell.OK("namespace/shop unchanged\ndeployment.apps/web created\nservice/web configured\n")
		case len(r.Args) > 0 && r.Args[0] == "diff":
			mu.Lock()
			ok := created[ns]
			mu.Unlock()
			if !ok {
				return shell.Fail(1, `Error from server (NotFound): namespaces "`+ns+`" not found`)
			}
			return shell.Result{Stdout: []byte(kustDiff), Code: 1, Err: errExit1}
		}
		return shell.Fail(1, "not in this test")
	}}
}

func missingManyNSFake(names ...string) *shell.Fake {
	var mu sync.Mutex
	created := map[string]bool{}
	errText := func() string {
		var b strings.Builder
		for _, n := range names {
			if created[n] {
				continue
			}
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(`Error from server (NotFound): namespaces "` + n + `" not found`)
		}
		return b.String()
	}
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		switch {
		case len(r.Args) >= 3 && r.Args[0] == "create" && r.Args[1] == "namespace":
			mu.Lock()
			created[r.Args[2]] = true
			mu.Unlock()
			return shell.OK("namespace/" + r.Args[2] + " created")
		case len(r.Args) > 0 && r.Args[0] == "kustomize":
			return shell.OK("kind: Deployment\n")
		case len(r.Args) > 0 && (r.Args[0] == "apply" || r.Args[0] == "diff"):
			mu.Lock()
			text := errText()
			mu.Unlock()
			if text != "" {
				return shell.Fail(1, text)
			}
			if r.Args[0] == "apply" {
				return shell.OK("deployment.apps/web created\n")
			}
			return shell.Result{Stdout: []byte(kustDiff), Code: 1, Err: errExit1}
		}
		return shell.Fail(1, "not in this test")
	}}
}

func openMissingNSDiff(t *testing.T, f *shell.Fake) (model, *shell.Fake, string) {
	t.Helper()
	root := kustTree(t)
	m := kustBrowse(t, root, f)
	selectTitle(t, &m, "kustomization.yaml")
	m = openKustDiff(t, m)
	return m, f, root
}

func assertCreatePrompt(t *testing.T, m model, title, ns string, many bool) {
	t.Helper()
	if m.asking() != title {
		t.Fatalf("title %q", m.asking())
	}
	view := ansi.Strip(m.View())
	wantQ := "create namespace " + ns + "?"
	if many {
		wantQ = "create namespaces " + ns + "?"
	}
	for _, w := range []string{wantQ, "y / n", "default no", "kind-dev", "re-run kustomize"} {
		if !strings.Contains(view, w) {
			t.Fatalf("confirm missing %q:\n%s", w, view)
		}
	}
	if !strings.Contains(m.logText, `namespaces "`) && !strings.Contains(view, `namespaces "`) {
		t.Fatalf("error output hidden under confirm:\n%s", view)
	}
}

func TestCreateNamespacePromptAfterKustomizeDiff(t *testing.T) {
	m, f, _ := openMissingNSDiff(t, missingNSFake("cluster1", ""))
	assertCreatePrompt(t, m, "+ create namespace", "cluster1", false)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "re-run kustomize diff") {
		t.Fatalf("then line missing:\n%s", view)
	}
	if !strings.Contains(view, "cluster") || !strings.Contains(view, "context") || !strings.Contains(view, "namespace") {
		t.Fatalf("detail labels missing:\n%s", view)
	}

	stayed := press(t, m, "enter")
	if stayed.asking() != "+ create namespace" {
		t.Fatalf("enter dismissed: %q", stayed.asking())
	}

	cancelled := press(t, m, "n")
	if cancelled.dlg != nil || cancelled.stage != "kout" || cancelled.subject != "diff" {
		t.Fatalf("cancel n: dlg=%v stage %q subject %q", cancelled.dlg != nil, cancelled.stage, cancelled.subject)
	}
	if cancelled.status != "cancelled" {
		t.Fatalf("cancel status %q", cancelled.status)
	}
	if !strings.Contains(cancelled.logText, `namespaces "cluster1" not found`) {
		t.Fatalf("diff output lost:\n%s", cancelled.logText)
	}
	if creates := calls(f, "create"); len(creates) != 0 {
		t.Fatalf("create after n: %v", creates)
	}
}

func TestCreateNamespaceCancelEscKeepsDiff(t *testing.T) {
	m, f, _ := openMissingNSDiff(t, missingNSFake("cluster1", ""))
	diffText := m.logText
	got := press(t, m, "esc")
	if got.dlg != nil || got.stage != "kout" || got.subject != "diff" || got.logText != diffText {
		t.Fatalf("esc: dlg=%v stage %q subject %q log changed=%v", got.dlg != nil, got.stage, got.subject, got.logText != diffText)
	}
	if len(calls(f, "create")) != 0 {
		t.Fatal("create after esc")
	}
}

func TestCreateNamespaceYesRerunsDiff(t *testing.T) {
	m, f, _ := openMissingNSDiff(t, missingNSFake("cluster1", ""))
	if n := len(calls(f, "diff")); n != 1 {
		t.Fatalf("diffs before yes: %d", n)
	}
	next, cmd := m.Update(key(t, "y"))
	m = settle(t, next.(model), cmd)
	if m.dlg != nil {
		t.Fatalf("dialog still open: %q", m.asking())
	}
	if m.stage != "kout" || m.subject != "diff" {
		t.Fatalf("after y: stage %q subject %q", m.stage, m.subject)
	}
	creates := calls(f, "create")
	if len(creates) != 1 || !strings.Contains(creates[0], "create namespace cluster1") || !strings.Contains(creates[0], "--kubeconfig /kc") || !strings.Contains(creates[0], "--context kind-dev") {
		t.Fatalf("create calls %v", creates)
	}
	if n := len(calls(f, "diff")); n != 2 {
		t.Fatalf("diffs after yes: %d %v", n, calls(f, "diff"))
	}
	if !strings.Contains(m.logText, "replicas") && !strings.Contains(m.logText, "Deployment shop/web") {
		t.Fatalf("rerun diff missing body:\n%s", m.logText)
	}
	if strings.Contains(m.logText, `namespaces "cluster1" not found`) {
		t.Fatalf("stale missing-ns error:\n%s", m.logText)
	}
}

func TestCreateNamespacePromptAfterKustomizeApply(t *testing.T) {
	root := kustTree(t)
	f := missingNSFake("cluster1", "")
	m := kustBrowse(t, root, f)
	selectTitle(t, &m, "kustomization.yaml")
	got := press(t, m, "A")
	if got.asking() != "⇪ kustomize apply" {
		t.Fatalf("apply confirm %q", got.asking())
	}
	next, cmd := got.Update(key(t, "y"))
	m = settle(t, next.(model), cmd)
	assertCreatePrompt(t, m, "+ create namespace", "cluster1", false)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "re-run kustomize apply") {
		t.Fatalf("then line missing:\n%s", view)
	}
	if !strings.Contains(m.logText, `namespaces "cluster1" not found`) {
		t.Fatalf("apply error hidden:\n%s", m.logText)
	}

	next, cmd = m.Update(key(t, "y"))
	m = settle(t, next.(model), cmd)
	if m.dlg != nil {
		t.Fatalf("dialog still open: %q", m.asking())
	}
	if m.stage != "kout" || m.subject != "apply" {
		t.Fatalf("after create: stage %q subject %q", m.stage, m.subject)
	}
	creates := calls(f, "create")
	if len(creates) != 1 || !strings.Contains(creates[0], "create namespace cluster1") {
		t.Fatalf("create calls %v", creates)
	}
	applies := calls(f, "apply")
	if len(applies) != 2 {
		t.Fatalf("apply calls %d %v", len(applies), applies)
	}
	if !strings.Contains(m.logText, "created") {
		t.Fatalf("rerun apply missing body:\n%s", m.logText)
	}
	view = ansi.Strip(m.View())
	if !strings.Contains(view, "exit 0") {
		t.Fatalf("successful apply missing:\n%s", view)
	}
}

func TestCreateNamespaceCreateError(t *testing.T) {
	m, f, _ := openMissingNSDiff(t, missingNSFake("cluster1", `Error from server (Forbidden): namespaces is forbidden`))
	diffText := m.logText
	next, cmd := m.Update(key(t, "y"))
	m = settle(t, next.(model), cmd)
	if m.dlg != nil {
		t.Fatalf("dialog still open: %q", m.asking())
	}
	if m.stage != "kout" || m.subject != "diff" || m.logText != diffText {
		t.Fatalf("output changed: stage %q subject %q", m.stage, m.subject)
	}
	if !strings.Contains(m.status, "forbidden") && !strings.Contains(m.status, "Forbidden") {
		t.Fatalf("status %q", m.status)
	}
	if m.toast.tone != toneErr || !strings.Contains(m.toast.text, "create namespace failed") {
		t.Fatalf("toast %+v", m.toast)
	}
	if n := len(calls(f, "diff")); n != 1 {
		t.Fatalf("diff reran after failed create: %d", n)
	}
}

func TestCreateNamespaceReadOnlySkipsPrompt(t *testing.T) {
	root := kustTree(t)
	f := missingNSFake("cluster1", "")
	m := kustBrowse(t, root, f)
	selectTitle(t, &m, "kustomization.yaml")
	m.readOnly = true
	m = openKustDiff(t, m)
	if m.dlg != nil || m.asking() != "" {
		t.Fatalf("prompt in read-only: %q", m.asking())
	}
	if m.stage != "kout" || m.subject != "diff" || !strings.Contains(m.logText, `namespaces "cluster1" not found`) {
		t.Fatalf("diff output missing: stage %q subject %q\n%s", m.stage, m.subject, m.logText)
	}
	if len(calls(f, "create")) != 0 {
		t.Fatal("created a namespace in read-only")
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "namespaces \"cluster1\" not found") {
		t.Fatalf("error hidden:\n%s", view)
	}
}

func TestCreateNamespaceReadOnlyBlocksYes(t *testing.T) {
	m, f, _ := openMissingNSDiff(t, missingNSFake("cluster1", ""))
	m.readOnly = true
	got := press(t, m, "y")
	if got.dlg == nil {
		t.Fatal("dialog should stay while read-only blocks yes")
	}
	if !strings.Contains(got.status, "read-only") {
		t.Fatalf("status %q", got.status)
	}
	if len(calls(f, "create")) != 0 {
		t.Fatal("created a namespace after blocked yes")
	}
}

func TestCreateNamespacesPromptAndRerunApply(t *testing.T) {
	root := kustTree(t)
	f := missingManyNSFake("cluster1", "observability")
	m := kustBrowse(t, root, f)
	selectTitle(t, &m, "kustomization.yaml")
	next, cmd := press(t, m, "A").Update(key(t, "y"))
	m = settle(t, next.(model), cmd)
	assertCreatePrompt(t, m, "+ create namespaces", "cluster1, observability", true)
	next, cmd = m.Update(key(t, "y"))
	m = settle(t, next.(model), cmd)
	creates := calls(f, "create")
	if len(creates) != 2 {
		t.Fatalf("create calls %v", creates)
	}
	joined := strings.Join(creates, " | ")
	if !strings.Contains(joined, "create namespace cluster1") || !strings.Contains(joined, "create namespace observability") {
		t.Fatalf("create calls %v", creates)
	}
	if len(calls(f, "apply")) != 2 {
		t.Fatalf("apply calls %v", calls(f, "apply"))
	}
	if m.subject != "apply" || !strings.Contains(m.logText, "created") {
		t.Fatalf("rerun apply:\n%s", m.logText)
	}
}

func TestCreateNamespaceIgnoredOnSuccessfulDiff(t *testing.T) {
	root := kustTree(t)
	m := kustBrowse(t, root, kustCluster())
	selectTitle(t, &m, "kustomization.yaml")
	m = openKustDiff(t, m)
	if m.dlg != nil {
		t.Fatalf("unexpected prompt %q", m.asking())
	}
}

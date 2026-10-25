package ui

import (
	"strings"
	"testing"

	"k2/internal/shell"
)

func TestTeaKubectlRunShowsExitAndDuration(t *testing.T) {
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		if len(r.Args) > 0 && r.Args[0] == "get" {
			return shell.OK("NAME    AGE\nquota   3d\n")
		}
		return shell.Fail(1, "not in this test")
	}}
	m := newTestModel(t, testEnv(t), f)
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	tm := start(t, m)
	send(t, tm, "U", "g", "e", "t", " ", "Q", "q", "enter")
	waitFor(t, tm, "exit 0 · done · ")
	final := finish(t, tm)
	if final.stage != "kout" || !strings.HasPrefix(final.status, "exit 0 · done · ") || !final.ran.set {
		t.Fatalf("stage %q status %q", final.stage, final.status)
	}
	calls := f.Calls()
	if got := calls[len(calls)-1].Line(); !strings.HasPrefix(got, "kubectl get Qq ") {
		t.Fatalf("ran %q", got)
	}
}

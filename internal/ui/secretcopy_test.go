package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"k2/internal/shell"
)

type copyCluster struct {
	secretCluster
	exists  bool
	failCmd string
}

func (c *copyCluster) fake() *shell.Fake {
	base := c.secretCluster.fake()
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		j := strings.Join(r.Args, " ")
		switch {
		case strings.Contains(j, "get projects"):
			return shell.Fail(1, `error: the server doesn't have a resource type "projects"`)
		case strings.Contains(j, "get ns -o"):
			return shell.OK("default\nshop\nstage\nprod\n")
		case strings.Contains(j, "--ignore-not-found"):
			if c.exists {
				return shell.OK("secret/" + r.Args[2])
			}
			return shell.OK("")
		case r.Args[0] == "create" || r.Args[0] == "replace":
			if c.failCmd != "" {
				return shell.Fail(1, c.failCmd)
			}
			return shell.OK("secret created")
		}
		return base.Handler(r)
	}}
}

func copyModel(t *testing.T, c *copyCluster) (model, *shell.Fake) {
	t.Helper()
	f := c.fake()
	m := newTestModel(t, testEnv(t), f)
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	next, cmd := m.openResource("secrets")
	m = settle(t, next.(model), cmd)
	return selectSecret(t, m, "db"), f
}

func applyCalls(f *shell.Fake) []string {
	var out []string
	for _, r := range f.Calls() {
		if r.Name == "kubectl" && len(r.Args) > 0 && (r.Args[0] == "create" || r.Args[0] == "replace") {
			out = append(out, strings.Join(r.Args, " ")+" <<"+r.Stdin)
		}
	}
	return out
}

func TestSecretCopyFlow(t *testing.T) {
	c := &copyCluster{secretCluster: secretCluster{data: map[string]map[string]string{"db": {"password": "hunter2"}}}}
	m, f := copyModel(t, c)
	if leg := ansi.Strip(m.legend()); !strings.Contains(leg, "T copy to ns") || !strings.Contains(leg, "w copy name") {
		t.Fatalf("legend %q", leg)
	}
	m = pressSettle(t, m, "T")
	if m.dlg == nil || m.dlg.act != "secretcopy" || strings.Join(m.dlg.options, ",") != "default,shop,stage,prod" || m.dlg.picked() != "default" {
		t.Fatalf("picker %+v", m.dlg)
	}
	m = typeText(t, m, "st")
	if strings.Join(m.dlg.options, ",") != "stage" {
		t.Fatalf("filter %v", m.dlg.options)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "copy secret db to namespace") || !strings.Contains(view, "type to filter") {
		t.Fatalf("view:\n%s", view)
	}
	m = pressSettle(t, m, "enter")
	if m.dlg == nil || m.dlg.dest != "stage" || m.dlg.input.Value() != "db" {
		t.Fatalf("name step %+v", m.dlg)
	}
	m = pressSettle(t, m, "enter")
	if m.dlg == nil || m.dlg.act != "confirm" || !strings.Contains(m.dlg.ask, "copy secret db to stage?") {
		t.Fatalf("confirm %+v", m.dlg)
	}
	if len(applyCalls(f)) != 0 {
		t.Fatal("copied before y")
	}
	m = pressSettle(t, m, "y")
	got := applyCalls(f)
	if m.dlg != nil || len(got) != 1 || !strings.HasPrefix(got[0], "create -f - -n stage") || strings.Contains(got[0], `"namespace"`) || !strings.Contains(got[0], `"name":"db"`) {
		t.Fatalf("dlg %+v calls %v", m.dlg, got)
	}
	if m.status != "copied secret db to stage" {
		t.Fatalf("status %q", m.status)
	}
}

func TestSecretCopyRenameOverwriteAndBack(t *testing.T) {
	c := &copyCluster{secretCluster: secretCluster{data: map[string]map[string]string{"db": {"password": "hunter2"}}}, exists: true}
	m, f := copyModel(t, c)
	m = pressSettle(t, m, "T", "down", "enter")
	if m.dlg.dest != "shop" {
		t.Fatalf("dest %q", m.dlg.dest)
	}
	m = pressSettle(t, m, "enter")
	if m.dlg == nil || m.dlg.msg != "that is the same secret: pick another namespace or name" {
		t.Fatalf("same %+v", m.dlg)
	}
	m = typeText(t, m, "-copy")
	m = pressSettle(t, m, "enter")
	if m.dlg == nil || m.dlg.act != "confirm" || !strings.Contains(m.dlg.ask, "shop/db-copy exists · overwrite it?") || !m.dlg.danger {
		t.Fatalf("overwrite %+v", m.dlg)
	}
	m = pressSettle(t, m, "esc")
	if m.dlg == nil || m.dlg.act != "secretcopy" || m.dlg.input.Value() != "db-copy" {
		t.Fatalf("back to name %+v", m.dlg)
	}
	m = pressSettle(t, m, "esc")
	if m.dlg == nil || m.dlg.dest != "" || m.dlg.picked() != "shop" {
		t.Fatalf("back to picker %+v", m.dlg)
	}
	m = pressSettle(t, m, "enter", "enter")
	if m.dlg == nil || m.dlg.input.Value() != "db" {
		t.Fatalf("name reset %+v", m.dlg)
	}
	m = typeText(t, m, "-2")
	m = pressSettle(t, m, "enter", "y")
	got := applyCalls(f)
	if len(got) != 1 || !strings.HasPrefix(got[0], "replace -f - -n shop") || !strings.Contains(got[0], `"name":"db-2"`) {
		t.Fatalf("calls %v", got)
	}
}

func TestSecretCopyBadNameAndFailure(t *testing.T) {
	c := &copyCluster{secretCluster: secretCluster{data: map[string]map[string]string{"db": {"password": "hunter2"}}}, failCmd: "forbidden: no create in stage"}
	m, _ := copyModel(t, c)
	m = pressSettle(t, m, "T")
	m = typeText(t, m, "stage")
	m = pressSettle(t, m, "enter", "backspace", "backspace")
	m = typeText(t, m, "B_")
	m = pressSettle(t, m, "enter")
	if m.dlg == nil || !strings.Contains(m.dlg.msg, "lowercase") {
		t.Fatalf("bad name %+v", m.dlg)
	}
	for range "dB_" {
		m = pressSettle(t, m, "backspace")
	}
	m = typeText(t, m, "db")
	m = pressSettle(t, m, "enter", "y")
	if m.dlg != nil || !strings.Contains(m.status, "forbidden: no create in stage") {
		t.Fatalf("status %q dlg %+v", m.status, m.dlg)
	}
}

func TestSecretCopyFilterNoMatch(t *testing.T) {
	c := &copyCluster{secretCluster: secretCluster{data: map[string]map[string]string{"db": {"password": "hunter2"}}}}
	m, _ := copyModel(t, c)
	m = pressSettle(t, m, "T")
	m = typeText(t, m, "zzz")
	m = pressSettle(t, m, "enter")
	if m.dlg == nil || m.dlg.msg != "no namespace matches" || m.dlg.dest != "" {
		t.Fatalf("%+v", m.dlg)
	}
}

func TestSecretCopyReadOnlyAndOtherKinds(t *testing.T) {
	c := &copyCluster{secretCluster: secretCluster{data: map[string]map[string]string{"db": {"password": "hunter2"}}}}
	m, f := copyModel(t, c)
	m = pressSettle(t, m, "R", "T")
	if m.dlg != nil || !strings.Contains(m.status, "read-only: copy secret blocked") {
		t.Fatalf("read-only: dlg %+v status %q", m.dlg, m.status)
	}
	m = pressSettle(t, m, "R", "T")
	if m.dlg == nil {
		t.Fatal("no picker after read-only off")
	}
	m = pressSettle(t, m, "enter", "enter")
	m.readOnly = true
	m = pressSettle(t, m, "y")
	if !strings.Contains(m.status, "read-only") {
		t.Fatalf("y in read-only: status %q", m.status)
	}
	if len(applyCalls(f)) != 0 {
		t.Fatal("copied in read-only")
	}

	d := actionModel(t, actionCluster())
	d = open(t, d, "deploy")
	d = pressSettle(t, d, "T")
	if d.dlg != nil {
		t.Fatalf("T on deployments %+v", d.dlg)
	}
	if strings.Contains(ansi.Strip(d.legend()), "copy to ns") {
		t.Fatal("copy to ns on deployments")
	}
}

func TestSecretCopyAllNamespaces(t *testing.T) {
	c := &copyCluster{secretCluster: secretCluster{data: map[string]map[string]string{"db": {"password": "hunter2"}}}}
	m, f := copyModel(t, c)
	m.namespace = ""
	m.content.SetItems(nil)
	next, cmd := m.openResource("secrets")
	m = settle(t, next.(model), cmd)
	if len(m.content.Items()) == 0 {
		t.Fatal("no rows")
	}
	m.content.Select(0)
	m = pressSettle(t, m, "T")
	if m.dlg == nil || m.dlg.ns != "shop" || m.dlg.name != "db" {
		t.Fatalf("picker %+v", m.dlg)
	}
	m = typeText(t, m, "prod")
	m = pressSettle(t, m, "enter", "enter", "y")
	if got := applyCalls(f); len(got) != 1 || !strings.HasPrefix(got[0], "create -f - -n prod") {
		t.Fatalf("calls %v", got)
	}
	var gets []string
	for _, r := range f.Calls() {
		if j := strings.Join(r.Args, " "); strings.HasPrefix(j, "get secret db -n shop -o json") {
			gets = append(gets, j)
		}
	}
	if len(gets) != 1 {
		t.Fatalf("source get %v", gets)
	}
}

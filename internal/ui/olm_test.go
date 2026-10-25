package ui

import (
	"strings"
	"testing"

	"k2/internal/shell"

	"github.com/charmbracelet/x/ansi"
)

const olmSubsJSON = `{"items":[{"metadata":{"name":"etcd","namespace":"operators"},
 "spec":{"name":"etcd","channel":"stable","source":"community"},
 "status":{"installedCSV":"etcd.v0.9.4","currentCSV":"etcd.v0.9.4","state":"AtLatestKnown"}}]}`

const olmCSVsJSON = `{"items":[
 {"metadata":{"name":"aaa.v1","namespace":"operators"},"spec":{"displayName":"A","version":"1"},"status":{"phase":"Failed"}},
 {"metadata":{"name":"etcd.v0.9.4","namespace":"operators"},"spec":{"displayName":"etcd","version":"0.9.4"},"status":{"phase":"Succeeded"}}]}`

const olmCSVJSON = `{"metadata":{"name":"etcd.v0.9.4","namespace":"operators"},
 "spec":{"displayName":"etcd","version":"0.9.4","customresourcedefinitions":{"owned":[{"name":"etcdclusters.etcd.database.coreos.com","kind":"EtcdCluster","version":"v1beta2"}]},
  "install":{"spec":{"deployments":[{"name":"etcd-operator"}]}}},"status":{"phase":"Succeeded"}}`

const olmPlansJSON = `{"items":[
 {"metadata":{"name":"install-auto","namespace":"operators"},"spec":{"clusterServiceVersionNames":["a.v1"],"approval":"Automatic","approved":true},"status":{"phase":"Complete"}},
 {"metadata":{"name":"install-man","namespace":"operators"},"spec":{"clusterServiceVersionNames":["etcd.v0.9.5"],"approval":"Manual","approved":false},"status":{"phase":"RequiresApproval"}}]}`

const olmCRDsJSON = `{"items":[{"metadata":{"name":"widgets.example.com"},
 "spec":{"group":"example.com","scope":"Namespaced","names":{"kind":"Widget","plural":"widgets"},"versions":[{"name":"v1","served":true,"storage":true}]}}]}`

const olmWidgetsJSON = `{"items":[{"metadata":{"name":"w1","namespace":"operators"}}]}`

func olmCluster(olm bool) *shell.Fake {
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		if r.Name != "kubectl" || len(r.Args) < 2 {
			return shell.Fail(1, "?")
		}
		line := strings.Join(r.Args, " ")
		if r.Args[0] == "patch" {
			return shell.OK("installplan.operators.coreos.com/install-man patched")
		}
		if r.Args[0] != "get" {
			return shell.Fail(1, "not in this test")
		}
		res := r.Args[1]
		if !olm && (strings.Contains(res, "operators.coreos.com") || strings.Contains(res, "openshift.io")) || !olm && (res == "subscriptions" || res == "routes") {
			return shell.Fail(1, `error: the server doesn't have a resource type "`+strings.Split(res, ".")[0]+`"`)
		}
		switch {
		case strings.HasPrefix(res, "subscriptions"):
			return shell.OK(olmSubsJSON)
		case strings.HasPrefix(res, "clusterserviceversions") && strings.Contains(line, " etcd.v0.9.4 "):
			return shell.OK(olmCSVJSON)
		case strings.HasPrefix(res, "clusterserviceversions"):
			return shell.OK(olmCSVsJSON)
		case strings.HasPrefix(res, "installplans"):
			return shell.OK(olmPlansJSON)
		case strings.HasPrefix(res, "customresourcedefinitions"):
			return shell.OK(olmCRDsJSON)
		case res == "widgets.example.com":
			return shell.OK(olmWidgetsJSON)
		}
		return shell.Fail(1, "not in this test")
	}}
}

func olmModel(t *testing.T, f *shell.Fake, prod bool) model {
	t.Helper()
	cfg := testEnv(t)
	if prod {
		cfg.Prod = []string{"kind-dev"}
	}
	m := newTestModel(t, cfg, f)
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "operators"
	return m
}

func TestSubscriptionJumpsToCSV(t *testing.T) {
	trueColor(t)
	m := open(t, olmModel(t, olmCluster(true), false), "sub")
	if m.kind != "subscriptions.operators.coreos.com" || !strings.Contains(m.content.Title, "CURRENT CSV") {
		t.Fatalf("subs %q %q", m.kind, m.content.Title)
	}
	row := m.content.Items()[0].(item)
	if colorBefore(row.title, "AtLatestKnown") != sgr(th.OK) {
		t.Fatal("state not green")
	}
	m = pressSettle(t, m, "enter")
	if m.kind != "clusterserviceversions.operators.coreos.com" || m.content.SelectedItem().(item).id != "etcd.v0.9.4" {
		t.Fatalf("jump: %s sel %v", m.kind, m.content.SelectedItem())
	}
	csvRow := m.content.SelectedItem().(item)
	if colorBefore(csvRow.title, "Succeeded") != sgr(th.OK) || colorBefore(m.content.Items()[0].(item).title, "Failed") != sgr(th.Danger) {
		t.Fatal("phase colors")
	}
	m = pressSettle(t, m, "esc")
	if !strings.HasPrefix(m.kind, "subscriptions") || m.content.SelectedItem().(item).id != "etcd" {
		t.Fatalf("back: %s", m.kind)
	}
}

func TestCSVOwnedView(t *testing.T) {
	m := open(t, olmModel(t, olmCluster(true), false), "csv")
	m = pressSettle(t, selectRow(t, m, "etcd.v0.9.4"), "enter")
	if m.stage != "view" || !strings.Contains(m.logText, "owned CRDs (1)") || !strings.Contains(m.logText, "etcd-operator") {
		t.Fatalf("view %s %q", m.stage, m.logText)
	}
	if !strings.Contains(m.toast.text, "csv etcd.v0.9.4") {
		t.Fatalf("toast %q", m.toast.text)
	}
	m = pressSettle(t, m, "esc")
	if m.stage != "res" {
		t.Fatal(m.stage)
	}
}

func TestApproveInstallPlan(t *testing.T) {
	trueColor(t)
	for _, prod := range []bool{false, true} {
		f := olmCluster(true)
		m := open(t, olmModel(t, f, prod), "ip")
		if !strings.Contains(m.content.Title, "APPROVAL") || !strings.Contains(m.content.Title, "APPROVED") {
			t.Fatalf("header %q", m.content.Title)
		}
		m = pressSettle(t, selectRow(t, m, "install-auto"), "a")
		if m.dlg != nil || !strings.Contains(m.status, "already approved") {
			t.Fatalf("auto plan: %q", m.status)
		}
		m = pressSettle(t, selectRow(t, m, "install-man"), "a")
		if m.dlg == nil || !strings.Contains(m.dlg.ask, "approve installplan install-man on kind-dev?") {
			t.Fatalf("no question %+v", m.dlg)
		}
		view := m.View()
		if prod != (colorBefore(view, "approve installplan install-man") == sgr(th.Danger)) {
			t.Fatalf("prod=%v: question color wrong", prod)
		}
		m = pressSettle(t, m, "y")
		var patch []string
		for _, c := range f.Calls() {
			if c.Args[0] == "patch" {
				patch = c.Args
			}
		}
		want := `patch installplan install-man -n operators --type merge -p {"spec":{"approved":true}}`
		if !strings.HasPrefix(strings.Join(patch, " "), want) {
			t.Fatalf("patch %v", patch)
		}
		if !strings.Contains(m.toast.text, "approved install-man") {
			t.Fatalf("toast %q", m.toast.text)
		}
	}
}

func TestCRDListsCustomResources(t *testing.T) {
	m := open(t, olmModel(t, olmCluster(true), false), "crd")
	if !strings.Contains(m.content.Title, "GROUP") || !strings.Contains(m.content.Title, "SCOPE") {
		t.Fatalf("header %q", m.content.Title)
	}
	m = pressSettle(t, m, "enter")
	if m.kind != "widgets.example.com" || m.content.Items()[0].(item).id != "w1" {
		t.Fatalf("CRs: %s %s", m.kind, m.status)
	}
	m = pressSettle(t, m, "esc")
	if m.kind != "customresourcedefinitions" || m.content.SelectedItem().(item).id != "widgets.example.com" {
		t.Fatalf("back %s", m.kind)
	}
}

func TestMissingOLMIsExplained(t *testing.T) {
	for _, q := range []string{"sub", "csv", "ip", "og", "rt", "subscriptions"} {
		m := open(t, olmModel(t, olmCluster(false), false), q)
		if !strings.Contains(m.status, "is not installed on this cluster") || m.stage == "res" {
			t.Errorf("%s: %s %q", q, m.stage, m.status)
		}
		if strings.Contains(ansi.Strip(m.View()), "panic") {
			t.Fatal("panic")
		}
	}

	m := olmModel(t, olmCluster(false), false)
	m.resources = []string{"pods", "deployments"}
	m = open(t, m, "subscriptions")
	if !strings.Contains(m.status, "subscriptions is not installed on this cluster (no OLM)") {
		t.Fatalf("status %q", m.status)
	}
}

func TestProjectsPicker(t *testing.T) {
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		if strings.Contains(r.Line(), "get projects") {
			return shell.OK("team-a\nteam-b\n")
		}
		return shell.Fail(1, "no")
	}}
	for _, q := range []string{"proj", "projects"} {
		m := open(t, olmModel(t, f, false), q)
		if m.stage != "projects" || len(m.content.Items()) != 3 {
			t.Fatalf("%s: %s %d", q, m.stage, len(m.content.Items()))
		}
	}
}

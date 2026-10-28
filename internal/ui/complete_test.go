package ui

import (
	"strings"
	"testing"
	"time"

	"k2/internal/config"
	"k2/internal/shell"

	"github.com/charmbracelet/x/ansi"
)

func nameCluster(t *testing.T, nameCalls *int) *shell.Fake {
	base := appCluster(t)
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		line := strings.Join(r.Args, " ")
		switch {
		case strings.HasPrefix(line, "get deployments -o name"):
			*nameCalls++
			return shell.OK("deployment.apps/web\ndeployment.apps/web-api\ndeployment.apps/kafka-connect\n")
		case strings.HasPrefix(line, "get pods -o name"):
			*nameCalls++
			return shell.OK("pod/web-7d9f-a\npod/web-7d9f-b\npod/db-0\n")
		case strings.HasPrefix(line, "get statefulsets -o name"):
			*nameCalls++
			return shell.Fail(1, "Error from server (Forbidden): statefulsets is forbidden")
		}
		return base.Handler(r)
	}}
}

func cmdLine(t *testing.T, m model, s string) model {
	t.Helper()
	m = pressSettle(t, m, ":")
	return typeText(t, m, s)
}

func TestXrayWideStartsFolded(t *testing.T) {
	m := open(t, appModel(t, appCluster(t)), "xray pods")
	if m.stage != "xray" {
		t.Fatalf("stage %s: %s", m.stage, m.status)
	}
	ids := strings.Join(rowIDs(m), " ")

	if !strings.Contains(ids, "container/shop/web-7d9f-a/") || strings.Contains(ids, "container/shop/db-0/") {
		t.Fatalf("default folds: %s", ids)
	}
	if !m.vs.xray.collapsed["pod/shop/db-0"] || m.vs.xray.collapsed["pod/shop/web-7d9f-a"] {
		t.Fatalf("collapsed %v", m.vs.xray.collapsed)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "SUMMARY") || !strings.Contains(view, "▸ db-0") || !strings.Contains(view, "container") {
		t.Fatalf("view:\n%s", view)
	}
	hint := "space fold · enter open · / filter · +/- expand/collapse all · tip: :xray deploy <name> or X on a row"
	if m.status != hint || !strings.Contains(view, "space fold · enter open · / filter") {
		t.Fatalf("hint %q\n%s", m.status, view)
	}
	m = pressSettle(t, m, "+")
	if ids := strings.Join(rowIDs(m), " "); !strings.Contains(ids, "container/shop/db-0/") {
		t.Fatalf("+ did not expand all: %s", ids)
	}
	m = pressSettle(t, m, "-")
	for _, id := range rowIDs(m) {
		if strings.HasPrefix(id, "container/") {
			t.Fatalf("- left %s open", id)
		}
	}

	m = open(t, appModel(t, appCluster(t)), "xray pod db-0")
	if ids := strings.Join(rowIDs(m), " "); !strings.Contains(ids, "container/shop/db-0/") {
		t.Fatalf("single tree folded: %s", ids)
	}

	cfg := testEnv(t)
	cfg.Keys.XrayExpand, cfg.Keys.XrayCollapse = config.Binding{"E"}, config.Binding{"ctrl+e"}
	m = newTestModel(t, cfg, appCluster(t))
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	if h := m.xrayHint(); !strings.Contains(h, "E/^e expand/collapse all") {
		t.Fatalf("hint %q", h)
	}
}

func TestXrayFilterKeepsAncestors(t *testing.T) {
	m := open(t, appModel(t, appCluster(t)), "xray deploy")
	m = pressSettle(t, m, "/")
	if m.dlg == nil || m.dlg.act != "xrayfilter" {
		t.Fatalf("no filter dialog: %+v", m.dlg)
	}
	m = typeText(t, m, "7d9f-a")
	m = pressSettle(t, m, "enter")
	ids := strings.Join(rowIDs(m), " ")
	for _, want := range []string{"deploy/shop/web", "rs/shop/web-7d9f", "pod/shop/web-7d9f-a", "container/shop/web-7d9f-a/"} {
		if !strings.Contains(ids, want) {
			t.Fatalf("filter lost %s: %s", want, ids)
		}
	}
	if strings.Contains(ids, "web-7d9f-b") || strings.Contains(ids, "svc/") || strings.Contains(ids, "web-old") {
		t.Fatalf("filter kept others: %s", ids)
	}
	if !strings.Contains(ansi.Strip(m.View()), "/7d9f-a") {
		t.Fatal("the active filter is not shown")
	}
	m = pressSettle(t, m, "/")
	m.dlg.input.SetValue("")
	m = pressSettle(t, m, "enter")
	if ids := strings.Join(rowIDs(m), " "); !strings.Contains(ids, "web-7d9f-b") || !strings.Contains(ids, "svc/shop/web") {
		t.Fatalf("cleared filter: %s", ids)
	}

	m = pressSettle(t, m, "/")
	m = typeText(t, m, "zzz")
	m = pressSettle(t, m, "enter")
	if !strings.Contains(m.status, "nothing named like zzz") {
		t.Fatalf("status %q", m.status)
	}
}

func TestXrayCompletion(t *testing.T) {
	at := fakeNow(t)
	n := 0
	m := appModel(t, nameCluster(t, &n))
	m = cmdLine(t, m, "xray ")
	if m.cmdHint != "deploy  sts  ds  rs  job  cronjob  pod  svc" {
		t.Fatalf("kinds %q", m.cmdHint)
	}
	m = typeText(t, m, "deployme")
	if m.cmdHint != "deploy" {
		t.Fatalf("alias %q", m.cmdHint)
	}
	m = pressSettle(t, m, "tab")
	if m.cmd.Value() != "xray deploy " || n != 1 {
		t.Fatalf("kind tab: %q calls %d", m.cmd.Value(), n)
	}
	if m.cmdHint != "kafka-connect  web  web-api" {
		t.Fatalf("names %q", m.cmdHint)
	}
	m = typeText(t, m, "connect")
	if m.cmdHint != "kafka-connect" {
		t.Fatalf("substring %q", m.cmdHint)
	}
	m = pressSettle(t, m, "tab")
	if m.cmd.Value() != "xray deploy kafka-connect" {
		t.Fatalf("name tab %q", m.cmd.Value())
	}

	m.cmd.SetValue("xray deploy w")
	m = pressSettle(t, m, "tab")
	if m.cmd.Value() != "xray deploy web" || m.cmdHint != "web  web-api" {
		t.Fatalf("prefix %q hint %q", m.cmd.Value(), m.cmdHint)
	}
	m = pressSettle(t, m, "tab")
	m = pressSettle(t, m, "down")
	if m.cmd.Value() != "xray deploy web-api" || m.cmdHint != "web  [web-api]" {
		t.Fatalf("cycle %q %q", m.cmd.Value(), m.cmdHint)
	}
	m = pressSettle(t, m, "up")
	if m.cmd.Value() != "xray deploy web" {
		t.Fatalf("up %q", m.cmd.Value())
	}

	m.cmd.SetValue("xray deploy ")
	m = typeText(t, m, "k")
	if n != 1 {
		t.Fatalf("cached names fetched again: %d", n)
	}
	*at = at.Add(namesTTL + time.Second)
	m = typeText(t, m, "a")
	if n != 2 || m.cmdHint != "kafka-connect" {
		t.Fatalf("stale cache: calls %d hint %q", n, m.cmdHint)
	}

	m = pressSettle(t, m, "esc")

	m = cmdLine(t, m, "xray sts ")
	if !strings.Contains(m.cmdHint, "could not list statefulsets: Error from server (Forbidden)") || m.load.active() {
		t.Fatalf("error hint %q", m.cmdHint)
	}

	m.cmd.SetValue("xray cm ")
	m = typeText(t, m, "x")
	if !strings.Contains(m.cmdHint, "no owner tree for cm") {
		t.Fatalf("unknown kind %q", m.cmdHint)
	}
}

func TestXrayBareCommandSuggestsKinds(t *testing.T) {
	m := appModel(t, appCluster(t))
	m = cmdLine(t, m, "xray")
	m = pressSettle(t, m, "enter")
	if !m.cmdOn || m.cmd.Value() != "xray " || m.cmdHint != "deploy  sts  ds  rs  job  cronjob  pod  svc" || m.toast.tone == toneErr {
		t.Fatalf("on %v value %q hint %q toast %+v", m.cmdOn, m.cmd.Value(), m.cmdHint, m.toast)
	}
	m = typeText(t, m, "po")
	m = pressSettle(t, m, "tab")
	if m.cmd.Value() != "xray pod " {
		t.Fatalf("value %q", m.cmd.Value())
	}
}

func TestLogsAndFavCompletion(t *testing.T) {
	n := 0
	m := appModel(t, nameCluster(t, &n))
	m = cmdLine(t, m, "logs db")
	if m.cmdHint != "db-0" {
		t.Fatalf("pods %q", m.cmdHint)
	}
	m.cmd.SetValue("logs app=web")
	m = typeText(t, m, ",")
	if strings.Contains(m.cmdHint, "db-0") {
		t.Fatalf("a selector got pod names: %q", m.cmdHint)
	}
	m = pressSettle(t, m, "esc")

	m.cfg.Favorites = []config.Favorite{{Name: "shop dev", Context: "kind-dev", Namespace: "shop"}, {Context: "prod-eu-1", Namespace: "payments"}}
	m = cmdLine(t, m, "fav sh")
	if m.cmdHint != "shop dev" {
		t.Fatalf("fav hint %q", m.cmdHint)
	}
	m = pressSettle(t, m, "tab")
	if m.cmd.Value() != "fav shop dev" {
		t.Fatalf("fav tab %q", m.cmd.Value())
	}
	m.cmd.SetValue("fav ")
	m = typeText(t, m, "pay")
	if m.cmdHint != "prod-eu-1/payments" {
		t.Fatalf("unnamed fav %q", m.cmdHint)
	}
	m = pressSettle(t, m, "enter")
	if m.context != "prod-eu-1" || m.namespace != "payments" {
		t.Fatalf("jump: %s/%s %q", m.context, m.namespace, m.status)
	}
}

func TestBracePaging(t *testing.T) {

	m := open(t, appModel(t, appCluster(t)), "xray pods")
	m = pressSettle(t, m, "+")
	m.content.Select(0)
	m = pressSettle(t, m, "}")
	if m.content.Index() == 0 {
		t.Fatal("} did not page the tree")
	}
	at := m.content.Index()
	m = pressSettle(t, m, "{")
	if m.content.Index() >= at {
		t.Fatal("{ did not page the tree back")
	}
	m = pressSettle(t, m, "G")
	if m.content.Index() != len(m.content.Items())-1 {
		t.Fatal("G in the tree")
	}
	m = pressSettle(t, m, "g", "j")
	if m.content.Index() != 1 {
		t.Fatalf("g j in the tree: %d", m.content.Index())
	}

	m = open(t, appModel(t, appCluster(t)), "helm all")
	m.content.Select(0)
	if m = pressSettle(t, m, "G"); m.content.Index() != len(m.content.Items())-1 || len(m.content.Items()) < 2 {
		t.Fatal("G in the helm table")
	}
	if m = pressSettle(t, m, "k"); m.content.Index() != len(m.content.Items())-2 {
		t.Fatal("k in the helm table")
	}

	for _, q := range []string{"pulse", "logs app=web"} {
		m = open(t, appModel(t, appCluster(t)), q)
		m.logText = strings.Repeat("line\n", 400)
		m.logStick, m.logAt = false, 0
		m = pressSettle(t, m, "}")
		if m.logAt == 0 {
			t.Fatalf("%s: } did not page", q)
		}
		at := m.logAt
		m = pressSettle(t, m, "{")
		if m.logAt >= at {
			t.Fatalf("%s: { did not page back", q)
		}
	}

	m = cmdLine(t, appModel(t, appCluster(t)), "{}")
	if m.cmd.Value() != "{}" {
		t.Fatalf("typed %q", m.cmd.Value())
	}
}

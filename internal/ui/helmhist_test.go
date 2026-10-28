package ui

import (
	"fmt"
	"strings"
	"testing"

	"k2/internal/shell"
	"k2/internal/state"

	"github.com/charmbracelet/x/ansi"
)

func rowsText(m model) string {
	var b strings.Builder
	for _, raw := range m.content.Items() {
		b.WriteString(ansi.Strip(raw.(item).plain) + "\n")
	}
	return b.String()
}

func histHelm() string {
	manifest := map[int]string{
		1: "---\nkind: Deployment\nmetadata:\n  name: web\nspec:\n  image: web:1\n---\nkind: ConfigMap\nmetadata:\n  name: web-old\ndata:\n  a: b\n",
		2: "---\nkind: Deployment\nmetadata:\n  name: web\nspec:\n  image: web:2\n---\nkind: ConfigMap\nmetadata:\n  name: web-old\ndata:\n  a: b\n",
		3: "---\nkind: Deployment\nmetadata:\n  name: web\nspec:\n  image: web:2\n---\nkind: Service\nmetadata:\n  name: web\nspec:\n  port: 80\n",
	}
	replicas := map[int]int{1: 2, 2: 2, 3: 3}
	status := map[int]string{1: "superseded", 2: "superseded", 3: "deployed"}
	var items []string
	for rev := 1; rev <= 3; rev++ {
		r := map[string]any{"name": "web", "namespace": "shop", "version": rev,
			"info":     map[string]any{"status": status[rev], "last_deployed": fmt.Sprintf("2026-10-2%dT08:00:00Z", rev), "description": fmt.Sprintf("change %d", rev)},
			"chart":    map[string]any{"metadata": map[string]any{"name": "nginx", "version": fmt.Sprintf("1.%d.0", rev), "appVersion": fmt.Sprintf("1.2%d", rev)}, "values": map[string]any{"replicas": 1}},
			"config":   map[string]any{"replicas": replicas[rev]},
			"manifest": manifest[rev]}
		items = append(items, fmt.Sprintf(`{"kind":"Secret","metadata":{"name":"sh.helm.release.v1.web.v%d","namespace":"shop","labels":{"owner":"helm","name":"web","version":"%d","status":%q}},"data":{"release":%q}}`,
			rev, rev, status[rev], helmPayload(r)))
	}
	return `{"kind":"List","items":[` + strings.Join(items, ",") + `]}`
}

func histCluster(t *testing.T) *shell.Fake {
	base := appCluster(t)
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		line := strings.Join(r.Args, " ")
		all := strings.Contains(line, " -A")
		switch {
		case r.Name == "helm":
			return shell.OK("ok")
		case strings.HasPrefix(line, "get secrets") && strings.Contains(line, "owner=helm") && strings.Contains(line, "-n empty"):
			return shell.OK(`{"items":[]}`)
		case strings.HasPrefix(line, "get secrets") && strings.Contains(line, "owner=helm"):
			return shell.OK(histHelm())
		case strings.HasPrefix(line, "get nodes -o json"):
			return shell.OK(`{"items":[{"kind":"Node","metadata":{"name":"node-1","creationTimestamp":"2026-10-23T08:00:00Z"}}]}`)
		case strings.HasPrefix(line, "get pods -o json") && strings.Contains(line, "-n empty"):
			return shell.OK(`{"items":[]}`)
		case strings.HasPrefix(line, "get pods -o json") && all && !strings.Contains(line, "-l "):
			res := base.Handler(r)
			lab := `{"kind":"Pod","metadata":{"name":"api-0","namespace":"lab","creationTimestamp":"2026-10-23T08:00:00Z"},"spec":{"containers":[{"name":"app"}]},"status":{"phase":"Running","containerStatuses":[{"ready":true,"restartCount":0,"state":{"running":{}}}]}}`
			res.Stdout = []byte(strings.Replace(string(res.Stdout), `{"items":[`, `{"items":[`+lab+",", 1))
			return res
		}
		return base.Handler(r)
	}}
}

func histOpen(t *testing.T, f *shell.Fake) model {
	t.Helper()
	lookPath = func(string) (string, error) { return "/usr/bin/helm", nil }
	t.Cleanup(func() { lookPath = defaultLookPath })
	m := appModel(t, f)
	m.cfg.Prod = []string{"kind-dev"}
	m = pressSettle(t, selectRow(t, open(t, m, "helm"), "web"), "t")
	if m.stage != "helmhist" || m.load.active() {
		t.Fatalf("history: stage %s load %v %s", m.stage, m.load.active(), m.status)
	}
	return m
}

func TestHelmHistoryRollbackToRevision(t *testing.T) {
	f := histCluster(t)
	m := histOpen(t, f)
	if got := strings.Join(rowIDs(m), " "); got != "rev/3 rev/2 rev/1" {
		t.Fatalf("rows %s", got)
	}
	rows := rowsText(m)
	for _, want := range []string{"●", "REVISION", "deployed", "nginx-1.3.0", "1.23", "change 1"} {
		if !strings.Contains(rows+ansi.Strip(m.View()), want) {
			t.Fatalf("history lacks %q:\n%s", want, ansi.Strip(m.View()))
		}
	}
	if !strings.Contains(ansi.Strip(m.View()), "shop/web · history") {
		t.Fatalf("title:\n%s", ansi.Strip(m.View()))
	}

	m = pressSettle(t, selectRow(t, m, "rev/3"), "r")
	if m.dlg != nil || !strings.Contains(m.toast.text, "revision 3 is current") || !strings.Contains(m.status, "select an older revision") {
		t.Fatalf("current rev: dlg %+v toast %q status %q", m.dlg, m.toast.text, m.status)
	}

	m = pressSettle(t, selectRow(t, m, "rev/1"), "r")
	if m.dlg == nil || !m.dlg.danger || !strings.Contains(m.dlg.ask, "roll back web from revision 3 to revision 1?") {
		t.Fatalf("confirm %+v", m.dlg)
	}
	view := ansi.Strip(m.View())
	for _, want := range []string{skull, "revision 3 → 1", "nginx-1.3.0 (app 1.23) → nginx-1.1.0 (app 1.21)", "kind-dev", "helm rollback web 1", "default no"} {
		if !strings.Contains(view, want) {
			t.Fatalf("popup lacks %q:\n%s", want, view)
		}
	}
	m = pressSettle(t, m, "n")
	if m.dlg != nil || len(helmCalls(f)) != 0 {
		t.Fatal("n rolled back")
	}
	m = pressSettle(t, m, "r", "y")
	if c := helmCalls(f); len(c) != 1 || c[0] != "rollback web 1 -n shop --kubeconfig /kc --kube-context kind-dev" {
		t.Fatalf("helm calls %v", c)
	}
	if m.load.active() || m.stage != "helmhist" {
		t.Fatalf("after rollback: load %v stage %s", m.load.active(), m.stage)
	}

	m = pressSettle(t, m, "esc")
	if m.stage != "helm" {
		t.Fatalf("back went to %s", m.stage)
	}
}

func TestHelmRevisionDiff(t *testing.T) {
	f := histCluster(t)
	m := histOpen(t, f)

	m = pressSettle(t, selectRow(t, m, "rev/1"), "D")
	if m.stage != "hdiff" {
		t.Fatalf("stage %s: %s", m.stage, m.status)
	}
	for _, want := range []string{
		"revision 1 (nginx-1.1.0) → revision 3 (nginx-1.3.0)",
		"=== ~ values (user-supplied)", "-replicas: 2", "+replicas: 3",
		"=== + Service/web (added)", "=== - ConfigMap/web-old (removed)",
		"=== ~ Deployment/web", "-  image: web:1", "+  image: web:2", "@@ ",
	} {
		if !strings.Contains(m.logText, want) {
			t.Fatalf("diff lacks %q:\n%s", want, m.logText)
		}
	}
	if !strings.Contains(ansi.Strip(m.View()), "diff shop/web 1→3") {
		t.Fatalf("title:\n%s", ansi.Strip(m.View()))
	}

	m = pressSettle(t, m, "esc")
	if m.stage != "helmhist" {
		t.Fatalf("back went to %s", m.stage)
	}

	m = pressSettle(t, selectRow(t, m, "rev/1"), " ")
	if m.vs.hist.mark != 1 || !strings.Contains(rowsText(m), "◆") {
		t.Fatalf("mark %d rows:\n%s", m.vs.hist.mark, rowsText(m))
	}
	m = pressSettle(t, selectRow(t, m, "rev/2"), "D")
	if !strings.Contains(m.logText, "revision 1 (nginx-1.1.0) → revision 2 (nginx-1.2.0)") ||
		!strings.Contains(m.logText, "# values (user-supplied): unchanged") ||
		!strings.Contains(m.logText, "1 changed, 0 added, 0 removed, 1 unchanged") {
		t.Fatalf("1→2 diff:\n%s", m.logText)
	}

	m = pressSettle(t, m, "esc")
	m = pressSettle(t, selectRow(t, m, "rev/1"), " ")
	m = pressSettle(t, selectRow(t, m, "rev/3"), "D")
	if m.stage != "helmhist" || !strings.Contains(m.toast.text, "nothing to diff") {
		t.Fatalf("diff of current: %s %q", m.stage, m.toast.text)
	}

	if len(helmCalls(f)) != 0 {
		t.Fatalf("helm ran: %v", helmCalls(f))
	}
}

func TestViewerSearch(t *testing.T) {
	m := histOpen(t, histCluster(t))
	m = pressSettle(t, selectRow(t, m, "rev/1"), "D")
	m = pressSettle(t, m, "/")
	if m.dlg == nil || m.dlg.act != "search" {
		t.Fatalf("no search dialog: %+v", m.dlg)
	}
	m = typeText(t, m, "image")
	m = pressSettle(t, m, "enter")
	if m.dlg != nil || !strings.Contains(m.status, "image: match 1 of 2") {
		t.Fatalf("search: %q", m.status)
	}
	m = pressSettle(t, m, "/", "enter")
	if !strings.Contains(m.status, "match 2 of 2") {
		t.Fatalf("next: %q", m.status)
	}
	m = pressSettle(t, m, "/")
	m = typeText(t, m, "nowhere-to-be-found")
	m = pressSettle(t, m, "enter")
	if !strings.Contains(m.status, "not found") {
		t.Fatalf("not found: %q", m.status)
	}
}

func TestHelmEmptyStateHint(t *testing.T) {
	m := appModel(t, histCluster(t))
	m.namespace = "empty"
	m = open(t, m, "helm")
	want := "no releases in empty — press 0 for all namespaces"
	if !strings.Contains(m.status, want) || !strings.Contains(ansi.Strip(m.View()), want) {
		t.Fatalf("empty state: %q\n%s", m.status, ansi.Strip(m.View()))
	}
}

func TestAllNamespacesToggle(t *testing.T) {
	f := histCluster(t)
	m := open(t, appModel(t, f), "pods")
	if m.stage != "workpods" {
		t.Fatalf("stage %s: %s", m.stage, m.status)
	}
	m = pressSettle(t, selectRow(t, m, "db-0"), "0")
	if m.namespace != "" || m.vs.nsPrev != "shop" {
		t.Fatalf("ns %q prev %q", m.namespace, m.vs.nsPrev)
	}
	if it, _ := m.selectedItem(); it.id != "shop/db-0" {
		t.Fatalf("selected %q, rows %v", it.id, rowIDs(m))
	}
	if !strings.Contains(strings.Join(rowIDs(m), " "), "lab/api-0") || !strings.Contains(ansi.Strip(m.View()), "all/") {
		t.Fatalf("all namespaces:\n%s", ansi.Strip(m.View()))
	}
	last := calls(f, "get")[len(calls(f, "get"))-1]
	if !strings.Contains(last, "-A") {
		t.Fatalf("not -A: %s", last)
	}

	m.save()
	st := state.Load(m.cfg.StateDir)
	if len(st.Panes) == 0 || st.Panes[0].NsPrev != "shop" || st.Panes[0].Namespace != "" {
		t.Fatalf("state %+v", st.Panes)
	}
	m = pressSettle(t, m, "0")
	if m.namespace != "shop" || m.vs.nsPrev != "" {
		t.Fatalf("back: ns %q prev %q", m.namespace, m.vs.nsPrev)
	}
	if it, _ := m.selectedItem(); it.id != "db-0" {
		t.Fatalf("selected %q", it.id)
	}
	if !strings.Contains(ansi.Strip(m.View()), "shop/") {
		t.Fatalf("title:\n%s", ansi.Strip(m.View()))
	}
}

func TestAllNamespacesClusterScoped(t *testing.T) {
	m := appModel(t, histCluster(t))
	m.resources = []string{"nodes", "pods", "deployments"}
	m = open(t, m, "nodes")
	if m.stage != "res" {
		t.Fatalf("stage %s: %s", m.stage, m.status)
	}
	m = pressSettle(t, m, "0")
	if m.namespace != "shop" || m.toast.text != "cluster-scoped" {
		t.Fatalf("ns %q toast %q", m.namespace, m.toast.text)
	}
}

func TestScaleDialogAcceptsZero(t *testing.T) {
	m := open(t, appModel(t, histCluster(t)), "deploy")
	m = pressSettle(t, selectRow(t, m, "web"), "S")
	if m.dlg == nil || m.dlg.act != "scale" {
		t.Fatalf("no scale dialog: %+v", m.dlg)
	}
	m = press(t, m, "backspace", "backspace", "backspace")
	m = typeText(t, m, "0")
	if m.dlg == nil || m.dlg.input.Value() != "0" || m.namespace != "shop" {
		t.Fatalf("dialog %+v ns %q", m.dlg, m.namespace)
	}
}

func TestAllNamespacesEmptyNamespace(t *testing.T) {
	m := appModel(t, histCluster(t))
	m.namespace = ""
	m.vs.nsPrev = "empty"
	m = open(t, m, "pods")
	if len(rowIDs(m)) == 0 {
		t.Fatal("no pods across all namespaces")
	}
	m = pressSettle(t, m, "0")
	if m.namespace != "empty" || len(rowIDs(m)) != 0 {
		t.Fatalf("ns %q rows %v", m.namespace, rowIDs(m))
	}
	if want := "no pods in empty — press 0 for all namespaces"; !strings.Contains(m.status, want) || !strings.Contains(ansi.Strip(m.View()), want) {
		t.Fatalf("hint %q\n%s", m.status, ansi.Strip(m.View()))
	}

	m.namespace, m.vs.nsPrev = "", ""
	m = pressSettle(t, m, "0")
	if m.toast.text != "already all namespaces" {
		t.Fatalf("toast %q", m.toast.text)
	}
}

func TestAllNamespacesXrayLogsPulse(t *testing.T) {
	f := histCluster(t)
	m := open(t, appModel(t, f), "xray pods")
	m = pressSettle(t, m, "0")
	if m.stage != "xray" || m.namespace != "" || !strings.Contains(ansi.Strip(m.View()), "all/xray") {
		t.Fatalf("xray all: %s %q\n%s", m.stage, m.namespace, ansi.Strip(m.View()))
	}
	last := calls(f, "get")[len(calls(f, "get"))-1]
	if !strings.HasPrefix(last, "get pods,replicasets") || !strings.Contains(last, "-A") {
		t.Fatalf("not -A: %s", last)
	}
	m = pressSettle(t, m, "0")
	if m.namespace != "shop" || m.stage != "xray" {
		t.Fatalf("xray back: %q %s", m.namespace, m.stage)
	}

	m = open(t, appModel(t, f), "logs app=web")
	if m.stage != "mlogs" || !strings.Contains(m.logText, "web-7d9f-aaaaa/app hello") || strings.Contains(m.logText, "shop/web-7d9f-aaaaa") {
		t.Fatalf("logs in shop:\n%s", m.logText)
	}
	m = pressSettle(t, m, "0")
	if m.namespace != "" || !strings.Contains(m.logText, "shop/web-7d9f-aaaaa/app hello from web-7d9f-aaaaa") {
		t.Fatalf("logs across all namespaces (%q):\n%s", m.namespace, m.logText)
	}
	m = pressSettle(t, m, "0")
	if m.namespace != "shop" || strings.Contains(m.logText, "shop/web-7d9f-aaaaa") {
		t.Fatalf("logs back in shop:\n%s", m.logText)
	}

	m = open(t, appModel(t, f), "pulse")
	m = pressSettle(t, m, "0")
	if m.namespace != "shop" || !strings.Contains(m.status, "one namespace") {
		t.Fatalf("pulse: %q %q", m.namespace, m.status)
	}
}

func TestHelmHistoryPaging(t *testing.T) {
	m := histOpen(t, histCluster(t))
	m = pressSettle(t, m, "G")
	if it, _ := m.selectedItem(); it.id != "rev/1" {
		t.Fatalf("G: %q", it.id)
	}
	m = pressSettle(t, m, "g")
	if it, _ := m.selectedItem(); it.id != "rev/3" {
		t.Fatalf("g: %q", it.id)
	}

	m = pressSettle(t, selectRow(t, m, "rev/1"), "D")
	m.height = 12
	m.refit()
	m = pressSettle(t, m, "}")
	if m.logAt == 0 && !m.logStick {
		t.Fatalf("} did not page the diff (rows %d, lines %d)", m.textRows(), len(m.textLines()))
	}
	m = pressSettle(t, m, "g")
	if m.logAt != 0 || m.logStick {
		t.Fatalf("g: at %d stick %v", m.logAt, m.logStick)
	}
}

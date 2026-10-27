package ui

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"k2/internal/config"
	"k2/internal/shell"
	"k2/internal/state"

	"github.com/charmbracelet/x/ansi"
)

func v7Tree(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../kube/testdata/xray.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const v7Deploy = `{"kind":"Deployment","metadata":{"name":"web","namespace":"shop","creationTimestamp":"2026-10-23T08:00:00Z"},
 "spec":{"replicas":2,"selector":{"matchLabels":{"app":"web"}},"template":{"metadata":{"labels":{"app":"web"}}}},"status":{"readyReplicas":1}}`

const v7Pods = `{"items":[
 {"metadata":{"name":"web-7d9f-aaaaa","namespace":"shop","creationTimestamp":"2026-10-23T08:01:00Z"},"spec":{"containers":[{"name":"app"}]},
  "status":{"phase":"Running","containerStatuses":[{"ready":true,"restartCount":0,"state":{"running":{}}}]}},
 {"metadata":{"name":"web-7d9f-bbbbb","namespace":"shop","creationTimestamp":"2026-10-23T08:02:00Z"},"spec":{"containers":[{"name":"app"}]},
  "status":{"phase":"Running","containerStatuses":[{"ready":true,"restartCount":0,"state":{"running":{}}}]}}]}`

func helmPayload(rel map[string]any) string {
	js, _ := json.Marshal(rel)
	var gz bytes.Buffer
	w := gzip.NewWriter(&gz)
	w.Write(js)
	w.Close()
	return base64.StdEncoding.EncodeToString([]byte(base64.StdEncoding.EncodeToString(gz.Bytes())))
}

func v7Helm() string {
	rel := func(name, ns string, rev int, status string) string {
		r := map[string]any{"name": name, "namespace": ns, "version": rev,
			"info":     map[string]any{"status": status, "last_deployed": "2026-10-2" + fmt.Sprint(rev) + "T08:00:00Z", "notes": "open http://" + name},
			"chart":    map[string]any{"metadata": map[string]any{"name": "nginx", "version": fmt.Sprintf("1.%d.0", rev), "appVersion": "1.25"}, "values": map[string]any{"replicas": 1, "debug": true}},
			"config":   map[string]any{"replicas": 3},
			"manifest": "---\nkind: Deployment\nmetadata:\n  name: " + name + "\n"}
		return fmt.Sprintf(`{"kind":"Secret","metadata":{"name":"sh.helm.release.v1.%s.v%d","namespace":%q,"labels":{"owner":"helm","name":%q,"version":"%d","status":%q}},"data":{"release":%q}}`,
			name, rev, ns, name, rev, status, helmPayload(r))
	}
	return `{"kind":"List","items":[` + rel("web", "shop", 1, "superseded") + "," + rel("web", "shop", 2, "deployed") + "," + rel("api", "lab", 1, "failed") + `]}`
}

func v7Cluster(t *testing.T) *shell.Fake {
	tree := v7Tree(t)
	helm := v7Helm()

	var list struct {
		Items []map[string]any `json:"items"`
	}
	json.Unmarshal([]byte(tree), &list)
	var pods []map[string]any
	for _, o := range list.Items {
		if o["kind"] == "Pod" {
			pods = append(pods, o)
		}
	}
	pj, _ := json.Marshal(map[string]any{"items": pods})
	treePods := string(pj)
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		line := strings.Join(r.Args, " ")
		if r.Name == "helm" {
			return shell.OK("ok")
		}
		switch {
		case strings.HasPrefix(line, "get pods,replicasets,deployments"):
			return shell.OK(tree)
		case strings.HasPrefix(line, "get pods,deployments,statefulsets,daemonsets,jobs,persistentvolumeclaims"):
			return shell.OK(tree)
		case strings.HasPrefix(line, "get events") && strings.Contains(line, "type=Warning"):
			return shell.OK(`{"items":[{"kind":"Event","metadata":{"name":"e","namespace":"shop"},"type":"Warning","reason":"BackOff","message":"Back-off restarting","count":4,"lastTimestamp":"2026-10-23T10:00:00Z","involvedObject":{"kind":"Pod","name":"web-7d9f-a"}}]}`)
		case strings.HasPrefix(line, "top pods"):
			return shell.OK("web-7d9f-a 100m 64Mi\nweb-7d9f-b 50m 32Mi\n")
		case strings.HasPrefix(line, "get secrets") && strings.Contains(line, "owner=helm"):
			return shell.OK(helm)
		case strings.HasPrefix(line, "get deployments web -o json"), strings.HasPrefix(line, "get deploy web -o json"):
			return shell.OK(v7Deploy)
		case strings.HasPrefix(line, "get deployments -o json"), strings.HasPrefix(line, "get deploy -o json"):
			return shell.OK(`{"items":[` + v7Deploy + `]}`)
		case strings.HasPrefix(line, "get pods -o json") && strings.Contains(line, "-l app=web"):
			return shell.OK(v7Pods)
		case strings.HasPrefix(line, "get pods -o json"):
			return shell.OK(treePods)
		case strings.HasPrefix(line, "get") && strings.Contains(line, "-o yaml"):
			return shell.OK("kind: " + r.Args[1] + "\nmetadata:\n  name: " + r.Args[2] + "\n")
		case strings.HasPrefix(line, "describe"):
			return shell.OK("Name: " + r.Args[2])
		case strings.HasPrefix(line, "logs "):
			pod := r.Args[1]
			return shell.OK("[pod/" + pod + "/app] 2026-10-23T10:00:01.5Z hello from " + pod + "\n[pod/" + pod + "/app] 2026-10-23T10:00:02Z second " + pod + "\n")
		}
		return shell.Fail(1, "not in this test: "+line)
	}}
}

func v7Model(t *testing.T, f *shell.Fake) model {
	t.Helper()
	m := newTestModel(t, testEnv(t), f)
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	return m
}

func rowIDs(m model) []string {
	var out []string
	for _, raw := range m.content.Items() {
		out = append(out, raw.(item).id)
	}
	return out
}

func helmCalls(f *shell.Fake) []string {
	var out []string
	for _, c := range f.Calls() {
		if c.Name == "helm" {
			out = append(out, strings.Join(c.Args, " "))
		}
	}
	return out
}

func TestXrayFromTableJumpAndBack(t *testing.T) {
	f := v7Cluster(t)
	m := open(t, v7Model(t, f), "deploy")
	if m.stage != "res" {
		t.Fatalf("stage %s: %s", m.stage, m.status)
	}
	m = pressSettle(t, selectRow(t, m, "web"), "X")
	if m.stage != "xray" || m.load.active() {
		t.Fatalf("stage %s load %v status %s", m.stage, m.load.active(), m.status)
	}
	ids := strings.Join(rowIDs(m), " ")
	for _, want := range []string{"deploy/shop/web", "svc/shop/web", "ing/shop/web", "rs/shop/web-7d9f", "pod/shop/web-7d9f-a", "container/shop/web-7d9f-a/side", "cm/shop/web-config", "secret/shop/db-creds", "pvc/shop/web-data"} {
		if !strings.Contains(ids, want) {
			t.Fatalf("tree lacks %s: %s", want, ids)
		}
	}
	view := ansi.Strip(m.View())
	for _, want := range []string{"Deployment", "├ ", "CrashLoopBackOff", "xray deploy/web", "fold"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view lacks %q:\n%s", want, view)
		}
	}

	m = pressSettle(t, selectRow(t, m, "rs/shop/web-7d9f"), " ")
	if strings.Contains(strings.Join(rowIDs(m), " "), "pod/shop/web-7d9f-a") || !strings.Contains(ansi.Strip(m.View()), "▸ web-7d9f") {
		t.Fatalf("not collapsed: %v", rowIDs(m))
	}
	m = pressSettle(t, m, " ")
	if !strings.Contains(strings.Join(rowIDs(m), " "), "pod/shop/web-7d9f-a") {
		t.Fatal("not expanded again")
	}

	m = pressSettle(t, selectRow(t, m, "cm/shop/web-config"), "y")
	if m.stage != "yaml" || !strings.Contains(m.logText, "kind: configmaps") {
		t.Fatalf("yaml %s %q", m.stage, m.logText)
	}
	m = pressSettle(t, m, "esc")
	if m.stage != "xray" {
		t.Fatalf("back from yaml: %s", m.stage)
	}

	m = pressSettle(t, selectRow(t, m, "pod/shop/web-7d9f-a"), "d")
	if m.stage != "describe" || !strings.Contains(m.logText, "web-7d9f-a") {
		t.Fatalf("describe %s %q", m.stage, m.logText)
	}
	m = pressSettle(t, m, "esc")
	if m.stage != "xray" {
		t.Fatalf("back from describe: %s", m.stage)
	}

	m = pressSettle(t, m, "enter")
	if m.stage != "workpods" {
		t.Fatalf("jump: %s %s", m.stage, m.status)
	}
	if it, _ := m.selectedItem(); it.id != "web-7d9f-a" {
		t.Fatalf("selected %q", it.id)
	}
	m = pressSettle(t, m, "esc")
	if m.stage != "xray" {
		t.Fatalf("back to the tree: %s", m.stage)
	}
	if it, _ := m.selectedItem(); it.id != "pod/shop/web-7d9f-a" {
		t.Fatalf("cursor %q", it.id)
	}

	m = pressSettle(t, selectRow(t, m, "container/shop/web-7d9f-a/side"), "enter")
	if m.stage != "logs" || m.vs.logCont != "side" || m.logPod != "shop/web-7d9f-a" {
		t.Fatalf("container log %s %q %q", m.stage, m.vs.logCont, m.logPod)
	}
	m = pressSettle(t, m, "esc")
	if m.stage != "xray" {
		t.Fatalf("back from the log: %s", m.stage)
	}

	n := 0
	for _, c := range f.Calls() {
		if strings.HasPrefix(strings.Join(c.Args, " "), "get pods,replicasets") {
			n++
		}
	}
	if n < 1 || n > 4 {
		t.Fatalf("%d tree reads", n)
	}

	m = pressSettle(t, m, "esc")
	if m.stage != "res" || m.kind == "" {
		t.Fatalf("esc from the tree: %s %s", m.stage, m.kind)
	}
	if it, _ := m.selectedItem(); it.id != "web" {
		t.Fatalf("esc from the tree selected %q, want web", it.id)
	}
}

func TestXrayCommandAndErrors(t *testing.T) {
	f := v7Cluster(t)
	m := open(t, v7Model(t, f), "xray cronjob")
	if m.stage != "xray" || !strings.Contains(strings.Join(rowIDs(m), " "), "job/shop/report-1") {
		t.Fatalf("cronjob tree %s %v %s", m.stage, rowIDs(m), m.status)
	}
	m = pressSettle(t, m, "esc")
	if m.stage != "nav" {
		t.Fatalf("esc from a command xray: %s", m.stage)
	}
	m = open(t, v7Model(t, f), "xray configmaps")
	if m.stage == "xray" || !strings.Contains(m.status, "no owner tree") {
		t.Fatalf("bad kind %s %s", m.stage, m.status)
	}
	m = open(t, v7Model(t, f), "xray deploy nope")
	if m.stage == "xray" || m.load.active() || !strings.Contains(m.status, "no deploy/nope") {
		t.Fatalf("missing %s %s", m.stage, m.status)
	}

	bad := &shell.Fake{Handler: func(shell.Request) shell.Result { return shell.Fail(1, "Forbidden") }}
	m = open(t, v7Model(t, bad), "xray deploy")
	if m.stage == "xray" || m.load.active() || !strings.Contains(m.status, "Forbidden") {
		t.Fatalf("error %s %v %s", m.stage, m.load.active(), m.status)
	}
}

func TestXrayRefreshKeepsCollapseAndCursor(t *testing.T) {
	f := v7Cluster(t)
	m := open(t, v7Model(t, f), "xray deploy web")
	m = pressSettle(t, selectRow(t, m, "rs/shop/web-old"), " ")
	m = selectRow(t, m, "svc/shop/web")
	m.beat = m.cfg.UI.RefreshEvery - 1
	next, cmd := m.Update(tickMsg{})
	m = settle(t, next.(model), cmd)
	if it, _ := m.selectedItem(); it.id != "svc/shop/web" || !m.vs.xray.collapsed["rs/shop/web-old"] {
		t.Fatalf("refresh lost state: %q %v", it.id, m.vs.xray.collapsed)
	}
}

func TestMultiLogsFromDeployment(t *testing.T) {
	f := v7Cluster(t)
	m := open(t, v7Model(t, f), "deploy")
	m = pressSettle(t, selectRow(t, m, "web"), "M")
	if m.stage != "mlogs" || m.load.active() {
		t.Fatalf("stage %s load %v %s", m.stage, m.load.active(), m.status)
	}
	if len(calls(f, "logs")) != 2 {
		t.Fatalf("logs calls %v", calls(f, "logs"))
	}
	for _, c := range calls(f, "logs") {
		if !strings.Contains(c, "--prefix --timestamps") || !strings.Contains(c, "--all-containers") || !strings.Contains(c, "--tail=80") {
			t.Fatalf("args %s", c)
		}
	}
	for _, want := range []string{"web-7d9f-aaaaa/app hello from web-7d9f-aaaaa", "web-7d9f-bbbbb/app second web-7d9f-bbbbb"} {
		if !strings.Contains(m.logText, want) {
			t.Fatalf("text lacks %q:\n%s", want, m.logText)
		}
	}
	view := ansi.Strip(m.View())
	for _, want := range []string{"follow", "2 pods", "1-4/4", "logs · deployments/web"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view lacks %q:\n%s", want, view)
		}
	}

	m.beat = m.cfg.Logs.MultiEvery - 1
	next, cmd := m.Update(tickMsg{})
	m = settle(t, next.(model), cmd)
	if got := strings.Count(m.logText, "\n"); got != 4 {
		t.Fatalf("%d lines after a poll:\n%s", got, m.logText)
	}
	last := calls(f, "logs")[len(calls(f, "logs"))-1]
	if !strings.Contains(last, "--since-time=2026-10-23T10:00:02Z") {
		t.Fatalf("poll args %s", last)
	}

	m = pressSettle(t, m, "/")
	if m.dlg == nil || m.dlg.act != "logfilter" {
		t.Fatalf("filter popup %+v", m.dlg)
	}
	m = typeText(t, m, "second")
	m = pressSettle(t, m, "enter")
	if strings.Contains(m.logText, "hello") || strings.Count(m.logText, "\n") != 2 {
		t.Fatalf("filtered:\n%s", m.logText)
	}

	m = pressSettle(t, m, "w")
	if !m.vs.wrapFlip {
		t.Fatal("wrap")
	}
	m = pressSettle(t, m, "y")
	if m.copy == nil {
		t.Fatal("copy mode")
	}
	m = pressSettle(t, m, "esc")

	m = pressSettle(t, m, "esc")
	if m.stage != "res" || m.vs.ml != nil {
		t.Fatalf("back %s", m.stage)
	}
}

func TestMultiLogsRingBufferAndColors(t *testing.T) {
	trueColor(t)
	f := v7Cluster(t)
	m := v7Model(t, f)
	m.cfg.Logs.MultiMaxLines = 3
	m = open(t, m, "logs app=web --tail=5")
	if m.stage != "mlogs" || m.vs.ml.q.Selector != "app=web" {
		t.Fatalf("stage %s", m.stage)
	}
	if strings.Count(m.logText, "\n") != 3 || m.vs.ml.dropped != 1 {
		t.Fatalf("ring buffer: %d dropped\n%s", m.vs.ml.dropped, m.logText)
	}
	if c := calls(f, "logs"); len(c) != 2 || !strings.Contains(c[0], "--tail=5") {
		t.Fatalf("calls %v", c)
	}

	a, b := sourceColor("web-7d9f-aaaaa/app"), sourceColor("web-7d9f-aaaaa/app")
	if a != b {
		t.Fatal("unstable color")
	}
	if got := m.colorMlog("web-7d9f-bbbbb/app second"); !strings.Contains(got, sgr(sourceColor("web-7d9f-bbbbb/app"))) {
		t.Fatalf("prefix not colored: %q", got)
	}

	f2 := v7Cluster(t)
	m = open(t, v7Model(t, f2), "logs ^web-7d9f-b")
	if c := calls(f2, "logs"); len(c) != 1 || !strings.HasPrefix(c[0], "logs web-7d9f-b ") {
		t.Fatalf("regex calls %v", c)
	}
	m = open(t, v7Model(t, f2), "logs --since=x web")
	if m.stage == "mlogs" || !strings.Contains(m.status, "logs:") {
		t.Fatalf("bad query %s %s", m.stage, m.status)
	}
}

func TestSiblingsRegex(t *testing.T) {
	cases := map[string][2]string{
		"web-7d9f8c-abcde": {"web-7d9f8c-zzzzz", "web2-7d9f8c-abcde"},
		"db-0":             {"db-12", "dbx-0"},
		"node-agent-x7k2p": {"node-agent-q1w2e", "node-agent-x7k2p-1"},
		"single":           {"single", "single-2"},
	}
	for name, c := range cases {
		re := siblingsRegex(name)
		if !re.MatchString(name) || !re.MatchString(c[0]) || re.MatchString(c[1]) {
			t.Errorf("%s: %s", name, re)
		}
	}
}

func TestHelmReleasesView(t *testing.T) {
	f := v7Cluster(t)
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	t.Cleanup(func() { lookPath = defaultLookPath })
	m := open(t, v7Model(t, f), "helm")
	if m.stage != "helm" || m.load.active() {
		t.Fatalf("stage %s %s", m.stage, m.status)
	}
	view := ansi.Strip(m.View())
	for _, want := range []string{"NAME", "REVISION", "STATUS", "CHART", "APP VERSION", "web", "deployed", "nginx-1.2.0", "1.25", "shop/helm"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view lacks %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "rollback") {
		t.Fatal("rollback offered without the helm CLI")
	}

	if c := calls(f, "get"); len(c) == 0 || !strings.Contains(c[0], "secrets -o json") || !strings.Contains(c[0], "-l owner=helm") || !strings.Contains(c[0], "-n shop") {
		t.Fatalf("calls %v", c)
	}
	m = selectRow(t, m, "web")
	checks := []struct{ key, stage, want string }{
		{"v", "yaml", "replicas: 3"},
		{"enter", "yaml", "user-supplied values"},
		{"V", "yaml", "debug: true"},
		{"M", "yaml", "kind: Deployment"},
		{"N", "view", "open http://web"},
	}
	for _, c := range checks {
		m = pressSettle(t, m, c.key)
		if m.stage != c.stage || !strings.Contains(m.logText, c.want) || m.load.active() {
			t.Fatalf("%s: stage %s load %v text:\n%s", c.key, m.stage, m.load.active(), m.logText)
		}
		m = pressSettle(t, m, "esc")
		if m.stage != "helm" {
			t.Fatalf("%s: back went to %s", c.key, m.stage)
		}
	}

	m = pressSettle(t, m, "t")
	if m.stage != "helmhist" || !strings.Contains(rowsText(m), "superseded") {
		t.Fatalf("history: stage %s rows:\n%s", m.stage, rowsText(m))
	}
	m = pressSettle(t, m, "esc")
	if m.stage != "helm" {
		t.Fatalf("history back went to %s", m.stage)
	}

	m = pressSettle(t, m, "V")
	if !strings.Contains(m.logText, "replicas: 3") {
		t.Fatalf("computed:\n%s", m.logText)
	}
	m = pressSettle(t, m, "esc")

	m = pressSettle(t, m, "r")
	if m.dlg != nil || !strings.Contains(m.status, "needs the helm CLI") {
		t.Fatalf("rollback without helm: %+v %s", m.dlg, m.status)
	}

	m = pressSettle(t, m, "0")
	if !m.vs.helmAll || !strings.Contains(ansi.Strip(m.View()), "NAMESPACE") || !strings.Contains(ansi.Strip(m.View()), "api") {
		t.Fatalf("all namespaces:\n%s", ansi.Strip(m.View()))
	}
	last := calls(f, "get")[len(calls(f, "get"))-1]
	if !strings.Contains(last, "-A") {
		t.Fatalf("not -A: %s", last)
	}
	m = pressSettle(t, m, "0")
	if m.vs.helmAll || strings.Contains(ansi.Strip(m.View()), "NAMESPACE") {
		t.Fatalf("back to shop:\n%s", ansi.Strip(m.View()))
	}
	if len(helmCalls(f)) != 0 {
		t.Fatalf("helm ran: %v", helmCalls(f))
	}
}

var defaultLookPath = lookPath

func TestHelmRollbackAndUninstallConfirm(t *testing.T) {
	f := v7Cluster(t)
	lookPath = func(string) (string, error) { return "/usr/bin/helm", nil }
	t.Cleanup(func() { lookPath = defaultLookPath })
	m := v7Model(t, f)
	m.cfg.Prod = []string{"kind-dev"}
	m = selectRow(t, open(t, m, "helm"), "web")
	if !strings.Contains(m.status, "r rollback, ctrl+d uninstall") {
		t.Fatalf("rollback not in the hint: %s", m.status)
	}
	m = pressSettle(t, m, "r")
	if m.dlg == nil || !m.dlg.danger || !strings.Contains(m.dlg.ask, "roll back web to revision 1?") {
		t.Fatalf("confirm %+v", m.dlg)
	}
	view := ansi.Strip(m.View())
	for _, want := range []string{skull, "helm rollback", "release shop/web (revision 2, nginx-1.2.0)", "default no"} {
		if !strings.Contains(view, want) {
			t.Fatalf("popup lacks %q:\n%s", want, view)
		}
	}
	m = pressSettle(t, m, "enter")
	if m.dlg == nil || len(helmCalls(f)) != 0 {
		t.Fatal("enter answered")
	}
	m = pressSettle(t, m, "y")
	if c := helmCalls(f); len(c) != 1 || c[0] != "rollback web -n shop --kubeconfig /kc --kube-context kind-dev" {
		t.Fatalf("helm calls %v", c)
	}
	if m.load.active() || m.stage != "helm" {
		t.Fatalf("spinner left on: %v %s", m.load.active(), m.stage)
	}

	m = pressSettle(t, m, "ctrl+d")
	if m.dlg == nil || !strings.Contains(m.dlg.ask, "uninstall release web?") {
		t.Fatalf("uninstall confirm %+v", m.dlg)
	}
	m = pressSettle(t, m, "n")
	if m.dlg != nil || len(helmCalls(f)) != 1 {
		t.Fatal("n ran the uninstall")
	}
	m = pressSettle(t, m, "ctrl+d", "y")
	if c := helmCalls(f); len(c) != 2 || !strings.HasPrefix(c[1], "uninstall web -n shop") {
		t.Fatalf("helm calls %v", c)
	}

	m.cfg.Helm.Binary = ""
	if m.helmCLI() != "" {
		t.Fatal("helm.binary empty")
	}
}

func TestFavoritesPicker(t *testing.T) {
	f := v7Cluster(t)
	cfg := testEnv(t)
	cfg.Favorites = []config.Favorite{{Name: "lab", Context: "kind-lab", Namespace: "lab", View: "helm all"}}
	m := newTestModel(t, cfg, f)
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	m = open(t, m, "deploy")
	m = pressSettle(t, m, "b")
	if m.dlg == nil || m.dlg.act != "favorites" || len(m.dlg.options) != 1 || !strings.Contains(m.dlg.notes[0], "config") {
		t.Fatalf("picker %+v", m.dlg)
	}

	m = pressSettle(t, m, "a")
	if len(m.favs) != 1 || m.favs[0] != (state.Favorite{Kubeconfig: "/kc", Context: "kind-dev", Namespace: "shop", View: m.kind}) || m.kind == "" {
		t.Fatalf("favs %+v", m.favs)
	}
	if st := state.Load(cfg.StateDir); len(st.Favorites) != 1 {
		t.Fatalf("not saved: %+v", st.Favorites)
	}
	view := ansi.Strip(m.View())
	for _, want := range []string{"★ favorites", "1 lab", "2 kind-dev · shop", ":" + m.kind} {
		if !strings.Contains(view, want) {
			t.Fatalf("picker lacks %q:\n%s", want, view)
		}
	}

	m = pressSettle(t, m, "a")
	if len(m.favs) != 1 || m.dlg.msg != "already pinned" {
		t.Fatalf("dup %+v", m.dlg)
	}
	m.dlg.idx = 0
	m = pressSettle(t, m, "x")
	if len(m.cfg.Favorites) != 1 || !strings.Contains(m.dlg.msg, "config.yaml") {
		t.Fatalf("removed a config entry: %+v", m.dlg)
	}

	m = pressSettle(t, m, "esc")
	m.namespace = "other"
	m = open(t, m, "pods")
	m = pressSettle(t, m, "b", "2")
	if m.dlg != nil || m.context != "kind-dev" || m.namespace != "shop" || m.stage != "res" || m.kind != m.favs[0].View {
		t.Fatalf("jump: dlg %v %s/%s %s %s", m.dlg != nil, m.context, m.namespace, m.stage, m.kind)
	}

	m = pressSettle(t, m, "b", "1")
	if m.context != "kind-lab" || m.namespace != "lab" || m.stage != "helm" || !m.vs.helmAll {
		t.Fatalf("config jump %s/%s %s", m.context, m.namespace, m.stage)
	}

	m.favs = append(m.favs, state.Favorite{Context: "kind-x", Namespace: "x"})
	m = pressSettle(t, m, "b")
	m.dlg.idx = 2
	m = pressSettle(t, m, "K")
	if m.favs[0].Context != "kind-x" || m.dlg.idx != 1 {
		t.Fatalf("move up %+v idx %d", m.favs, m.dlg.idx)
	}
	m = pressSettle(t, m, "J")
	if m.favs[1].Context != "kind-x" {
		t.Fatalf("move down %+v", m.favs)
	}
	m = pressSettle(t, m, "x")
	if len(m.favs) != 1 || m.favs[0].Context != "kind-dev" {
		t.Fatalf("remove %+v", m.favs)
	}
	m = pressSettle(t, m, "esc")
	if m.dlg != nil {
		t.Fatal("esc")
	}

	m = open(t, m, "fav")
	if m.dlg == nil || m.dlg.act != "favorites" {
		t.Fatal(":fav")
	}
}

func TestFavoritesKeyInTextFields(t *testing.T) {
	m := v7Model(t, v7Cluster(t))
	m = pressSettle(t, m, ":")
	m = typeText(t, m, "b")
	if m.dlg != nil || m.cmd.Value() != "b" {
		t.Fatalf("b in the command line: %v %q", m.dlg != nil, m.cmd.Value())
	}
}

func TestPulse(t *testing.T) {
	f := v7Cluster(t)
	m := open(t, v7Model(t, f), "pulse")
	if m.stage != "pulse" || m.load.active() {
		t.Fatalf("stage %s %s", m.stage, m.status)
	}
	for _, want := range []string{"PULSE  shop · kind-dev", "PODS  4", "running", "crashing   1", "deployments   0/1 ready", "statefulsets  1/1 ready",
		"jobs", "1 failed", "USAGE", "cpu", "WARNINGS  1", "BackOff ×4", "TROUBLE"} {
		if !strings.Contains(m.logText, want) {
			t.Fatalf("pulse lacks %q:\n%s", want, m.logText)
		}
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "shop/pulse") {
		t.Fatalf("title:\n%s", view)
	}

	before := len(f.Calls())
	m.beat = m.cfg.UI.RefreshEvery - 1
	next, cmd := m.Update(tickMsg{})
	m = settle(t, next.(model), cmd)
	if len(f.Calls()) <= before || m.stage != "pulse" {
		t.Fatal("no refresh")
	}

	m2 := v7Model(t, f)
	m2.namespace = ""
	m2 = open(t, m2, "pulse")
	if m2.stage == "pulse" || !strings.Contains(m2.status, "needs a namespace") {
		t.Fatalf("no namespace: %s %s", m2.stage, m2.status)
	}
	m = pressSettle(t, m, "esc")
	if m.stage != "nav" {
		t.Fatalf("back %s", m.stage)
	}
}

func TestPulseUsageColors(t *testing.T) {
	m := v7Model(t, v7Cluster(t))
	if got := m.usageLine("cpu", 95, 100, false, cpuText); !strings.HasPrefix(got, "  "+markErr) || !strings.Contains(got, " 95%") {
		t.Fatalf("crit %q", got)
	}
	if got := m.usageLine("cpu", 75, 100, true, cpuText); !strings.HasPrefix(got, "  "+markWarn) || !strings.Contains(got, "requests ~") {
		t.Fatalf("warn %q", got)
	}
	if got := m.usageLine("memory", 10, 0, false, memText); !strings.Contains(got, "no limits") {
		t.Fatalf("no limits %q", got)
	}
	if cpuText(1250) != "1.25" || cpuText(350) != "350m" || memText(512*1024*1024) != "512Mi" || memText(1.5*1024*1024*1024) != "1.5Gi" {
		t.Fatal("formats")
	}
}

func TestExtraScreensSaveAsHome(t *testing.T) {
	f := v7Cluster(t)
	m := open(t, v7Model(t, f), "helm")
	m.save()
	st := state.Load(m.cfg.StateDir)
	if len(st.Panes) == 0 || st.Panes[0].Stage != "nav" {
		t.Fatalf("saved %+v", st.Panes)
	}
}

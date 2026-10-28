package ui

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"k2/internal/shell"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

const actionPods = `{"items":[
 {"metadata":{"name":"web-1","namespace":"shop","creationTimestamp":"2026-10-22T10:00:00Z"},
  "spec":{"nodeName":"n","initContainers":[{"name":"migrate"}],"containers":[{"name":"app"},{"name":"side"}]},
  "status":{"phase":"Running","containerStatuses":[{"ready":true,"restartCount":5,"state":{}},{"ready":true,"restartCount":0,"state":{}}]}},
 {"metadata":{"name":"db-0","namespace":"shop","creationTimestamp":"2026-10-20T10:00:00Z"},
  "spec":{"nodeName":"n","containers":[{"name":"pg"}]},
  "status":{"phase":"Running","containerStatuses":[{"ready":true,"restartCount":1,"state":{}}]}},
 {"metadata":{"name":"cache-0","namespace":"shop","creationTimestamp":"2026-10-21T10:00:00Z"},
  "spec":{"nodeName":"n","containers":[{"name":"redis"}]},
  "status":{"phase":"Pending","containerStatuses":[{"ready":false,"restartCount":0,"state":{}}]}}
]}`

const actionJobPods = `{"items":[
 {"metadata":{"name":"pi-x7","namespace":"shop","creationTimestamp":"2026-10-22T10:00:00Z"},
  "spec":{"containers":[{"name":"pi"}]},"status":{"phase":"Succeeded"}}]}`

const actionDeploys = `{"items":[
 {"metadata":{"name":"web","namespace":"shop","creationTimestamp":"2026-10-22T10:00:00Z"},
  "spec":{"replicas":3},"status":{"readyReplicas":3,"updatedReplicas":3,"availableReplicas":3}}]}`

const actionCron = `{"items":[
 {"metadata":{"name":"nightly","namespace":"shop","creationTimestamp":"2026-10-22T10:00:00Z"},
  "spec":{"schedule":"0 2 * * *"},"status":{}}]}`

const actionJobs = `{"items":[
 {"metadata":{"name":"pi","namespace":"shop","creationTimestamp":"2026-10-22T10:00:00Z"},
  "spec":{"completions":1},"status":{"succeeded":1,"conditions":[{"type":"Complete","status":"True"}]}},
 {"metadata":{"name":"zz","namespace":"shop","creationTimestamp":"2026-10-22T10:00:00Z"},
  "spec":{"completions":1},"status":{"failed":1,"conditions":[{"type":"Failed","status":"True"}]}}]}`

const actionEvents = `{"items":[
 {"metadata":{"name":"e1","namespace":"shop"},"type":"Warning","reason":"BackOff","message":"Back-off restarting",
  "involvedObject":{"kind":"Pod","name":"web-1"},"lastTimestamp":"2026-10-22T10:05:00Z"},
 {"metadata":{"name":"e0","namespace":"shop"},"type":"Normal","reason":"Pulled","message":"pulled image",
  "involvedObject":{"kind":"Pod","name":"web-1"},"lastTimestamp":"2026-10-22T10:01:00Z"}]}`

func actionCluster() *shell.Fake {
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		if len(r.Args) < 2 {
			return shell.Fail(1, "?")
		}
		switch r.Args[0] {
		case "get":
			switch r.Args[1] {
			case "pods":
				if strings.Contains(strings.Join(r.Args, " "), "-l job-name=pi") {
					return shell.OK(actionJobPods)
				}
				return shell.OK(actionPods)
			case "deployments", "deploy", "dp":
				return shell.OK(actionDeploys)
			case "cronjobs":
				return shell.OK(actionCron)
			case "jobs":
				return shell.OK(actionJobs)
			case "events", "ev":
				return shell.OK(actionEvents)
			}
			return shell.Fail(1, `error: the server doesn't have a resource type "`+r.Args[1]+`"`)
		case "logs":
			return shell.OK("line from " + strings.Join(r.Args, " "))
		case "scale", "rollout", "create", "label", "annotate", "patch":
			return shell.OK("ok")
		}
		return shell.Fail(1, "not in this test")
	}}
}

func trueColor(t *testing.T) {
	t.Helper()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
}

func actionModel(t *testing.T, f *shell.Fake) model {
	t.Helper()
	m := newTestModel(t, testEnv(t), f)
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	n, c := m.openPods()
	return settle(t, n.(model), c)
}

func selectRow(t *testing.T, m model, id string) model {
	t.Helper()
	for i, raw := range m.content.Items() {
		if raw.(item).id == id {
			m.content.Select(i)
			return m
		}
	}
	t.Fatalf("no row %q", id)
	return m
}

func calls(f *shell.Fake, verb string) []string {
	var out []string
	for _, c := range f.Calls() {
		if c.Name == "kubectl" && len(c.Args) > 0 && c.Args[0] == verb {
			out = append(out, strings.Join(c.Args, " "))
		}
	}
	return out
}

func typeText(t *testing.T, m model, s string) model {
	t.Helper()
	for _, r := range s {
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = settle(t, next.(model), cmd)
	}
	return m
}

func open(t *testing.T, m model, q string) model {
	t.Helper()
	next, cmd := m.runCommand(q)
	return settle(t, next.(model), cmd)
}

func TestContainerPickerForLogs(t *testing.T) {
	f := actionCluster()
	m := selectRow(t, actionModel(t, f), "web-1")
	m = pressSettle(t, m, "enter")
	if m.dlg == nil || strings.Join(m.dlg.options, ",") != "app,side,migrate" || m.dlg.notes[2] != "init" {
		t.Fatalf("picker %+v", m.dlg)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "logs · container") || !strings.Contains(view, "migrate  init") {
		t.Fatalf("picker not drawn:\n%s", view)
	}
	m = pressSettle(t, m, "down", "enter")
	if m.dlg != nil || m.stage != "logs" || m.vs.logCont != "side" {
		t.Fatalf("stage %s cont %q", m.stage, m.vs.logCont)
	}
	logs := calls(f, "logs")
	if len(logs) == 0 || !strings.Contains(logs[len(logs)-1], "web-1 -n shop") || !strings.Contains(logs[len(logs)-1], "-c side") {
		t.Fatalf("logs %v", logs)
	}
	if l := m.paneLabel(m.focus); l != "web-1/logs · side" {
		t.Fatalf("label %q", l)
	}

	m = pressSettle(t, selectRow(t, actionModel(t, f), "web-1"), "enter", "esc")
	if m.dlg != nil || m.stage != "workpods" {
		t.Fatalf("cancel: %s", m.stage)
	}
}

func TestSingleContainerNoPicker(t *testing.T) {
	f := actionCluster()
	m := pressSettle(t, selectRow(t, actionModel(t, f), "db-0"), "enter")
	if m.dlg != nil || m.stage != "logs" || m.vs.logCont != "" {
		t.Fatalf("stage %s dlg %v", m.stage, m.dlg)
	}
}

func TestPreviousLogsToggle(t *testing.T) {
	f := actionCluster()
	m := pressSettle(t, selectRow(t, actionModel(t, f), "db-0"), "enter", "p")
	logs := calls(f, "logs")
	if !m.vs.logPrev || !strings.Contains(logs[len(logs)-1], "--previous") {
		t.Fatalf("prev %v %v", m.vs.logPrev, logs)
	}
	if l := m.paneLabel(m.focus); !strings.HasSuffix(l, "· previous") {
		t.Fatalf("label %q", l)
	}
	if !strings.Contains(m.logText, "--previous") || !strings.Contains(ansi.Strip(m.logsBody(100)), "previous") {
		t.Fatalf("text %q", m.logText)
	}
	n := len(calls(f, "logs"))
	next, _ := m.Update(tickMsg(time.Now()))
	m = next.(model)
	if len(calls(f, "logs")) != n {
		t.Fatal("previous logs were polled")
	}
	m = pressSettle(t, m, "p")
	logs = calls(f, "logs")
	if m.vs.logPrev || strings.Contains(logs[len(logs)-1], "--previous") || strings.Contains(m.paneLabel(m.focus), "previous") {
		t.Fatalf("back to live: %v", logs)
	}
}

func TestShellArgsAndPicker(t *testing.T) {
	got := strings.Join(shellArgs("/kc", "kind-dev", "shop", "web-1", "side"), " ")
	want := "exec -it web-1 -n shop --kubeconfig /kc --context kind-dev -c side -- sh -c " + shellScript
	if got != want {
		t.Fatalf("args\n%s\n%s", got, want)
	}
	if shellScript != "command -v bash >/dev/null && exec bash || exec sh" {
		t.Fatal(shellScript)
	}
	f := actionCluster()
	m := pressSettle(t, selectRow(t, actionModel(t, f), "web-1"), "s")
	if m.dlg == nil || m.dlg.act != "shell" || !strings.Contains(ansi.Strip(m.View()), "shell · container") {
		t.Fatalf("no picker %+v", m.dlg)
	}

	cfg := testEnv(t)
	cfg.UI.EditorInline = false
	m2 := newTestModel(t, cfg, f)
	m2.kubeconfig, m2.context, m2.namespace = "/kc", "kind-dev", "shop"
	n, c := m2.openPods()
	m2 = selectRow(t, settle(t, n.(model), c), "db-0")
	next, cmd := m2.Update(key(t, "s"))
	m2 = next.(model)
	if cmd == nil || m2.ed != nil || m2.dlg != nil {
		t.Fatalf("fallback: cmd %v ed %v", cmd != nil, m2.ed)
	}
	next, _ = m2.Update(shellDoneMsg{})
	if next.(model).status != "shell closed" {
		t.Fatal(next.(model).status)
	}
}

func TestScale(t *testing.T) {
	trueColor(t)
	f := actionCluster()
	m := open(t, actionModel(t, f), "deploy")
	if m.stage != "res" || len(m.content.Items()) != 1 {
		t.Fatalf("deploy table: %s %d", m.stage, len(m.content.Items()))
	}
	if h := m.content.Title; !strings.Contains(h, "READY") || !strings.Contains(h, "UP-TO-DATE") {
		t.Fatalf("header %q", h)
	}
	m = pressSettle(t, m, "S")
	if m.dlg == nil || m.dlg.input.Value() != "3" {
		t.Fatalf("scale popup %+v", m.dlg)
	}
	m = pressSettle(t, m, "backspace")
	m = typeText(t, m, "x")
	m = pressSettle(t, m, "enter")
	if m.dlg == nil || !strings.Contains(m.dlg.msg, "number") {
		t.Fatalf("validation %+v", m.dlg)
	}
	m = pressSettle(t, m, "backspace")
	m = typeText(t, m, "5")
	m = pressSettle(t, m, "enter")
	if s := calls(f, "scale"); len(s) != 1 || !strings.HasPrefix(s[0], "scale deploy/web -n shop --replicas=5 --kubeconfig /kc --context kind-dev") {
		t.Fatalf("scale %v", s)
	}
	if m.dlg != nil || !strings.Contains(m.toast.text, "scaled deploy/web to 5") {
		t.Fatalf("toast %q", m.toast.text)
	}

	f = actionCluster()
	cfg := testEnv(t)
	cfg.Prod = []string{"kind-dev"}
	m = newTestModel(t, cfg, f)
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	m = open(t, m, "deploy")
	m = pressSettle(t, m, "S", "enter")
	if m.dlg == nil || !strings.Contains(m.dlg.ask, "scale deploy/web to 3 on kind-dev?") || len(calls(f, "scale")) != 0 {
		t.Fatalf("prod ask %+v", m.dlg)
	}
	view := m.View()
	if !strings.Contains(ansi.Strip(view), skull+" ⇅ scale") || colorBefore(view, "scale deploy/web to 3 on") != sgr(th.Danger) {
		t.Fatalf("prod popup not red:\n%s", ansi.Strip(view))
	}
	m = pressSettle(t, m, "n")
	if m.dlg == nil || m.dlg.ask != "" {
		t.Fatal("n should return to the field")
	}
	m = pressSettle(t, m, "enter", "y")
	if len(calls(f, "scale")) != 1 || m.dlg != nil {
		t.Fatalf("prod scale %v", calls(f, "scale"))
	}
}

func TestRolloutRestart(t *testing.T) {
	f := actionCluster()
	m := open(t, actionModel(t, f), "deploy")
	m = pressSettle(t, m, "r")
	if m.dlg == nil || !strings.Contains(ansi.Strip(m.View()), "rollout restart deploy/web?") {
		t.Fatal("no question")
	}
	m = pressSettle(t, m, "n")
	if m.dlg != nil || len(calls(f, "rollout")) != 0 {
		t.Fatal("n must cancel")
	}
	m = pressSettle(t, m, "r", "y")
	if r := calls(f, "rollout"); len(r) != 1 || !strings.HasPrefix(r[0], "rollout restart deploy/web -n shop") {
		t.Fatalf("rollout %v", r)
	}
	if !strings.Contains(m.toast.text, "restarted deploy/web") {
		t.Fatalf("toast %q", m.toast.text)
	}

	m = pressSettle(t, actionModel(t, f), "r")
	if m.dlg != nil {
		t.Fatal("restart on pods")
	}
}

func TestCopyOSC52(t *testing.T) {
	var got []string
	old := clipboardWrite
	clipboardWrite = func(s string) error { got = append(got, s); return nil }
	t.Cleanup(func() { clipboardWrite = old })
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", "")
	t.Setenv("TMUX", "")
	m := pressSettle(t, selectRow(t, actionModel(t, actionCluster()), "db-0"), "w")
	want := "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte("db-0")) + "\a"
	if len(got) != 1 || got[0] != want || !strings.Contains(m.toast.text, "copied db-0 via OSC 52 (terminal must support it)") {
		t.Fatalf("copy %q toast %q", got, m.toast.text)
	}
	m = pressSettle(t, open(t, m, "deploy"), "W")
	dec, _ := base64.StdEncoding.DecodeString(strings.TrimSuffix(strings.TrimPrefix(got[1], "\x1b]52;c;"), "\a"))
	if string(dec) != "deploy/web -n shop" {
		t.Fatalf("ref %q", dec)
	}
}

func TestColumnSort(t *testing.T) {
	m := actionModel(t, actionCluster())
	names := func(m model) string {
		var n []string
		for _, raw := range m.content.Items() {
			n = append(n, raw.(item).id)
		}
		return strings.Join(n, ",")
	}
	if names(m) != "cache-0,db-0,web-1" {
		t.Fatalf("kubectl order %s", names(m))
	}
	m = pressSettle(t, m, ">")
	if m.vs.sortCol != "NAME" || !strings.Contains(tableHeader(m.content, m.vs), "NAME▲") {
		t.Fatalf("sort %q header %q", m.vs.sortCol, tableHeader(m.content, m.vs))
	}
	m = pressSettle(t, m, "=")
	if names(m) != "web-1,db-0,cache-0" || !strings.Contains(tableHeader(m.content, m.vs), "NAME▼") {
		t.Fatalf("desc %s", names(m))
	}

	m = pressSettle(t, m, ">", ">", ">")
	if m.vs.sortCol != "RESTARTS" || names(m) != "web-1,db-0,cache-0" {
		t.Fatalf("restarts desc: %s %s", m.vs.sortCol, names(m))
	}
	m = pressSettle(t, m, "=")
	if names(m) != "cache-0,db-0,web-1" {
		t.Fatalf("restarts asc %s", names(m))
	}
	m = pressSettle(t, m, "<")
	if m.vs.sortCol != "STATUS" || names(m)[:7] != "cache-0" {
		t.Fatalf("status %s %s", m.vs.sortCol, names(m))
	}

	m = pressSettle(t, m, ">", ">", ">", ">", ">", ">")
	if m.vs.sortCol != "AGE" || names(m) != "web-1,cache-0,db-0" {
		t.Fatalf("age %s %s", m.vs.sortCol, names(m))
	}

	m = selectRow(t, m, "cache-0")
	rows := loadRows(m.k, m.logID, "workpods", "pods", "/kc", "kind-dev", "shop", "", m.tableWidth())()
	m.applyRows(rows.(rowsMsg))
	if names(m) != "web-1,cache-0,db-0" || m.content.SelectedItem().(item).id != "cache-0" {
		t.Fatalf("refresh %s sel %s", names(m), m.content.SelectedItem().(item).id)
	}

	m = pressSettle(t, m, ">")
	if m.vs.sortCol != "" || names(m) != "cache-0,db-0,web-1" || strings.ContainsAny(tableHeader(m.content, m.vs), "▲▼") {
		t.Fatalf("off: %q %s", m.vs.sortCol, names(m))
	}
}

func TestFilterTitleAndClear(t *testing.T) {
	for _, q := range []string{"pods", "deploy", "events"} {
		m := actionModel(t, actionCluster())
		if q != "pods" {
			m = open(t, m, q)
		}
		m = pressSettle(t, m, "/")
		m = typeText(t, m, "w")
		if !strings.Contains(m.paneLabel(m.focus), "· /w") {
			t.Fatalf("%s: label %q", q, m.paneLabel(m.focus))
		}
		m = pressSettle(t, m, "enter")
		if m.content.FilterState() != list.FilterApplied || !strings.Contains(m.paneLabel(m.focus), "· /w") {
			t.Fatalf("%s: applied %v %q", q, m.content.FilterState(), m.paneLabel(m.focus))
		}
		stage := m.stage
		m = pressSettle(t, m, "esc")
		if m.content.FilterState() != list.Unfiltered || m.stage != stage || strings.Contains(m.paneLabel(m.focus), "/w") {
			t.Fatalf("%s: esc %v %s", q, m.content.FilterState(), m.stage)
		}
		m = pressSettle(t, m, "esc")
		if m.stage == stage {
			t.Fatalf("%s: second esc should go back", q)
		}
	}
}

func TestEventsView(t *testing.T) {
	trueColor(t)
	for _, q := range []string{"events", "ev"} {
		m := open(t, actionModel(t, actionCluster()), q)
		if m.stage != "res" || !strings.HasPrefix(m.content.Title, "LAST SEEN") {
			t.Fatalf("%s: %s %q", q, m.stage, m.content.Title)
		}
		first := m.content.Items()[0].(item)
		warn := m.content.Items()[1].(item)
		if first.id != "e0" || warn.id != "e1" {
			t.Fatalf("order %s %s", first.id, warn.id)
		}
		if colorBefore(warn.title, "Warning") != sgr(th.Warn) || colorBefore(warn.title, "Back-off") != sgr(th.Warn) {
			t.Fatalf("warning row not colored: %q", warn.title)
		}
		if colorBefore(first.title, "Normal") == sgr(th.Warn) {
			t.Fatalf("normal row colored %q", first.title)
		}
		if calls := calls(m.k.Run.(*shell.Fake), "get"); !strings.Contains(calls[len(calls)-1], "--sort-by=.lastTimestamp") {
			t.Fatalf("no sort-by: %v", calls)
		}
	}
}

func TestCronJobTrigger(t *testing.T) {
	f := actionCluster()
	m := open(t, actionModel(t, f), "cronjobs")
	if !strings.Contains(m.content.Title, "SCHEDULE") || !strings.Contains(m.content.Title, "LAST SCHEDULE") {
		t.Fatalf("cronjob header %q", m.content.Title)
	}
	m = pressSettle(t, m, "t")
	if m.dlg == nil || !strings.Contains(m.dlg.ask, "run cronjob/nightly now as job nightly-manual-") {
		t.Fatalf("ask %+v", m.dlg)
	}
	m = pressSettle(t, m, "y")
	c := calls(f, "create")
	if len(c) != 1 || !strings.HasPrefix(c[0], "create job --from=cronjob/nightly nightly-manual-") || !strings.Contains(c[0], " -n shop") {
		t.Fatalf("create %v", c)
	}
	if !strings.Contains(m.toast.text, "created job nightly-manual-") {
		t.Fatalf("toast %q", m.toast.text)
	}
	long := manualJobName(strings.Repeat("a", 60), time.Date(2026, 10, 22, 9, 8, 7, 0, time.UTC))
	if len(long) > 52 || !strings.HasSuffix(long, "-manual-1022090807") {
		t.Fatalf("long name %q", long)
	}
}

func TestJobsDrillToPods(t *testing.T) {
	trueColor(t)
	f := actionCluster()
	m := open(t, actionModel(t, f), "jobs")
	if !strings.Contains(m.content.Title, "COMPLETIONS") || !strings.Contains(m.content.Title, "DURATION") {
		t.Fatalf("jobs header %q", m.content.Title)
	}
	bad := m.content.Items()[1].(item)
	if colorBefore(bad.title, "Failed") != sgr(th.Danger) {
		t.Fatalf("failed not red %q", bad.title)
	}
	m = pressSettle(t, selectRow(t, m, "pi"), "enter")
	if m.stage != "workpods" || len(m.content.Items()) != 1 || m.content.Items()[0].(item).id != "pi-x7" {
		t.Fatalf("job pods: %s %d", m.stage, len(m.content.Items()))
	}
	if l := m.paneLabel(m.focus); l != "shop/pods of job/pi" {
		t.Fatalf("label %q", l)
	}
	m = pressSettle(t, m, "esc")
	if m.stage != "res" || m.kind != "jobs" || m.content.SelectedItem().(item).id != "pi" || m.vs.podSel != "" {
		t.Fatalf("back: %s %s", m.stage, m.kind)
	}
}

func TestLabelAnnotate(t *testing.T) {
	f := actionCluster()
	m := pressSettle(t, selectRow(t, actionModel(t, f), "db-0"), "L")
	if m.dlg == nil || m.dlg.act != "label" {
		t.Fatal("no label popup")
	}
	m = typeText(t, m, "oops")
	m = pressSettle(t, m, "enter")
	if m.dlg == nil || !strings.Contains(m.dlg.msg, "key=value") {
		t.Fatalf("validation %+v", m.dlg)
	}
	m.dlg.input.SetValue("team=db old-")
	m = pressSettle(t, m, "enter")
	if l := calls(f, "label"); len(l) != 1 || !strings.HasPrefix(l[0], "label pod/db-0 -n shop --overwrite team=db old-") {
		t.Fatalf("label %v", l)
	}
	m = pressSettle(t, m, "A")
	m.dlg.input.SetValue("note=hi")
	m = pressSettle(t, m, "enter")
	if a := calls(f, "annotate"); len(a) != 1 || !strings.HasPrefix(a[0], "annotate pod/db-0 -n shop --overwrite note=hi") {
		t.Fatalf("annotate %v", a)
	}
	if _, err := parsePairs("a=1 b- c=x=y"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "=x", "-", "plain"} {
		if _, err := parsePairs(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestEditCustomResourceArgs(t *testing.T) {
	cfg := testEnv(t)
	cfg.UI.EditorInline = false
	m := newTestModel(t, cfg, nil)
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	m.stage, m.kind = "res", "widgets.example.com"
	m.content.SetItems([]list.Item{item{id: "w1", title: "w1", desc: "shop"}})
	_, cmd := m.editSelected()
	if cmd == nil {
		t.Fatal("no edit command")
	}
}

func TestBuiltinAliases(t *testing.T) {
	m := actionModel(t, actionCluster())
	m = open(t, m, "dp")
	if m.stage != "res" || m.kind != "deployments" {
		t.Fatalf("dp -> %s %s (%s)", m.stage, m.kind, m.status)
	}

	m.resources = []string{"componentstatuses", "cs", "deployments", "pods"}
	if m.canonical("cs") != "cs" {
		t.Fatal("cluster name")
	}

	m = open(t, m, "sub")
	if !strings.Contains(m.status, "not installed on this cluster (no OLM)") {
		t.Fatalf("status %q", m.status)
	}
	m = open(t, m, "rt")
	if !strings.Contains(m.status, "(not OpenShift)") {
		t.Fatalf("status %q", m.status)
	}

	cfg := testEnv(t)
	cfg.Aliases = map[string]string{"dp": "cronjobs"}
	m2 := newTestModel(t, cfg, actionCluster())
	m2.kubeconfig, m2.context, m2.namespace = "/kc", "kind-dev", "shop"
	m2 = open(t, m2, "dp")
	if m2.kind != "cronjobs" {
		t.Fatalf("config alias: %s", m2.kind)
	}

	if c := m.cmdCandidates("cs"); !strings.Contains(strings.Join(c, " "), "csv") {
		t.Fatalf("candidates %v", c)
	}

	m = open(t, m, "subscriptions.operators.coreos.com")
	if !strings.Contains(m.status, "no OLM") && !strings.Contains(m.status, "unknown") {
		t.Fatalf("status %q", m.status)
	}
}

func TestInlineShell(t *testing.T) {
	bin := t.TempDir()
	script := "#!/bin/sh\necho \"ARGS:$*\"\nread x\n[ \"$x\" = fail ] && exit 3\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "kubectl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, input := range []string{"bye", "fail"} {
		h := newEdHarness(t, selectRow(t, actionModel(t, actionCluster()), "db-0"))
		h.keys("s")
		if !h.m.ed.live() || h.m.ed.kind != "shell" {
			t.Fatalf("no inline shell: %s", h.m.status)
		}
		if l := h.m.paneLabel(h.m.focus); l != `shell db-0 · shell · ctrl+a k2` {
			t.Fatalf("title %q", l)
		}
		h.pump(func() bool { return strings.Contains(h.screen(), "ARGS:exec -it db-0 -n shop") }, "exec args")
		if !strings.Contains(strings.ReplaceAll(h.screen(), "\n", ""), "-- sh -c command -v bash >/dev/null && exec bash || exec sh") {
			t.Fatalf("screen %q", h.screen())
		}
		h.typeText(input)
		h.keys("enter")
		if input == "bye" {
			h.pump(func() bool { return h.m.ed == nil }, "shell exit")
			if h.m.status != "shell closed" || h.m.stage != "workpods" {
				t.Fatalf("after exit: %q %s", h.m.status, h.m.stage)
			}
			continue
		}
		h.pump(func() bool { return h.m.ed != nil && h.m.ed.done }, "failed shell")
		if !strings.Contains(h.m.status, "shell failed (exit 3)") {
			t.Fatalf("status %q", h.m.status)
		}
		h.keys("esc")
		if h.m.ed != nil {
			t.Fatal("back should close the failed shell")
		}
	}
}

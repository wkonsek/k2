package ui

import (
	"fmt"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"testing"

	"k2/internal/shell"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

const rollingDeploy = `{"items":[
 {"metadata":{"name":"web","namespace":"shop","creationTimestamp":"2026-10-22T10:00:00Z"},
  "spec":{"replicas":3},"status":{"readyReplicas":2,"updatedReplicas":3,"availableReplicas":2}}]}`

const rollingSts = `{"items":[
 {"metadata":{"name":"kafka","namespace":"shop","creationTimestamp":"2026-10-22T10:00:00Z"},
  "spec":{"replicas":2},"status":{"readyReplicas":1,"replicas":2}}]}`

func rollingCluster() *shell.Fake {
	base := actionCluster()
	var acted atomic.Bool
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		if len(r.Args) >= 2 && r.Args[0] == "get" {
			switch r.Args[1] {
			case "deployments", "deploy", "dp":
				if acted.Load() {
					return shell.OK(rollingDeploy)
				}
			case "statefulsets", "sts":
				return shell.OK(rollingSts)
			}
		}
		if len(r.Args) > 0 {
			switch r.Args[0] {
			case "scale", "rollout", "label", "annotate", "create", "patch":
				acted.Store(true)
			}
		}
		return base.Handler(r)
	}}
}

func noPanic(t *testing.T, what string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("%s panicked: %v\n%s", what, r, debug.Stack())
		}
	}()
	fn()
}

func TestRowProblemsWithoutStatusColumn(t *testing.T) {
	heads := []string{"NAME", "READY", "UP-TO-DATE", "AVAILABLE", "AGE"}
	cells := []string{"web", "2/3", "3", "2", "5m"}
	for _, col := range []int{-1, -2, 5, 99} {
		noPanic(t, fmt.Sprintf("rowProblems(statusCol=%d)", col), func() {
			mk := rowProblems(heads, cells, col)

			if !mk.name || mk.cells[1] != toneWarn {
				t.Fatalf("statusCol %d: marks %+v", col, mk)
			}
		})
	}
	noPanic(t, "paintRow(colorCol=-1)", func() {
		_, plain := paintRow(cells, []int{4, 5, 10, 9, 5}, heads, -1, 0, -1, 4, nil)
		if !strings.Contains(plain, "2/3") {
			t.Fatalf("row %q", plain)
		}
	})
}

func TestRollingDeploymentLists(t *testing.T) {
	f := rollingCluster()
	m := actionModel(t, f)
	noPanic(t, "list statefulsets", func() {
		m = open(t, m, "sts")
		if !strings.Contains(ansi.Strip(m.View()), "1/2") {
			t.Fatalf("no 1/2 row:\n%s", ansi.Strip(m.View()))
		}
	})
}

func TestRefreshAfterRestartNoSort(t *testing.T) {
	f := rollingCluster()
	m := open(t, actionModel(t, f), "deploy")
	if m.vs.sortCol != "" {
		t.Fatalf("sort %q, want none", m.vs.sortCol)
	}
	noPanic(t, "r y", func() { m = pressSettle(t, m, "r", "y") })
	if r := calls(f, "rollout"); len(r) != 1 {
		t.Fatalf("rollout %v", r)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "2/3") || !strings.Contains(m.toast.text, "restarted deploy/web") {
		t.Fatalf("toast %q view:\n%s", m.toast.text, view)
	}
}

func TestRefreshAfterScaleNoSort(t *testing.T) {
	f := rollingCluster()
	m := open(t, actionModel(t, f), "deploy")
	noPanic(t, "S 3 enter", func() {
		m = pressSettle(t, m, "S", "ctrl+u")
		m = typeText(t, m, "3")
		m = pressSettle(t, m, "enter")
	})
	if s := calls(f, "scale"); len(s) != 1 || !strings.Contains(s[0], "--replicas=3") {
		t.Fatalf("scale %v", s)
	}
	if !strings.Contains(ansi.Strip(m.View()), "2/3") {
		t.Fatalf("no refresh:\n%s", ansi.Strip(m.View()))
	}
}

func TestActionKeysAudit(t *testing.T) {
	old := clipboardRun
	clipboardRun = func(string, string) (string, error) { return "test", nil }
	t.Cleanup(func() { clipboardRun = old })
	plan := map[string][][]string{
		"deploy":   {{"r", "y"}, {"r", "n"}, {"S", "enter"}, {"S", "esc"}, {"L", "esc"}, {"A", "esc"}, {"w"}, {"W"}, {">"}, {"<", "="}, {"/", "esc"}},
		"sts":      {{"r", "y"}, {"S", "enter"}, {">", ">"}},
		"pods":     {{"p"}, {"w"}, {"W"}, {"<"}, {"="}, {"enter", "esc"}},
		"cronjobs": {{"t", "y"}, {"t", "n"}},
		"jobs":     {{"enter", "esc"}, {"="}},
		"events":   {{">"}, {"="}},
	}
	type shape struct {
		w, h  int
		split bool
	}
	for _, sh := range []shape{{120, 40, false}, {40, 12, false}, {120, 40, true}} {
		for v, runs := range plan {
			for _, ks := range runs {
				name := fmt.Sprintf("%dx%d split=%v :%s %v", sh.w, sh.h, sh.split, v, ks)
				noPanic(t, name, func() {
					m := actionModel(t, rollingCluster())
					n, c := m.Update(tea.WindowSizeMsg{Width: sh.w, Height: sh.h})
					m = settle(t, n.(model), c)
					if sh.split {
						m = pressSettle(t, m, "ctrl+a", "v")
						m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
					}
					m = open(t, m, v)
					_ = m.View()
					for _, k := range ks {
						m = pressSettle(t, m, k)
						_ = m.View()
					}
				})
			}
		}
	}
}

func TestMoreKeysAudit(t *testing.T) {
	for _, view := range []string{"ip", "sub", "csv", "catsrc", "og", "crd"} {
		for _, ks := range [][]string{{"a", "y"}, {"a", "n"}, {"a", "esc"}, {">", "="}} {
			noPanic(t, fmt.Sprintf(":%s %v", view, ks), func() {
				m := open(t, olmModel(t, olmCluster(true), false), view)
				_ = m.View()
				for _, k := range ks {
					m = pressSettle(t, m, k)
					_ = m.View()
				}
			})
		}
	}
	root := pathTree(t)
	for _, steps := range [][]string{
		{"ctrl+u", "tab", "tab", "tab", "enter"},
		{"ctrl+u", "enter"},
		{"tab", "tab", "esc"},
		{"ctrl+w", "tab", "backspace", "tab", "enter"},
	} {
		noPanic(t, fmt.Sprintf("kustomize p %v", steps), func() {
			m := openPathDialog(t, root)
			for _, k := range steps {
				m = press(t, m, k)
				_ = m.View()
			}
		})
	}

	noPanic(t, "empty picker", func() {
		m := actionModel(t, actionCluster())
		m.dlg = m.newDialog("logs", "x", "pod", "shop", "web-1")
		m = pressSettle(t, m, "enter")
		_ = m.View()
	})
}

const kafkaTrace = "[2026-10-23 06:40:01,123] ERROR Fatal error during KafkaServer startup. Prepare to shutdown (kafka.server.KafkaServer)\n" +
	"org.apache.kafka.common.KafkaException: Failed to acquire lock on file .lock in /var/lib/kafka/data. A Kafka instance in another process or thread is using this directory.\n" +
	"\tat org.apache.kafka.storage.internals.log.LogManager.lockLogDirs(LogManager.scala:291)\n" +
	"\tat kafka.log.LogManager.<init>(LogManager.scala:111)\n" +
	"\t\tat kafka.server.KafkaServer.startup(KafkaServer.scala:258) with a very long tail that certainly does not fit into a narrow split pane\n" +
	"\t... 12 more\r\n" +
	"progress 10%\rprogress 100%\a\n" +
	"\x1b[2J\x1b[Hcursor junk\x1b]0;title\a after\n" +
	"\x1b[31mred colored line\x1b[0m\tthen a tab\n" +
	"Caused by: java.io.IOException: No space left on device"

func assertPaneWidths(t *testing.T, screen string, width int) {
	t.Helper()
	for i, line := range strings.Split(screen, "\n") {
		plain := ansi.Strip(line)
		for _, r := range plain {
			if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
				t.Fatalf("line %d holds control %q:\n%q", i, r, plain)
			}
		}
		if w := ansi.StringWidth(line); w != width {
			t.Fatalf("line %d is %d cells, want %d:\n%q", i, w, width, plain)
		}
	}
}

func showLog(m model, text string) model {
	m.stage, m.logPod, m.logText = "logs", "shop/kafka-0", text
	m.vs.logPrev, m.logStick, m.logBusy = true, true, false
	return m
}

func TestLogTabsKeepPaneWidth(t *testing.T) {
	trueColor(t)
	for _, size := range [][2]int{{120, 40}, {80, 24}, {57, 20}} {
		m := actionModel(t, actionCluster())
		n, c := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = settle(t, n.(model), c)
		m = showLog(m, kafkaTrace)
		assertPaneWidths(t, m.View(), size[0])
		if size[0] == 120 && !strings.Contains(ansi.Strip(m.View()), "        at kafka.log.LogManager") {
			t.Fatalf("tab not expanded to the 8-column stop:\n%s", ansi.Strip(m.View()))
		}

		m = pressSettle(t, m, "ctrl+a", "v")
		m = showLog(m, kafkaTrace)
		assertPaneWidths(t, m.View(), size[0])
	}
}

func TestViewerTabsKeepPaneWidth(t *testing.T) {
	trueColor(t)
	for _, stage := range []string{"describe", "yaml", "view", "kout"} {
		m := actionModel(t, actionCluster())
		m.stage, m.logText = stage, kafkaTrace
		assertPaneWidths(t, m.View(), m.width)
	}
}

func TestSanitizeLine(t *testing.T) {
	cases := map[string]string{
		"plain":                       "plain",
		"\tat x":                      "        at x",
		"ab\tc":                       "ab      c",
		"12345678\tx":                 "12345678        x",
		"\x1b[31mab\x1b[0m\tc":        "\x1b[31mab\x1b[0m      c",
		"a\x07c\x08d":                 "acd",
		"progress 10%\rprogress 100%": "progress 100%",
		"crlf\r":                      "crlf",
		"\x1b[2J\x1b[Hx\x1b[?25l":     "x",
		"\x1b]8;;http://x\x1b\\link\x1b]8;;\x1b\\": "link",
		"x\x1b":    "x",
		"é\tz":     "é       z",
		"\u0085c1": "c1",
		"日本\tx":    "日本    x",
	}
	for in, want := range cases {
		if got := sanitizeLine(in); got != want {
			t.Errorf("sanitizeLine(%q) = %q, want %q", in, got, want)
		}
	}

	if got := clipLine("\tabcd", 10); ansi.StringWidth(got) != 10 || strings.Contains(got, "\t") {
		t.Fatalf("clipLine %q", got)
	}
}

func failingActions() *shell.Fake {
	base := actionCluster()
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		if len(r.Args) > 0 {
			switch r.Args[0] {
			case "scale", "rollout", "create", "label", "annotate", "patch":
				return shell.Fail(1, "Error from server (Forbidden): nope")
			}
		}
		return base.Handler(r)
	}}
}

func TestActionSpinnerStops(t *testing.T) {
	type action struct {
		view string
		keys []string
		text string
	}
	actions := map[string]action{
		"restart":  {"deploy", []string{"r", "y"}, ""},
		"scale":    {"deploy", []string{"S", "ctrl+u"}, "4"},
		"trigger":  {"cronjobs", []string{"t", "y"}, ""},
		"label":    {"deploy", []string{"L"}, "team=a"},
		"annotate": {"deploy", []string{"A"}, "note=x"},
	}
	for name, a := range actions {
		for _, fail := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s fail=%v", name, fail), func(t *testing.T) {
				f := actionCluster()
				if fail {
					f = failingActions()
				}
				m := open(t, actionModel(t, f), a.view)
				m = pressSettle(t, m, a.keys...)
				if a.text != "" {
					m = typeText(t, m, a.text)
					m = pressSettle(t, m, "enter")
				}
				if m.dlg != nil {
					t.Fatalf("dialog still open: %+v", m.dlg)
				}
				if m.load.active() {
					t.Fatalf("spinner still running: %+v", m.load)
				}
				if fail != strings.Contains(m.toast.text, "failed") {
					t.Fatalf("toast %q", m.toast.text)
				}
			})
		}
	}

	for _, fail := range []bool{false, true} {
		f := olmCluster(true)
		if fail {
			base := f
			f = &shell.Fake{Handler: func(r shell.Request) shell.Result {
				if len(r.Args) > 0 && r.Args[0] == "patch" {
					return shell.Fail(1, "forbidden")
				}
				return base.Handler(r)
			}}
		}
		m := open(t, olmModel(t, f, true), "ip")
		for i, raw := range m.content.Items() {
			if raw.(item).extra["approval"] == "Manual" && raw.(item).extra["approved"] != "true" {
				m.content.Select(i)
				break
			}
		}
		m = pressSettle(t, m, "a", "y")
		if m.load.active() {
			t.Fatalf("approve fail=%v: spinner still running", fail)
		}
	}
}

func TestActionResultFollowsItsPane(t *testing.T) {
	f := rollingCluster()
	m := open(t, actionModel(t, f), "deploy")
	origin := m.logID
	m = pressSettle(t, m, "r")
	next, cmd := m.Update(key(t, "y"))
	m = next.(model)
	if !m.load.active() {
		t.Fatal("no spinner while running")
	}

	m = pressSettle(t, m, "ctrl+a", "v")
	if m.logID == origin {
		t.Fatal("focus did not move")
	}
	m.load = load{token: 9999, what: "other"}
	m = settle(t, m, cmd)
	if m.load.token != 9999 {
		t.Fatalf("focused pane's load was touched: %+v", m.load)
	}
	found := false
	for i, p := range m.panes {
		if i != m.focus && p.logID == origin {
			found = true
			if p.load.active() {
				t.Fatalf("origin pane still spinning: %+v", p.load)
			}
			if p.status != "done" || !strings.Contains(ansi.Strip(p.content.Title), "READY") {
				t.Fatalf("origin pane status %q", p.status)
			}
		}
	}
	if !found {
		t.Fatal("origin pane not found")
	}
}

func (m model) asking() string {
	if m.dlg == nil || m.dlg.ask == "" {
		return ""
	}
	return m.dlg.title
}

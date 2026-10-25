package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/x/ansi"

	"k2/internal/config"
	"k2/internal/kube"
)

func TestHintsUseDefaultKeys(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	if got := m.podsHint(); got != "enter logs, d describe, ctrl+f forward, s shell, e edit, ctrl+d delete" {
		t.Fatalf("pods hint %q (d now describes)", got)
	}
	if got := m.scrollKeys(); got != "jk" {
		t.Fatalf("scroll %q", got)
	}
	if m.yn() != "y / n" {
		t.Fatalf("yn %q", m.yn())
	}
}

func TestHintsFollowRemappedKeys(t *testing.T) {
	cfg := testEnv(t)
	cfg.Keys.Delete = config.Binding{"ctrl+y"}
	cfg.Keys.Edit = config.Binding{"E"}
	cfg.Keys.Select = config.Binding{"right", "enter"}
	cfg.Keys.ScrollDown = config.Binding{"down", "J"}
	cfg.Keys.ScrollUp = config.Binding{"up", "K"}
	cfg.Keys.Vim = config.Binding{"V"}
	cfg.Keys.PortForward = config.Binding{"+"}
	cfg.Keys.Yes = config.Binding{"Y"}
	cfg.Keys.No = config.Binding{"X"}
	cfg.Keys.Kustomize = config.Binding{"k"}
	m := newTestModel(t, cfg, nil)

	if got := m.podsHint(); got != "right logs, d describe, + forward, s shell, E edit, ctrl+y delete" {
		t.Fatalf("pods hint %q", got)
	}
	m.stage = "workpods"
	if l := ansi.Strip(m.legend()); !strings.Contains(l, "E edit") || !strings.Contains(l, "^y delete") {
		t.Fatalf("legend %q", l)
	}

	m.content.SetItems([]list.Item{item{id: "web", title: "web"}})
	next, cmd := m.handleContentEnter()
	m = settle(t, next.(model), cmd)
	if m.status != "↓↑ scroll    y copy    m mark    V vim    p previous" {
		t.Fatalf("logs status %q", m.status)
	}
	if body := ansi.Strip(m.logsBody(80)); !strings.Contains(body, "↓↑    w wrap off    y copy    m mark    V vim") {
		t.Fatalf("logs header %q", body)
	}

	m.stage = "nav"
	m = press(t, m, "f")
	if it := m.content.Items()[0].(item); it.title != "no forwards · + on a pod or service starts one" {
		t.Fatalf("forwards placeholder %q", it.title)
	}
	m = press(t, m, "ctrl+a", "h", "ctrl+a", "x")
	if v := ansi.Strip(m.View()); m.dlg == nil || !strings.Contains(v, "close pane?  Y / X") {
		t.Fatalf("popup does not show the remapped yes/no:\n%s", v)
	}
	m = press(t, m, "X")
	if len(m.panes) != 2 {
		t.Fatal("remapped no did not keep the pane")
	}
}

func TestLegendTabKeys(t *testing.T) {
	cfg := testEnv(t)
	cfg.Keys.PrevTab, cfg.Keys.NextTab = config.Binding{"{"}, config.Binding{"}"}
	m := press(t, newTestModel(t, cfg, nil), "ctrl+a", "t")
	if !strings.Contains(ansi.Strip(m.legend()), "^a {/} tabs") {
		t.Fatalf("legend %q", ansi.Strip(m.legend()))
	}
	if one := newTestModel(t, testEnv(t), nil); strings.Contains(ansi.Strip(one.legend()), "tabs") {
		t.Fatal("tab keys shown with one tab")
	}
}

func TestLogCopyLegendAndMark(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	m.stage = "logs"
	m.logPod = "shop/web"
	m.logText = "one\ntwo"
	m.logStick = true
	if leg := ansi.Strip(m.legend()); !strings.Contains(leg, "y copy") || !strings.Contains(leg, "m mark") {
		t.Fatalf("legend %q", leg)
	}
	m.zoom = true
	if leg := ansi.Strip(m.legend()); !strings.Contains(leg, "y copy") || !strings.Contains(leg, "m mark") || !strings.Contains(leg, "F split") {
		t.Fatalf("zoom legend %q", leg)
	}
	m.zoom = false
	m = press(t, m, "m")
	if len(m.vs.logMarks) != 1 || m.vs.logMarks[0] != 1 {
		t.Fatalf("marks %v", m.vs.logMarks)
	}
	if body := ansi.Strip(m.logsBody(40)); !strings.Contains(body, " mark ") {
		t.Fatalf("no rule:\n%s", body)
	}
	m = press(t, m, "m")
	if len(m.vs.logMarks) != 0 {
		t.Fatalf("second m did not clear: %v", m.vs.logMarks)
	}
	m = press(t, m, "m")
	m.applyLogs(logsMsg{id: m.logID, pod: logKey(m.logPod, m.vs.logOpts()), text: "three", partial: true})
	var nums []int
	for _, row := range m.visualLines(40) {
		nums = append(nums, row.num)
	}
	if len(nums) != 4 || nums[0] != 0 || nums[1] != 1 || nums[2] != logMarkNum || nums[3] != 2 {
		t.Fatalf("rows %v text %q marks %v", nums, m.logText, m.vs.logMarks)
	}
}

func TestMlogMarkStaysUnderNewLines(t *testing.T) {
	m := newTestModel(t, testEnv(t), nil)
	t1 := time.Date(2026, 10, 25, 8, 0, 1, 0, time.UTC)
	t2 := t1.Add(time.Second)
	t3 := t2.Add(time.Second)
	v := &mlogView{
		lines: []kube.LogLine{
			{Pod: "web-a", Container: "app", At: t1, Text: "one"},
			{Pod: "web-b", Container: "app", At: t2, Text: "two"},
		},
		cur:    kube.LogCursor{"web-a/app": t1, "web-b/app": t2},
		colors: map[string]color{},
		polls:  1,
	}
	m.stage = "mlogs"
	m.logStick = true
	m.vs.ml = v
	m.logText = v.render()
	if leg := ansi.Strip(m.legend()); !strings.Contains(leg, "y copy") || !strings.Contains(leg, "m mark") {
		t.Fatalf("legend %q", leg)
	}
	m = press(t, m, "m")
	if len(m.vs.ml.marks) != 1 || m.vs.ml.marks[0] != 1 {
		t.Fatalf("marks %v", m.vs.ml.marks)
	}
	if body := ansi.Strip(m.logsBody(60)); !strings.Contains(body, " mark ") {
		t.Fatalf("no rule:\n%s", body)
	}
	m.applyMlog(mlogMsg{
		pane: m.logID,
		key:  mlogKey(m.namespace, m.vs.ml.q),
		lines: []kube.LogLine{
			{Pod: "web-a", Container: "app", At: t3, Text: "three"},
		},
	})
	var nums []int
	for _, row := range m.visualLines(60) {
		nums = append(nums, row.num)
	}
	if len(m.vs.ml.marks) != 1 || m.vs.ml.marks[0] != 1 || len(nums) != 4 || nums[2] != logMarkNum || nums[3] != 2 {
		t.Fatalf("after poll marks %v rows %v", m.vs.ml.marks, nums)
	}
	m.cfg.Logs.MultiMaxLines = 3
	m.applyMlog(mlogMsg{
		pane: m.logID,
		key:  mlogKey(m.namespace, m.vs.ml.q),
		lines: []kube.LogLine{
			{Pod: "web-a", Container: "app", At: t3.Add(time.Second), Text: "four"},
		},
	})
	if len(m.vs.ml.lines) != 3 || len(m.vs.ml.marks) != 1 || m.vs.ml.marks[0] != 0 || m.vs.ml.lines[0].Text != "two" {
		t.Fatalf("after trim lines %+v marks %v", m.vs.ml.lines, m.vs.ml.marks)
	}
}

package ui

import (
	"strings"
	"sync/atomic"
	"testing"

	"k2/internal/kube"
	"k2/internal/shell"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

const limitPods = `{"items":[
 {"metadata":{"name":"web-1","namespace":"shop","creationTimestamp":"2026-10-22T10:00:00Z"},
  "spec":{"nodeName":"n","containers":[
    {"name":"app","resources":{"limits":{"cpu":"200m","memory":"64Mi"}}},
    {"name":"side","resources":{"limits":{"cpu":"300m","memory":"64Mi"}}}]},
  "status":{"phase":"Running","containerStatuses":[{"ready":true,"restartCount":0,"state":{}},{"ready":true,"restartCount":0,"state":{}}]}},
 {"metadata":{"name":"db-0","namespace":"shop","creationTimestamp":"2026-10-20T10:00:00Z"},
  "spec":{"nodeName":"n","containers":[{"name":"pg","resources":{"limits":{"memory":"1Gi"}}}]},
  "status":{"phase":"Running","containerStatuses":[{"ready":true,"restartCount":0,"state":{}}]}},
 {"metadata":{"name":"cache-0","namespace":"shop","creationTimestamp":"2026-10-21T10:00:00Z"},
  "spec":{"nodeName":"n","containers":[{"name":"redis"}]},
  "status":{"phase":"Pending","containerStatuses":[{"ready":false,"restartCount":0,"state":{}}]}}
]}`

func metricsCluster(top string, topCalls *int32, args *[]string) *shell.Fake {
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		joined := strings.Join(r.Args, " ")
		switch {
		case strings.HasPrefix(joined, "get pods"):
			return shell.OK(limitPods)
		case strings.HasPrefix(joined, "top "):
			atomic.AddInt32(topCalls, 1)
			*args = append(*args, joined)
			if top == "" {
				return shell.Fail(1, "error: Metrics API not available")
			}
			return shell.OK(top)
		case strings.HasPrefix(joined, "version"):
			return shell.OK(`{"clientVersion":{"gitVersion":"v1.31.2"}}`)
		}
		return shell.Fail(1, "not in this test")
	}}
}

func cellOf(t *testing.T, m model, id, col string) (string, item) {
	t.Helper()
	for _, raw := range m.content.Items() {
		it := raw.(item)
		if it.id != id {
			continue
		}
		for i, h := range it.heads {
			if h == col {
				return it.cells[i], it
			}
		}
		t.Fatalf("no %s column in %v", col, it.heads)
	}
	t.Fatalf("no row %s", id)
	return "", item{}
}

func TestPodMetricsColumns(t *testing.T) {
	var n int32
	var args []string
	f := metricsCluster("web-1   480m   40Mi\ndb-0   12m   900Mi\n", &n, &args)
	m := v4Model(t, f)
	if got := m.content.Items()[0].(item).heads; strings.Join(got, " ") != "NAME READY STATUS RESTARTS CPU MEM IP NODE AGE" {
		t.Fatalf("heads %v", got)
	}
	if c, _ := cellOf(t, m, "web-1", "CPU"); c != "…" {
		t.Fatalf("before the first fetch: %q", c)
	}
	m = tickOnce(t, m)
	if n != 1 || args[0] != "top pods --no-headers --request-timeout=5s -n shop --kubeconfig /kc --context kind-dev" {
		t.Fatalf("top calls %d %v", n, args)
	}
	for id, want := range map[string][2]string{"web-1": {"480m", "40Mi"}, "db-0": {"12m", "900Mi"}, "cache-0": {"-", "-"}} {
		cpu, _ := cellOf(t, m, id, "CPU")
		mem, _ := cellOf(t, m, id, "MEM")
		if cpu != want[0] || mem != want[1] {
			t.Fatalf("%s: %s %s", id, cpu, mem)
		}
	}

	if view := ansi.Strip(m.View()); !strings.Contains(m.content.Title, "CPU") || !strings.Contains(view, "█████ 96%") || !strings.Contains(view, "12m") || !strings.Contains(view, "CPU") {
		t.Fatalf("not drawn:\n%s\n%s", m.content.Title, view)
	}
	wide := resize(t, m, 180, 40)
	if view := ansi.Strip(wide.View()); !strings.Contains(view, "480m █████ 96%") {
		t.Fatalf("not drawn:\n%s\n%s", m.content.Title, view)
	}

	for i := 0; i < 5; i++ {
		m = tickOnce(t, m)
	}
	if n != 1 {
		t.Fatalf("fetched %d times within metrics_every", n)
	}

	m.beat += m.cfg.UI.MetricsEvery
	m = tickOnce(t, m)
	if n != 2 {
		t.Fatalf("not refetched: %d", n)
	}

	for _, a := range args {
		if !strings.HasPrefix(a, "top pods ") {
			t.Fatalf("unexpected top call %q", a)
		}
	}

	m.namespace = ""
	m.metrics = nil
	m = tickOnce(t, m)
	if last := args[len(args)-1]; !strings.Contains(last, " -A ") {
		t.Fatalf("all namespaces: %q", last)
	}
}

func TestPodMetricsUnavailable(t *testing.T) {
	trueColor(t)
	var n int32
	var args []string
	m := v4Model(t, metricsCluster("", &n, &args))
	m = tickOnce(t, m)
	cpu, it := cellOf(t, m, "web-1", "CPU")
	if cpu != "N/A" || it.tones[4] != toneMuted || it.tones[5] != toneMuted {
		t.Fatalf("cpu %q tones %v", cpu, it.tones)
	}
	if !strings.Contains(m.status, "pod metrics N/A on kind-dev (no metrics API)") || m.toast.text != "" {
		t.Fatalf("status %q toast %q", m.status, m.toast.text)
	}
	if !strings.Contains(it.title, dimStyle.Render(fitCell("N/A", it.widths[4]))) {
		t.Fatalf("N/A not muted: %q", it.title)
	}

	m.status = "pods"
	m.beat += m.cfg.UI.MetricsEvery
	m = tickOnce(t, m)
	if n != 2 || m.status != "pods" {
		t.Fatalf("calls %d status %q", n, m.status)
	}

	rows := loadRows(m.k, m.logID, "workpods", "pods", "/kc", "kind-dev", "shop", "", m.tableWidth())()
	m.applyRows(rows.(rowsMsg))
	if len(m.content.Items()) != 3 {
		t.Fatalf("rows %d", len(m.content.Items()))
	}
	if c, _ := cellOf(t, m, "db-0", "MEM"); c != "N/A" {
		t.Fatalf("after refresh: %q", c)
	}
}

func TestPodMetricsLimitColors(t *testing.T) {
	var n int32
	var args []string

	m := v4Model(t, metricsCluster("web-1 480m 110Mi\ndb-0 900m 900Mi\ncache-0 1m 1Mi\n", &n, &args))
	m.cfg.UI.MetricsBars = false
	m = tickOnce(t, m)
	_, web := cellOf(t, m, "web-1", "CPU")
	_, db := cellOf(t, m, "db-0", "CPU")
	_, cache := cellOf(t, m, "cache-0", "CPU")
	if web.tones[4] != toneErr || web.tones[5] != toneWarn {
		t.Fatalf("web tones %v", web.tones)
	}
	if _, ok := db.tones[4]; ok || db.tones[5] != toneWarn {
		t.Fatalf("db tones %v", db.tones)
	}
	if len(cache.tones) != 0 {
		t.Fatalf("cache tones %v", cache.tones)
	}
}

func TestPodMetricsSort(t *testing.T) {
	var n int32
	var args []string
	m := v4Model(t, metricsCluster("web-1 1 64Mi\ndb-0 250m 1Gi\n", &n, &args))
	m = tickOnce(t, m)
	ids := func() string {
		var out []string
		for _, raw := range m.content.Items() {
			out = append(out, raw.(item).id)
		}
		return strings.Join(out, ",")
	}
	m.vs.sortCol = "CPU"
	m.resort()
	if ids() != "db-0,web-1,cache-0" || !strings.Contains(tableHeader(m.content, m.vs), "CPU▲") {
		t.Fatalf("cpu asc %s", ids())
	}
	m.vs.sortCol, m.vs.sortDesc = "MEM", true
	m.resort()
	if ids() != "cache-0,db-0,web-1" {
		t.Fatalf("mem desc %s", ids())
	}

	m.beat += m.cfg.UI.MetricsEvery
	m = tickOnce(t, m)
	if ids() != "cache-0,db-0,web-1" {
		t.Fatalf("after refetch %s", ids())
	}
}

func TestPodMetricsNarrowAndOff(t *testing.T) {
	var n int32
	var args []string
	m := v4Model(t, metricsCluster("web-1 480m 40Mi\n", &n, &args))
	m = tickOnce(t, m)
	for _, w := range []int{40, 60, 120, 200} {
		reflowList(&m.content, w)
		_, it := cellOf(t, m, "web-1", "CPU")
		narrow := w < 70
		if hidden := it.widths[4] == 0 && it.widths[5] == 0; hidden != narrow {
			t.Fatalf("width %d: widths %v", w, it.widths)
		}
		for _, raw := range m.content.Items() {
			if lw := ansi.StringWidth(raw.(item).plain); lw > w {
				t.Fatalf("width %d: row %d wide: %q", w, lw, raw.(item).plain)
			}
		}
		if narrow == strings.Contains(m.content.Title, "CPU") {
			t.Fatalf("width %d: header %q", w, m.content.Title)
		}
		if !narrow && !strings.Contains(it.plain, "480m") {
			t.Fatalf("width %d: no value", w)
		}
	}

	cfg := testEnv(t)
	cfg.UI.PodMetrics = false
	n = 0
	off := newTestModel(t, cfg, metricsCluster("web-1 1m 1Mi\n", &n, &args))
	off.kubeconfig, off.context, off.namespace = "/kc", "kind-dev", "shop"
	next, c := off.openPods()
	off = settle(t, next.(model), c)
	off = tickOnce(t, off)
	if heads := strings.Join(off.content.Items()[0].(item).heads, " "); strings.Contains(heads, "CPU") || n != 0 {
		t.Fatalf("off: heads %q calls %d", heads, n)
	}
}

func resize(t *testing.T, m model, w, h int) model {
	t.Helper()
	next, cmd := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return settle(t, next.(model), cmd)
}

func TestPodMetricsBars(t *testing.T) {
	trueColor(t)
	cases := []struct {
		used, limit, request string
		want                 string
		tn                   tone
	}{
		{"480m", "500", "", "96%", toneErr},
		{"350m", "500", "", "70%", toneWarn},
		{"100m", "500", "", "20%", toneOK},
		{"100m", "", "200", "~50%", toneOK},
		{"900m", "", "500", "~180%", toneErr},
		{"100m", "", "", "", 0},
		{"N/A", "500", "", "", 0},
	}
	for _, c := range cases {
		me := newMeter(c.used, c.limit, c.request, kube.ParseCPU, 70, 90)
		if (c.want == "") != !me.ok || (me.ok && (me.pctText() != c.want || me.tone != c.tn)) {
			t.Errorf("%+v: %+v %q", c, me, me.pctText())
		}
	}

	if me := newMeter("350m", "500", "", kube.ParseCPU, 30, 60); me.tone != toneErr {
		t.Fatalf("custom thresholds: %v", me.tone)
	}

	me := newMeter("480m", "500", "", kube.ParseCPU, 70, 90)
	me.valW, me.pctW = 4, 3
	for w, want := range map[int]string{14: "480m █████ 96%", 13: "█████ 96%", 9: "█████ 96%", 8: "480m", 4: "480m"} {
		got, tn, _ := me.cell("480m", w)
		if ansi.Strip(got) != want {
			t.Errorf("width %d: %q, want %q", w, ansi.Strip(got), want)
		}
		if want == "480m" && tn != toneErr {
			t.Errorf("width %d: value only must be red, got %v", w, tn)
		}
	}
	if got, _, _ := me.cell("480m", 14); !strings.Contains(got, errStyle.Render("█████")) {
		t.Fatalf("bar not red: %q", got)
	}

	var n int32
	var args []string
	f := metricsCluster("web-1 480m 110Mi\ndb-0 12m 900Mi\ncache-0 1m 1Mi\n", &n, &args)
	for _, size := range []struct {
		w          int
		bar, value bool
		cols       bool
	}{{200, true, true, true}, {115, true, false, true}, {110, false, true, true}, {80, false, false, false}} {
		m := resize(t, v4Model(t, f), size.w, 30)
		m = tickOnce(t, m)
		view := ansi.Strip(m.View())
		assertPaneWidths(t, m.View(), size.w)
		if strings.Contains(view, "█") != size.bar || strings.Contains(view, "480m") != size.value || strings.Contains(view, "CPU") != size.cols {
			t.Errorf("width %d: bar %v value %v cols %v\n%s", size.w, strings.Contains(view, "█"), strings.Contains(view, "480m"), strings.Contains(view, "CPU"), view)
		}
	}

	m := resize(t, v4Model(t, f), 200, 30)
	m = tickOnce(t, m)
	if _, it := cellOf(t, m, "db-0", "CPU"); it.meters[4].ok || !it.meters[5].ok {
		t.Fatalf("db-0 meters %+v", it.meters)
	}

	m = resize(t, v4Model(t, f), 200, 30)
	m.cfg.UI.MetricsBars = false
	m = tickOnce(t, m)
	if strings.Contains(ansi.Strip(m.View()), "█") {
		t.Fatal("bars with metrics_bars: false")
	}
}

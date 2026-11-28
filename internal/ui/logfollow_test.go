package ui

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"k2/internal/shell"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

var feedBase = time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)

type feed struct {
	mu       sync.Mutex
	produced int
	drop     bool
	args     []string
	running  atomic.Int32
}

func (f *feed) produce(n int) {
	f.mu.Lock()
	f.produced += n
	f.mu.Unlock()
}

func (f *feed) total() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.produced
}

func (f *feed) dropConnection() {
	f.mu.Lock()
	f.drop = true
	f.mu.Unlock()
}

func (f *feed) calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.args...)
}

func feedLine(i int) string {
	at := feedBase.Add(time.Duration(i) * 100 * time.Microsecond)
	return fmt.Sprintf("%s seq-%06d", at.Format(time.RFC3339Nano), i)
}

func (f *feed) fake() *shell.Fake { return f.over(containerCluster()) }

func (f *feed) over(inner *shell.Fake) *shell.Fake {
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		line := strings.Join(r.Args, " ")
		if len(r.Args) == 0 || r.Args[0] != "logs" || !slices.Contains(r.Args, "-f") {
			return inner.Handler(r)
		}
		prefix := ""
		if slices.Contains(r.Args, "--prefix") {
			prefix = "[pod/" + r.Args[1] + "/app] "
		}
		f.mu.Lock()
		f.args = append(f.args, line)
		from := 0
		for _, a := range r.Args {
			if v, ok := strings.CutPrefix(a, "--since-time="); ok {
				at, _ := time.Parse(time.RFC3339, v)
				from = int(at.Sub(feedBase) / (100 * time.Microsecond))
			}
			if v, ok := strings.CutPrefix(a, "--tail="); ok {
				var n int
				fmt.Sscan(v, &n)
				from = max(0, f.produced-n)
			}
		}
		f.drop = false
		f.mu.Unlock()
		f.running.Add(1)
		defer f.running.Add(-1)
		sent := from
		for {
			f.mu.Lock()
			upto, drop := f.produced, f.drop
			f.mu.Unlock()
			for ; sent < upto; sent++ {
				r.OnLine(prefix + feedLine(sent))
			}
			if drop {
				return shell.Fail(1, "connection reset")
			}
			select {
			case <-r.Ctx.Done():
				return shell.Fail(137, "killed")
			case <-time.After(time.Millisecond):
			}
		}
	}}
}

type driver struct {
	t    *testing.T
	m    model
	msgs chan tea.Msg
	stop chan struct{}
}

func newDriver(t *testing.T, m model) *driver {
	d := &driver{t: t, m: m, msgs: make(chan tea.Msg, 4096), stop: make(chan struct{})}
	t.Cleanup(func() { close(d.stop) })
	return d
}

func (d *driver) run(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	go func() {
		msg := cmd()
		if msg == nil {
			return
		}
		select {
		case d.msgs <- msg:
		case <-d.stop:
		}
	}()
}

func (d *driver) handle(msg tea.Msg) {
	switch msg := msg.(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			d.run(c)
		}
	case logWakeMsg, logsMsg, mlogMsg, openedMsg, logRangeMsg:
		next, cmd := d.m.Update(msg)
		d.m = next.(model)
		d.run(cmd)
	default:
	}
}

func (d *driver) press(keys ...string) {
	d.t.Helper()
	for _, k := range keys {
		next, cmd := d.m.Update(key(d.t, k))
		d.m = next.(model)
		d.run(cmd)
	}
}

func (d *driver) until(what string, cond func() bool) {
	d.t.Helper()
	deadline := time.After(20 * time.Second)
	for !cond() {
		select {
		case msg := <-d.msgs:
			d.handle(msg)
		case <-time.After(5 * time.Millisecond):
		case <-deadline:
			d.t.Fatalf("timed out waiting for %s\nstage %s stick %v lines %d\n%s", what, d.m.stage, d.m.logStick, strings.Count(d.m.logText, "\n")+1, d.m.status)
		}
	}
}

func (d *driver) idle(ms int) {
	end := time.After(time.Duration(ms) * time.Millisecond)
	for {
		select {
		case msg := <-d.msgs:
			d.handle(msg)
		case <-end:
			return
		}
	}
}

func seqs(t *testing.T, text string) []int {
	t.Helper()
	var out []int
	for _, l := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		var n int
		if _, err := fmt.Sscanf(l, "seq-%d", &n); err != nil {
			t.Fatalf("not a sequence line %q", l)
		}
		out = append(out, n)
	}
	return out
}

func contiguous(t *testing.T, text string) []int {
	t.Helper()
	got := seqs(t, text)
	for i := 1; i < len(got); i++ {
		if got[i] != got[i-1]+1 {
			t.Fatalf("gap or duplicate after %d: next is %d (position %d of %d)", got[i-1], got[i], i, len(got))
		}
	}
	return got
}

func streamModel(t *testing.T, f *feed, maxLines int) *driver {
	t.Helper()
	m := selectRow(t, actionModel(t, f.fake()), "db-0")
	m.cfg.Logs.MaxLines = maxLines
	m.height, m.width = 30, 100
	d := newDriver(t, m)
	d.press("l")
	if d.m.stage != "logs" {
		t.Fatalf("stage %s", d.m.stage)
	}
	return d
}

func TestStreamDeliversEveryLineOfAHighRateLog(t *testing.T) {
	f := &feed{}
	d := streamModel(t, f, 20000)
	d.until("the process", func() bool { return f.running.Load() == 1 })
	const total = 12000
	for sent := 0; sent < total; sent += 1500 {
		f.produce(1500)
		d.idle(15)
	}
	d.until("every line", func() bool {
		return strings.HasSuffix(strings.TrimRight(d.m.logText, "\n"), fmt.Sprintf("seq-%06d", total-1))
	})
	got := contiguous(t, d.m.logText)
	if got[0] != 0 || len(got) != total {
		t.Fatalf("first %d, %d lines, want 0 and %d", got[0], len(got), total)
	}
	if calls := f.calls(); len(calls) != 1 || !strings.Contains(calls[0], "logs web-1") && !strings.Contains(calls[0], "logs db-0") {
		t.Fatalf("calls %v", calls)
	}
	if head := strings.SplitN(ansi.Strip(d.m.logsBody(90)), "\n", 2)[0]; !strings.HasPrefix(head, "follow ") {
		t.Fatalf("header %q", head)
	}
}

func TestStreamKeepsTheBufferCapped(t *testing.T) {
	f := &feed{}
	d := streamModel(t, f, 200)
	d.until("the process", func() bool { return f.running.Load() == 1 })
	for i := 0; i < 20; i++ {
		f.produce(2500)
		d.idle(10)
	}
	d.until("the newest line", func() bool {
		return strings.HasSuffix(strings.TrimRight(d.m.logText, "\n"), fmt.Sprintf("seq-%06d", f.total()-1))
	})
	got := contiguous(t, d.m.logText)
	if len(got) > 200 {
		t.Fatalf("buffer holds %d lines", len(got))
	}
}

func TestStreamPauseFreezesTheViewAndCatchesUp(t *testing.T) {
	f := &feed{}
	d := streamModel(t, f, 100)
	d.until("the process", func() bool { return f.running.Load() == 1 })
	f.produce(150)
	d.until("the first lines", func() bool {
		return strings.HasSuffix(strings.TrimRight(d.m.logText, "\n"), "seq-000149")
	})
	d.press("g")
	if d.m.logStick {
		t.Fatal("g did not pause")
	}
	before := bodyRows(ansi.Strip(d.m.logsBody(90)))
	frozen := d.m.logText
	for i := 0; i < 30; i++ {
		f.produce(200)
		d.idle(10)
		if got := bodyRows(ansi.Strip(d.m.logsBody(90))); got != before {
			t.Fatalf("paused view moved after %d lines\nbefore:\n%s\nafter:\n%s", f.total(), before, got)
		}
	}
	d.until("the pending count", func() bool {
		return strings.Contains(strings.SplitN(ansi.Strip(d.m.logsBody(90)), "\n", 2)[0], "paused +")
	})
	d.idle(200)
	if n := strings.Count(d.m.logText, "\n") + 1; n > 200 {
		t.Fatalf("paused buffer grew to %d lines", n)
	}
	if !strings.HasPrefix(d.m.logText, frozen[:len(frozen)/2]) {
		t.Fatal("paused text was rewritten")
	}

	d.press("G")
	if !d.m.logStick {
		t.Fatal("G did not resume")
	}
	f.produce(10)
	d.until("catch up", func() bool {
		return strings.HasSuffix(strings.TrimRight(d.m.logText, "\n"), fmt.Sprintf("seq-%06d", f.total()-1))
	})
	got := contiguous(t, d.m.logText)
	if len(got) > 100 || d.m.vs.logPend != "" {
		t.Fatalf("after catch up: %d lines pending %d bytes", len(got), len(d.m.vs.logPend))
	}
}

func TestStreamReconnectsWithoutGapsOrDuplicates(t *testing.T) {
	f := &feed{}
	d := streamModel(t, f, 20000)
	d.until("the process", func() bool { return f.running.Load() == 1 })
	f.produce(3000)
	d.until("first batch", func() bool { return strings.Contains(d.m.logText, "seq-002999") })
	for round := 0; round < 3; round++ {
		f.dropConnection()
		f.produce(2000)
		d.until("a new connection", func() bool { return len(f.calls()) > round+1 && f.running.Load() == 1 })
		f.produce(1000)
	}
	d.until("the last line", func() bool {
		return strings.HasSuffix(strings.TrimRight(d.m.logText, "\n"), fmt.Sprintf("seq-%06d", f.total()-1))
	})
	got := contiguous(t, d.m.logText)
	if got[0] != 0 || len(got) != f.total() {
		t.Fatalf("%d lines from %d, produced %d", len(got), got[0], f.total())
	}
	for _, c := range f.calls()[1:] {
		if !strings.Contains(c, "--since-time=") || strings.Contains(c, "--tail") {
			t.Fatalf("reconnect args %s", c)
		}
	}
}

func TestStreamProcessIsCancelledWhenTheViewCloses(t *testing.T) {
	f := &feed{}
	d := streamModel(t, f, 400)
	d.until("the process", func() bool { return f.running.Load() == 1 })
	f.produce(50)
	d.until("lines", func() bool { return strings.Contains(d.m.logText, "seq-000049") })

	d.press("esc")
	if d.m.stage == "logs" {
		t.Fatal("still in logs")
	}
	d.until("the process to stop", func() bool { return f.running.Load() == 0 })

	d.press("l")
	d.until("the process again", func() bool { return f.running.Load() == 1 })
	d.press("p")
	if !d.m.vs.logPrev {
		t.Fatal("p did not switch to previous logs")
	}
	d.until("the live process to stop", func() bool { return f.running.Load() == 0 })
	for _, c := range f.calls() {
		if strings.Contains(c, "--previous") {
			t.Fatalf("previous logs were followed: %s", c)
		}
	}

	d.press("p")
	d.until("following again", func() bool { return f.running.Load() == 1 })
	Shutdown(d.m)
	d.until("quit to stop it", func() bool { return f.running.Load() == 0 })
}

func TestStreamSwitchesContainerCleanly(t *testing.T) {
	f := &feed{}
	m := selectRow(t, actionModel(t, f.fake()), "web-1")
	d := newDriver(t, m)
	next, cmd := d.m.openLogs("shop/web-1", "app", false)
	d.m = next.(model)
	d.run(cmd)
	d.press("tab")
	d.until("the first container", func() bool { return f.running.Load() == 1 })
	next, cmd = d.m.openLogs("shop/web-1", "side", false)
	d.m = next.(model)
	d.run(cmd)
	d.press("tab")
	d.until("the second container", func() bool { return len(f.calls()) >= 2 })
	d.until("one process", func() bool { return f.running.Load() == 1 })
	last := f.calls()[len(f.calls())-1]
	if !strings.Contains(last, "-c side") {
		t.Fatalf("args %s", last)
	}
}

func sternText(t *testing.T, m model) map[string][]int {
	t.Helper()
	out := map[string][]int{}
	for _, l := range strings.Split(strings.TrimRight(m.logText, "\n"), "\n") {
		var pod string
		var n int
		if _, err := fmt.Sscanf(strings.Replace(l, "/app ", " ", 1), "%s seq-%d", &pod, &n); err != nil {
			t.Fatalf("bad stern line %q", l)
		}
		out[pod] = append(out[pod], n)
	}
	return out
}

func TestSternStreamsEveryLineAndFreezesWhenPaused(t *testing.T) {
	f := &feed{}
	m := appModel(t, f.over(appCluster(t)))
	m.cfg.Logs.MultiMaxLines = 100
	m.height, m.width = 30, 120
	d := newDriver(t, m)
	next, cmd := d.m.runCommand("logs app=web")
	d.m = next.(model)
	d.run(cmd)
	if d.m.stage != "mlogs" {
		t.Fatalf("stage %s", d.m.stage)
	}
	d.until("two processes", func() bool { return f.running.Load() == 2 })
	f.produce(120)
	d.until("the first lines", func() bool { return strings.Count(d.m.logText, "seq-000119") == 2 })

	d.press("g")
	if d.m.logStick {
		t.Fatal("g did not pause")
	}
	before := bodyRows(ansi.Strip(d.m.logsBody(110)))
	for i := 0; i < 20; i++ {
		f.produce(300)
		d.idle(10)
		if got := bodyRows(ansi.Strip(d.m.logsBody(110))); got != before {
			t.Fatalf("paused stern view moved\nbefore:\n%s\nafter:\n%s", before, got)
		}
	}
	d.until("the pending count", func() bool {
		return strings.Contains(strings.SplitN(ansi.Strip(d.m.logsBody(110)), "\n", 2)[0], "paused +")
	})
	d.idle(200)
	f.produce(8)
	d.idle(200)
	d.press("G")
	d.until("catch up", func() bool { return strings.Count(d.m.logText, fmt.Sprintf("seq-%06d", f.total()-1)) == 2 })
	for pod, got := range sternText(t, d.m) {
		for i := 1; i < len(got); i++ {
			if got[i] != got[i-1]+1 {
				t.Fatalf("%s: gap after %d: %d", pod, got[i-1], got[i])
			}
		}
	}
	if len(d.m.vs.ml.lines) > 100 || len(d.m.vs.ml.pend) != 0 {
		t.Fatalf("buffer %d pending %d", len(d.m.vs.ml.lines), len(d.m.vs.ml.pend))
	}

	d.press("esc")
	d.until("both processes to stop", func() bool { return f.running.Load() == 0 })
}

const kafkaLine = "[2026-11-20 20:37:02,136] INFO [SocketServer listenerType=BROKER, nodeId=4] Failed authentication with /10.128.2.12 (channelId=abc,def) (SSL handshake failed) (org.apache.kafka.common.network.Selector)"

func TestStreamColonPodsSwitchesToThePodList(t *testing.T) {
	f := &feed{}
	d := streamModel(t, f, 20000)
	d.m.resources = []string{"pods", "pods.metrics.k8s.io", "podsecuritypolicies"}
	d.until("the process", func() bool { return f.running.Load() == 1 })
	f.produce(4000)
	d.until("streamed lines", func() bool { return strings.Contains(d.m.logText, "seq-") })
	d.press(":")
	for _, r := range "pods" {
		d.press(string(r))
	}
	if d.m.cmd.Value() != "pods" {
		t.Fatalf("typed %q", d.m.cmd.Value())
	}
	d.press("enter")
	d.until("the pod list", func() bool { return d.m.stage == "workpods" })
	if d.m.kind != "pods" {
		t.Fatalf("kind %q stage %s status %q", d.m.kind, d.m.stage, d.m.status)
	}
	d.until("the stream stops", func() bool { return f.running.Load() == 0 })
}

func TestStreamColonUnknownShowsAnError(t *testing.T) {
	f := &feed{}
	d := streamModel(t, f, 200)
	d.until("the process", func() bool { return f.running.Load() == 1 })
	f.produce(20)
	d.until("a line", func() bool { return strings.Contains(d.m.logText, "seq-") })
	d.press(":")
	for _, r := range "no-such-kind" {
		d.press(string(r))
	}
	d.press("enter")
	if d.m.stage != "logs" {
		t.Fatalf("left logs: %s", d.m.stage)
	}
	if !strings.Contains(d.m.status, "unknown resource") {
		t.Fatalf("silent failure: %q", d.m.status)
	}
}

func TestStreamSinceWindowDumpsRawLinesWithoutFollowing(t *testing.T) {
	body := kafkaLine + "\n" + strings.Replace(kafkaLine, "20:37:02,136", "20:37:03,200", 1) + "\n"
	inner := actionCluster()
	orig := inner.Handler
	inner.Handler = func(r shell.Request) shell.Result {
		if len(r.Args) > 0 && r.Args[0] == "logs" && !slices.Contains(r.Args, "-f") {
			return shell.OK(body)
		}
		return orig(r)
	}
	f := &feed{}
	fk := f.over(inner)
	var opened string
	lessCommand = func(path string) *exec.Cmd {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		opened = string(b)
		return exec.Command("true")
	}
	t.Cleanup(func() {
		lessCommand = defaultLessCommand
	})
	m := actionModel(t, fk)
	m.cfg.Logs.MaxLines = 20000
	m.cfg.UI.EditorInline = false
	m.cfg.UI.Animations = false
	m.height, m.width = 30, 100
	d := newDriver(t, selectRow(t, m, "db-0"))
	d.press("l")
	d.until("the process", func() bool { return f.running.Load() == 1 })
	f.produce(3000)
	d.until("streamed lines", func() bool { return strings.Contains(d.m.logText, "seq-") })
	start := time.Now()
	d.press("2")
	d.until("the follow stops", func() bool { return f.running.Load() == 0 })
	d.until("less opens", func() bool { return opened != "" })
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("since-window dump took %s", took)
	}
	if opened != body {
		t.Fatalf("pager file was rewritten:\n got %q\nwant %q", opened, body)
	}
	var dump string
	for _, line := range calls(fk, "logs") {
		if strings.Contains(line, "--since=60m") {
			dump = line
		}
		if strings.Contains(line, "--since=60m") && (strings.Contains(line, "--timestamps") || strings.Contains(line, "--since-time=") || strings.Contains(line, "--tail=")) {
			t.Fatalf("dump args %s", line)
		}
		if strings.Contains(line, "--since=60m") {
			for _, a := range strings.Fields(line) {
				if a == "-f" || a == "--follow" {
					t.Fatalf("dump followed: %s", line)
				}
			}
		}
	}
	if dump == "" {
		t.Fatalf("no --since=60m call in %v", calls(fk, "logs"))
	}
}

func TestStreamReopenShowsTheConfiguredTail(t *testing.T) {
	const tail, total = 50, 200
	f := &feed{}
	m := selectRow(t, actionModel(t, f.fake()), "db-0")
	m.cfg.Logs.MaxLines = 20000
	m.cfg.Logs.Tail = tail
	m.height, m.width = 30, 100
	d := newDriver(t, m)
	d.press("l")
	if d.m.stage != "logs" {
		t.Fatalf("stage %s", d.m.stage)
	}
	d.until("the process", func() bool { return f.running.Load() == 1 })
	f.produce(total)
	d.until("the first open", func() bool {
		return strings.HasSuffix(strings.TrimRight(d.m.logText, "\n"), fmt.Sprintf("seq-%06d", total-1))
	})

	d.press("q")
	if d.m.stage == "logs" {
		t.Fatal("still in logs")
	}
	d.until("the process to stop", func() bool { return f.running.Load() == 0 })

	d.press("l")
	if d.m.stage != "logs" {
		t.Fatalf("reopen stage %s", d.m.stage)
	}
	assertReopenedTail(t, d, f, tail, total)
}

func TestStreamReopenAfterEnterShowsTheConfiguredTail(t *testing.T) {
	const tail, total = 50, 180
	f := &feed{}
	m := selectRow(t, actionModel(t, f.fake()), "db-0")
	m.cfg.Logs.MaxLines = 20000
	m.cfg.Logs.Tail = tail
	m.height, m.width = 30, 100
	d := newDriver(t, m)
	d.press("enter")
	if d.m.stage != "logs" || d.m.vs.logCont != "pg" {
		t.Fatalf("enter stage %s cont %q", d.m.stage, d.m.vs.logCont)
	}
	d.until("the process", func() bool { return f.running.Load() == 1 })
	f.produce(total)
	d.until("the first open", func() bool {
		return strings.HasSuffix(strings.TrimRight(d.m.logText, "\n"), fmt.Sprintf("seq-%06d", total-1))
	})

	d.press("q")
	if d.m.stage == "logs" {
		t.Fatal("still in logs")
	}
	d.until("the process to stop", func() bool { return f.running.Load() == 0 })

	d.press("enter")
	if d.m.stage != "logs" {
		t.Fatalf("reopen stage %s", d.m.stage)
	}
	assertReopenedTail(t, d, f, tail, total)
}

func TestStreamSwitchBackToContainerShowsTheConfiguredTail(t *testing.T) {
	const tail, total = 50, 160
	f := &feed{}
	m := selectRow(t, actionModel(t, f.fake()), "web-1")
	m.cfg.Logs.MaxLines = 20000
	m.cfg.Logs.Tail = tail
	m.height, m.width = 30, 100
	d := newDriver(t, m)
	next, cmd := d.m.openLogs("shop/web-1", "app", false)
	d.m = next.(model)
	d.run(cmd)
	d.press("tab")
	d.until("the first container", func() bool { return f.running.Load() == 1 })
	f.produce(total)
	d.until("app lines", func() bool {
		return strings.HasSuffix(strings.TrimRight(d.m.logText, "\n"), fmt.Sprintf("seq-%06d", total-1))
	})

	next, cmd = d.m.openLogs("shop/web-1", "side", false)
	d.m = next.(model)
	d.run(cmd)
	d.press("tab")
	d.until("the second container", func() bool {
		return d.m.vs.logCont == "side" && f.running.Load() == 1 && len(f.calls()) >= 2
	})
	d.until("side lines", func() bool {
		return strings.Contains(d.m.logText, fmt.Sprintf("seq-%06d", total-1))
	})

	next, cmd = d.m.openLogs("shop/web-1", "app", false)
	d.m = next.(model)
	d.run(cmd)
	d.press("tab")
	if d.m.vs.logCont != "app" {
		t.Fatalf("switched back to %q", d.m.vs.logCont)
	}
	assertReopenedTail(t, d, f, tail, total)
}

func TestSternReopenShowsTheConfiguredTail(t *testing.T) {
	const tail, total = 50, 140
	f := &feed{}
	m := appModel(t, f.over(appCluster(t)))
	m.cfg.Logs.MultiMaxLines = 20000
	m.cfg.Logs.Tail = tail
	m.height, m.width = 30, 120
	d := newDriver(t, m)
	next, cmd := d.m.runCommand("logs app=web")
	d.m = next.(model)
	d.run(cmd)
	if d.m.stage != "mlogs" {
		t.Fatalf("stage %s", d.m.stage)
	}
	d.until("two processes", func() bool { return f.running.Load() == 2 })
	f.produce(total)
	d.until("the first lines", func() bool { return strings.Count(d.m.logText, fmt.Sprintf("seq-%06d", total-1)) == 2 })

	d.press("esc")
	d.until("both processes to stop", func() bool { return f.running.Load() == 0 })

	next, cmd = d.m.runCommand("logs app=web")
	d.m = next.(model)
	d.run(cmd)
	if d.m.stage != "mlogs" {
		t.Fatalf("reopen stage %s", d.m.stage)
	}
	d.until("two processes again", func() bool { return f.running.Load() == 2 })
	d.until("the tailed lines after reopen", func() bool {
		return strings.Count(d.m.logText, fmt.Sprintf("seq-%06d", total-tail)) == 2 &&
			strings.Count(d.m.logText, fmt.Sprintf("seq-%06d", total-1)) == 2
	})
	for pod, got := range sternText(t, d.m) {
		if len(got) == 0 || got[0] != total-tail || got[len(got)-1] != total-1 {
			t.Fatalf("%s reopen lines %v, want %d..%d", pod, got, total-tail, total-1)
		}
		for i := 1; i < len(got); i++ {
			if got[i] != got[i-1]+1 {
				t.Fatalf("%s: gap after %d: %d", pod, got[i-1], got[i])
			}
		}
	}
	calls := f.calls()
	if len(calls) < 4 {
		t.Fatalf("calls %v", calls)
	}
	for _, c := range calls[len(calls)-2:] {
		if !strings.Contains(c, fmt.Sprintf("--tail=%d", tail)) || strings.Contains(c, "--since-time=") {
			t.Fatalf("reopen args %s", c)
		}
	}
}

func assertReopenedTail(t *testing.T, d *driver, f *feed, tail, total int) {
	t.Helper()
	d.until("the process again", func() bool { return f.running.Load() == 1 })
	d.until("the tailed lines after reopen", func() bool {
		return strings.Contains(d.m.logText, fmt.Sprintf("seq-%06d", total-tail)) &&
			strings.HasSuffix(strings.TrimRight(d.m.logText, "\n"), fmt.Sprintf("seq-%06d", total-1))
	})
	if strings.Contains(ansi.Strip(d.m.logsBody(90)), "no log lines yet") {
		t.Fatal("reopen showed an empty follow")
	}
	got := contiguous(t, d.m.logText)
	if got[0] != total-tail || got[len(got)-1] != total-1 {
		t.Fatalf("reopen lines %d..%d, want %d..%d", got[0], got[len(got)-1], total-tail, total-1)
	}
	calls := f.calls()
	if len(calls) < 2 {
		t.Fatalf("calls %v", calls)
	}
	last := calls[len(calls)-1]
	if !strings.Contains(last, fmt.Sprintf("--tail=%d", tail)) || strings.Contains(last, "--since-time=") {
		t.Fatalf("reopen args %s", last)
	}
}

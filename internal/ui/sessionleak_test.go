package ui

import (
	"runtime"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func goroutines() int {
	runtime.GC()
	runtime.Gosched()
	return runtime.NumGoroutine()
}

func waitGoroutines(want, slop int, d time.Duration) int {
	deadline := time.Now().Add(d)
	n := goroutines()
	for n > want+slop && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
		n = goroutines()
	}
	return n
}

func TestSessionDoesNotLeak(t *testing.T) {
	base := waitGoroutines(0, 1000, 50*time.Millisecond)
	const cycles = 50

	f := &feed{}
	d := streamModel(t, f, 200)
	d.until("the process", func() bool { return f.running.Load() == 1 })
	for i := 0; i < cycles; i++ {
		d.press("q")
		d.until("stopped", func() bool { return f.running.Load() == 0 && d.m.stage != "logs" })
		d.press("l")
		d.until("started", func() bool { return f.running.Load() == 1 && d.m.stage == "logs" })
	}
	d.press("q")
	d.until("final logs stop", func() bool { return f.running.Load() == 0 })

	mf := &feed{}
	mm := appModel(t, mf.over(appCluster(t)))
	mm.height, mm.width = 30, 120
	md := newDriver(t, mm)
	for i := 0; i < cycles; i++ {
		next, cmd := md.m.runCommand("logs app=web")
		md.m = next.(model)
		md.run(cmd)
		md.until("mlogs", func() bool { return md.m.stage == "mlogs" })
		md.press("esc")
		md.until("left mlogs", func() bool { return md.m.stage != "mlogs" && mf.running.Load() == 0 })
	}

	m := selectRow(t, actionModel(t, containerCluster()), "web-1")
	m.width, m.height = 220, 40
	for i := 0; i < cycles; i++ {
		m = pressSettle(t, m, "enter")
		if !m.inContainers() {
			t.Fatalf("cycle %d: expected containers, stage %s", i, m.stage)
		}
		m = pressSettle(t, m, "q")
		if m.inContainers() {
			t.Fatalf("cycle %d: still in containers", i)
		}
		m = selectRow(t, m, "web-1")
		m = pressSettle(t, m, "d")
		if m.stage != "describe" {
			t.Fatalf("cycle %d: stage %s", i, m.stage)
		}
		m = pressSettle(t, m, "q")
		if m.stage == "describe" {
			t.Fatalf("cycle %d: still describing", i)
		}
		m = selectRow(t, m, "web-1")
		next, cmd := m.openPortForward()
		m = next.(model)
		if cmd != nil {
			m = settle(t, m, cmd)
		}
		if m.pfOn {
			m = pressSettle(t, m, "esc")
		}
		if m.pfOn {
			t.Fatalf("cycle %d: forward popup still open", i)
		}
	}

	got := waitGoroutines(base, 8, 3*time.Second)
	if got > base+8 {
		t.Fatalf("after %d cycles goroutines %d, baseline %d", cycles, got, base)
	}
	if f.running.Load() != 0 || mf.running.Load() != 0 {
		t.Fatalf("follow still running: single %d multi %d", f.running.Load(), mf.running.Load())
	}
}

func TestWatchToggleDoesNotMultiplyTicks(t *testing.T) {
	m := selectRow(t, actionModel(t, containerCluster()), "web-1")
	m.height, m.width = 220, 40
	m = pressSettle(t, m, "d")
	if m.stage != "describe" {
		t.Fatalf("stage %s", m.stage)
	}
	base := waitGoroutines(0, 1000, 50*time.Millisecond)
	for i := 0; i < 50; i++ {
		next, cmd := m.toggleWatch()
		m = next.(model)
		if cmd != nil {
			done := make(chan tea.Msg, 1)
			go func(c tea.Cmd) { done <- c() }(cmd)
			select {
			case <-done:
			case <-time.After(80 * time.Millisecond):
			}
		}
	}
	m.vs.watchOn = false
	got := waitGoroutines(base, 6, 2*time.Second)
	if got > base+6 {
		t.Fatalf("watch toggles leaked goroutines: %d vs baseline %d", got, base)
	}
}

func TestSpinnerTicksStopWithLoad(t *testing.T) {
	m := actionModel(t, actionCluster())
	base := waitGoroutines(0, 1000, 50*time.Millisecond)
	for i := 0; i < 50; i++ {
		cmd := m.startLoad("loading logs", true)
		if cmd != nil {
			done := make(chan tea.Msg, 1)
			go func(c tea.Cmd) { done <- c() }(cmd)
			select {
			case msg := <-done:
				next, more := m.Update(msg)
				m = next.(model)
				_ = more
			case <-time.After(30 * time.Millisecond):
			}
		}
		m.load = load{}
		m.spinning = false
	}
	got := waitGoroutines(base, 6, 2*time.Second)
	if got > base+6 {
		t.Fatalf("spinner ticks leaked goroutines: %d vs baseline %d", got, base)
	}
}

package ui

import (
	"os"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func frameBudget() time.Duration {
	base := 16 * time.Millisecond
	if v := os.Getenv("K2_BUDGET_MS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			base = time.Duration(n) * time.Millisecond
		}
	}
	mult := 4
	if os.Getenv("CI") != "" {
		mult = 8
	}
	if v := os.Getenv("K2_BUDGET_MULT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			mult = n
		}
	}
	return base * time.Duration(mult)
}

func updateWatchdog(t *testing.T, m model, msg tea.Msg, max time.Duration) (model, tea.Cmd) {
	t.Helper()
	done := make(chan struct{})
	var next tea.Model
	var cmd tea.Cmd
	go func() {
		next, cmd = m.Update(msg)
		close(done)
	}()
	select {
	case <-done:
		return next.(model), cmd
	case <-time.After(max):
		t.Fatalf("Update blocked for more than %s on %T", max, msg)
	}
	return m, nil
}

func timeUpdateView(t *testing.T, m model, msg tea.Msg) (model, time.Duration) {
	t.Helper()
	start := time.Now()
	next, _ := updateWatchdog(t, m, msg, 200*time.Millisecond)
	m = next
	_ = m.View()
	return m, time.Since(start)
}

func budgetTimes(t *testing.T, name string, n int, setup func() model, step func(*model)) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		m := setup()
		_ = m.View()
		samples := make([]time.Duration, n)
		for i := 0; i < n; i++ {
			start := time.Now()
			step(&m)
			_ = m.View()
			samples[i] = time.Since(start)
		}
		sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
		med := samples[n/2]
		p95 := samples[min(n-1, (n*95)/100)]
		budget := frameBudget()
		t.Logf("n=%d median=%s p95=%s budget=%s", n, med, p95, budget)
		if p95 > budget {
			t.Fatalf("%s p95 %s exceeds budget %s (median %s)", name, p95, budget, med)
		}
	})
}

func TestBudgetTypicalActions(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	n := 21
	pods := func() model {
		m, _ := bigCluster(t, 5000, 2000)
		return m
	}
	budgetTimes(t, "view", n, pods, func(m *model) {})
	budgetTimes(t, "scroll", n, pods, func(m *model) {
		var cmd tea.Cmd
		*m, cmd = updateWatchdog(t, *m, key(t, "j"), 200*time.Millisecond)
		_ = cmd
	})
	budgetTimes(t, "page", n, pods, func(m *model) {
		*m, _ = updateWatchdog(t, *m, key(t, "pgdown"), 200*time.Millisecond)
	})
	budgetTimes(t, "jump", n, pods, func(m *model) {
		k := "G"
		if m.logAt != 0 || m.content.Index() > 0 {
			k = "g"
		}
		*m, _ = updateWatchdog(t, *m, key(t, k), 200*time.Millisecond)
	})
	budgetTimes(t, "filter", n, func() model {
		m := pods()
		m, _ = updateWatchdog(t, m, key(t, "/"), 200*time.Millisecond)
		_ = m.View()
		return m
	}, func(m *model) {
		*m, _ = updateWatchdog(t, *m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}}, 200*time.Millisecond)
	})
	budgetTimes(t, "sort", n, pods, func(m *model) {
		*m, _ = updateWatchdog(t, *m, key(t, ">"), 200*time.Millisecond)
	})
	budgetTimes(t, "command", n, func() model {
		m := pods()
		m, _ = updateWatchdog(t, m, key(t, ":"), 200*time.Millisecond)
		_ = m.View()
		return m
	}, func(m *model) {
		*m, _ = updateWatchdog(t, *m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}}, 200*time.Millisecond)
	})
	budgetTimes(t, "resize", n, pods, func(m *model) {
		w := 200
		if m.width == 200 {
			w = 180
		}
		*m, _ = updateWatchdog(t, *m, tea.WindowSizeMsg{Width: w, Height: 50}, 200*time.Millisecond)
	})
	budgetTimes(t, "panes", n, func() model {
		m := withSplits(pods(), 2)
		_ = m.View()
		return m
	}, func(m *model) {
		*m, _ = updateWatchdog(t, *m, key(t, "o"), 200*time.Millisecond)
	})
	budgetTimes(t, "pods-command", n, pods, func(m *model) {
		next, cmd := m.runCommand("pods")
		nm := next.(model)
		if cmd != nil {
			nm, _ = updateWatchdog(t, nm, tea.KeyMsg{Type: tea.KeyEsc}, 200*time.Millisecond)
		}
		*m = nm
	})
	budgetTimes(t, "logs-open", n, pods, func(m *model) {
		next, _ := m.openLogs("shop/web-frontend-production-deployment-7d9f8c6b5d-00000", "app", false)
		*m = next.(model)
		*m, _ = updateWatchdog(t, *m, key(t, "q"), 200*time.Millisecond)
	})
	budgetTimes(t, "log-scroll", n, func() model {
		m, _ := bigCluster(t, 20, 0)
		m = fillLogs(m, 100000)
		_ = m.View()
		m, _ = updateWatchdog(t, m, key(t, "g"), 200*time.Millisecond)
		_ = m.View()
		return m
	}, func(m *model) {
		*m, _ = updateWatchdog(t, *m, key(t, "pgdown"), 200*time.Millisecond)
	})
	budgetTimes(t, "secrets-view", n, func() model {
		m, _ := bigCluster(t, 20, 2000)
		header, items := packSecrets(2000, 196)
		m = loadTable(m, "res", "secrets", header, items)
		_ = m.View()
		return m
	}, func(m *model) {})
}

func TestTrimLogDropMatchesSplit(t *testing.T) {
	s := "a\nb\nc\nd\n"
	got, n := trimLogDrop(s, 2)
	if n != 3 || got != "d\n" {
		t.Fatalf("got %q drop %d", got, n)
	}
	if dropHeadLines("a\nb\nc", 1) != "b\nc" || headLines("a\nb\nc", 2) != "a\nb" {
		t.Fatalf("head/drop mismatch %q %q", dropHeadLines("a\nb\nc", 1), headLines("a\nb\nc", 2))
	}
}

func TestReflowSkipsUnchangedWidth(t *testing.T) {
	_, items := packPods(200, 80)
	lst := list.New(items, rowDelegate{width: 80}, 80, 10)
	lst.SetShowTitle(false)
	reflowList(&lst, 80)
	first := lst.Items()[0].(item).title
	reflowList(&lst, 80)
	if lst.Items()[0].(item).title != first {
		t.Fatal("second reflow at the same width rewrote rows")
	}
	reflowList(&lst, 40)
	if sameWidths(lst.Items()[0].(item).widths, items[0].(item).widths) {
		t.Fatal("narrow reflow left old widths")
	}
}

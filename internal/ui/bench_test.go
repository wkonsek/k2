package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func BenchmarkTableView(b *testing.B) {
	m, _ := bigCluster(b, 5000, 2000)
	m = withSplits(m, 2)
	_ = m.View()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.View()
	}
}

func BenchmarkTableScroll(b *testing.B) {
	m, _ := bigCluster(b, 5000, 2000)
	_ = m.View()
	keys := []string{"j", "j", "k", "pgdown", "pgup"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m = doUpdateView(m, key(b, keys[i%len(keys)]))
	}
}

func BenchmarkTableJump(b *testing.B) {
	m, _ := bigCluster(b, 5000, 2000)
	_ = m.View()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			m = doUpdateView(m, key(b, "G"))
		} else {
			m = doUpdateView(m, key(b, "g"))
		}
	}
}

func BenchmarkTableFilterKey(b *testing.B) {
	m, _ := bigCluster(b, 5000, 2000)
	m = doUpdateView(m, key(b, "/"))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m = doUpdateView(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
		if i%8 == 7 {
			m = doUpdateView(m, key(b, "backspace"))
		}
	}
}

func BenchmarkTableSort(b *testing.B) {
	m, _ := bigCluster(b, 5000, 2000)
	_ = m.View()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m = doUpdateView(m, key(b, ">"))
	}
}

func BenchmarkSecretsView(b *testing.B) {
	m, _ := bigCluster(b, 100, 2000)
	header, items := packSecrets(2000, 196)
	m = loadTable(m, "res", "secrets", header, items)
	_ = m.View()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.View()
	}
}

func BenchmarkLogView100k(b *testing.B) {
	m, _ := bigCluster(b, 20, 0)
	m = fillLogs(m, 100000)
	_ = m.View()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.View()
	}
}

func BenchmarkLogScroll100k(b *testing.B) {
	m, _ := bigCluster(b, 20, 0)
	m = fillLogs(m, 100000)
	m = doUpdateView(m, key(b, "g"))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m = doUpdateView(m, key(b, "pgdown"))
	}
}

func BenchmarkLogCatchUp(b *testing.B) {
	m, _ := bigCluster(b, 20, 0)
	m = fillLogs(m, 100000)
	m.cfg.Logs.MaxLines = 100000
	_ = m.View()
	more := strings.Repeat("2026-11-23T10:00:01.000000000Z catch-up line\n", 1000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.logStick = false
		m.vs.logPend = more
		m.vs.logNew = 1000
		m = doUpdateView(m, key(b, "G"))
	}
}

func BenchmarkLogBatchApply(b *testing.B) {
	m, _ := bigCluster(b, 20, 0)
	m = fillLogs(m, 100000)
	m.cfg.Logs.MaxLines = 100000
	_ = m.View()
	batch := strings.TrimRight(strings.Repeat("2026-11-23T10:00:01.000000000Z stream-line\n", 1000), "\n")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.applyLogs(logsMsg{id: m.logID, pod: logKey(m.logPod, m.vs.logOpts()), text: batch, partial: true, stream: true})
		_ = m.View()
	}
}

func BenchmarkCommandLine(b *testing.B) {
	m, _ := bigCluster(b, 5000, 2000)
	m = doUpdateView(m, key(b, ":"))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m = doUpdateView(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		m = doUpdateView(m, key(b, "backspace"))
	}
}

func BenchmarkContainersView(b *testing.B) {
	m := modelFor(b, envFor(b), nil)
	header, items, err := packTable(
		[]string{"NAME", "IMAGE", "READY", "STATE", "RESTARTS", "PORTS", "CPU", "MEM", "AGE"},
		containerRows(40),
		containerRefs(40),
		nil, 3, 196, "shop",
	)
	if err != nil {
		b.Fatal(err)
	}
	m = loadTable(m, "workpods", "pods", header, items)
	m.vs.podSel, m.vs.podTitle = "shop/web", "web"
	_ = m.View()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.View()
	}
}

func BenchmarkThemePaintRow(b *testing.B) {
	_, items := packPods(1, 120)
	it := items[0].(item)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = paintRow(it.cells, it.widths, it.heads, it.colorCol, it.flex, it.dimCol, it.ageIdx, it.tones, it.meters)
	}
}

func containerRows(n int) [][]string {
	rows := make([][]string, n)
	for i := 0; i < n; i++ {
		rows[i] = []string{"app-" + itoa(i), "registry.example.com/team/app:1.2.3", "1/1", "Running", "0", "8080,9090", "50m", "64Mi", "3d"}
	}
	return rows
}

func containerRefs(n int) [][2]string {
	out := make([][2]string, n)
	for i := 0; i < n; i++ {
		out[i] = [2]string{"shop", "app-" + itoa(i)}
	}
	return out
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var d [12]byte
	n := len(d)
	for i > 0 {
		n--
		d[n] = byte('0' + i%10)
		i /= 10
	}
	return string(d[n:])
}

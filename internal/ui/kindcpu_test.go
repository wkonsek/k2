package ui

import (
	"strings"
	"testing"

	"k2/internal/kube"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func kindCPULine(t *testing.T, cpus int, pct float64) (string, string) {
	t.Helper()
	prev := hostCPUs
	hostCPUs = func() int { return cpus }
	t.Cleanup(func() { hostCPUs = prev })
	m := appModel(t, nil)
	tile := kindTile{status: "running", usageOK: true, usage: kube.KindUsage{CPU: pct, MemUsed: 1 << 30, MemPct: 10, OK: true}}
	lines := m.kindUsageLines(tile, 60)
	return lines[0], ansi.Strip(lines[0])
}

func TestKindCPUCoresAndHostBar(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	cases := []struct {
		name string
		cpus int
		pct  float64
		text string
		bar  string
		tone string
	}{
		{"single core", 1, 100, "CPU  1.0c (100%)", "■■■■■", "err"},
		{"multi core", 20, 139, "CPU  1.4c (7%)", "■□□□□", "ok"},
		{"idle", 20, 20, "CPU  0.2c (1%)", "■□□□□", "ok"},
		{"warm", 8, 600, "CPU  6.0c (75%)", "■■■■□", "warn"},
		{"high load", 20, 1900, "CPU  19.0c (95%)", "■■■■■", "err"},
	}
	for _, c := range cases {
		raw, plain := kindCPULine(t, c.cpus, c.pct)
		style := map[string]lipgloss.Style{"ok": okStyle, "warn": warnStyle, "err": errStyle}[c.tone]
		if !strings.HasPrefix(plain, c.text+"  "+c.bar) {
			t.Fatalf("%s: line %q, want %q", c.name, plain, c.text+"  "+c.bar)
		}
		if !strings.Contains(raw, style.Render(c.bar)) {
			t.Fatalf("%s: wrong bar colour %q", c.name, raw)
		}
		if strings.Contains(plain, "139%") {
			t.Fatalf("%s: raw docker percent leaked %q", c.name, plain)
		}
	}
}

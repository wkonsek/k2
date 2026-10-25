package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"k2/internal/theme"
)

func TestStatusTone(t *testing.T) {
	cases := map[string]tone{
		"Running": toneOK, "Active": toneOK, "Bound": toneOK, "Ready": toneOK, "AtLatestKnown": toneOK,
		"Completed": toneDone, "Succeeded": toneOK, "Complete": toneDone, "InstallReady": toneWarn, "TRANSIENT_FAILURE": toneErr, "CONNECTING": toneWarn, "READY": toneOK,
		"Pending": toneWarn, "ContainerCreating": toneWarn, "PodInitializing": toneWarn, "Init:0/1": toneWarn,
		"Terminating": toneWarn, "Unknown": toneWarn, "NotReady": toneWarn, "RequiresApproval": toneWarn,
		"Installing": toneWarn, "UpgradePending": toneWarn,
		"Error": toneErr, "CrashLoopBackOff": toneErr, "ImagePullBackOff": toneErr, "ErrImagePull": toneErr,
		"OOMKilled": toneErr, "Evicted": toneErr, "RunContainerError": toneErr, "Init:CrashLoopBackOff": toneErr,
		"CreateContainerConfigError": toneErr, "Failed": toneErr, "InvalidImageName": toneErr,
		"": toneNormal, "-": toneNormal, "Opaque": toneNormal,
	}
	for s, want := range cases {
		if got := statusTone(s); got != want {
			t.Errorf("statusTone(%q) = %d, want %d", s, got, want)
		}
	}
}

func TestToneColorsOnEveryTheme(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	defer useTheme(theme.Nord())
	for _, name := range theme.Names() {
		p, _ := theme.Lookup(name)
		useTheme(p)
		want := map[string]lipgloss.Color{"Running": p.OK, "Completed": p.Accent2, "Pending": p.Warn, "CrashLoopBackOff": p.Danger, "Opaque": p.Text}
		for status, c := range want {
			if got := statusStyle(status).Render("x"); !strings.HasPrefix(got, sgr(c)) {
				t.Errorf("%s: %s rendered %q, want color %q", name, status, got, sgr(c))
			}
		}
	}
}

func TestRowProblems(t *testing.T) {
	heads := []string{"NAME", "READY", "STATUS", "RESTARTS", "IP", "NODE", "AGE"}
	check := func(cells []string, name bool, cells2 map[int]tone) {
		t.Helper()
		mk := rowProblems(heads, cells, 2)
		if mk.name != name || len(mk.cells) != len(cells2) {
			t.Fatalf("%v: name=%v cells=%v", cells, mk.name, mk.cells)
		}
		for c, tn := range cells2 {
			if mk.cells[c] != tn {
				t.Fatalf("%v: column %d tone %v, want %v", cells, c, mk.cells[c], tn)
			}
		}
	}
	check([]string{"web", "1/1", "Running", "0", "-", "-", "1m"}, false, nil)

	check([]string{"web", "0/1", "Running", "0", "-", "-", "1m"}, true, map[int]tone{1: toneWarn})

	check([]string{"web", "1/1", "Running", "28", "-", "-", "13d"}, false, map[int]tone{3: toneWarn})
	check([]string{"web", "1/1", "Running", "28 (13d ago)", "-", "-", "13d"}, false, map[int]tone{3: toneWarn})
	check([]string{"web", "1/1", "Running", "3 (2h10m ago)", "-", "-", "1d"}, false, map[int]tone{3: toneWarn})

	check([]string{"web", "1/1", "Running", "3 (5m ago)", "-", "-", "1d"}, false, map[int]tone{3: toneErr})
	check([]string{"web", "1/1", "Running", "1 (40s ago)", "-", "-", "1d"}, false, map[int]tone{3: toneErr})

	check([]string{"web", "1/2", "CrashLoopBackOff", "7", "-", "-", "1m"}, true, map[int]tone{1: toneWarn, 3: toneWarn})
	check([]string{"web", "-", "Pending", "0", "-", "-", "1m"}, true, nil)
	check([]string{"db", "1/1", "Terminating", "0", "-", "-", "1m"}, true, nil)
	check([]string{"job", "0/1", "Completed", "0", "-", "-", "1m"}, false, nil)

	mk := rowProblems([]string{"NAME", "AGE"}, []string{"cm terminating", "1m"}, -1)
	if !mk.name || mk.cells[0] != toneWarn {
		t.Fatal("terminating object not marked")
	}
	if mk := rowProblems([]string{"NAME", "TYPE"}, []string{"s", "Opaque"}, 1); mk.name || len(mk.cells) != 0 {
		t.Fatal("plain object marked")
	}
}

func TestPaintRowColors(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	defer useTheme(theme.Nord())
	for _, name := range theme.Names() {
		p, _ := theme.Lookup(name)
		useTheme(p)
		heads := []string{"NAME", "READY", "STATUS", "RESTARTS", "AGE"}
		widths := []int{6, 5, 16, 8, 4}
		row, plain := paintRow([]string{"web", "0/1", "CrashLoopBackOff", "4", "3m"}, widths, heads, 2, 0, -1, 4, nil)
		if !strings.Contains(plain, "web") || !strings.Contains(plain, "CrashLoopBackOff") {
			t.Fatalf("plain %q", plain)
		}
		for cell, c := range map[string]lipgloss.Color{"web": p.Warn, "0/1": p.Warn, "CrashLoopBackOff": p.Danger, "4": p.Warn, "3m": p.Muted} {
			if !strings.Contains(row, sgr(c)+cell) {
				t.Errorf("%s: %q not drawn with %q in %q", name, cell, sgr(c), row)
			}
		}
		healthy, _ := paintRow([]string{"api", "1/1", "Running", "0", "3m"}, widths, heads, 2, 0, -1, 4, nil)
		if !strings.Contains(healthy, sgr(p.Text)+"api") {
			t.Errorf("%s: healthy name not in the text color: %q", name, healthy)
		}

		old, _ := paintRow([]string{"api", "1/1", "Running", "28", "3m"}, widths, heads, 2, 0, -1, 4, nil)
		if !strings.Contains(old, sgr(p.Text)+"api") || !strings.Contains(old, sgr(p.Warn)+"28") {
			t.Errorf("%s: restarted row %q", name, old)
		}
		fresh, _ := paintRow([]string{"api", "1/1", "Running", "2 (1m ago)", "3m"}, []int{6, 5, 16, 10, 4}, heads, 2, 0, -1, 4, nil)
		if !strings.Contains(fresh, sgr(p.Text)+"api") || !strings.Contains(fresh, sgr(p.Danger)+"2 (1m ago)") {
			t.Errorf("%s: fresh restart row %q", name, fresh)
		}
	}
}

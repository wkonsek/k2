package ui

import (
	"regexp"
	"strings"

	"k2/internal/kustomize"

	"github.com/charmbracelet/x/ansi"
)

var klogAlert = regexp.MustCompile(`^[EW]\d{4} \d\d:\d\d:\d\d`)

func isAlert(line string) bool {
	t := strings.TrimSpace(ansi.Strip(line))
	for _, icon := range []string{"✗ ", "! "} {
		t = strings.TrimPrefix(t, icon)
	}
	if klogAlert.MatchString(t) {
		return true
	}
	c := kustomize.Classify(t)
	return c == kustomize.Error || c == kustomize.Warning
}

func (m model) alertsWrap() bool { return m.stage == "kout" || m.stage == "view" }

func (m model) wrapsLine(l string) bool {
	return m.wrapping() || m.alertsWrap() && isAlert(l)
}

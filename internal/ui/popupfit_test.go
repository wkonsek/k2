package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func popupLines(m model) []string {
	x, _, w := m.popupRect()
	var out []string
	for _, l := range strings.Split(ansi.Strip(m.withDialog(strings.Repeat(strings.Repeat(" ", m.width)+"\n", m.height-1)+strings.Repeat(" ", m.width))), "\n") {
		if ansi.StringWidth(l) > x {
			out = append(out, ansi.Cut(l, x, x+w))
		}
	}
	return out
}

func noEllipsis(t *testing.T, what string, m model) {
	t.Helper()
	for _, l := range popupLines(m) {
		if strings.Contains(l, "…") || strings.Contains(l, "...") {
			t.Fatalf("%s: truncated line %q\n%s", what, l, strings.Join(popupLines(m), "\n"))
		}
	}
}

func TestKindCreatePopupHasNoStrayEllipsis(t *testing.T) {
	withKindCLIs(t)
	for _, size := range [][2]int{{120, 40}, {160, 45}, {100, 50}} {
		m := open(t, appModel(t, kindFake(t, nil)), "kind")
		m.width, m.height = size[0], size[1]
		m = pressSettle(t, m, "A")
		setHelmField(m.dlg, "image", "kindest/node:v1.37.0")
		noEllipsis(t, "kind create", m)
		for _, key := range []string{"name", "planes", "workers", "image"} {
			m = focusKindField(t, m, key)
			noEllipsis(t, "kind create focus "+key, m)
		}
		m = focusKindField(t, m, "c:metallb")
		m = press(t, m, " ")
		m = press(t, m, "down")
		noEllipsis(t, "kind create ns row", m)
	}
}

func TestKindConfirmAndHelmFormHaveNoStrayEllipsis(t *testing.T) {
	withKindCLIs(t)
	m := open(t, appModel(t, kindFake(t, nil)), "kind")
	m = pressSettle(t, m, "A")
	m = focusKindField(t, m, "c:metallb")
	m = press(t, m, " ")
	next, cmd := m.submitKindForm()
	m = settle(t, next.(model), cmd)
	if m.dlg == nil || m.dlg.ask == "" {
		t.Fatal("no confirm")
	}
	noEllipsis(t, "kind confirm", m)

	withHelmCLI(t)
	h := open(t, appModel(t, helmInstallFake(t, "")), "helm")
	h = pressSettle(t, h, "A")
	if h.dlg == nil || h.dlg.act != "helminstall" {
		t.Fatalf("helm form %+v", h.dlg)
	}
	setHelmField(h.dlg, "name", "web")
	setHelmField(h.dlg, "chart", "bitnami/nginx")
	noEllipsis(t, "helm install", h)
}

func TestKindFormScrollsInShortTerminal(t *testing.T) {
	withKindCLIs(t)
	m := open(t, appModel(t, kindFake(t, nil)), "kind")
	m.width, m.height = 120, 20
	m = pressSettle(t, m, "A")
	for _, f := range m.dlg.hfields {
		if strings.HasPrefix(f.key, "c:") {
			m = focusKindField(t, m, f.key)
			m = press(t, m, " ")
		}
	}
	m = focusKindField(t, m, "n:kafbat")
	box := ansi.Strip(m.dialogBox(m.popupRectW()))
	if h := strings.Count(box, "\n") + 1; h > m.height {
		t.Fatalf("popup %d rows in a %d row terminal:\n%s", h, m.height, box)
	}
	if !strings.Contains(box, "↑ ") || !strings.Contains(box, "kafbat") || !strings.Contains(box, "confirm") {
		t.Fatalf("scroll window:\n%s", box)
	}
	m = focusKindField(t, m, "name")
	box = ansi.Strip(m.dialogBox(m.popupRectW()))
	if !strings.Contains(box, "↓ ") || !strings.Contains(box, "name") || strings.Contains(box, "…") {
		t.Fatalf("top window:\n%s", box)
	}
}

func (m model) popupRectW() int {
	_, _, w := m.popupRect()
	return w
}

package ui

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestHelmValuesNotStalePodYAML(t *testing.T) {
	f := appCluster(t)
	m := open(t, appModel(t, f), "pods")
	next, cmd := m.yamlOf("pods", "shop", "web-7d9f-aaaaa", "")
	m = settle(t, next.(model), cmd)
	if m.stage != "yaml" || !strings.Contains(m.logText, "kind: pods") || !strings.Contains(m.logText, "web-7d9f-aaaaa") {
		t.Fatalf("pod yaml: stage %s\n%s", m.stage, m.logText)
	}
	podYAML := m.logText
	m = pressSettle(t, m, "esc")
	m = selectRow(t, open(t, m, "helm"), "web")
	checks := []struct{ key, want string }{
		{"enter", "# user-supplied values of shop/web"},
		{"V", "# computed values of shop/web"},
		{"M", "kind: Deployment"},
	}
	for _, c := range checks {
		m = pressSettle(t, m, c.key)
		view := ansi.Strip(m.View())
		if m.stage != "yaml" || !strings.Contains(m.logText, c.want) {
			t.Fatalf("%s: stage %s text:\n%s", c.key, m.stage, m.logText)
		}
		if strings.Contains(m.logText, podYAML) || strings.Contains(m.logText, "kind: pods") || strings.Contains(m.logText, "web-7d9f-aaaaa") ||
			strings.Contains(view, "kind: pods") || strings.Contains(view, "web-7d9f-aaaaa") {
			t.Fatalf("%s: stale pod yaml still shown:\n%s\nview:\n%s", c.key, m.logText, view)
		}
		m.applyYAML(yamlMsg{id: m.logID, text: "kind: Pod\nmetadata:\n  name: zookeeper-1\n"})
		if strings.Contains(m.logText, "zookeeper-1") || strings.Contains(m.logText, "kind: Pod") || !strings.Contains(m.logText, c.want) {
			t.Fatalf("%s: late yamlMsg overwrote helm text:\n%s", c.key, m.logText)
		}
		m.vs.watchOn = true
		m.vs.watchName = "zookeeper-1"
		m.applyWatch(watchMsg{id: m.logID, stage: "yaml", text: "kind: Pod\nmetadata:\n  name: zookeeper-1\n"})
		if strings.Contains(m.logText, "zookeeper-1") || !strings.Contains(m.logText, c.want) {
			t.Fatalf("%s: watchMsg overwrote helm text:\n%s", c.key, m.logText)
		}
		m = pressSettle(t, m, "esc")
		if m.stage != "helm" {
			t.Fatalf("%s: back went to %s", c.key, m.stage)
		}
	}
}

func TestHelmEditValues(t *testing.T) {
	withHelmCLI(t)
	f := helmInstallFake(t, "")
	m := appModel(t, f)
	m.cfg.UI.EditorInline = false
	m = selectRow(t, open(t, m, "helm"), "web")

	m.readOnly = true
	if got := press(t, m, "e"); got.asking() != "" || !strings.Contains(got.status, "read-only") {
		t.Fatalf("read-only list: confirm %q status %q", got.asking(), got.status)
	}
	m.readOnly = false
	m = pressSettle(t, m, "enter")
	if !strings.Contains(m.logText, "user-supplied values") {
		t.Fatalf("values view:\n%s", m.logText)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "v edit") && !strings.Contains(m.status, "v edit") {
		t.Fatalf("values view missing edit hint:\nstatus %q\n%s", m.status, view)
	}
	m.readOnly = true
	if got := press(t, m, "e"); got.asking() != "" || !strings.Contains(got.status, "read-only") {
		t.Fatalf("read-only values view: confirm %q status %q", got.asking(), got.status)
	}
	m.readOnly = false
	m = pressSettle(t, m, "esc")

	var opened string
	old := vimCommand
	vimCommand = func(path string) *exec.Cmd {
		opened = path
		return exec.Command("true")
	}
	t.Cleanup(func() { vimCommand = old })

	next, cmd := m.Update(key(t, "e"))
	m = next.(model)
	if opened == "" || cmd == nil {
		t.Fatalf("e on list: opened %q cmd %v", opened, cmd != nil)
	}
	before, err := os.ReadFile(opened)
	if err != nil {
		t.Fatal(err)
	}
	r, ok := m.selectedRelease()
	if !ok {
		t.Fatal("no release")
	}
	msg := helmValuesEditResult(opened, string(before), r, "nginx", "1.2.0", nil)
	if !msg.unchanged || msg.err != nil {
		t.Fatalf("unchanged result %+v", msg)
	}
	next, cmd = m.Update(msg)
	m = settle(t, next.(model), cmd)
	if m.asking() != "" || !strings.Contains(m.status, "no changes") {
		t.Fatalf("unchanged: confirm %q status %q", m.asking(), m.status)
	}
	if len(helmVerbCalls(f, "upgrade")) != 0 {
		t.Fatalf("upgrade ran on no changes: %v", helmVerbCalls(f, "upgrade"))
	}

	opened = ""
	next, _ = m.Update(key(t, "e"))
	m = next.(model)
	if opened == "" {
		t.Fatal("e did not open a file")
	}
	if err := os.WriteFile(opened, []byte("replicas: 9\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r, ok = m.selectedRelease()
	if !ok {
		t.Fatal("no release")
	}
	msg = helmValuesEditResult(opened, string(before), r, "nginx", "1.2.0", nil)
	if msg.unchanged || msg.err != nil || !strings.Contains(msg.text, "replicas: 9") {
		t.Fatalf("changed result %+v", msg)
	}
	next, cmd = m.Update(msg)
	m = settle(t, next.(model), cmd)
	if m.dlg == nil || !strings.Contains(m.dlg.ask, "upgrade web in shop?") || m.stage == "htpl" {
		t.Fatalf("confirm %+v stage %s", m.dlg, m.stage)
	}
	if strings.Contains(ansi.Strip(m.View()), "enter preview") {
		t.Fatalf("landed on the upgrade form:\n%s", ansi.Strip(m.View()))
	}
	m = pressSettle(t, m, "y")
	got := helmVerbCalls(f, "upgrade")
	if len(got) != 1 || !strings.Contains(got[0], "upgrade web") || !strings.Contains(got[0], "-n shop") || !strings.Contains(got[0], "-f ") {
		t.Fatalf("upgrade args %v", got)
	}

	m = selectRow(t, open(t, appModel(t, f), "helm"), "web")
	m.cfg.UI.EditorInline = false
	m = pressSettle(t, m, "enter")
	opened = ""
	next, _ = m.Update(key(t, "e"))
	m = next.(model)
	if opened == "" {
		t.Fatal("e from values view did not open a file")
	}
	orig, err := os.ReadFile(opened)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(opened, []byte("replicas: 11\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r, ok = m.selectedRelease()
	if !ok {
		t.Fatal("no release from values view")
	}
	msg = helmValuesEditResult(opened, string(orig), r, "nginx", "1.2.0", nil)
	next, cmd = m.Update(msg)
	m = settle(t, next.(model), cmd)
	if m.dlg == nil || !strings.Contains(m.dlg.ask, "upgrade web in shop?") {
		t.Fatalf("values-view confirm %+v", m.dlg)
	}
}

func TestHelmEditValuesNeedsCLI(t *testing.T) {
	lookPath = func(string) (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { lookPath = defaultLookPath })
	m := selectRow(t, open(t, appModel(t, appCluster(t)), "helm"), "web")
	if got := press(t, m, "e"); got.asking() != "" || !strings.Contains(got.status, "needs the helm CLI") {
		t.Fatalf("status %q confirm %q", got.status, got.asking())
	}
}

func TestHelmLegendDropsValuesKeepsKeys(t *testing.T) {
	withHelmCLI(t)
	f := helmInstallFake(t, "")
	m := selectRow(t, open(t, appModel(t, f), "helm"), "web")
	l := ansi.Strip(m.legend())
	if !strings.Contains(l, "enter values") || !strings.Contains(l, "v edit values") || !strings.Contains(l, "V computed") || !strings.Contains(l, "u upgrade") || strings.Contains(l, "e edit values") {
		t.Fatalf("legend %q", l)
	}
	next := pressSettle(t, m, "enter")
	if next.stage != "yaml" || !strings.Contains(next.logText, "# user-supplied values of shop/web") {
		t.Fatalf("enter: stage %s\n%s", next.stage, next.logText)
	}
}

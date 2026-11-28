package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"k2/internal/kube"
	"k2/internal/shell"

	"github.com/charmbracelet/x/ansi"
)

func stubVim(t *testing.T) *string {
	t.Helper()
	opened := ""
	old := vimCommand
	vimCommand = func(p string) *exec.Cmd { opened = p; return exec.Command("true") }
	t.Cleanup(func() { vimCommand = old })
	return &opened
}

func TestHelmVEditsEnterPreviews(t *testing.T) {
	withHelmCLI(t)
	opened := stubVim(t)
	f := helmInstallFake(t, "")
	m := appModel(t, f)
	m.cfg.UI.EditorInline = false
	m = selectRow(t, open(t, m, "helm"), "web")

	pre := pressSettle(t, m, "enter")
	if pre.stage != "yaml" || !strings.Contains(pre.logText, "# user-supplied values of shop/web") || *opened != "" {
		t.Fatalf("enter: stage %s opened %q\n%s", pre.stage, *opened, pre.logText)
	}

	next, cmd := m.Update(key(t, "v"))
	m = next.(model)
	if *opened == "" || cmd == nil || m.stage != "helm" {
		t.Fatalf("v: opened %q cmd %v stage %s", *opened, cmd != nil, m.stage)
	}
	raw, err := os.ReadFile(*opened)
	if err != nil || !strings.Contains(string(raw), "replicas: 3") {
		t.Fatalf("v file %q: %v", raw, err)
	}
	if err := os.WriteFile(*opened, []byte("replicas: 8\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r, _ := m.selectedRelease()
	msg := helmValuesEditResult(*opened, string(raw), r, "nginx", "1.2.0", nil)
	next, cmd = m.Update(msg)
	m = settle(t, next.(model), cmd)
	if m.dlg == nil || !strings.Contains(m.dlg.ask, "upgrade web in shop?") {
		t.Fatalf("confirm %+v", m.dlg)
	}
	m = pressSettle(t, m, "n")
	if len(helmVerbCalls(f, "upgrade")) != 0 {
		t.Fatalf("n must not upgrade: %v", helmVerbCalls(f, "upgrade"))
	}
	next, cmd = m.Update(msg)
	m = settle(t, next.(model), cmd)
	m = pressSettle(t, m, "y")
	got := helmVerbCalls(f, "upgrade")
	if len(got) != 1 || !strings.Contains(got[0], "upgrade web") || !strings.Contains(got[0], "-f ") {
		t.Fatalf("upgrade %v", got)
	}
}

func TestHelmVBlockedReadOnlyAndEStillWorks(t *testing.T) {
	withHelmCLI(t)
	opened := stubVim(t)
	m := appModel(t, helmInstallFake(t, ""))
	m.cfg.UI.EditorInline = false
	m = selectRow(t, open(t, m, "helm"), "web")
	m.readOnly = true
	got := press(t, m, "v")
	if *opened != "" || !strings.Contains(got.status, "read-only") {
		t.Fatalf("read-only v: opened %q status %q", *opened, got.status)
	}
	m.readOnly = false
	press(t, m, "e")
	if *opened == "" {
		t.Fatal("e should still edit")
	}
}

func TestHelmVPreviewsWithoutCLI(t *testing.T) {
	lookPath = func(string) (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { lookPath = defaultLookPath })
	m := selectRow(t, open(t, appModel(t, appCluster(t)), "helm"), "web")
	m = pressSettle(t, m, "v")
	if m.stage != "yaml" || !strings.Contains(m.logText, "# user-supplied values of shop/web") {
		t.Fatalf("stage %s\n%s", m.stage, m.logText)
	}
}

func TestHelmValuesKeyRebindable(t *testing.T) {
	withHelmCLI(t)
	opened := stubVim(t)
	m := appModel(t, helmInstallFake(t, ""))
	m.cfg.UI.EditorInline = false
	m.cfg.Keys.HelmValues = []string{"x"}
	m = selectRow(t, open(t, m, "helm"), "web")
	press(t, m, "x")
	if *opened == "" {
		t.Fatal("rebound key did not edit")
	}
	if l := ansi.Strip(m.legend()); !strings.Contains(l, "x edit values") {
		t.Fatalf("legend %q", l)
	}
}

func chartSecrets(name, ns string, values map[string]any) string {
	r := map[string]any{"name": name, "namespace": ns, "version": 1,
		"info":     map[string]any{"status": "deployed", "last_deployed": "2026-10-21T08:00:00Z"},
		"chart":    map[string]any{"metadata": map[string]any{"name": "chart", "version": "1.0.0"}},
		"config":   values,
		"manifest": ""}
	return fmt.Sprintf(`{"kind":"List","items":[{"kind":"Secret","metadata":{"name":"sh.helm.release.v1.%s.v1","namespace":%q,"labels":{"owner":"helm","name":%q,"version":"1","status":"deployed"}},"data":{"release":%q}}]}`,
		name, ns, name, helmPayload(r))
}

type chartsRig struct {
	m      model
	f      *shell.Fake
	opened *string
}

func newChartsRig(t *testing.T, deployed string) chartsRig {
	t.Helper()
	withKindCLIs(t)
	opened := stubVim(t)
	base := kindFake(t, []string{"lab"})
	parent := base.Handler
	secrets := chartSecrets("kafbat-ui", "cluster1", map[string]any{"replicas": 2})
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		line := strings.Join(r.Args, " ")
		if r.Name == "helm" && strings.HasPrefix(line, "get values ") {
			return shell.OK(deployed)
		}
		if r.Name == "kubectl" && strings.HasPrefix(line, "get secrets") && strings.Contains(line, "owner=helm") {
			return shell.OK(secrets)
		}
		return parent(r)
	}}
	m := appModel(t, f)
	home := os.Getenv("HOME")
	if err := os.MkdirAll(filepath.Join(home, ".kube"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(kube.KindKubeconfig("lab", home), []byte("apiVersion: v1\nkind: Config\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m = open(t, m, "kind")
	m = selectKindTile(t, m, "lab")
	m = pressSettle(t, m, "e")
	if m.dlg == nil || m.dlg.act != "kindcharts" {
		t.Fatalf("charts form %+v status %q", m.dlg, m.status)
	}
	return chartsRig{m: m, f: f, opened: opened}
}

func (r *chartsRig) editValues(t *testing.T, chart, content string) {
	t.Helper()
	*r.opened = ""
	r.m = focusKindField(t, r.m, "c:"+chart)
	r.m = pressSettle(t, r.m, "v")
	if *r.opened == "" {
		t.Fatal("editor was not opened")
	}
	if content != "" {
		if err := os.WriteFile(*r.opened, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	next, cmd := r.m.Update(kindValuesDoneMsg{})
	r.m = settle(t, next.(model), cmd)
}

func chartRow(m model, chart string) string {
	re := regexp.MustCompile(regexp.QuoteMeta(chart) + `(?: \(values\))?\s+(?:yes|no|upgrade)\b`)
	for _, line := range strings.Split(ansi.Strip(m.View()), "\n") {
		if loc := re.FindString(line); loc != "" {
			return loc
		}
	}
	return ""
}

const kafbatDeployed = "replicas: 2\nresources:\n  limits:\n    cpu: 100m\n"

func TestKindChartsVPrefillsDeployedValues(t *testing.T) {
	r := newChartsRig(t, kafbatDeployed)
	if row := chartRow(r.m, "kafbat"); !strings.Contains(row, "yes") || strings.Contains(row, "upgrade") {
		t.Fatalf("row before: %q", row)
	}
	r.m = focusKindField(t, r.m, "c:kafbat")
	r.m = pressSettle(t, r.m, "v")
	if *r.opened == "" {
		t.Fatal("editor was not opened")
	}
	raw, err := os.ReadFile(*r.opened)
	if err != nil || string(raw) != kafbatDeployed {
		t.Fatalf("prefill %q err %v", raw, err)
	}
	var got string
	for _, c := range helmCalls(r.f) {
		if strings.HasPrefix(c, "get values ") {
			got = c
		}
	}
	for _, want := range []string{"get values kafbat-ui", "-o yaml", "-n cluster1", "--kube-context kind-lab", "--kubeconfig " + kube.KindKubeconfig("lab", os.Getenv("HOME"))} {
		if !strings.Contains(got, want) {
			t.Fatalf("get values call %q lacks %q", got, want)
		}
	}
}

func TestKindChartsEmptyDeployedValuesAndNotInstalled(t *testing.T) {
	r := newChartsRig(t, "null\n")
	r.m = focusKindField(t, r.m, "c:kafbat")
	r.m = pressSettle(t, r.m, "v")
	raw, err := os.ReadFile(*r.opened)
	if err != nil || string(raw) != "{}\n" {
		t.Fatalf("empty prefill %q err %v", raw, err)
	}
	before := len(helmVerbCalls(r.f, "get"))
	r.editValues(t, "metallb", "")
	if len(helmVerbCalls(r.f, "get")) != before {
		t.Fatal("not-installed chart must not read deployed values")
	}
	raw, err = os.ReadFile(*r.opened)
	if err != nil || strings.Contains(string(raw), "replicas") {
		t.Fatalf("not-installed seed %q err %v", raw, err)
	}
}

func TestKindChartsUpgradeMarkOnlyOnRealChange(t *testing.T) {
	r := newChartsRig(t, kafbatDeployed)

	r.editValues(t, "kafbat", "")
	if row := chartRow(r.m, "kafbat"); !strings.Contains(row, "yes") || strings.Contains(row, "upgrade") || strings.Contains(row, "(values)") {
		t.Fatalf("no-op edit marked: %q", row)
	}

	r.editValues(t, "kafbat", "resources:\n    limits:\n        cpu: 100m\n# note\nreplicas: 2\n")
	if row := chartRow(r.m, "kafbat"); strings.Contains(row, "upgrade") {
		t.Fatalf("whitespace-only edit marked: %q", row)
	}

	r.editValues(t, "kafbat", "replicas: 5\n")
	row := chartRow(r.m, "kafbat")
	if !strings.Contains(row, "kafbat (values)") || !strings.Contains(row, "upgrade") || strings.Contains(row, "yes") {
		t.Fatalf("real change row: %q", row)
	}
	if !strings.Contains(r.m.View(), warnStyle.Render("upgrade")) {
		t.Fatalf("upgrade not in warn color")
	}

	r.editValues(t, "kafbat", kafbatDeployed)
	if row := chartRow(r.m, "kafbat"); strings.Contains(row, "upgrade") || strings.Contains(row, "(values)") || !strings.Contains(row, "yes") {
		t.Fatalf("revert did not clear: %q", row)
	}
	next, cmd := r.m.submitKindCharts()
	m := settle(t, next.(model), cmd)
	if m.asking() != "" || !strings.Contains(m.status, "no chart changes") {
		t.Fatalf("reverted should be a no-op: ask %q status %q", m.asking(), m.status)
	}
}

func TestKindChartsEnterUpgradesEditedValues(t *testing.T) {
	r := newChartsRig(t, kafbatDeployed)
	r.editValues(t, "kafbat", "replicas: 5\n")
	edited := *r.opened
	next, cmd := r.m.submitKindCharts()
	m := settle(t, next.(model), cmd)
	view := ansi.Strip(m.View())
	if m.dlg == nil || !strings.Contains(view, kindDetail("upgrade", "kafbat → cluster1 (values)")) {
		t.Fatalf("confirm:\n%s", view)
	}
	if strings.Contains(view, kindDetail("install", "")) || strings.Contains(view, kindDetail("uninstall", "")) {
		t.Fatalf("only an upgrade expected:\n%s", view)
	}
	m = pressSettle(t, m, "y")
	ups := helmVerbCalls(r.f, "upgrade")
	if len(ups) != 1 || !strings.HasPrefix(ups[0], "upgrade kafbat-ui ") || !strings.Contains(ups[0], "-n cluster1") || !strings.Contains(ups[0], "-f "+edited) {
		t.Fatalf("upgrade calls %v", ups)
	}
	if strings.Count(ups[0], " -f ") != 1 {
		t.Fatalf("edited file must be the only values file: %q", ups[0])
	}
	if len(helmVerbCalls(r.f, "install")) != 0 {
		t.Fatalf("unexpected install %v", helmVerbCalls(r.f, "install"))
	}
}

func TestKindValuesEqualIsSemantic(t *testing.T) {
	cases := []struct {
		a, b string
		diff bool
	}{
		{"a: 1\nb: [1, 2]\n", "b:\n  - 1\n  - 2\n# c\na: 1\n", false},
		{"{}\n", "# only a comment\n", false},
		{"null\n", "{}", false},
		{"a: 1\n", "a: 2\n", true},
		{"a: 1\n", "a: 1\nb: 2\n", true},
		{"a: \"1\"\n", "a: 1\n", true},
		{"a: 1\n", ": : :\n", true},
	}
	for _, c := range cases {
		if got := kindValuesDiffer(c.a, c.b); got != c.diff {
			t.Errorf("differ(%q, %q) = %v want %v", c.a, c.b, got, c.diff)
		}
	}
}

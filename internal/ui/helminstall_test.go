package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"k2/internal/shell"

	"github.com/charmbracelet/x/ansi"
)

func withHelmCLI(t *testing.T) {
	t.Helper()
	lookPath = func(string) (string, error) { return "/usr/bin/helm", nil }
	t.Cleanup(func() { lookPath = defaultLookPath })
}

const searchOut = `NAME                   	CHART VERSION	APP VERSION	DESCRIPTION
kafbat-ui/kafka-ui     	1.4.0        	v1.2.0     	A UI for Apache Kafka
bitnami/nginx          	18.2.0       	1.27.0     	nginx
`

const searchVers = `NAME                   	CHART VERSION	APP VERSION	DESCRIPTION
kafbat-ui/kafka-ui     	1.4.0        	v1.2.0     	A UI for Apache Kafka
kafbat-ui/kafka-ui     	1.3.1        	v1.1.0     	A UI for Apache Kafka
`

const kafkaManifest = `---
apiVersion: v1
kind: Service
metadata:
  name: kafbat-ui
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kafbat-ui
`

func helmInstallFake(t *testing.T, secrets string) *shell.Fake {
	t.Helper()
	base := appCluster(t)
	var mu sync.Mutex
	cur := secrets
	if cur == "" {
		cur = appHelm()
	}
	parent := base.Handler
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		if r.Name == "helm" {
			line := strings.Join(r.Args, " ")
			switch {
			case strings.HasPrefix(line, "search repo kafbat-ui/kafka-ui") && strings.Contains(line, "--versions"):
				return shell.OK(searchVers)
			case strings.HasPrefix(line, "search repo"):
				return shell.OK(searchOut)
			case strings.HasPrefix(line, "show values"):
				return shell.OK("replicaCount: 1\nservice:\n  type: ClusterIP\n")
			case strings.HasPrefix(line, "template "):
				return shell.OK(kafkaManifest)
			case strings.HasPrefix(line, "install "):
				if strings.Contains(line, "already-there") {
					return shell.Fail(1, "Error: cannot re-use a name that is still in use")
				}
				return shell.OK("NAME: kafbat-ui\nNAMESPACE: kafka\nSTATUS: deployed\nNOTES:\nGet the UI:\n  kubectl -n kafka port-forward svc/kafbat-ui 8080:80\n")
			case strings.HasPrefix(line, "upgrade "):
				return shell.OK("Release \"web\" has been upgraded. Happy Helming!\nNOTES:\nupgraded notes\n")
			default:
				return shell.OK("ok")
			}
		}
		line := strings.Join(r.Args, " ")
		if strings.HasPrefix(line, "get secrets") && strings.Contains(line, "owner=helm") {
			mu.Lock()
			out := cur
			mu.Unlock()
			return shell.OK(out)
		}
		return parent(r)
	}}
}

func TestHelmInstallFormAndPreview(t *testing.T) {
	withHelmCLI(t)
	f := helmInstallFake(t, "")
	m := appModel(t, f)
	m = open(t, m, "helm")
	m = pressSettle(t, m, "A")
	if m.dlg == nil || m.dlg.act != "helminstall" {
		t.Fatalf("form %+v", m.dlg)
	}
	view := ansi.Strip(m.View())
	for _, want := range []string{"helm install", "release", "chart", "version", "namespace", "create ns", "values", "shop"} {
		if !strings.Contains(view, want) {
			t.Fatalf("form lacks %q:\n%s", want, view)
		}
	}
	if strings.Count(view, "enter preview") != 1 {
		t.Fatalf("hint should appear once:\n%s", view)
	}
	if strings.Contains(view, "enter previews, or opens edit") {
		t.Fatalf("long hint still shown:\n%s", view)
	}
	setHelmField(m.dlg, "name", "kafbat-ui")
	setHelmField(m.dlg, "chart", "kafbat-ui/kafka-ui")
	setHelmField(m.dlg, "version", "1.4.0")
	setHelmField(m.dlg, "ns", "kafka")
	m.dlg.hcreate = true
	m = settle(t, m, nil)
	next, cmd := m.submitHelmForm()
	m = settle(t, next.(model), cmd)
	if m.stage != "htpl" || !strings.Contains(m.logText, "Service ×1") || !strings.Contains(m.logText, "kind: Deployment") {
		t.Fatalf("preview stage %s text:\n%s", m.stage, m.logText)
	}
	m = pressSettle(t, m, "enter")
	if m.dlg == nil || !m.dlg.danger || !strings.Contains(m.dlg.ask, "install kafbat-ui into kafka?") {
		t.Fatalf("confirm %+v", m.dlg)
	}
	view = ansi.Strip(m.View())
	for _, want := range []string{"helm install", "release kafka/kafbat-ui", "chart kafbat-ui/kafka-ui@1.4.0", "kind-dev", "default no"} {
		if !strings.Contains(view, want) {
			t.Fatalf("confirm lacks %q:\n%s", want, view)
		}
	}
	m = pressSettle(t, m, "enter")
	before := helmVerbCalls(f, "install")
	if len(before) != 0 {
		t.Fatalf("enter installed: %v", before)
	}
	m = pressSettle(t, m, "y")
	got := helmVerbCalls(f, "install")
	if len(got) != 1 || !strings.Contains(got[0], "install kafbat-ui kafbat-ui/kafka-ui -n kafka") || !strings.Contains(got[0], "--create-namespace") || !strings.Contains(got[0], "--version 1.4.0") || !strings.Contains(got[0], "--kube-context kind-dev") {
		t.Fatalf("install args %v", got)
	}
	if strings.Contains(got[0], "--wait") {
		t.Fatal("unexpected --wait")
	}
	if !strings.Contains(m.status, "installed kafbat-ui") || !strings.Contains(m.logText, "port-forward") {
		t.Fatalf("after install stage %s status %q text %s", m.stage, m.status, m.logText)
	}
	if m.namespace != "kafka" {
		t.Fatalf("namespace after install: %s", m.namespace)
	}
}

func TestHelmInstallCommandPrefill(t *testing.T) {
	withHelmCLI(t)
	dir := t.TempDir()
	values := filepath.Join(dir, "values.yaml")
	if err := os.WriteFile(values, []byte("replicaCount: 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := helmInstallFake(t, "")
	m := appModel(t, f)
	next, cmd := m.runCommand("helm install kafbat-ui kafbat-ui/kafka-ui -n kafka --create-namespace -f " + values + " --version 1.4.0")
	m = settle(t, next.(model), cmd)
	if m.dlg == nil || m.dlg.act != "helminstall" {
		t.Fatalf("form %+v status %q", m.dlg, m.status)
	}
	if helmFieldValue(m.dlg, "name") != "kafbat-ui" || helmFieldValue(m.dlg, "chart") != "kafbat-ui/kafka-ui" || helmFieldValue(m.dlg, "version") != "1.4.0" || helmFieldValue(m.dlg, "ns") != "kafka" || !m.dlg.hcreate || helmFieldValue(m.dlg, "values") != values {
		t.Fatalf("prefill name=%s chart=%s ver=%s ns=%s create=%v values=%s", helmFieldValue(m.dlg, "name"), helmFieldValue(m.dlg, "chart"), helmFieldValue(m.dlg, "version"), helmFieldValue(m.dlg, "ns"), m.dlg.hcreate, helmFieldValue(m.dlg, "values"))
	}
}

func TestHelmUpgradePreviewAndConfirm(t *testing.T) {
	withHelmCLI(t)
	f := helmInstallFake(t, "")
	m := appModel(t, f)
	m.cfg.Prod = []string{"kind-dev"}
	m = selectRow(t, open(t, m, "helm"), "web")
	m = pressSettle(t, m, "u")
	if m.dlg == nil || m.dlg.act != "helminstall" || !strings.Contains(m.dlg.title, "upgrade") {
		t.Fatalf("upgrade form %+v", m.dlg)
	}
	if helmFieldValue(m.dlg, "name") != "web" || helmFieldValue(m.dlg, "ns") != "shop" {
		t.Fatalf("locked fields %s %s", helmFieldValue(m.dlg, "name"), helmFieldValue(m.dlg, "ns"))
	}
	for _, f := range m.dlg.hfields {
		if (f.key == "name" || f.key == "chart" || f.key == "ns") && !f.locked {
			t.Fatalf("%s not locked", f.key)
		}
		if f.key == "create" {
			t.Fatal("create-namespace on upgrade")
		}
	}
	if helmFieldValue(m.dlg, "values") == "" {
		t.Fatal("values not prefilled")
	}
	next, cmd := m.submitHelmForm()
	m = settle(t, next.(model), cmd)
	if m.stage != "htpl" || !strings.Contains(m.logText, "upgrade web") {
		t.Fatalf("upgrade preview %s:\n%s", m.stage, m.logText)
	}
	preview := ansi.Strip(m.View())
	if !strings.Contains(preview, "enter confirm") || !strings.Contains(preview, "esc back") {
		t.Fatalf("htpl hint missing confirm/back:\n%s", preview)
	}
	m = pressSettle(t, m, "enter")
	if m.dlg == nil || !strings.Contains(m.dlg.ask, "upgrade web in shop?") {
		t.Fatalf("confirm %+v", m.dlg)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, skull) || !strings.Contains(view, "production") {
		t.Fatalf("prod confirm:\n%s", view)
	}
	m = pressSettle(t, m, "y")
	got := helmVerbCalls(f, "upgrade")
	if len(got) != 1 || !strings.Contains(got[0], "upgrade web") || !strings.Contains(got[0], "-n shop") || strings.Contains(got[0], "--create-namespace") || strings.Contains(got[0], "--wait") {
		t.Fatalf("upgrade args %v", got)
	}
}

func TestHelmInstallReadOnlyBlocks(t *testing.T) {
	withHelmCLI(t)
	f := helmInstallFake(t, "")
	m := appModel(t, f)
	m = open(t, m, "helm")
	m.readOnly = true
	if got := press(t, m, "A"); got.dlg != nil || !strings.Contains(got.status, "read-only") {
		t.Fatalf("install key: dlg %v status %q", got.dlg != nil, got.status)
	}
	m = selectRow(t, m, "web")
	if got := press(t, m, "u"); got.dlg != nil || !strings.Contains(got.status, "read-only") {
		t.Fatalf("upgrade key: dlg %v status %q", got.dlg != nil, got.status)
	}
	next, cmd := m.runCommand("helm install kafbat-ui kafbat-ui/kafka-ui -n kafka")
	got := settle(t, next.(model), cmd)
	if got.dlg != nil || !strings.Contains(got.status, "read-only") {
		t.Fatalf("cmdline: dlg %v status %q", got.dlg != nil, got.status)
	}
	m.readOnly = false
	m = pressSettle(t, m, "A")
	setHelmField(m.dlg, "name", "kafbat-ui")
	setHelmField(m.dlg, "chart", "kafbat-ui/kafka-ui")
	next, cmd = m.submitHelmForm()
	m = settle(t, next.(model), cmd)
	m = pressSettle(t, m, "enter")
	if m.dlg == nil || m.dlg.ask == "" {
		t.Fatal("expected confirm")
	}
	m.readOnly = true
	got = press(t, m, "y")
	if len(helmVerbCalls(f, "install")) != 0 || !strings.Contains(got.status, "read-only") {
		t.Fatalf("confirm yes: calls %v status %q", helmVerbCalls(f, "install"), got.status)
	}
}

func TestHelmInstallNeedsCLI(t *testing.T) {
	lookPath = func(string) (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { lookPath = defaultLookPath })
	m := open(t, appModel(t, appCluster(t)), "helm")
	m = pressSettle(t, m, "A")
	if m.dlg != nil || !strings.Contains(m.status, "needs the helm CLI") {
		t.Fatalf("status %q dlg %v", m.status, m.dlg != nil)
	}
}

func TestHelmInstallErrorPopup(t *testing.T) {
	withHelmCLI(t)
	f := helmInstallFake(t, "")
	m := appModel(t, f)
	m = open(t, m, "helm")
	m = pressSettle(t, m, "A")
	setHelmField(m.dlg, "name", "already-there")
	setHelmField(m.dlg, "chart", "kafbat-ui/kafka-ui")
	next, cmd := m.submitHelmForm()
	m = settle(t, next.(model), cmd)
	m = pressSettle(t, m, "enter", "y")
	if m.dlg == nil || m.dlg.act != "error" || !strings.Contains(strings.Join(m.dlg.details, "\n"), "cannot re-use a name") {
		t.Fatalf("error popup %+v last %q", m.dlg, m.lastErr.text)
	}
}

func TestParseHelmCmd(t *testing.T) {
	op, ok, err := parseHelmCmd("install kafbat-ui kafbat-ui/kafka-ui -n kafka --create-namespace -f values.yaml --version 1.4.0")
	if !ok || err != "" || op.name != "kafbat-ui" || op.chart != "kafbat-ui/kafka-ui" || op.ns != "kafka" || !op.createNS || op.values != "values.yaml" || op.version != "1.4.0" {
		t.Fatalf("%+v ok=%v err=%q", op, ok, err)
	}
	if _, ok, _ := parseHelmCmd("all"); ok {
		t.Fatal("all is not install")
	}
}

func helmVerbCalls(f *shell.Fake, verb string) []string {
	var out []string
	for _, c := range helmCalls(f) {
		if strings.HasPrefix(c, verb+" ") {
			out = append(out, c)
		}
	}
	return out
}

func TestHelmUpgradeValuesPathEllipsis(t *testing.T) {
	withHelmCLI(t)
	f := helmInstallFake(t, "")
	m := appModel(t, f)
	m = selectRow(t, open(t, m, "helm"), "web")
	m = pressSettle(t, m, "u")
	if m.dlg == nil {
		t.Fatal("no form")
	}
	long := "/tmp/k2-helm-54192100-very-long-directory-name/kafbat-ui-values.yaml"
	setHelmField(m.dlg, "values", long)
	view := ansi.Strip(m.View())
	if strings.Contains(view, long) {
		t.Fatalf("full path shown:\n%s", view)
	}
	if !strings.Contains(view, "…") || !strings.Contains(view, "values.yaml") {
		t.Fatalf("expected mid ellipsis:\n%s", view)
	}
	if strings.Count(view, "enter preview") != 1 {
		t.Fatalf("hint should appear once:\n%s", view)
	}
}

func TestHelmFormWalksEveryField(t *testing.T) {
	withHelmCLI(t)
	f := helmInstallFake(t, "")
	m := appModel(t, f)
	m = open(t, m, "helm")
	m = pressSettle(t, m, "A")
	walkHelmFields(t, m, "install")

	m = appModel(t, f)
	m = selectRow(t, open(t, m, "helm"), "web")
	m = pressSettle(t, m, "u")
	walkHelmFields(t, m, "upgrade")
}

func walkHelmFields(t *testing.T, m model, kind string) {
	t.Helper()
	if m.dlg == nil || m.dlg.act != "helminstall" || len(m.dlg.hfields) == 0 {
		t.Fatalf("%s: no form %+v", kind, m.dlg)
	}
	n := len(m.dlg.hfields)
	start := m.dlg.idx
	unlocked := 0
	for _, f := range m.dlg.hfields {
		if !f.locked {
			unlocked++
		}
		if f.input.Prompt == "" {
			t.Fatalf("%s field %s: input not created with textinput.New", kind, f.key)
		}
	}
	if unlocked == 0 {
		t.Fatalf("%s: no movable fields", kind)
	}
	for _, key := range []string{"tab", "shift+tab", "down", "up"} {
		m.dlg.idx = start
		m = m.moveHelmField(0)
		seen := map[int]bool{m.dlg.idx: true}
		for i := 0; i < n*2+2; i++ {
			m = pressSettle(t, m, key)
			if m.dlg == nil {
				t.Fatalf("%s %s: form closed at step %d", kind, key, i)
			}
			if m.dlg.idx < 0 || m.dlg.idx >= n {
				t.Fatalf("%s %s: idx %d out of %d", kind, key, m.dlg.idx, n)
			}
			if m.dlg.hfields[m.dlg.idx].locked {
				t.Fatalf("%s %s landed on locked %s", kind, key, m.dlg.hfields[m.dlg.idx].key)
			}
			seen[m.dlg.idx] = true
			_ = m.View()
		}
		if key == "tab" || key == "shift+tab" {
			continue
		}
		for i, f := range m.dlg.hfields {
			if f.locked {
				continue
			}
			if !seen[i] {
				t.Fatalf("%s %s never reached %s", kind, key, f.key)
			}
		}
	}
	last := m.dlg.hfields[n-1].key
	if last != "defaults" {
		t.Fatalf("%s last field %s, want defaults", kind, last)
	}
}

func TestHelmFormTypesJAndK(t *testing.T) {
	withHelmCLI(t)
	f := helmInstallFake(t, "")
	m := appModel(t, f)
	m = pressSettle(t, open(t, m, "helm"), "A")
	if m.dlg == nil {
		t.Fatal("no form")
	}
	cases := []struct {
		key, typed string
	}{
		{"name", "kafbat"},
		{"chart", "kube-prometheus-stack"},
		{"version", "1.2.3-jk"},
		{"ns", "jetstack"},
		{"values", "values-jk.yaml"},
	}
	for _, tc := range cases {
		for i, f := range m.dlg.hfields {
			if f.key == tc.key {
				m.dlg.idx = i
				break
			}
		}
		m = m.moveHelmField(0)
		start := m.dlg.idx
		setHelmField(m.dlg, tc.key, "")
		for _, r := range tc.typed {
			m = pressSettle(t, m, string(r))
			if m.dlg == nil {
				t.Fatalf("%s: form closed while typing %q", tc.key, tc.typed)
			}
			if m.dlg.idx != start || m.dlg.hfields[m.dlg.idx].key != tc.key {
				t.Fatalf("typing %q into %s moved to %s", string(r), tc.key, m.dlg.hfields[m.dlg.idx].key)
			}
		}
		if got := helmFieldValue(m.dlg, tc.key); got != tc.typed {
			t.Fatalf("%s: got %q want %q", tc.key, got, tc.typed)
		}
	}
	for _, f := range m.dlg.hfields {
		if !f.toggle && !f.action {
			continue
		}
		for i, hf := range m.dlg.hfields {
			if hf.key == f.key {
				m.dlg.idx = i
				break
			}
		}
		m = m.moveHelmField(0)
		start := m.dlg.idx
		m = pressSettle(t, m, "j")
		if m.dlg == nil || m.dlg.idx != start {
			t.Fatalf("j on %s moved the field (idx %d -> %v)", f.key, start, m.dlg)
		}
		m = pressSettle(t, m, "k")
		if m.dlg == nil || m.dlg.idx != start {
			t.Fatalf("k on %s moved the field (idx %d -> %v)", f.key, start, m.dlg)
		}
	}
}

func TestHelmUpgradeFocusedValuesPathDoesNotWrap(t *testing.T) {
	withHelmCLI(t)
	f := helmInstallFake(t, "")
	m := appModel(t, f)
	m = selectRow(t, open(t, m, "helm"), "web")
	m = pressSettle(t, m, "u")
	if m.dlg == nil {
		t.Fatal("no form")
	}
	long := "/tmp/k2-helm-2583312956/kafbat-ui-values.yaml"
	setHelmField(m.dlg, "values", long)
	for i, f := range m.dlg.hfields {
		if f.key == "values" {
			m = m.moveHelmField(i - m.dlg.idx)
			break
		}
	}
	if m.dlg.hfields[m.dlg.idx].key != "values" {
		t.Fatalf("focused %s", m.dlg.hfields[m.dlg.idx].key)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "ctrl+e") || !strings.Contains(view, "edit") {
		t.Fatalf("focused values footer missing ctrl+e edit:\n%s", view)
	}
	for _, line := range strings.Split(view, "\n") {
		trim := strings.TrimSpace(line)
		if trim == "yaml" || strings.HasPrefix(trim, "yaml ") {
			t.Fatalf("path wrapped onto column 0:\n%s", view)
		}
	}
	if !strings.Contains(view, "values") || !(strings.Contains(view, "kafbat-ui-values.yaml") || strings.Contains(view, "values.yaml") || strings.Contains(view, "…")) {
		t.Fatalf("focused values path missing:\n%s", view)
	}
}

func focusHelmField(t *testing.T, m model, key string) model {
	t.Helper()
	if m.dlg == nil {
		t.Fatal("no form")
	}
	for i, f := range m.dlg.hfields {
		if f.key == key {
			return m.moveHelmField(i - m.dlg.idx)
		}
	}
	t.Fatalf("no field %s", key)
	return m
}

func TestHelmFormCtrlEEditsValues(t *testing.T) {
	withHelmCLI(t)
	f := helmInstallFake(t, "")
	m := appModel(t, f)
	m.cfg.UI.EditorInline = false
	m = selectRow(t, open(t, m, "helm"), "web")
	m = pressSettle(t, m, "u")
	m = focusHelmField(t, m, "values")
	orig := helmFieldValue(m.dlg, "values")
	if orig == "" {
		t.Fatal("values not prefilled")
	}
	before, err := os.ReadFile(orig)
	if err != nil {
		t.Fatal(err)
	}
	var opened string
	old := vimCommand
	vimCommand = func(path string) *exec.Cmd {
		opened = path
		return exec.Command("true")
	}
	t.Cleanup(func() { vimCommand = old })
	next, cmd := m.Update(key(t, "ctrl+e"))
	m = next.(model)
	if opened == "" || cmd == nil {
		t.Fatalf("ctrl+e: opened %q cmd %v", opened, cmd != nil)
	}
	if opened != orig {
		t.Fatalf("ctrl+e opened %q, want existing values %q", opened, orig)
	}
	after, err := os.ReadFile(orig)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("ctrl+e changed the values file:\n%s", after)
	}
}

func TestHelmChartDefaultsPreviewDoesNotClobberValues(t *testing.T) {
	withHelmCLI(t)
	f := helmInstallFake(t, "")
	m := appModel(t, f)
	m = selectRow(t, open(t, m, "helm"), "web")
	m = pressSettle(t, m, "u")
	orig := helmFieldValue(m.dlg, "values")
	if orig == "" {
		t.Fatal("values not prefilled")
	}
	before, err := os.ReadFile(orig)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(before), "replicas: 3") {
		t.Fatalf("user values:\n%s", before)
	}
	m.readOnly = true
	m = focusHelmField(t, m, "defaults")
	next, cmd := m.submitHelmForm()
	m = settle(t, next.(model), cmd)
	if m.stage != "yaml" || !strings.Contains(m.logText, "chart defaults") || !strings.Contains(m.logText, "replicaCount: 1") {
		t.Fatalf("defaults preview %s:\n%s", m.stage, m.logText)
	}
	if m.dlg != nil {
		t.Fatalf("dialog still open %+v", m.dlg)
	}
	if m.vs.hop == nil || m.vs.hop.values != orig {
		t.Fatalf("hop.values clobbered: %+v", m.vs.hop)
	}
	after, err := os.ReadFile(orig)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("values file overwritten:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if strings.Contains(string(after), "replicaCount:") {
		t.Fatal("chart defaults written into the values file")
	}
	m = pressSettle(t, m, "esc")
	if m.dlg == nil || m.dlg.act != "helminstall" {
		t.Fatalf("back to form: %+v stage %s", m.dlg, m.stage)
	}
	if helmFieldValue(m.dlg, "values") != orig || m.vs.hop.values != orig {
		t.Fatalf("values path after back: field %q hop %q", helmFieldValue(m.dlg, "values"), m.vs.hop.values)
	}
	got, err := os.ReadFile(orig)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(before) {
		t.Fatalf("values file changed after back:\n%s", got)
	}
}

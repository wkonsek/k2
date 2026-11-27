package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k2/internal/config"
	"k2/internal/kube"
	"k2/internal/shell"
	"k2/internal/state"

	"github.com/charmbracelet/x/ansi"
)

func writeMasterKey(t *testing.T, dir, name string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	body := "apiVersion: v1\nkind: Secret\nmetadata:\n  name: sealed-secrets-key\n  namespace: kube-system\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func withSealedMasterKey(t *testing.T, m *model, path string) {
	t.Helper()
	m.cfg.Helm.Charts = map[string]config.Chart{
		"sealed-secrets": {MasterKeyPath: path},
	}
}

func TestKindMasterKeyPickerAndConfirm(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, nil)
	m := appModel(t, f)
	home := os.Getenv("HOME")
	path := writeMasterKey(t, filepath.Join(home, "secrets"), "master.yml")
	m = pressSettle(t, open(t, m, "kind"), "A")
	m = focusKindField(t, m, "name")
	m = pressSettle(t, m, "m")
	if m.dlg == nil || m.dlg.act != "kindcreate" {
		t.Fatalf("m in name opened %v", m.dlg)
	}
	if !strings.HasSuffix(helmFieldValue(m.dlg, "name"), "m") {
		t.Fatalf("m should type into name: %q", helmFieldValue(m.dlg, "name"))
	}
	m = focusKindField(t, m, "c:metallb")
	m = pressSettle(t, m, "m")
	if m.dlg == nil || m.dlg.act != "kindcreate" {
		t.Fatalf("m on metallb opened %v", m.dlg)
	}
	view := ansi.Strip(m.View())
	if strings.Contains(view, "m master key") {
		t.Fatalf("metallb footer should not mention master key:\n%s", view)
	}
	m = focusKindField(t, m, "c:sealed-secrets")
	view = ansi.Strip(m.View())
	if !strings.Contains(view, "m") || !strings.Contains(view, "master key") {
		t.Fatalf("sealed-secrets footer:\n%s", view)
	}
	m = pressSettle(t, m, "m")
	if m.dlg == nil || m.dlg.act != "fpath" {
		t.Fatalf("m should open yaml picker: %+v", m.dlg)
	}
	m = pressSettle(t, m, "esc")
	if m.dlg == nil || m.dlg.act != "kindcreate" {
		t.Fatalf("esc should return to wizard: %+v", m.dlg)
	}
	m = focusKindField(t, m, "c:sealed-secrets")
	m = pressSettle(t, m, "m")
	m = pressSettle(t, m, "ctrl+u")
	m = typeField(t, m, path)
	m = pressSettle(t, m, "enter")
	if m.dlg == nil || m.dlg.act != "kindcreate" {
		t.Fatalf("pick should return to wizard: %+v", m.dlg)
	}
	if m.vs.kop == nil || m.kindSealedMasterKey(m.vs.kop) == "" {
		t.Fatalf("path not stored: %+v", m.vs.kop)
	}
	var on bool
	var note string
	for _, f := range m.dlg.hfields {
		if f.key == "c:sealed-secrets" {
			on, note = f.on, f.note
		}
	}
	if !on || !strings.Contains(note, "master.yml") {
		t.Fatalf("row on=%v note=%q", on, note)
	}
	view = ansi.Strip(m.View())
	if !strings.Contains(view, "yes · ") || !strings.Contains(view, "master.yml") {
		t.Fatalf("row should show path:\n%s", view)
	}
	next, cmd := m.submitKindForm()
	m = settle(t, next.(model), cmd)
	view = ansi.Strip(m.View())
	if m.dlg == nil || !strings.Contains(view, kindDetail("master key", config.HomePath(path))) {
		t.Fatalf("confirm lacks master key:\n%s", view)
	}
}

func TestKindMasterKeyApplyBeforeHelmInstall(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, nil)
	m := appModel(t, f)
	home := os.Getenv("HOME")
	path := writeMasterKey(t, filepath.Join(home, "secrets"), "master.yml")
	withSealedMasterKey(t, &m, "~/secrets/master.yml")
	m = pressSettle(t, open(t, m, "kind"), "A")
	m = focusKindField(t, m, "c:sealed-secrets")
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "master.yml") || !strings.Contains(view, " · ") {
		t.Fatalf("config path should prefill:\n%s", view)
	}
	for i := range m.dlg.hfields {
		if m.dlg.hfields[i].key == "c:sealed-secrets" {
			m.dlg.hfields[i].on = true
		}
	}
	next, cmd := m.submitKindForm()
	m = settle(t, next.(model), cmd)
	view = ansi.Strip(m.View())
	if !strings.Contains(view, kindDetail("charts", "sealed-secrets")) || !strings.Contains(view, "master key") {
		t.Fatalf("confirm:\n%s", view)
	}
	m = pressSettle(t, m, "y")
	var applyAt, installAt int
	applyAt, installAt = -1, -1
	for i, c := range f.Calls() {
		line := c.Name + " " + strings.Join(c.Args, " ")
		if c.Name == "kubectl" && len(c.Args) > 0 && c.Args[0] == "apply" && strings.Contains(line, path) {
			applyAt = i
		}
		if c.Name == "helm" && strings.HasPrefix(strings.Join(c.Args, " "), "install sealed-secrets ") {
			installAt = i
		}
	}
	if applyAt < 0 || installAt < 0 || applyAt >= installAt {
		t.Fatalf("apply=%d install=%d calls=%v", applyAt, installAt, callSummary(f))
	}
	log := kindJobLog(m, "k2")
	if !strings.Contains(log, "applying sealed-secrets master key") {
		t.Fatalf("log missing apply step:\n%s", log)
	}
}

func TestKindMasterKeyMissingFileBlocksConfirm(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, nil)
	m := appModel(t, f)
	withSealedMasterKey(t, &m, "/no/such/master.yml")
	m = pressSettle(t, open(t, m, "kind"), "A")
	for i := range m.dlg.hfields {
		if m.dlg.hfields[i].key == "c:sealed-secrets" {
			m.dlg.hfields[i].on = true
		}
	}
	next, cmd := m.submitKindForm()
	m = settle(t, next.(model), cmd)
	if m.dlg == nil || m.dlg.act != "kindcreate" || !strings.Contains(m.dlg.msg, "master key file not found") {
		t.Fatalf("missing file: act=%v msg=%q", m.dlg, func() string {
			if m.dlg == nil {
				return ""
			}
			return m.dlg.msg
		}())
	}
	if len(helmCalls(f)) != 0 {
		t.Fatalf("helm should not run: %v", helmCalls(f))
	}
}

func TestKindChartsMasterKeyPicker(t *testing.T) {
	withKindCLIs(t)
	f := kindFake(t, []string{"lab"})
	m := appModel(t, f)
	home := os.Getenv("HOME")
	if err := os.MkdirAll(filepath.Join(home, ".kube"), 0o700); err != nil {
		t.Fatal(err)
	}
	kc := kube.KindKubeconfig("lab", home)
	if err := os.WriteFile(kc, []byte("apiVersion: v1\nkind: Config\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := writeMasterKey(t, filepath.Join(home, "secrets"), "master.yml")
	m.kubeconfig, m.context = kc, kube.KindContext("lab")
	m = pressSettle(t, open(t, m, "kind"), "e")
	if m.dlg == nil || m.dlg.act != "kindcharts" {
		t.Fatalf("charts form %+v", m.dlg)
	}
	m = focusKindField(t, m, "c:istio")
	m = pressSettle(t, m, "m")
	if m.dlg == nil || m.dlg.act != "kindcharts" {
		t.Fatalf("m on istio: %+v", m.dlg)
	}
	m = focusKindField(t, m, "c:sealed-secrets")
	m = pressSettle(t, m, "m")
	if m.dlg == nil || m.dlg.act != "fpath" {
		t.Fatalf("m should open picker: %+v", m.dlg)
	}
	m = pressSettle(t, m, "ctrl+u")
	m = typeField(t, m, path)
	m = pressSettle(t, m, "enter")
	if m.dlg == nil || m.dlg.act != "kindcharts" {
		t.Fatalf("back to charts: %+v", m.dlg)
	}
	next, cmd := m.submitKindCharts()
	m = settle(t, next.(model), cmd)
	view := ansi.Strip(m.View())
	if m.dlg == nil || !strings.Contains(view, kindDetail("install", "sealed-secrets")) || !strings.Contains(view, kindDetail("master key", config.HomePath(path))) {
		t.Fatalf("charts confirm:\n%s", view)
	}
	m = pressSettle(t, m, "y")
	var applyAt, installAt int
	applyAt, installAt = -1, -1
	for i, c := range f.Calls() {
		line := strings.Join(c.Args, " ")
		if c.Name == "kubectl" && strings.HasPrefix(line, "apply -f "+path) {
			applyAt = i
		}
		if c.Name == "helm" && strings.HasPrefix(line, "install sealed-secrets ") {
			installAt = i
		}
	}
	if applyAt < 0 || installAt < 0 || applyAt >= installAt {
		t.Fatalf("apply=%d install=%d calls=%v", applyAt, installAt, callSummary(f))
	}
}

func helmWithApply(t *testing.T) *shell.Fake {
	t.Helper()
	base := helmInstallFake(t, "")
	parent := base.Handler
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		line := strings.Join(r.Args, " ")
		if r.Name == "kubectl" && strings.HasPrefix(line, "apply -f") {
			return shell.OK("secret/sealed-secrets-key created")
		}
		if r.Name == "helm" && (strings.HasPrefix(line, "install sealed-secrets ") || strings.HasPrefix(line, "template sealed-secrets ")) {
			if strings.HasPrefix(line, "template ") {
				return shell.OK("apiVersion: v1\nkind: Service\nmetadata:\n  name: sealed-secrets\n")
			}
			return shell.OK("NAME: sealed-secrets\nNAMESPACE: kube-system\nSTATUS: deployed\nNOTES:\ncontroller installed\n")
		}
		return parent(r)
	}}
}

func TestHelmMasterKeyRowPickerAndInstall(t *testing.T) {
	withHelmCLI(t)
	f := helmWithApply(t)
	m := appModel(t, f)
	home := os.Getenv("HOME")
	path := writeMasterKey(t, filepath.Join(home, "secrets"), "master.yml")
	withSealedMasterKey(t, &m, "~/secrets/master.yml")
	m = pressSettle(t, open(t, m, "helm"), "A")
	if helmFieldValue(m.dlg, "masterkey") != "" {
		t.Fatalf("master key shown too early: %q", helmFieldValue(m.dlg, "masterkey"))
	}
	setHelmField(m.dlg, "chart", "sealed-secrets")
	m.maybeFillCatalog()
	if helmFieldValue(m.dlg, "masterkey") != "~/secrets/master.yml" {
		t.Fatalf("prefill %q", helmFieldValue(m.dlg, "masterkey"))
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "master key") {
		t.Fatalf("form lacks master key:\n%s", view)
	}
	m = focusHelmFormField(t, m, "name")
	m = pressSettle(t, m, "m")
	if m.dlg == nil || m.dlg.act != "helminstall" || !strings.Contains(helmFieldValue(m.dlg, "name"), "m") {
		t.Fatalf("m should type in release: act=%v name=%q", func() string {
			if m.dlg == nil {
				return ""
			}
			return m.dlg.act
		}(), helmFieldValue(m.dlg, "name"))
	}
	setHelmField(m.dlg, "name", "sealed-secrets")
	m = focusHelmFormField(t, m, "masterkey")
	view = ansi.Strip(m.View())
	if !strings.Contains(view, "ctrl+p") || !strings.Contains(view, "browse") {
		t.Fatalf("master key footer:\n%s", view)
	}
	m = pressSettle(t, m, "ctrl+p")
	if m.dlg == nil || m.dlg.act != "fpath" {
		t.Fatalf("ctrl+p should open picker: %+v", m.dlg)
	}
	m = pressSettle(t, m, "esc")
	if m.dlg == nil || m.dlg.act != "helminstall" {
		t.Fatalf("esc back: %+v", m.dlg)
	}
	next, cmd := m.submitHelmForm()
	m = settle(t, next.(model), cmd)
	if m.stage != "htpl" || !strings.Contains(m.logText, "master key") {
		t.Fatalf("preview stage %s text:\n%s", m.stage, m.logText)
	}
	m = pressSettle(t, m, "enter")
	view = ansi.Strip(m.View())
	if m.dlg == nil || !strings.Contains(view, "master key "+config.HomePath(path)) {
		t.Fatalf("confirm lacks master key:\n%s", view)
	}
	m = pressSettle(t, m, "y")
	var applyAt, installAt int
	applyAt, installAt = -1, -1
	for i, c := range f.Calls() {
		line := strings.Join(c.Args, " ")
		if c.Name == "kubectl" && strings.HasPrefix(line, "apply -f "+path) {
			applyAt = i
		}
		if c.Name == "helm" && strings.HasPrefix(line, "install sealed-secrets ") {
			installAt = i
		}
	}
	if applyAt < 0 || installAt < 0 || applyAt >= installAt {
		t.Fatalf("apply=%d install=%d calls=%v", applyAt, installAt, callSummary(f))
	}
}

func TestHelmMasterKeyMissingFile(t *testing.T) {
	withHelmCLI(t)
	f := helmInstallFake(t, "")
	m := appModel(t, f)
	withSealedMasterKey(t, &m, "/no/such/master.yml")
	m = pressSettle(t, open(t, m, "helm"), "A")
	setHelmField(m.dlg, "chart", "sealed-secrets")
	m.maybeFillCatalog()
	next, cmd := m.submitHelmForm()
	m = settle(t, next.(model), cmd)
	if m.dlg == nil || m.dlg.act != "helminstall" || !strings.Contains(m.dlg.msg, "master key file not found") {
		t.Fatalf("missing file: %+v msg=%q", m.dlg, func() string {
			if m.dlg == nil {
				return ""
			}
			return m.dlg.msg
		}())
	}
	if len(helmVerbCalls(f, "install")) != 0 {
		t.Fatalf("helm install ran: %v", helmVerbCalls(f, "install"))
	}
}

func focusHelmFormField(t *testing.T, m model, key string) model {
	t.Helper()
	if m.dlg == nil {
		t.Fatal("no form")
	}
	for i, f := range m.dlg.hfields {
		if f.key == key {
			return m.moveHelmFieldTo(i)
		}
	}
	t.Fatalf("no field %s", key)
	return m
}

func TestSealedMasterKeyIsRemembered(t *testing.T) {
	m := appModel(t, kindFake(t, nil))
	home := os.Getenv("HOME")
	picked := writeMasterKey(t, filepath.Join(home, "secrets"), "picked.yml")
	m.rememberSealedKey(picked, m.configuredKindSealedKey())
	if m.sealedKey != picked {
		t.Fatalf("remembered %q", m.sealedKey)
	}
	if st := state.Load(m.cfg.StateDir); st.SealedMasterKey != picked {
		t.Fatalf("state %q", st.SealedMasterKey)
	}
	reload := appModel(t, kindFake(t, nil))
	reload.cfg.StateDir = m.cfg.StateDir
	reload.sealedKey = state.Load(m.cfg.StateDir).SealedMasterKey
	op := &kindOp{}
	reload.ensureKindMasterKeys(op)
	if op.masterKeys["sealed-secrets"] != picked || reload.kindSealedMasterKey(op) != picked {
		t.Fatalf("kind prefill %v", op.masterKeys)
	}
	if got := reload.prefillSealedMasterKey("sealed-secrets", "sealed-secrets"); got != picked {
		t.Fatalf("helm prefill %q", got)
	}

	configured := writeMasterKey(t, filepath.Join(home, "secrets"), "config.yml")
	withSealedMasterKey(t, &reload, configured)
	op = &kindOp{}
	reload.ensureKindMasterKeys(op)
	if op.masterKeys["sealed-secrets"] != configured || reload.prefillSealedMasterKey("sealed-secrets", "sealed-secrets") != configured {
		t.Fatalf("config must win: %v", op.masterKeys)
	}
	reload.rememberSealedKey(configured, configured)
	if reload.sealedKey != picked {
		t.Fatalf("picking the configured path must not replace the remembered one: %q", reload.sealedKey)
	}

	m.rememberSealedKey("", "")
	if m.sealedKey != "" || state.Load(m.cfg.StateDir).SealedMasterKey != "" {
		t.Fatal("clearing the picker must forget the path")
	}
	op = &kindOp{}
	m.ensureKindMasterKeys(op)
	if _, ok := op.masterKeys["sealed-secrets"]; ok {
		t.Fatalf("nothing to prefill after clearing: %v", op.masterKeys)
	}
}

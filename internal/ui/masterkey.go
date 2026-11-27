package ui

import (
	"os"
	"strings"

	"k2/internal/config"
	"k2/internal/kube"
	"k2/internal/shell"

	tea "github.com/charmbracelet/bubbletea"
)

func kindFieldIsSealedSecrets(key string) bool {
	return strings.HasPrefix(key, "c:") && config.IsSealedSecretsName(strings.TrimPrefix(key, "c:"))
}

func (m model) chartMasterKey(name string, stored map[string]string) string {
	if stored != nil {
		if v, ok := stored[name]; ok {
			return v
		}
	}
	if c, ok := config.LookupChart(m.cfg.Charts(), name); ok {
		return c.MasterKeyPath
	}
	return ""
}

func (m *model) ensureKindMasterKeys(op *kindOp) {
	if op == nil {
		return
	}
	if op.masterKeys == nil {
		op.masterKeys = map[string]string{}
	}
	if _, ok := op.masterKeys["sealed-secrets"]; !ok {
		if c, ok := config.LookupChart(m.cfg.Charts(), "sealed-secrets"); ok && c.MasterKeyPath != "" {
			op.masterKeys["sealed-secrets"] = c.MasterKeyPath
		}
	}
}

func (m model) kindSealedMasterKey(op *kindOp) string {
	if op == nil {
		return ""
	}
	return strings.TrimSpace(m.chartMasterKey("sealed-secrets", op.masterKeys))
}

func kindSelectsSealedSecrets(groups ...[]string) bool {
	for _, group := range groups {
		for _, n := range group {
			if config.IsSealedSecretsName(n) {
				return true
			}
		}
	}
	return false
}

func (m model) defaultSealedMasterKey(chart, catalog string) string {
	if c, ok := config.LookupChart(m.cfg.Charts(), or(catalog, chart)); ok && c.IsSealedSecrets() {
		return c.MasterKeyPath
	}
	if config.IsSealedSecretsName(or(catalog, chart)) {
		if c, ok := config.LookupChart(m.cfg.Charts(), "sealed-secrets"); ok {
			return c.MasterKeyPath
		}
	}
	return ""
}

func helmShowsMasterKey(op *helmOp, chart string) bool {
	if op == nil {
		return false
	}
	if config.IsSealedSecretsName(op.catalog) || config.IsSealedSecretsName(chart) {
		return true
	}
	return false
}

func (m model) validateMasterKeyFile(path string) (string, string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", ""
	}
	p := config.ExpandHome(path)
	st, err := os.Stat(p)
	if err != nil || st.IsDir() {
		return "", "master key file not found: " + config.HomePath(p)
	}
	return p, ""
}

func emitApplyMasterKey(run shell.Runner, path, namespace, kubeconfig, context string, createNS bool, emit func(string)) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	expanded := config.ExpandHome(path)
	if emit != nil {
		emit("applying sealed-secrets master key " + config.HomePath(expanded))
	}
	return kube.ApplyMasterKey(run, expanded, namespace, kubeconfig, context, createNS, emit)
}

func (m model) openKindMasterKeyPicker() (tea.Model, tea.Cmd) {
	if m.dlg == nil || m.dlg.idx < 0 || m.dlg.idx >= len(m.dlg.hfields) || !kindFieldIsSealedSecrets(m.dlg.hfields[m.dlg.idx].key) {
		return m, nil
	}
	m.dlg.hfields[m.dlg.idx].on = true
	m.stashKindForm()
	if m.vs.kop == nil {
		return m, nil
	}
	op := *m.vs.kop
	if op.form == "" {
		if m.dlg != nil && m.dlg.act == "kindcharts" {
			op.form = "kindcharts"
		} else {
			op.form = "kindcreate"
		}
	}
	op.valuesChart = "sealed-secrets"
	m.vs.kop = &op
	current := m.kindSealedMasterKey(&op)
	return m.openYAMLPath("sealed-secrets master key", yamlPickerStart(current),
		func(m model, path string) (tea.Model, tea.Cmd) {
			if m.vs.kop == nil {
				return m, nil
			}
			op := *m.vs.kop
			if op.masterKeys == nil {
				op.masterKeys = map[string]string{}
			}
			op.masterKeys["sealed-secrets"] = path
			m.vs.kop = &op
			m.reopenKindForm()
			return m, nil
		},
		func(m model) (tea.Model, tea.Cmd) {
			m.reopenKindForm()
			return m, nil
		})
}

func (m model) openHelmMasterKeyPicker() (tea.Model, tea.Cmd) {
	m.stashHelmForm()
	if m.vs.hop == nil {
		return m, nil
	}
	current := strings.TrimSpace(m.vs.hop.masterKey)
	if current == "" {
		current = m.defaultSealedMasterKey(m.vs.hop.chart, m.vs.hop.catalog)
	}
	return m.openYAMLPath("sealed-secrets master key", yamlPickerStart(current),
		func(m model, path string) (tea.Model, tea.Cmd) {
			if m.vs.hop == nil {
				return m, nil
			}
			m.vs.hop.masterKey = path
			m.reopenHelmForm()
			return m.focusHelmField("masterkey"), nil
		},
		func(m model) (tea.Model, tea.Cmd) {
			m.reopenHelmForm()
			return m.focusHelmField("masterkey"), nil
		})
}

func (m model) focusHelmField(key string) model {
	if m.dlg == nil {
		return m
	}
	for i, f := range m.dlg.hfields {
		if f.key == key {
			return m.moveHelmFieldTo(i)
		}
	}
	return m
}

func (m model) moveHelmFieldTo(idx int) model {
	d := m.dlg
	if d == nil || len(d.hfields) == 0 {
		return m
	}
	if idx < 0 || idx >= len(d.hfields) {
		return m
	}
	if d.idx >= 0 && d.idx < len(d.hfields) {
		helmBlur(&d.hfields[d.idx])
	}
	d.idx = idx
	helmFocus(&d.hfields[d.idx])
	d.cands, d.candBase, d.candAt = nil, "", -1
	return m
}

func (m *model) syncHelmMasterKeyField() {
	if m.dlg == nil || m.dlg.act != "helminstall" {
		return
	}
	chart := helmFieldValue(m.dlg, "chart")
	catalog := ""
	if m.vs.hop != nil {
		catalog = m.vs.hop.catalog
	}
	show := helmShowsMasterKey(m.vs.hop, chart) || config.IsSealedSecretsName(catalog)
	if !show {
		if c, ok := config.LookupChart(m.cfg.Charts(), or(catalog, chart)); ok && c.IsSealedSecrets() {
			show = true
		}
	}
	idx := -1
	for i, f := range m.dlg.hfields {
		if f.key == "masterkey" {
			idx = i
			break
		}
	}
	if show && idx < 0 {
		path := ""
		if m.vs.hop != nil {
			path = strings.TrimSpace(m.vs.hop.masterKey)
		}
		if path == "" {
			path = m.defaultSealedMasterKey(chart, catalog)
			if m.vs.hop != nil && path != "" {
				m.vs.hop.masterKey = path
			}
		}
		field := helmField{key: "masterkey", label: "master key", input: helmFieldInput(path, "path to Secret YAML")}
		at := len(m.dlg.hfields)
		for i, f := range m.dlg.hfields {
			if f.key == "values" {
				at = i + 1
				break
			}
		}
		fields := make([]helmField, 0, len(m.dlg.hfields)+1)
		fields = append(fields, m.dlg.hfields[:at]...)
		fields = append(fields, field)
		fields = append(fields, m.dlg.hfields[at:]...)
		if m.dlg.idx >= at {
			m.dlg.idx++
		}
		m.dlg.hfields = fields
		return
	}
	if !show && idx >= 0 {
		if m.vs.hop != nil {
			m.vs.hop.masterKey = strings.TrimSpace(m.dlg.hfields[idx].input.Value())
		}
		fields := append([]helmField{}, m.dlg.hfields[:idx]...)
		fields = append(fields, m.dlg.hfields[idx+1:]...)
		if m.dlg.idx > idx {
			m.dlg.idx--
		} else if m.dlg.idx >= len(fields) {
			m.dlg.idx = len(fields) - 1
		}
		m.dlg.hfields = fields
	}
}

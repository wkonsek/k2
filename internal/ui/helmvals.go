package ui

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"k2/internal/config"
	"k2/internal/kube"
)

func helmValuesCacheKey(ctx, ns, name string) string {
	sum := sha256.Sum256([]byte(ctx + "\x00" + ns + "\x00" + name))
	return hex.EncodeToString(sum[:])
}

func (m model) helmValuesCachePath(ctx, ns, name string) string {
	dir := m.cfg.StateDir
	if dir == "" {
		dir = config.DefaultDir()
	}
	return filepath.Join(dir, "helm-values", helmValuesCacheKey(ctx, ns, name)+".yaml")
}

func (m model) preferHelmValues(ctx, ns, name, current string) string {
	if ctx == "" || name == "" || current == "" {
		return current
	}
	b, err := os.ReadFile(m.helmValuesCachePath(ctx, ns, name))
	if err != nil {
		return current
	}
	cached := string(b)
	ok, err := kube.YAMLEqual(cached, current)
	if err != nil || !ok {
		return current
	}
	return cached
}

func (m model) writeHelmValuesCache(ctx, ns, name, text string) {
	if ctx == "" || name == "" {
		return
	}
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return
	}
	text += "\n"
	path := m.helmValuesCachePath(ctx, ns, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	_ = os.WriteFile(path, []byte(text), 0o600)
}

func (m model) releaseValuesYAML(r kube.HelmRelease) string {
	return m.preferHelmValues(m.context, r.Namespace, r.Name, kube.ValuesYAML(r.Config))
}

func (m *model) saveHelmValuesFromHop() {
	op := m.vs.hop
	if op == nil || op.values == "" || op.name == "" {
		return
	}
	b, err := os.ReadFile(op.values)
	if err != nil {
		return
	}
	ns := op.ns
	if ns == "" {
		ns = op.current.Namespace
	}
	m.writeHelmValuesCache(m.context, ns, op.name, string(b))
}

func (m *model) saveKindValuesCaches(op kindOp) {
	if len(op.values) == 0 {
		return
	}
	ctx := kube.KindContext(op.name)
	for chart, path := range op.values {
		if path == "" {
			continue
		}
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		ns, name := m.kindChartRelease(op, chart)
		m.writeHelmValuesCache(ctx, ns, name, string(b))
	}
}

func (m model) kindChartRelease(op kindOp, chart string) (ns, name string) {
	if r, ok := op.releases[chart]; ok && r.name != "" {
		return r.namespace, r.name
	}
	if c, ok := config.LookupChart(m.cfg.Charts(), chart); ok {
		return c.Namespace, c.HelmRelease()
	}
	if op.namespaces != nil {
		ns = op.namespaces[chart]
	}
	return ns, chart
}

func formatHelmValues(text string) string {
	pretty, err := kube.FormatYAML(text, false)
	if err != nil || pretty == "" {
		return text
	}
	return pretty
}

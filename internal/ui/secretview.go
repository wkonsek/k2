package ui

import (
	"regexp"
	"strconv"
	"strings"

	"k2/internal/kube"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (v *viewState) takeSecret(msg viewMsg) {
	if !msg.secret {
		return
	}
	v.secretAlt, v.secretHas = msg.alt, msg.certs
}

func secretTag(stage string, vs viewState, subject, kind string) string {
	switch {
	case stage == "view" && vs.secret != "" && vs.secretRef == subject:
		if vs.secret == "cert" {
			return "certificates"
		}
		return "decoded"
	case stage == "yaml" && kube.IsSecretKind(vs.watchKind) && kube.IsSecretKind(kind) && vs.watchName != "":
		return "encoded"
	}
	return ""
}

func (m model) secretMode() string {
	if m.stage == "view" && m.vs.secret != "" && m.vs.secretRef == m.subject {
		return m.vs.secret
	}
	return ""
}

func (m model) secretCycles() bool {
	if m.secretMode() != "" {
		return true
	}
	return m.stage == "yaml" && kube.IsSecretKind(m.kind) && kube.IsSecretKind(m.vs.watchKind) && m.vs.watchName != ""
}

func (m model) secretLegend() string {
	switch m.secretMode() {
	case "decoded":
		if m.vs.secretHas {
			return "certificates"
		}
		return "encoded"
	case "cert":
		return "encoded"
	}
	return "decode"
}

func (m model) fetchSecretViews(ns, name string) tea.Cmd {
	kc, ctx, pane, k := m.kubeconfig, m.context, m.logID, m.k
	return func() tea.Msg {
		v, err := k.SecretViews(kc, ctx, ns, name)
		if err != nil {
			return viewMsg{id: pane, err: err}
		}
		return viewMsg{id: pane, text: v.Decoded, alt: v.Certs, certs: v.HasCerts, secret: true}
	}
}

func (m model) openSecret(ns, name string, keepWatch bool) (tea.Model, tea.Cmd) {
	m.subject = name
	if ns != "" {
		m.subject = ns + "/" + name
	}
	m.logText = ""
	m.logStick = false
	m.logAt = 0
	m.stage = "view"
	m.vs.vback = ""
	m.vs.secret = "decoded"
	m.vs.secretRef = m.subject
	m.vs.secretAlt = ""
	m.vs.secretHas = false
	if !keepWatch {
		m.armWatch("secrets", ns, name)
	}
	m.status = "decoded secret"
	return m, tea.Batch(m.startLoad("decoding secret", false), m.fetchSecretViews(ns, name))
}

func (m model) cycleSecret() (tea.Model, tea.Cmd) {
	if m.load.active() {
		return m, nil
	}
	switch m.secretMode() {
	case "":
		return m.openSecret(m.vs.watchNS, m.vs.watchName, true)
	case "decoded":
		if m.vs.secretHas && m.vs.secretAlt != "" {
			m.vs.secret = "cert"
			m.logText = m.vs.secretAlt
			m.logStick = false
			m.logAt = 0
			m.status = "secret certificates"
			return m, nil
		}
	}
	m.vs.secret = ""
	m.logText = ""
	m.logStick = false
	m.logAt = 0
	m.stage = "yaml"
	m.status = m.kind
	return m, tea.Batch(m.startLoad("loading yaml", false), m.fetchYAML(m.vs.watchName, m.vs.watchNS))
}

var certDays = regexp.MustCompile(`^expires in (\d+) days?$`)

func certTagStyle(tag string) lipgloss.Style {
	switch {
	case strings.HasPrefix(tag, "expired"):
		return errStyle
	case tag == "not valid yet" || tag == "expires today":
		return warnStyle
	}
	if m := certDays.FindStringSubmatch(tag); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil && n <= kube.CertSoonDays {
			return warnStyle
		}
	}
	return okStyle
}

func colorCert(lines []string) []string {
	out := make([]string, len(lines))
	base := 0
	for i, l := range lines {
		trim := strings.TrimLeft(l, " ")
		indent := len(l) - len(trim)
		switch {
		case trim == "":
			out[i] = l
		case indent == 0:
			base = 0
			out[i] = keyStyle.Render(l)
		case strings.HasPrefix(trim, "["):
			out[i] = dimStyle.Render(l)
		case strings.HasPrefix(trim, "certificate ") && strings.Contains(trim, " of "):
			base = 0
			out[i] = dimStyle.Bold(true).Render(l)
		default:
			if base == 0 {
				base = indent
			}
			out[i] = colorCertRow(l, base, indent)
		}
	}
	return out
}

func colorCertRow(l string, base, indent int) string {
	const labelWidth = 14
	if indent >= base+labelWidth || len(l) < base+labelWidth {
		return itemStyle.Render(l)
	}
	label, value := l[:base+labelWidth], l[base+labelWidth:]
	head := dimStyle.Render(label)
	if strings.TrimSpace(label) == "validity" {
		return head + certTagStyle(strings.TrimSpace(value)).Render(value)
	}
	return head + itemStyle.Render(value)
}

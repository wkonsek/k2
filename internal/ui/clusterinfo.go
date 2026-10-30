package ui

import (
	"strings"
	"time"

	"k2/internal/kube"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

type clusterInfo struct {
	v       kube.Versions
	loading bool
	done    bool
}

type clusterInfoMsg struct {
	key string
	v   kube.Versions
}

const infoGrace = time.Second
const infoPad = 12

func infoKey(kubeconfig, context string) string { return kubeconfig + "\x00" + context }

func (m *model) clusterInfoCmd() tea.Cmd {
	if !m.cfg.UI.ClusterInfo {
		return nil
	}
	key := infoKey(m.kubeconfig, m.context)
	cur, cached := m.cinfo[key]
	if cur.loading || (cached && key == m.cinfoKey) {
		return nil
	}
	m.cinfoKey = key
	m.setInfo(key, clusterInfo{v: cur.v, loading: true, done: cur.done})
	k, kc, ctx := m.k, m.kubeconfig, m.context
	return func() tea.Msg {
		ch := make(chan kube.Versions, 1)
		go func() { ch <- k.ClusterVersions(kc, ctx) }()
		select {
		case v := <-ch:
			return clusterInfoMsg{key: key, v: v}
		case <-time.After(2*kube.InfoTimeout + infoGrace):
			return clusterInfoMsg{key: key}
		}
	}
}

func (m *model) applyClusterInfo(msg clusterInfoMsg) {
	v := msg.v
	if v.Client == "" {
		v.Client = m.cinfo[msg.key].v.Client
	}
	m.setInfo(msg.key, clusterInfo{v: v, done: true})
}

func (m *model) setInfo(key string, ci clusterInfo) {
	next := make(map[string]clusterInfo, len(m.cinfo)+1)
	for k, v := range m.cinfo {
		next[k] = v
	}
	next[key] = ci
	m.cinfo = next
}

func (m model) infoBlock(inner int) string {
	tok := m.tokenRows(inner)
	if !m.cfg.UI.ClusterInfo {
		if tok == "" {
			return ""
		}
		return tok + "\n"
	}
	ci := m.cinfo[infoKey(m.kubeconfig, m.context)]
	waiting := !ci.done
	pending := dimStyle.Render("…")
	if m.cfg.UI.Animations {
		frames := spinner.MiniDot.Frames
		pending = dimStyle.Render(frames[m.beat%len(frames)])
	}
	value := func(s string) string {
		switch {
		case s != "":
			return itemStyle.Render(s)
		case waiting:
			return pending
		default:
			return dimStyle.Render("N/A")
		}
	}
	var b strings.Builder
	row := func(label, val string) {
		b.WriteString(clipLine(dimStyle.Render(padRight(label, infoPad))+val, inner) + "\n")
	}
	row("k2", itemStyle.Render(or(m.version, "dev")))
	row("kubectl", value(fitVersion(ci.v.Client, inner-infoPad)))
	if m.context != "" {
		if ci.done && ci.v.Server == "" {
			row("k8s server", errStyle.Render("unreachable"))
		} else {
			row("k8s server", value(fitVersion(ci.v.Server, inner-infoPad)))
		}
		if ci.v.OpenShift != "" {
			row("ocp", itemStyle.Render(ci.v.OpenShift))
		}
	}
	b.WriteString(tok)
	b.WriteString("\n")
	return b.String()
}

func fitVersion(s string, room int) string {
	if len(s) <= room {
		return s
	}
	if i := strings.IndexAny(s, "-+"); i > 0 {
		return s[:i]
	}
	return s
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s + " "
	}
	return s + strings.Repeat(" ", n-len(s))
}

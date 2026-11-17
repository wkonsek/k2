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
	listed  bool
	at      time.Time
	tries   int
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
	if cur.loading {
		return nil
	}
	retry := cur.done && cur.v.Server == "" && (cur.listed || time.Since(cur.at) >= infoRetry(cur.tries))
	if cached && key == m.cinfoKey && !retry {
		return nil
	}
	m.cinfoKey = key
	m.setInfo(key, clusterInfo{v: cur.v, loading: true, done: cur.done, listed: cur.listed, at: cur.at, tries: cur.tries})
	k, kc, ctx := m.k, m.kubeconfig, m.context
	return func() tea.Msg {
		return clusterInfoMsg{key: key, v: k.ClusterVersions(kc, ctx)}
	}
}

func infoRetry(tries int) time.Duration {
	switch {
	case tries <= 1:
		return 2 * time.Second
	case tries == 2:
		return 8 * time.Second
	default:
		return 30 * time.Second
	}
}

func (m *model) applyClusterInfo(msg clusterInfoMsg) {
	prev := m.cinfo[msg.key]
	v := msg.v
	if v.Client == "" {
		v.Client = prev.v.Client
	}
	if prev.listed || v.Server != "" {
		v.Reachable = true
	}
	m.setInfo(msg.key, clusterInfo{v: v, done: true, listed: prev.listed, at: time.Now(), tries: prev.tries + 1})
}

func (m *model) noteClusterListed(kubeconfig, context string) {
	if kubeconfig == "" && context == "" {
		return
	}
	key := infoKey(kubeconfig, context)
	ci := m.cinfo[key]
	if ci.v.Server != "" {
		return
	}
	ci.v.Reachable = true
	if !ci.listed {
		ci.listed = true
		ci.at = time.Time{}
	}
	m.setInfo(key, ci)
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
			if ci.v.Reachable {
				row("k8s server", dimStyle.Render("unknown"))
			} else {
				row("k8s server", errStyle.Render("unreachable"))
			}
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

package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

const histCap = 50

type viewHist struct {
	stage, kind, namespace string
	sel, filter            string
	vs                     viewState
}

func cloneMarkSet(m map[string]bool) map[string]bool {
	if m == nil {
		return nil
	}
	out := make(map[string]bool, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func cloneVS(vs viewState) viewState {
	vs.marks = cloneMarkSet(vs.marks)
	vs.sortBack.marks = cloneMarkSet(vs.sortBack.marks)
	if vs.logMarks != nil {
		vs.logMarks = append([]int(nil), vs.logMarks...)
	}
	if vs.kpos != nil {
		out := make(map[string]kmark, len(vs.kpos))
		for k, v := range vs.kpos {
			out[k] = v
		}
		vs.kpos = out
	}
	return vs
}

func (h viewHist) id() string {
	return h.stage + "\x00" + h.kind + "\x00" + h.namespace + "\x00" + h.vs.parent + "\x00" + h.vs.podSel
}

func (h viewHist) label() string {
	switch h.stage {
	case "workpods":
		if _, _, ok := parseContainerSel(h.vs.podSel); ok {
			return "containers"
		}
		return "pods"
	case "res":
		if i := strings.IndexByte(h.kind, '.'); i > 0 {
			return h.kind[:i]
		}
		return or(h.kind, "resource")
	case "fwds":
		return "forwards"
	case "mlogs":
		return "logs"
	default:
		return or(h.stage, "back")
	}
}

func histable(stage string) bool {
	switch stage {
	case "workpods", "res", "helm", "pulse", "xray", "fwds", "mlogs", "kind":
		return true
	}
	return false
}

func (m model) histable() bool { return histable(m.stage) }

func (m model) captureView() viewHist {
	h := viewHist{stage: m.stage, kind: m.kind, namespace: m.namespace, vs: cloneVS(m.vs)}
	if m.content.FilterState() != list.Unfiltered {
		h.filter = m.content.FilterValue()
	}
	if it, ok := m.selectedItem(); ok {
		h.sel = it.id
	}
	return h
}

func (m *model) rememberView() {
	if m.histSkip || !m.histable() {
		return
	}
	cur := m.captureView()
	if n := len(m.hist); n > 0 && m.hist[n-1].id() == cur.id() {
		return
	}
	m.hist = append(m.hist, cur)
	if len(m.hist) > histCap {
		m.hist = append([]viewHist(nil), m.hist[len(m.hist)-histCap:]...)
	}
}

func (m *model) clearHist() {
	m.hist = nil
	m.histRest = nil
}

func (m *model) dropHistIfCurrent() {
	if n := len(m.hist); n > 0 && m.hist[n-1].id() == m.captureView().id() {
		m.hist = m.hist[:n-1]
	}
}

func (m model) popHist() (tea.Model, tea.Cmd, bool) {
	if len(m.hist) == 0 {
		return m, nil, false
	}
	h := m.hist[len(m.hist)-1]
	m.hist = m.hist[:len(m.hist)-1]
	next, cmd := m.restoreView(h)
	return next, cmd, true
}

func (m model) restoreView(h viewHist) (tea.Model, tea.Cmd) {
	m.namespace = h.namespace
	m.vs = cloneVS(h.vs)
	m.kind = h.kind
	m.vs.pick = h.sel
	m.histRest = &viewHist{sel: h.sel, filter: h.filter}
	m.histSkip = true
	var next tea.Model
	var cmd tea.Cmd
	switch h.stage {
	case "workpods":
		next, cmd = m.openList("workpods", "pods")
	case "res":
		next, cmd = m.openList("res", h.kind)
	case "helm":
		next, cmd = m.openHelm()
	case "pulse":
		next, cmd = m.openPulse()
	case "kind":
		m.kindSnap.sel = m.vs.kindSel
		next, cmd = m.openKind()
	case "xray":
		kind, name := "", ""
		if x := m.vs.xray; x != nil {
			kind, name = x.kind, x.name
		}
		next, cmd = m.openXray(kind, name)
	case "mlogs":
		if v := m.vs.ml; v != nil {
			next, cmd = m.openMultiLogs(v.q, v.title, v.from)
		} else {
			m.stage = "nav"
			m.histSkip = false
			m.histRest = nil
			return m, nil
		}
	case "fwds":
		cmd = m.showForwards()
		m.histSkip = false
		return m, cmd
	default:
		m.stage = "nav"
		m.histSkip = false
		m.histRest = nil
		return m, nil
	}
	nm := next.(model)
	nm.histSkip = false
	return nm, cmd
}

func (m *model) applyHistRest() {
	r := m.histRest
	if r == nil {
		return
	}
	m.histRest = nil
	if r.filter != "" {
		m.content.SetFilterText(r.filter)
		m.content.SetFilterState(list.FilterApplied)
	}
	if r.sel == "" {
		return
	}
	for i, raw := range m.content.VisibleItems() {
		if it, ok := raw.(item); ok && it.id == r.sel {
			m.content.Select(i)
			return
		}
	}
}

func (m model) goHome() (tea.Model, tea.Cmd) {
	m.clearHist()
	if m.stage == "kcmd" {
		m.jqOn = false
		m.sh.Blur()
		m.jq.Blur()
	}
	if m.stage == "nav" {
		return m, nil
	}
	m.cancelLoad()
	m.stage = "nav"
	m.status = m.context
	return m, nil
}

func (m model) backLegend() string {
	if n := len(m.hist); n > 0 && m.histable() && !m.inContainers() && m.vs.parent == "" {
		return m.hist[n-1].label()
	}
	return "back"
}

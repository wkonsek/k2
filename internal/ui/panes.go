package ui

import (
	"fmt"
	"strings"
	"time"

	"k2/internal/config"

	"k2/internal/kube"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
)

type paneSnap struct {
	content     list.Model
	portInput   textinput.Model
	stage       string
	pod         string
	localPort   string
	remotePort  string
	suggestions []kube.PortSuggestion
	sugIndex    int
	logPod      string
	logText     string
	logBusy     bool
	logSince    time.Time
	logAt       int
	logStick    bool
	logID       string
	status      string
	kind        string
	subject     string
	dback       string
	kubeconfig  string
	context     string
	namespace   string
	load        load
	ran         runInfo
	ed          *editor
	vs          viewState
}

func (m *model) snap() paneSnap {
	return paneSnap{
		content: m.content, portInput: m.portInput, stage: m.stage,
		pod: m.pod, localPort: m.localPort, remotePort: m.remotePort,
		suggestions: m.suggestions, sugIndex: m.sugIndex,
		logPod: m.logPod, logText: m.logText, logBusy: m.logBusy, logSince: m.logSince, logAt: m.logAt, logStick: m.logStick, logID: m.logID, status: m.status, kind: m.kind, subject: m.subject, dback: m.dback,
		kubeconfig: m.kubeconfig, context: m.context, namespace: m.namespace, load: m.load, ran: m.ran, ed: m.ed, vs: m.vs,
	}
}

func (m *model) restore(s paneSnap) {
	m.content, m.portInput, m.stage = s.content, s.portInput, s.stage
	m.pod, m.localPort, m.remotePort = s.pod, s.localPort, s.remotePort
	m.suggestions, m.sugIndex = s.suggestions, s.sugIndex
	m.load, m.ran = s.load, s.ran
	m.logPod, m.logText, m.logBusy, m.logSince, m.logAt, m.logStick, m.logID, m.status, m.kind, m.subject = s.logPod, s.logText, s.logBusy, s.logSince, s.logAt, s.logStick, s.logID, s.status, s.kind, s.subject
	if s.kubeconfig != "" || s.context != "" || s.namespace != "" {
		m.kubeconfig, m.context, m.namespace = s.kubeconfig, s.context, s.namespace
	}
	m.dback = s.dback
	m.ed = s.ed
	m.vs = s.vs
}

func freshPane(keys config.Keys) paneSnap {
	content := list.New(nil, contentDelegate(40), 40, 12)
	content.KeyMap = listKeyMap(keys)
	content.SetShowStatusBar(false)
	content.SetFilteringEnabled(true)
	content.SetShowHelp(false)
	content.Styles.Title = titleStyle
	ti := textinput.New()
	ti.Placeholder = "8080:80"
	ti.CharLimit = 32
	ti.Width = 24
	ti.PromptStyle = keyStyle
	ti.TextStyle = itemStyle
	return paneSnap{content: content, portInput: ti, stage: "nav", status: ":", logID: newPaneID()}
}

func (m *model) commit() {
	if len(m.panes) == 0 {
		m.panes = []paneSnap{m.snap()}
		m.focus = 0
		return
	}
	if m.focus < 0 || m.focus >= len(m.panes) {
		m.focus = 0
	}
	m.panes[m.focus] = m.snap()
}

func (m *model) focusPane(i int) {
	if len(m.panes) == 0 || i < 0 || i >= len(m.panes) || i == m.focus {
		return
	}
	m.commit()
	m.focus = i
	m.restore(m.panes[i])
}

func (m model) tableWidth() int { return m.paneTableWidth(m.logID) }

func (m model) paneTableWidth(id string) int {
	w, _ := m.paneBox(id)
	return max(w-4, 24)
}

func (m *model) fitContent(mainW, panelH int) {
	_, _ = mainW, panelH
	set := func(id, stage string, lst *list.Model) {
		w, h := m.paneBox(id)
		listW, listH := max(w-4, 10), max(h-3, 3)
		lst.SetSize(listW, listH)
		if stage == "workpods" || stage == "res" || stage == "xray" || stage == "helm" || stage == "helmhist" {
			lst.SetShowTitle(false)
			lst.SetDelegate(rowDelegate{width: listW})
			reflowList(lst, listW)
			return
		}
		if stage == "kdir" || stage == "kust" {
			lst.SetShowTitle(true)
			lst.SetDelegate(dirDelegate{width: listW})
			return
		}
		if stage == "fwds" {
			lst.SetShowTitle(false)
			lst.SetDelegate(fwdDelegate{width: listW})
			return
		}
		lst.SetShowTitle(true)
		lst.SetDelegate(contentDelegate(listW))
	}
	set(m.logID, m.stage, &m.content)
	for i := range m.panes {
		if i != m.focus {
			set(m.panes[i].logID, m.panes[i].stage, &m.panes[i].content)
		}
	}
}

func (m model) layout() (sideW, mainW, panelH int) {
	w, h := m.width, m.height
	if w < 1 {
		w = 100
	}
	if h < 1 {
		h = 24
	}
	if m.zoom {
		sideW = 0
		mainW = w - 2
		if mainW < 16 {
			mainW = 16
		}
		panelH = h - 1 - m.tabBarRows()
		if panelH < 8 {
			panelH = 8
		}
		return
	}
	sideW = m.cfg.UI.SidebarWidth
	chrome := sideW + 4
	if w-chrome < 36 {
		sideW = m.cfg.UI.SidebarNarrowWidth
		chrome = sideW + 4
	}
	mainW = w - chrome
	if mainW < 16 {
		mainW = 16
	}
	panelH = h - 3 - m.tabBarRows()
	if panelH < 8 {
		panelH = 8
	}
	return
}

func (m model) snapBody(s paneSnap, innerW, innerH int) string {
	alt := m
	alt.content = s.content
	alt.portInput = s.portInput
	alt.stage = s.stage
	alt.kind = s.kind
	alt.kubeconfig = s.kubeconfig
	alt.context = s.context
	alt.namespace = s.namespace
	alt.pod = s.pod
	alt.logPod = s.logPod
	alt.logText = s.logText
	alt.logAt = s.logAt
	alt.logStick = s.logStick
	alt.status = s.status
	alt.load, alt.ran = s.load, s.ran
	alt.vs = s.vs
	alt.logID = s.logID
	alt.boxH = innerH
	return alt.rightBody(innerW)
}

func logLabel(pod string) string {
	name := pod
	if i := strings.LastIndex(pod, "/"); i >= 0 && i < len(pod)-1 {
		name = pod[i+1:]
	}
	if name == "" {
		return "logs"
	}
	return name + "/logs"
}

func scopeLabel(ns, kind string) string {
	if ns == "" {
		ns = "all"
	}
	return ns + "/" + kind
}

func (m model) paneLabel(i int) string {
	s := m.snap()
	if i != m.focus && i < len(m.panes) {
		s = m.panes[i]
	}
	if s.ed != nil {
		return s.ed.title(m.cfg.Keys.Prefix.Label())
	}
	label := "home"
	switch s.stage {
	case "nav", "":
		label = "home"
	case "logs":
		label = logLabel(s.logPod)
		if s.vs.logCont != "" {
			label += " · " + s.vs.logCont
		}
		if s.vs.logPrev {
			label += " · previous"
		}
	case "kdir", "kust":
		label = "kustomize"
	case "kcmd":
		label = "kubectl"
	case "kout":
		label = or(s.subject, "kustomize")
	case "workpods":
		label = scopeLabel(s.namespace, "pods")
		if s.vs.podTitle != "" {
			label += " of " + s.vs.podTitle
		}
	case "res":
		label = scopeLabel(s.namespace, or(s.kind, "resource"))
	case "xray":
		label = scopeLabel(s.namespace, "xray")
		if x := s.vs.xray; x != nil {
			label = scopeLabel(s.namespace, "xray "+x.kind)
			if x.name != "" {
				label += "/" + x.name
			}
		}
	case "helm":
		ns := s.namespace
		if s.vs.helmAll {
			ns = ""
		}
		label = scopeLabel(ns, "helm")
	case "helmhist":
		label = "helm history"
		if h := s.vs.hist; h != nil {
			label = h.ns + "/" + h.name + " · history"
			if h.mark != 0 {
				label += fmt.Sprintf(" · ◆ %d", h.mark)
			}
		}
	case "hdiff":
		label = or(s.subject, "diff")
	case "pulse":
		label = scopeLabel(s.namespace, "pulse")
	case "mlogs":
		label = "logs"
		if v := s.vs.ml; v != nil {
			label = "logs · " + v.title
			if v.filter != "" {
				label += " · /" + v.filter
			}
		}
	case "yaml":
		label = or(s.subject, s.kind)
	case "describe":
		label = "describe " + s.subject
	case "fwds":
		label = "forwards"
	case "projects":
		label = "namespace"
	case "contexts":
		label = "context"
	case "files":
		label = "kubeconfig"
	default:
		label = s.stage
	}
	if s.stage == "workpods" || s.stage == "res" || s.stage == "fwds" || s.stage == "xray" || s.stage == "helm" || s.stage == "helmhist" {

		if f := s.content.FilterValue(); f != "" && s.content.FilterState() != list.Unfiltered {
			label += " · /" + f
		}
	}
	if s.load.active() {
		label += " · " + m.loadLabel(s.load)
	}
	return label
}

func (m model) dangerous() bool {
	return m.cfg.IsProd(m.context)
}

func (m model) paneHot(i int) bool {
	ctx := m.context
	if i != m.focus && i < len(m.panes) {
		ctx = m.panes[i].context
	}
	return m.cfg.IsProd(ctx)
}

func (m model) accent() lipgloss.Color {
	if m.dangerous() {
		return th.Danger
	}
	return th.Accent
}

const skull = "☠"

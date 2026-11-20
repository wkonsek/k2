package ui

import (
	"fmt"
	"strconv"
	"strings"

	"k2/internal/kube"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

const containerSelPrefix = "containers:"

type podSort struct {
	col   string
	desc  bool
	marks map[string]bool
}

func containerSel(ns, pod string) string { return containerSelPrefix + ns + "/" + pod }

func parseContainerSel(sel string) (ns, pod string, ok bool) {
	rest, ok := strings.CutPrefix(sel, containerSelPrefix)
	if !ok {
		return "", "", false
	}
	ns, pod, ok = strings.Cut(rest, "/")
	return ns, pod, ok && pod != ""
}

func (m model) inContainers() bool {
	if m.stage != "workpods" {
		return false
	}
	_, _, ok := parseContainerSel(m.vs.podSel)
	return ok
}

func containersLabel(ns, pod string, n int) string {
	return fmt.Sprintf("Containers(%s/%s)[%d]", ns, pod, n)
}

func isContainerHeads(heads []string) bool {
	return len(heads) > 4 && heads[0] == "NAME" && heads[1] == "PF" && heads[2] == "IMAGE"
}

func getContainerItems(k kube.Client, kubeconfig, context, ns, pod string, width int, metrics bool) (string, []list.Item, error) {
	o, err := k.Object(kubeconfig, context, "pods", ns, pod)
	if err != nil {
		return "", nil, err
	}
	var usage map[string]kube.Usage
	ok := false
	if metrics {
		usage, err = k.TopContainers(kubeconfig, context, ns, pod)
		ok = err == nil
	}
	return packKube(kube.ContainerTable(o, usage, ok), width, ns)
}

func (m model) openContainers() (tea.Model, tea.Cmd) {
	it, ok := m.selectedItem()
	if !ok {
		return m, nil
	}
	ns, name := kube.SplitRef(it.id, m.namespace)
	if it.desc != "" {
		ns = it.desc
	}
	if ns == "" || name == "" {
		return m, nil
	}
	m.vs.podSelBack, m.vs.podTitleBack = m.vs.podSel, m.vs.podTitle
	m.vs.podSel, m.vs.podTitle = containerSel(ns, name), ""
	m.vs.sortBack = podSort{col: m.vs.sortCol, desc: m.vs.sortDesc, marks: m.vs.marks}
	m.vs.sortCol, m.vs.sortDesc, m.vs.marks = "", false, nil
	m.vs.pick = ""
	return m.openList("workpods", "pods")
}

func (m model) backToPods() (tea.Model, tea.Cmd) {
	ns, pod, _ := parseContainerSel(m.vs.podSel)
	m.vs.podSel, m.vs.podTitle = m.vs.podSelBack, m.vs.podTitleBack
	m.vs.podSelBack, m.vs.podTitleBack = "", ""
	m.vs.sortCol, m.vs.sortDesc, m.vs.marks = m.vs.sortBack.col, m.vs.sortBack.desc, m.vs.sortBack.marks
	m.vs.sortBack = podSort{}
	m.vs.pick = pod
	if m.namespace == "" {
		m.vs.pick = ns + "/" + pod
	}
	return m.openList("workpods", "pods")
}

func (m model) containersHint() string {
	k := m.cfg.Keys
	return k.Select.Label() + " logs, " + k.Previous.Label() + " previous, " + k.Shell.Label() + " shell, " + podForwardKey(k) + " forward, " + k.Describe.Label() + " describe pod, " + k.Back.Label() + " pods"
}

func (m model) containerRef() (ns, pod, container string, ok bool) {
	ns, pod, ok = parseContainerSel(m.vs.podSel)
	if !ok {
		return "", "", "", false
	}
	it, has := m.selectedItem()
	if !has {
		return "", "", "", false
	}
	name := it.extra["container"]
	return ns, pod, name, name != ""
}

func (m model) containerKey(key string) (tea.Model, tea.Cmd, bool) {
	if !m.inContainers() {
		return m, nil, false
	}
	km := m.cfg.Keys
	ns, pod, container, ok := m.containerRef()
	switch {
	case km.Select.Has(key), km.PodLogs.Has(key), km.Previous.Has(key):
		if !ok {
			return m, nil, true
		}
		prev := km.Previous.Has(key) && !km.PodLogs.Has(key) && !km.Select.Has(key)
		next, cmd := m.openLogs(ns+"/"+pod, container, prev)
		return next, cmd, true
	case km.Shell.Has(key):
		if next, cmd, blocked := m.readOnlyBlock("shell"); blocked {
			return next, cmd, true
		}
		if !ok {
			return m, nil, true
		}
		next, cmd := m.openShell(ns, pod, container)
		return next, cmd, true
	case km.PortForward.Has(key):
		if next, cmd, blocked := m.readOnlyBlock("port-forward"); blocked {
			return next, cmd, true
		}
		if !ok {
			return m, nil, true
		}
		next, cmd := m.openPortForward()
		return next, cmd, true
	case km.Describe.Has(key):
		next, cmd := m.describeSelected()
		return next, cmd, true
	case km.Delete.Has(key), km.Scale.Has(key), km.SetImage.Has(key), km.Restart.Has(key), km.Trigger.Has(key),
		km.Label.Has(key), km.Annotate.Has(key), km.Edit.Has(key), km.Decode.Has(key), km.Xray.Has(key),
		km.MultiLogs.Has(key), km.AllNamespaces.Has(key), km.Approve.Has(key):
		m.status = "that works on pods: " + km.Back.Label() + " goes back to the pod list"
		return m, m.notify(toneWarn, "not for containers"), true
	}
	return m, nil, false
}

func (m model) paintContainers(header string, items []list.Item, width int) (string, []list.Item) {
	warn, crit := m.cfg.UI.MetricsWarn, m.cfg.UI.MetricsCrit
	out := make([]list.Item, len(items))
	var heads []string
	var widths []int
	for i, raw := range items {
		it, ok := raw.(item)
		if !ok || !isContainerHeads(it.heads) {
			out[i] = raw
			continue
		}
		cells := append([]string(nil), it.cells...)
		tones := map[int]tone{}
		if m.containerForwarded(it) {
			cells[1] = "●"
			tones[1] = toneOK
		}
		state := strings.TrimSpace(cells[4])
		for c, h := range it.heads {
			if c >= len(cells) {
				break
			}
			v := strings.TrimSpace(cells[c])
			switch h {
			case "READY":
				if v == "false" && statusTone(state) != toneDone && state != "Pending" {
					tones[c] = toneWarn
				}
			case "CPU", "MEM", "%CPU/R", "%MEM/R", "CPU/R:L", "MEM/R:L":
				if v == "n/a" || v == "-" {
					tones[c] = toneMuted
				}
			case "%CPU/L", "%MEM/L":
				if v == "n/a" || v == "-" {
					tones[c] = toneMuted
				} else if p, err := strconv.Atoi(v); err == nil {
					if t := thresholdTone(float64(p), warn, crit); t != toneOK {
						tones[c] = t
					}
				}
			case "PROBES(L:R)":
				if v == "off:off" {
					tones[c] = toneMuted
				}
			case "INIT":
				if v == "false" {
					tones[c] = toneMuted
				}
			}
		}
		it.cells, it.tones = cells, tones
		w := fitWidths(it.natural, it.heads, it.flex, it.ageIdx, width)
		it.widths = w
		it.title, it.plain = paintRow(it.cells, w, it.heads, it.colorCol, it.flex, it.dimCol, it.ageIdx, it.tones)
		heads, widths = it.heads, w
		out[i] = it
	}
	if heads != nil {
		header = headerLine(heads, widths)
	}
	return header, out
}

func (m model) containerForwarded(it item) bool {
	ns, pod, ok := parseContainerSel(m.vs.podSel)
	if !ok {
		return false
	}
	ports := strings.Split(it.extra["ports"], ",")
	for _, f := range m.forwards {
		if f.Namespace != ns || f.Pod != pod || (f.Kind != "" && f.Kind != "pod") || f.Context != m.context {
			continue
		}
		for _, p := range ports {
			if p != "" && p == f.Remote {
				return true
			}
		}
	}
	return false
}

func containerPortsFirst(sug []kube.PortSuggestion, ports string) []kube.PortSuggestion {
	own := map[string]bool{}
	for _, p := range strings.Split(ports, ",") {
		if p != "" {
			own[p] = true
		}
	}
	var first, rest []kube.PortSuggestion
	for _, s := range sug {
		if own[s.Remote] {
			first = append(first, s)
		} else {
			rest = append(rest, s)
		}
	}
	return append(first, rest...)
}

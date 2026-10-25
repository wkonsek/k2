package ui

import (
	"strings"

	"k2/internal/kube"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

var clusterKinds = map[string]bool{
	"nodes": true, "node": true, "no": true, "namespaces": true, "namespace": true, "ns": true,
	"persistentvolumes": true, "pv": true, "storageclasses": true, "sc": true,
	"customresourcedefinitions": true, "crd": true, "crds": true,
	"clusterroles": true, "clusterrolebindings": true, "priorityclasses": true, "pc": true,
	"ingressclasses": true, "runtimeclasses": true, "csidrivers": true, "csinodes": true,
	"volumesnapshotclasses": true, "certificatesigningrequests": true, "csr": true,
	"mutatingwebhookconfigurations": true, "validatingwebhookconfigurations": true,
	"apiservices": true, "projects": true, "clusterversions": true, "clusteroperators": true, "co": true,
	"clusterserviceversions.operators.coreos.com/cluster": true,
}

func (m model) clusterScoped() bool {
	if m.stage != "res" {
		return false
	}
	if clusterKinds[strings.ToLower(kube.BaseKind(m.kind))] || clusterKinds[strings.ToLower(m.kind)] {
		return true
	}
	rows := m.content.Items()
	if len(rows) == 0 {
		return false
	}
	for _, raw := range rows {
		if it, ok := raw.(item); ok && it.desc != "" {
			return false
		}
	}
	return true
}

func (m model) nsTarget() (ns, prev string, ok bool) {
	if m.namespace != "" {
		return "", m.namespace, true
	}
	if m.vs.nsPrev != "" {
		return m.vs.nsPrev, "", true
	}
	return "", "", false
}

func pickAfter(it item, ns string) string {
	name := it.id
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if ns == "" {
		if it.desc == "" {
			return name
		}
		return it.desc + "/" + name
	}
	if it.desc == ns || it.desc == "" {
		return name
	}
	return ""
}

func (m model) toggleAllNS() (tea.Model, tea.Cmd) {
	key := m.cfg.Keys.AllNamespaces.Label()
	switch {
	case m.stage == "pulse":
		m.status = "the pulse reads one namespace (" + m.cfg.Keys.Namespace.Label() + " picks another)"
		return m, m.notify(toneWarn, "pulse: one namespace at a time")
	case m.clusterScoped():
		m.status = m.kind + " are cluster-scoped: no namespaces to switch"
		return m, m.notify(toneWarn, "cluster-scoped")
	case (m.stage == "workpods" || m.stage == "res") && m.vs.podSel != "":
		m.status = "the pods of " + or(m.vs.podTitle, "a workload") + " are in its namespace"
		return m, m.notify(toneWarn, "pods of one workload: one namespace")
	case m.stage == "xray" && m.vs.xray != nil && m.vs.xray.name != "":
		m.status = "the tree of one object stays in its namespace (:xray " + m.vs.xray.kind + " lists all)"
		return m, m.notify(toneWarn, "tree of one object: one namespace")
	}
	ns, prev, ok := m.nsTarget()
	if !ok {
		m.status = "already all namespaces (" + m.cfg.Keys.Namespace.Label() + " picks one)"
		return m, m.notify(toneWarn, "already all namespaces")
	}
	m.namespace, m.vs.nsPrev = ns, prev
	m.save()
	where := or(ns, "all namespaces")
	switch m.stage {
	case "workpods", "res":
		if m.content.FilterState() != list.Unfiltered {
			m.content.ResetFilter()
		}
		m.vs.pick = ""
		if it, ok := m.selectedItem(); ok {
			m.vs.pick = pickAfter(it, ns)
		}
		next, cmd := m.openListToggled(m.stage, m.kind)
		if nm, ok := next.(model); ok {
			nm.status = where + " · " + key + " again for " + or(prev, "the namespace before")
			if ns != "" {
				nm.status = where + " · " + key + " for all namespaces"
			}
			return nm, cmd
		}
		return next, cmd
	case "xray":
		x := m.vs.xray
		m.vs.pick = ""
		if it, ok := m.selectedItem(); ok {
			m.vs.pick = it.id
		}
		return m.openXray(x.kind, "")
	case "mlogs":
		v := m.vs.ml
		if v == nil {
			return m, nil
		}

		next, cmd := m.openMultiLogs(v.q, v.title, v.from)
		if nm, ok := next.(model); ok {
			nm.status = "logs of " + v.title + " in " + where
			return nm, cmd
		}
		return next, cmd
	}
	return m, nil
}

func (m model) emptyHint() string {
	press := " — press " + m.cfg.Keys.AllNamespaces.Label() + " for all namespaces"
	what, ns := "", m.namespace
	switch m.stage {
	case "helm":
		what, ns = "releases", m.helmNS()
	case "xray":
		if m.vs.xray == nil || len(m.vs.xray.roots) > 0 {
			return ""
		}
		what = m.vs.xray.kind
		if m.vs.xray.name != "" {
			return "no " + what + "/" + m.vs.xray.name + " in " + or(ns, "all namespaces")
		}
	case "workpods":
		what = "pods"
	case "res":
		if m.clusterScoped() {
			return "no " + m.kind
		}
		what = m.kind
	default:
		return ""
	}
	if ns == "" {
		return "no " + what + " in all namespaces"
	}
	return "no " + what + " in " + ns + press
}

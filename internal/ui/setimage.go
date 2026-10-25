package ui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

var setImageKinds = map[string]bool{
	"pods": true, "deployments": true, "statefulsets": true, "daemonsets": true,
	"replicasets": true, "cronjobs": true,
}

type imageField struct {
	name  string
	init  bool
	old   string
	input textinput.Model
}

type imagesMsg struct {
	seq   int
	items []imageField
	err   error
}

func podSpecOf(obj map[string]any) map[string]any {
	dig := func(m map[string]any, path ...string) map[string]any {
		for _, p := range path {
			next, ok := m[p].(map[string]any)
			if !ok {
				return nil
			}
			m = next
		}
		return m
	}
	switch strings.ToLower(fmt.Sprint(obj["kind"])) {
	case "pod":
		return dig(obj, "spec")
	case "cronjob":
		return dig(obj, "spec", "jobTemplate", "spec", "template", "spec")
	}
	return dig(obj, "spec", "template", "spec")
}

func containerImages(obj map[string]any) []imageField {
	spec := podSpecOf(obj)
	var out []imageField
	for _, list := range []struct {
		key  string
		init bool
	}{{"containers", false}, {"initContainers", true}} {
		items, _ := spec[list.key].([]any)
		for _, c := range items {
			cm, _ := c.(map[string]any)
			name, _ := cm["name"].(string)
			image, _ := cm["image"].(string)
			if name != "" {
				out = append(out, imageField{name: name, init: list.init, old: image})
			}
		}
	}
	return out
}

func (m model) openSetImage(kind, ns, name string) (tea.Model, tea.Cmd) {
	d := m.newDialog("setimage", "⧉ set image "+kind+"/"+name, kind, ns, name)
	d.msg = "reading containers…"
	m.dlgSeq++
	d.seq = m.dlgSeq
	m.dlg = d
	k, kc, ctx, seq := m.k, m.kubeconfig, m.context, d.seq
	return m, func() tea.Msg {
		obj, err := k.GetJSON(kc, ctx, "get", kind+"/"+name, "-n", ns)
		if err != nil {
			return imagesMsg{seq: seq, err: err}
		}
		items := containerImages(obj)
		if len(items) == 0 {
			return imagesMsg{seq: seq, err: errors.New(kind + "/" + name + " has no containers")}
		}
		return imagesMsg{seq: seq, items: items}
	}
}

func (m model) applyImages(msg imagesMsg) (tea.Model, tea.Cmd) {
	if m.dlg == nil || m.dlg.act != "setimage" || m.dlg.seq != msg.seq {
		return m, nil
	}
	if msg.err != nil {
		m.dlg = nil
		m.status = m.errLine(fmt.Errorf("set image: %w", msg.err))
		return m, m.notify(toneErr, "set image failed")
	}
	d := *m.dlg
	d.msg = ""
	d.images = make([]imageField, len(msg.items))
	for i, f := range msg.items {
		ti := textinput.New()
		ti.Prompt = "› "
		ti.PromptStyle, ti.TextStyle = keyStyle, itemStyle
		ti.CharLimit = 512
		ti.SetValue(f.old)
		ti.CursorEnd()
		f.input = ti
		d.images[i] = f
	}
	d.images[0].input.Focus()
	d.idx = 0
	m.dlg = &d
	return m, nil
}

func (m model) setImageKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	d := m.dlg
	if len(d.images) == 0 {
		return m, nil
	}

	d.images = append([]imageField(nil), d.images...)
	move := 0
	switch msg.String() {
	case "tab", "down":
		move = 1
	case "shift+tab", "up":
		move = -1
	}
	if move != 0 {
		d.images[d.idx].input.Blur()
		d.idx = (d.idx + move + len(d.images)) % len(d.images)
		d.images[d.idx].input.Focus()
		return m, nil
	}
	var cmd tea.Cmd
	d.images[d.idx].input, cmd = d.images[d.idx].input.Update(msg)
	d.msg = ""
	return m, cmd
}

func (m model) submitSetImage() (tea.Model, tea.Cmd) {
	d := m.dlg
	if len(d.images) == 0 {
		return m, nil
	}
	var pairs, changes []string
	for _, f := range d.images {
		v := strings.TrimSpace(f.input.Value())
		if v == "" {
			d.msg = "the image of " + f.name + " is empty"
			return m, nil
		}
		if strings.ContainsAny(v, " \t") {
			d.msg = "the image of " + f.name + " has a space"
			return m, nil
		}
		if v == f.old {
			continue
		}
		pairs = append(pairs, f.name+"="+v)
		label := f.name
		if f.init {
			label += " (init)"
		}
		changes = append(changes, label+": "+f.old+" → "+v)
	}
	if len(pairs) == 0 {
		d.msg = "nothing changed"
		return m, nil
	}
	ref := d.kind + "/" + d.name
	args := append([]string{"set", "image", ref, "-n", d.ns}, pairs...)
	cmd := m.kubectlAction("set image "+ref, args...)
	d.details = append([]string{target(d.kind, d.ns, d.name), m.contextLine()}, changes...)
	d.ask = fmt.Sprintf("set %d image(s) on %s?", len(pairs), ref)
	d.pending, d.pendMsg = cmd, "setting image…"
	return m, nil
}

func (d *dialog) imageRows(inner int) []string {
	nameW := 0
	for _, f := range d.images {
		nameW = max(nameW, len(f.name))
	}
	nameW = min(nameW, max(8, inner/3))
	var out []string
	for i, f := range d.images {
		mark, st := "  ", dimStyle
		if i == d.idx {
			mark, st = "▸ ", keyStyle
		}
		note := "      "
		if f.init {
			note = " init "
		}
		label := st.Render(mark+fitWidth(f.name, nameW)+strings.Repeat(" ", max(0, nameW-len(f.name)))) + dimStyle.Render(note)
		in := f.input
		in.Width = max(8, inner-2-nameW-6-3)
		out = append(out, label+in.View())
	}
	return out
}

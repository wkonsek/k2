package ui

import (
	"fmt"
	"strconv"
	"strings"

	"k2/internal/forward"
	"k2/internal/kube"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func (m model) openPortForward() (tea.Model, tea.Cmd) {
	kind, ns, name, ok := m.selectedRef()
	if !ok {
		return m, nil
	}
	fk := kube.ForwardKind(kind)
	if fk == "" {
		m.status = kind + " cannot be forwarded (pods, services, deployments, statefulsets)"
		return m, nil
	}
	if m.kubeconfig == "" || m.context == "" {
		m.status = "select kubeconfig and context first"
		return m, nil
	}
	if ns == "" {
		ns = "default"
	}
	m.pod, m.pfKind = ns+"/"+name, fk
	m.suggestions = m.k.TargetPortSuggestions(m.kubeconfig, m.context, fk, ns, name)
	m.sugIndex = 0
	m.portInput.SetValue("")
	if len(m.suggestions) > 0 {
		m.portInput.SetValue(m.suggestions[0].Spec())
	}
	m.portInput.CursorEnd()
	m.portInput.Focus()
	m.pfOn, m.pfBusy, m.pfMsg, m.pfWait = true, "", "", false
	return m, nil
}

func (m *model) closePopup() {
	m.pfOn, m.pfBusy, m.pfMsg, m.pfWait = false, "", "", false
	m.portInput.Blur()
}

func (m model) pfTarget() string {
	_, name := kube.SplitRef(m.pod, "")
	return forward.KindOr(m.pfKind) + "/" + name
}

func (m model) updatePopup(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	km := m.cfg.Keys
	key := msg.String()
	if !printable(msg) && km.Quit.Has(key) {
		m.killEditors()
		m.quitting = true
		return m, tea.Quit
	}
	if m.pfWait {
		if km.Cancel.Has(key) {

			m.closePopup()
		}
		return m, nil
	}
	if m.pfBusy != "" {
		switch {
		case km.Yes.Has(key):
			m.pfBusy, m.pfWait = "", true
			m.pfMsg = "replacing " + m.localPort + "…"
			m.dropLocal(m.localPort)
			m.save()
			ns, name := kube.SplitRef(m.pod, "")
			return m, replaceForwardCmd(m.k, m.logDir(), m.kubeconfig, m.context, ns, m.pfKind, name, m.localPort, m.remotePort)
		case km.No.Has(key):
			m.pfBusy = ""
			m.pfMsg = "kept the port; pick another"
		}
		return m, nil
	}
	switch {
	case km.Cancel.Has(key):
		m.closePopup()
		m.status = "port-forward cancelled"
		return m, nil
	case km.Submit.Has(key):
		return m.submitPopup()
	case km.Complete.Has(key) || !printable(msg) && km.ScrollDown.Has(key):
		m.cycleSuggestion(1)
		return m, nil
	case key == "shift+tab" || !printable(msg) && km.ScrollUp.Has(key):
		m.cycleSuggestion(-1)
		return m, nil
	}
	var cmd tea.Cmd
	m.portInput, cmd = m.portInput.Update(msg)
	m.pfMsg = ""
	return m, cmd
}

func (m *model) cycleSuggestion(step int) {
	n := len(m.suggestions)
	if n == 0 {
		return
	}
	m.sugIndex = (m.sugIndex + step + n) % n
	m.portInput.SetValue(m.suggestions[m.sugIndex].Spec())
	m.portInput.CursorEnd()
	m.pfMsg = ""
}

func parsePorts(v string) (local, remote string, ok bool) {
	v = strings.TrimSpace(v)
	local, remote, found := strings.Cut(v, ":")
	if !found {
		remote = local
	}
	local, remote = strings.TrimSpace(local), strings.TrimSpace(remote)
	for _, p := range []string{local, remote} {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return "", "", false
		}
	}
	return local, remote, true
}

func (m model) submitPopup() (tea.Model, tea.Cmd) {
	local, remote, ok := parsePorts(m.portInput.Value())
	if !ok {
		m.pfMsg = "type local:remote, e.g. 8080:80"
		return m, nil
	}
	m.localPort, m.remotePort = local, remote
	for _, f := range m.forwards {
		if f.Local == local && forward.Up(f) {
			m.pfBusy = "replace"
			m.pfMsg = fmt.Sprintf("port %s already forwards %s. replace it?  %s", local, forward.Target(f), m.yn())
			return m, nil
		}
	}
	m.pfWait = true
	m.pfMsg = "starting…"
	ns, name := kube.SplitRef(m.pod, "")
	return m, spawnForwardCmd(m.k, m.logDir(), m.kubeconfig, m.context, ns, m.pfKind, name, local, remote)
}

func (m model) applyForward(msg fwdMsg) (tea.Model, tea.Cmd) {
	m.pfWait = false
	if msg.err != nil {
		errText := m.errLine(msg.err)
		if m.pfOn && !msg.retried && strings.Contains(strings.ToLower(errText), "address already in use") {
			m.pfBusy = "inuse"
			m.pfMsg = "port " + m.localPort + " is in use. kill it and forward again?  " + m.yn()
			return m, nil
		}
		m.status = errText
		if m.pfOn {
			m.pfMsg = errText
		}
		return m, m.notify(toneErr, "forward failed")
	}
	m.forwards = append(m.forwards, msg.f)
	m.save()
	m.closePopup()
	what := msg.f.Local + " → " + forward.Target(msg.f)
	m.status = "forwarding " + what
	if m.stage == "fwds" {
		m.content.SetItems(m.forwardItems())
	}

	return m, tea.Batch(m.notify(toneOK, "forward "+what), ownerCmd(m.k, msg.f))
}

func (m model) popupBox(width int) string {
	inner := width - 4
	hot := m.cfg.IsProd(m.context)
	title := "⇄ port-forward " + m.pfTarget()
	if hot {
		title = skull + " " + title
	}
	var lines []string
	where := m.context
	if ns, _ := kube.SplitRef(m.pod, ""); ns != "" {
		where += " · " + ns
	}
	lines = append(lines, dimStyle.Render(fitWidth(where, inner)), "", dimStyle.Render("local:remote"), m.portInput.View(), "")
	if len(m.suggestions) == 0 {
		lines = append(lines, dimStyle.Render(fitWidth("no ports found, type local:remote", inner)))
	}
	for i, s := range m.suggestions {
		text := fitWidth(fmt.Sprintf("%-13s %s", s.Spec(), s.Source), inner-2)
		if i == m.sugIndex {
			lines = append(lines, selectedItemStyle.Render("▸ "+text))
		} else {
			lines = append(lines, itemStyle.Render("  "+text))
		}
	}
	lines = append(lines, "")
	k := m.cfg.Keys
	switch {
	case m.pfBusy != "":

		lines = append(lines, errStyle.Render(m.pfMsg), dimStyle.Render("default no · enter does nothing"))
	case m.pfMsg != "":
		st := dimStyle
		if !m.pfWait {
			st = errStyle
		}
		lines = append(lines, st.Render(m.pfMsg))
	default:
		lines = append(lines, keyStyle.Render(k.Complete.Short())+dimStyle.Render(" next  ")+keyStyle.Render(k.Submit.Short())+
			dimStyle.Render(" start  ")+keyStyle.Render(k.Cancel.Short())+dimStyle.Render(" cancel"))
	}
	body := make([]string, 0, len(lines))
	for _, l := range lines {
		body = append(body, strings.Split(ansi.Hardwrap(l, inner, true), "\n")...)
	}
	return drawBox(strings.Join(body, "\n"), width, len(body)+2, true, title, "", hot)
}

func (m model) popupRect() (x, y, w int) {
	ox, oy := 0, m.tabBarRows()
	r := m.paneArea()
	if !m.zoom {
		sideW, _, _ := m.layout()
		ox, oy = sideW+3, oy+1
		if pr, ok := m.rects(m.tab)[m.logID]; ok {
			r = pr
		}
	}
	w = min(max(36, r.W*2/3), max(r.W-2, 20), 64)
	h := 12 + len(m.suggestions)
	x = ox + r.X + (r.W-w)/2
	y = oy + r.Y + max(0, (r.H-h)/2)
	return max(x, 0), max(y, 0), w
}

func overlay(screen, box string, x, y int) string {
	lines := strings.Split(screen, "\n")
	for i, b := range strings.Split(box, "\n") {
		row := y + i
		if row < 0 || row >= len(lines) {
			continue
		}
		base := lines[row]
		bw := ansi.StringWidth(b)
		left := ansi.Truncate(base, x, "")
		if pad := x - ansi.StringWidth(left); pad > 0 {
			left += strings.Repeat(" ", pad)
		}
		right := ansi.TruncateLeft(base, x+bw, "")
		reset := ""
		if strings.Contains(base, "\x1b[") || strings.Contains(b, "\x1b[") {
			reset = "\x1b[0m"
		}
		lines[row] = left + reset + b + reset + right
	}
	return strings.Join(lines, "\n")
}

func (m model) withPopup(screen string) string {
	if !m.pfOn {
		return screen
	}
	x, y, w := m.popupRect()
	return overlay(screen, lipgloss.NewStyle().Render(m.popupBox(w)), x, y)
}

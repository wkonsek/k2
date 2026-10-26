package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"k2/internal/config"
	"k2/internal/forward"
	"k2/internal/kube"
	"k2/internal/shell"

	tea "github.com/charmbracelet/bubbletea"
)

const backoffReset = 30 * time.Second

type pfAuto struct {
	attempts int
	next     time.Time
	busy     bool
	gaveUp   bool
	upSince  time.Time
}

type pfRestartMsg struct {
	old      int
	oldStart time.Time
	f        forward.Forward
	note     string
	err      error
	auto     bool
	pane     string
	token    int
}

type pfOwnerMsg struct {
	pid   int
	owner string
}

func restartForwardCmd(k kube.Client, logDir string, f forward.Forward, auto bool, pane string, token int) tea.Cmd {
	return func() tea.Msg {
		nf, note, err := forward.Restart(k.Run, logDir, f)
		return pfRestartMsg{old: f.PID, oldStart: f.Started, f: nf, note: note, err: err, auto: auto, pane: pane, token: token}
	}
}

func ownerCmd(k kube.Client, f forward.Forward) tea.Cmd {
	if forward.KindOr(f.Kind) != "pod" || f.Owner != "" {
		return nil
	}
	return func() tea.Msg {
		return pfOwnerMsg{pid: f.PID, owner: forward.OwnerOf(k.Run, f.Kubeconfig, f.Context, or(f.Namespace, "default"), f.Pod)}
	}
}

func (m model) applyOwner(msg pfOwnerMsg) (tea.Model, tea.Cmd) {
	for i := range m.forwards {
		if m.forwards[i].PID == msg.pid && msg.owner != "" {
			m.forwards[i].Owner = msg.owner
			m.save()
		}
	}
	return m, nil
}

func (m model) selectedForward() int {
	sel := m.content.SelectedItem()
	if sel == nil {
		return -1
	}
	pid, err := strconv.Atoi(sel.(item).id)
	if err != nil {
		return -1
	}
	for i, f := range m.forwards {
		if f.PID == pid {
			return i
		}
	}
	return -1
}

func (m model) restartSelected() (tea.Model, tea.Cmd) {
	i := m.selectedForward()
	if i < 0 {
		m.status = "no forward selected"
		return m, nil
	}
	f := m.forwards[i]
	st := m.pfState(f.Local)
	if st.busy {
		m.status = "already restarting " + f.Local
		return m, nil
	}
	st.busy, st.gaveUp = true, false
	what := "restarting " + f.Local + " → " + forward.Target(f)
	m.status = what + "…"
	spin := m.startLoad(what, false)
	return m, tea.Batch(spin, restartForwardCmd(m.k, m.logDir(), f, false, m.logID, m.load.token))
}

func (m model) toggleAuto() (tea.Model, tea.Cmd) {
	i := m.selectedForward()
	if i < 0 {
		m.status = "no forward selected"
		return m, nil
	}
	f := &m.forwards[i]
	if m.autoOn(*f) {
		f.Auto = "off"
	} else {
		f.Auto = "on"
		*m.pfState(f.Local) = pfAuto{}
	}
	m.save()
	m.refreshForwards()
	m.status = fmt.Sprintf("forward %s: auto-restart %s", f.Local, f.Auto)
	return m, nil
}

func wantsAutoRestart(cfg config.Config, f forward.Forward) bool {
	switch f.Auto {
	case "on":
		return true
	case "off":
		return false
	}
	return cfg.PortForward.AutoRestart
}

func resumeAny(cfg config.Config, fs []forward.Forward) bool {
	for _, f := range fs {
		if !forward.Up(f) && wantsAutoRestart(cfg, f) {
			return true
		}
	}
	return false
}

func (m model) autoOn(f forward.Forward) bool {
	return wantsAutoRestart(m.cfg, f)
}

func (m *model) pfState(local string) *pfAuto {
	if m.pfAuto == nil {
		m.pfAuto = map[string]*pfAuto{}
	}
	st := m.pfAuto[local]
	if st == nil {
		st = &pfAuto{}
		m.pfAuto[local] = st
	}
	return st
}

func (m model) backoff(n int) time.Duration {
	d := time.Duration(max(1, m.cfg.PortForward.RestartBackoff)) * time.Second
	for i := 0; i < n && d < config.MaxBackoff*time.Second; i++ {
		d *= 2
	}
	return min(d, config.MaxBackoff*time.Second)
}

func (m *model) autoRestarts() []tea.Cmd {
	if m.resuming {
		return nil
	}
	var cmds []tea.Cmd
	t := now()
	for _, f := range m.forwards {
		st := m.pfState(f.Local)
		if forward.Up(f) {
			if st.upSince.IsZero() {
				st.upSince = t
			}
			if st.attempts > 0 && t.Sub(st.upSince) >= backoffReset {
				st.attempts, st.next = 0, time.Time{}
			}
			continue
		}
		st.upSince = time.Time{}
		if !m.autoOn(f) || st.busy || st.gaveUp {
			continue
		}
		if st.next.IsZero() {
			st.next = t.Add(m.backoff(st.attempts))
			continue
		}
		if t.Before(st.next) {
			continue
		}
		st.busy = true
		st.attempts++
		cmds = append(cmds, restartForwardCmd(m.k, m.logDir(), f, true, "", 0))
	}
	return cmds
}

func (m model) applyRestart(msg pfRestartMsg) (tea.Model, tea.Cmd) {
	idx := -1
	for i, f := range m.forwards {
		if f.PID == msg.old && f.Started.Equal(msg.oldStart) {
			idx = i
		}
	}
	if !msg.auto {

		m.withPane(msg.pane, func() {
			if m.load.token == msg.token {
				m.load = load{}
			}
		})
	}
	if idx < 0 {

		if msg.err == nil {
			forward.Stop(msg.f.PID)
		}
		return m, nil
	}
	local := m.forwards[idx].Local
	st := m.pfState(local)
	st.busy = false
	if msg.err != nil {

		m.forwards[idx].Reason = "restart failed: " + shortErr(msg.err)
		m.noteErr("restart of port-forward " + local + " → " + forward.Display(m.forwards[idx]) + " failed:\n\n" + msg.err.Error())
		m.save()
		if !msg.auto {
			m.status = "restart of " + local + " failed: " + shell.OneLine(msg.err.Error())
			m.refreshForwards()
			return m, m.notify(toneErr, "restart of "+local+" failed: "+shortErr(msg.err))
		}
		st.next = now().Add(m.backoff(st.attempts))
		if max := m.cfg.PortForward.MaxRestarts; max > 0 && st.attempts >= max {
			st.gaveUp = true
			m.status = fmt.Sprintf("pf %s: gave up after %s", local, plural(st.attempts, "restart", "restarts"))
			m.refreshForwards()
			return m, m.notify(toneWarn, m.status)
		}
		m.refreshForwards()
		return m, nil
	}
	if w := m.pfWatch; w != nil {
		delete(w.offsets, msg.old)
	}
	m.forwards[idx] = msg.f
	st.upSince, st.next = now(), time.Time{}
	if !msg.auto {
		st.attempts, st.gaveUp = 0, false
	}
	m.save()
	m.refreshForwards()
	text := "pf " + local + " restarted"
	if msg.auto {
		text = "pf " + local + " restored"
	}
	if msg.note != "" {
		text += " via " + msg.f.Owner
		if strings.HasPrefix(msg.note, "guessed owner") {
			text += " (guessed owner)"
		}
	}
	if msg.note != "" && !msg.auto {
		text = msg.note
	}
	m.status = text
	return m, m.notify(toneOK, text)
}

func (m model) pfStatus(f forward.Forward) string {
	if forward.Up(f) {
		return "alive"
	}
	if st := m.pfAuto[f.Local]; st != nil && !st.gaveUp && (st.busy || (m.autoOn(f) && !st.next.IsZero())) {
		return fmt.Sprintf("restarting (%d)", st.attempts+boolInt(!st.busy))
	}
	return "dead"
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (m *model) refreshForwards() {
	if m.stage == "fwds" {
		idx := m.content.Index()
		m.content.SetItems(m.forwardItems())
		m.content.Select(idx)
	}
}

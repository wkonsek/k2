package ui

import (
	"time"

	"k2/internal/forward"
	"k2/internal/kube"
	"k2/internal/shell"

	tea "github.com/charmbracelet/bubbletea"
)

type pfWatch struct {
	offsets map[int]int64
	probing bool
	next    time.Time
}

type pfProbeMsg struct{ dead map[int]string }

func (m *model) watch() *pfWatch {
	if m.pfWatch == nil {
		m.pfWatch = &pfWatch{offsets: map[int]int64{}}
	}
	return m.pfWatch
}

func (m *model) watchForwards() []tea.Cmd {
	if m.resuming {
		return nil
	}
	w := m.watch()
	changed := false
	var cmds []tea.Cmd
	for i := range m.forwards {
		f := &m.forwards[i]
		if f.Reason != "" {
			continue
		}
		if st := m.pfAuto[f.Local]; st != nil && st.busy {
			continue
		}
		if !forward.Alive(f.PID) {
			if f.Log == "" {
				continue
			}
			reason, line := forward.ExitReason(f.Log)
			cmds = append(cmds, m.markDead(f, reason, line))
			changed = true
			continue
		}
		sc := forward.ScanLog(f.Log, w.offsets[f.PID])
		w.offsets[f.PID] = sc.Offset
		if sc.Via != "" && sc.Via != f.Via && forward.KindOr(f.Kind) != "pod" {
			f.Via = sc.Via
			changed = true
		}
		if sc.Reason != "" {
			forward.Stop(f.PID)
			cmds = append(cmds, m.markDead(f, sc.Reason, sc.Line))
			changed = true
		}
	}
	if changed {
		m.save()
		m.refreshForwards()
	}
	every := m.cfg.PortForward.HealthEvery
	if every <= 0 || w.probing || len(m.forwards) == 0 || now().Before(w.next) {
		return cmds
	}
	w.next = now().Add(time.Duration(every) * time.Second)
	var up []forward.Forward
	for _, f := range m.forwards {
		if forward.Up(f) {
			up = append(up, f)
		}
	}
	if len(up) == 0 {
		return cmds
	}
	w.probing = true
	return append(cmds, probeCmd(m.k, up))
}

func probeCmd(k kube.Client, fs []forward.Forward) tea.Cmd {
	return func() tea.Msg { return pfProbeMsg{dead: forward.Probe(k.Run, fs)} }
}

func (m model) applyProbe(msg pfProbeMsg) (tea.Model, tea.Cmd) {
	m.watch().probing = false
	var cmd tea.Cmd
	for i := range m.forwards {
		f := &m.forwards[i]
		why, ok := msg.dead[f.PID]
		if !ok || !forward.Up(*f) {
			continue
		}
		if st := m.pfAuto[f.Local]; st != nil && st.busy {
			continue
		}
		forward.Stop(f.PID)
		cmd = m.markDead(f, why, "")
	}
	if cmd != nil {
		m.save()
		m.refreshForwards()
	}
	return m, cmd
}

func (m *model) markDead(f *forward.Forward, reason, line string) tea.Cmd {
	f.Reason = reason
	full := "port-forward " + f.Local + " → " + forward.Display(*f) + " (" + or(f.Namespace, "default") + ", " + forward.Where(*f) + ") is dead: " + reason
	if line != "" {
		full += "\n\nkubectl said: " + line
	}
	if m.autoOn(*f) {
		full += "\n\nportforward.auto_restart restarts it" + ownerHint(*f) + "."
	} else {
		full += "\n\n" + m.cfg.Keys.PfRestart.Label() + " in the forwards list restarts it" + ownerHint(*f) + "."
	}
	m.noteErr(full)
	m.status = "pf " + f.Local + " dead · " + reason
	return m.notify(toneErr, m.status)
}

func ownerHint(f forward.Forward) string {
	if forward.KindOr(f.Kind) == "pod" && f.Owner != "" {
		return " (through " + f.Owner + " when the pod is gone)"
	}
	return ""
}

func shortErr(err error) string {
	s := []rune(shell.OneLine(err.Error()))
	if len(s) > 100 {
		s = append(s[:99], '…')
	}
	return string(s)
}

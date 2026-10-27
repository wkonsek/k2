package ui

import (
	"os"
	"os/exec"
	"path/filepath"

	"k2/internal/config"
	"k2/internal/kube"
	"k2/internal/paneterm"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	if mm, ok := next.(model); ok {
		mm.syncTheme()
		return mm, cmd
	}
	return next, cmd
}

func (m model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if next, cmd, ok := m.screenMsg(msg); ok {
		return next, cmd
	}
	switch msg := msg.(type) {
	case tickMsg:
		m.beat++
		if m.stage == "fwds" && m.content.FilterState() == list.Unfiltered {
			idx := m.content.Index()
			m.content.SetItems(m.forwardItems())
			m.content.Select(idx)
		}
		if m.dlg == nil {
			if m.stage == "workpods" || m.stage == "res" {
				bumpList(&m.content)
			}
			for i := range m.panes {
				if i == m.focus {
					continue
				}
				if m.panes[i].stage == "workpods" || m.panes[i].stage == "res" {
					bumpList(&m.panes[i].content)
				}
			}
		}
		var cmds []tea.Cmd
		cmds = append(cmds, tick(), m.clusterInfoCmd(), m.tokenCmd())
		m.tokenClock()
		cmds = append(cmds, m.watchForwards()...)
		cmds = append(cmds, m.autoRestarts()...)
		cmds = append(cmds, m.metricsCmds()...)

		if m.stage == "logs" && m.logPod != "" && !m.logBusy && !m.vs.logPrev {
			m.logBusy = true
			cmds = append(cmds, fetchLogs(m.k, m.cfg.Logs.Tail, m.logID, m.logPod, m.namespace, m.kubeconfig, m.context, m.logSince, m.vs.logOpts()))
		}
		for i := range m.panes {
			if i == m.focus {
				continue
			}
			p := &m.panes[i]
			if p.stage == "logs" && p.logPod != "" && !p.logBusy && !p.vs.logPrev {
				p.logBusy = true
				cmds = append(cmds, fetchLogs(m.k, m.cfg.Logs.Tail, p.logID, p.logPod, p.namespace, p.kubeconfig, p.context, p.logSince, p.vs.logOpts()))
			}
		}
		if m.beat%max(1, m.cfg.UI.RefreshEvery) == 0 {
			cmds = append(cmds, m.refreshVisible()...)
		}
		cmds = append(cmds, m.screenTick()...)
		return m, tea.Batch(cmds...)
	case spinner.TickMsg:
		return m, m.spinTick(msg)
	case imagesMsg:
		return m.applyImages(msg)
	case tokenMsg:
		m.applyToken(msg)
		return m, nil
	case clusterInfoMsg:
		m.applyClusterInfo(msg)
		return m, nil
	case metricsMsg:
		m.applyMetrics(msg)
		return m, nil
	case openedMsg:
		return m, m.applyOpened(msg)
	case noticeMsg:

		if !m.cfgNote {
			m.status = msg.text
		}
		return m, m.notify(toneOK, msg.text)
	case toastExpiredMsg:
		m.expireToast(msg)
		return m, nil
	case pfRestartMsg:
		return m.applyRestart(msg)
	case pfOwnerMsg:
		return m.applyOwner(msg)
	case pfProbeMsg:
		return m.applyProbe(msg)
	case namesMsg:
		return m.applyNames(msg)
	case resumedMsg:
		m.resuming = false
		m.forwards = msg.forwards
		m.save()
		if m.stage == "fwds" {
			idx := m.content.Index()
			m.content.SetItems(m.forwardItems())
			m.content.Select(idx)
		}
		if msg.note != "" {
			m.status = msg.note
		}
		return m, m.notify(toneNormal, msg.note)
	case resListMsg:
		if msg.ctx == m.context {
			m.resources = msg.names
			m.resContext = msg.ctx
			if m.cmdOn && m.cmdHint == "kubeconfig  context  namespace  pods  forward  forwards" {
				m.cmdHint = m.cfg.Keys.Complete.Label() + " completes cluster resources"
			}
		}
		return m, nil
	case yamlMsg:
		m.applyYAML(msg)
		return m, nil
	case describeMsg:
		m.applyDescribe(msg)
		return m, nil
	case viewMsg:
		m.applyView(msg)
		if msg.what != "" {
			if msg.err != nil {
				return m, m.notify(toneErr, msg.what+" failed")
			}
			return m, m.notify(toneOK, msg.what)
		}
		if msg.err != nil {
			return m, m.notify(toneErr, "decode failed")
		}
		return m, m.notify(toneOK, "secret decoded")
	case nsMsg:
		if m.restored {
			return m, nil
		}
		if msg.ctx != "" && m.context == "" {
			m.context = msg.ctx
		}
		if m.namespace == "" && msg.ns != "" {
			m.namespace = msg.ns
			if m.stage == "nav" && !m.cfgNote {
				m.status = msg.ns
			}
		}
		return m, nil
	case logsMsg:
		m.applyLogs(msg)
		return m, nil
	case progressMsg:
		m.withPane(msg.id, func() {
			switch {
			case m.load.token != msg.token:
			case msg.what != "":
				m.load.what, m.load.done, m.load.total = msg.what, 0, 0
			default:
				m.load.done, m.load.total = msg.done, msg.total
			}
		})
		return m, listen(msg.ch)
	case kOutMsg:
		var cmd tea.Cmd
		if msg.id == "" {
			cmd = m.showOutput(msg)
		} else {
			m.withPane(msg.id, func() { cmd = m.showOutput(msg) })
		}
		return m, cmd
	case kScanMsg:
		if msg.root != m.kroot {
			return m, nil
		}
		return m, m.showKList(msg.root, msg.dirs)
	case rowsMsg:
		m.applyRows(msg)
		return m, nil
	case lessDoneMsg:
		return m, nil
	case vimReadyMsg:
		if msg.err != nil {
			m.status = m.errLine(msg.err)
			return m, nil
		}
		path := msg.path
		var next tea.Model = m
		var cmd tea.Cmd
		if !m.withPane(msg.pane, func() {
			next, cmd = m.runEditor("vim", "vim "+filepath.Base(path), filepath.Base(path), path,
				func() *exec.Cmd { return vimCommand(path) },
				func(error) tea.Msg { _ = os.Remove(path); return vimDoneMsg{} })
			m = next.(model)
		}) {
			_ = os.Remove(path)
		}
		return m, cmd
	case paneterm.OutputMsg:
		return m, m.editorOutput(msg)
	case paneterm.ExitMsg:
		return m.editorExited(msg)
	case vimDoneMsg:
		if m.stage == "logs" {
			m.logStick = true
			m.logBusy = true
			return m, m.fetchLogs()
		}
		return m, nil
	case shellDoneMsg:
		m.status = "shell closed"
		if msg.err != nil {
			m.status = "shell: " + msg.err.Error()
		}
		return m, nil
	case kcountMsg:
		return m.setKustCount(msg), nil
	case clipDoneMsg:
		return m.applyClip(msg)
	case prefixTimeoutMsg:
		return m.endPrefix(msg), nil
	case repeatTimeoutMsg:
		return m.endRepeat(msg), nil
	case editDoneMsg:

		apply := func() { m.applyEditDone(msg) }
		if msg.pane == "" || !m.withPane(msg.pane, apply) {

			apply()
		}

		if msg.err != nil {
			return m, m.notify(toneErr, msg.what+" failed")
		}
		return m, m.notify(toneOK, msg.what)
	case fwdMsg:
		return m.applyForward(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, m.relayout()

	case tea.KeyMsg:
		km := m.cfg.Keys
		key := msg.String()
		if m.copy != nil {
			return m.copyKey(msg)
		}

		if (m.pfOn || m.dlg != nil) && altLeak(msg) && !m.inputEditKey(key) {
			return m, nil
		}

		if m.pfOn {
			return m.updatePopup(msg)
		}

		if m.dlg != nil {
			return m.updateDialog(msg)
		}

		afterPrefix := false
		if m.repeatOn {

			if next, cmd, ok := m.repeatKey(key); ok {
				return next, cmd
			}
			m.repeatOn = false
		}
		if m.prefixOn {
			next, cmd, done := m.afterPrefix(msg)
			if done {
				return next, cmd
			}
			m, afterPrefix = next, true
		} else if m.prefixTakes(key) {
			return m.startPrefix()
		}

		if m.ed != nil && !afterPrefix {
			next, ok := m.editorKey(msg)
			if ok {
				return next, nil
			}
			m = next
		}

		if printable(msg) && m.inputFocused() {
			return m.typeInto(msg)
		}

		if name := km.DirectAction(key); name != "" && !m.inputEditKey(key) {
			if next, cmd, ok := m.paneAction(name); ok {
				return next, cmd
			}
		}
		switch {
		case km.LastError.Has(key) && !m.cmdOn:
			return m.openLastErr()
		case km.Fullscreen.Has(key):
			if !m.cmdOn {
				m.zoom = !m.zoom
				return m, m.relayout()
			}
		}

		if altLeak(msg) && m.inputFocused() && !m.inputEditKey(key) {
			return m, nil
		}
		if km.Quit.Has(key) {
			m.killEditors()
			m.quitting = true
			return m, tea.Quit
		}
		if m.cmdOn {
			return m.updateCmd(msg)
		}
		if km.Command.Has(key) {
			m.cmdOn = true
			m.cmd.SetValue("")
			m.cmd.Focus()
			m.cmdCycle = nil
			m.cmdCycleAt = -1
			m.cmdHint = "kubeconfig  context  namespace  pods  forward  forwards"
			var cmd tea.Cmd
			if m.kubeconfig != "" && m.context != "" && m.resContext != m.context {
				cmd = fetchResources(m.k, m.kubeconfig, m.context)
			}
			return m, cmd
		}
		filtering := m.listFiltering()
		if m.stage == "kcmd" {
			return m.updateShell(msg)
		}

		// same key as delete. on home it closes the pane, then the tab, then quits.
		if !filtering && km.Delete.Has(key) && !m.inputEditKey(key) && m.stage == "nav" {
			if m.tabs[m.tab].root.Count() > 1 {
				m.closePane()
				return m, m.relayout()
			}
			if len(m.tabs) > 1 {
				m.closeTab()
				return m, m.relayout()
			}
			m.killEditors()
			m.quitting = true
			return m, tea.Quit
		}
		if !filtering {
			if id := m.jumpTarget(key); id != "" {
				return m.jump(key)
			}
		}

		if m.stage == "fwds" && !filtering && len(m.forwards) > 0 {
			switch {
			case km.PfRestart.Has(key):
				return m.restartSelected()
			case km.PfAuto.Has(key):
				return m.toggleAuto()
			}
		}
		if !filtering {
			if next, cmd, ok := m.screenKey(key); ok {
				return next, cmd
			}
			if next, cmd, ok := m.tableAction(key); ok {
				return next, cmd
			}
			switch {
			case km.Delete.Has(key) && (m.stage == "workpods" || m.stage == "res" || m.stage == "fwds"):
				return m.askDelete()
			case km.KustomizeDelete.Has(key) && (m.stage == "kdir" || m.stage == "kust"):

				return m.askKust("delete")
			case km.PortForward.Has(key) && (m.stage == "workpods" || m.stage == "res"):
				return m.openPortForward()
			case km.Describe.Has(key) && (m.stage == "workpods" || m.stage == "res"):
				return m.describeSelected()
			case km.Decode.Has(key):
				if m.stage == "res" && kube.IsSecretKind(m.kind) && m.content.SelectedItem() != nil {
					it := m.content.SelectedItem().(item)
					m.subject = it.id
					m.logText = ""
					m.logStick = false
					m.logAt = 0
					m.stage = "view"
					m.vs.vback = ""
					m.status = "decoded secret"
					return m, tea.Batch(m.startLoad("decoding secret", false), m.decodeSelected())
				}
			case km.Edit.Has(key):
				if (m.stage == "kust" || m.stage == "kdir") && m.content.SelectedItem() != nil {
					m.status = "opening vim…"
					return m.editKustomization()
				}
				if (m.stage == "workpods" || m.stage == "res") && m.content.SelectedItem() != nil {
					m.status = "opening vim…"
					return m.editSelected()
				}
			}
		}
		switch {
		case km.Vim.Has(key):
			if !filtering && m.viewing() {
				m.status = "vim"
				return m, m.vimView()
			}
		case km.Wrap.Has(key) && m.viewing() && !filtering:
			m.toggleWrap()
			return m, nil
		case km.CopyMode.Has(key) && m.viewing() && !filtering && !m.load.active():
			return m.enterCopy()
		case km.LogMark.Has(key) && (m.stage == "logs" || m.stage == "mlogs") && !filtering && !m.load.active():
			m.toggleLogMark()
			return m, nil
		case km.ScrollDown.Has(key):
			if m.viewing() && !filtering {
				m.scroll(1)
				return m, nil
			}
		case km.ScrollUp.Has(key):
			if m.viewing() && !filtering {
				m.scroll(-1)
				return m, nil
			}
		case km.PageDown.Has(key):
			if m.viewing() && !filtering {
				m.scroll(m.textRows() - 1)
				return m, nil
			}
		case km.PageUp.Has(key):
			if m.viewing() && !filtering {
				m.scroll(1 - m.textRows())
				return m, nil
			}
		case km.Top.Has(key):
			if m.viewing() && !filtering {
				m.logStick = false
				m.logAt = 0
				if m.stage == "logs" || m.stage == "mlogs" {
					m.vs.logHold = true
					m.vs.logLine = 0
					m.vs.logSub = 0
				}
				return m, nil
			}
		case km.Bottom.Has(key):
			if m.viewing() && !filtering {
				m.logStick = true
				m.clearLogHold()
				return m, nil
			}
		case km.KustomizePath.Has(key) && (m.stage == "kdir" || m.stage == "kust") && !filtering:
			return m.openKPath()
		case km.Scan.Has(key):
			if m.stage == "kdir" && !filtering {
				m.status = "scanning…"
				return m, m.scanKustomize()
			}
		case km.Apply.Has(key):
			if (m.stage == "kust" || m.stage == "kdir") && !filtering {
				return m.askKust("apply")
			}
		case km.DeleteAll.Has(key) || km.Diff.Has(key):
			if km.DeleteAll.Has(key) && m.stage == "fwds" && !filtering && len(m.forwards) > 0 {
				return m.askStopAll()
			}
			if km.Diff.Has(key) && (m.stage == "kust" || m.stage == "kdir") && !filtering {
				if _, ok := m.selectedKust(); !ok && m.content.SelectedItem() != nil {
					return m.notKust()
				}
				if _, ok := m.selectedKust(); ok {
					dir, _ := m.selectedKust()
					if m.kubeconfig == "" || m.context == "" {
						m.status = "select kubeconfig and context first"
						return m, nil
					}
					m.rememberK()
					m.kfile = dir
					m.subject = "diff"
					m.kback = m.stage
					m.logText = ""
					m.logStick = false
					m.logAt = 0
					m.stage = "kout"
					m.status = "diff " + config.HomePath(dir)
					return m, tea.Batch(m.startLoad("diffing", false), m.kustomizeCmd("diff", dir))
				}
			}
		case km.Back.Has(key):
			if m.cancelLoad() {
				return m, nil
			}

			if filtering || m.stage != "nav" && m.content.FilterState() == list.FilterApplied {
				m.content.ResetFilter()
				m.status = "filter cleared"
				return m, nil
			}
			if next, cmd, ok := m.screenBack(); ok {
				return next, cmd
			}
			if m.vs.parent != "" && (m.stage == "workpods" || m.stage == "res") {
				return m.backToParent()
			}
			if m.stage != "nav" {
				return m, m.back()
			}
			return m, nil
		case km.Left.Has(key):
			if filtering {
				break
			}
			if next, cmd, ok := m.screenBack(); ok {
				return next, cmd
			}
			if m.vs.parent != "" && (m.stage == "workpods" || m.stage == "res") {
				return m.backToParent()
			}
			if m.stage != "nav" {
				return m, m.back()
			}
		case km.Select.Has(key):
			if filtering {
				break
			}
			if m.stage == "nav" {

				return m.openNamed("pods")
			}
			if m.stage == "fwds" {
				return m, nil
			}
			return m.handleContentEnter()
		}
	}

	var cmd tea.Cmd
	if m.stage == "nav" {

		return m, nil
	}
	m.content, cmd = m.content.Update(msg)
	return m, cmd
}

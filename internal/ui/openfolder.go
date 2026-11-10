package ui

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"

	"k2/internal/config"

	tea "github.com/charmbracelet/bubbletea"
)

type launcher func(dir string, argv []string) error

func startDetached(dir string, argv []string) error {
	c := exec.Command(argv[0], argv[1:]...)
	c.Dir = dir
	c.Stdin, c.Stdout, c.Stderr = nil, nil, nil
	detach(c)
	if err := c.Start(); err != nil {
		return err
	}
	go c.Wait()
	return nil
}

func (m model) folderApp() string {
	f := strings.Fields(m.cfg.Editor.OpenFolder)
	if len(f) == 0 {
		return "code"
	}
	return filepath.Base(f[0])
}

func (m model) folderHere() string {
	if m.stage == "kust" {
		if sel := m.content.SelectedItem(); sel != nil && sel.(item).id != "" {
			return sel.(item).id
		}
	}
	return m.kroot
}

func (m model) openFolder() (tea.Model, tea.Cmd) {
	if next, cmd, blocked := m.readOnlyBlock("edit"); blocked {
		return next, cmd
	}
	dir := m.folderHere()
	if dir == "" {
		return m, nil
	}
	argv := strings.Fields(m.cfg.Editor.OpenFolder)
	if len(argv) == 0 {
		argv = []string{"code"}
	}
	argv = append(argv, dir)
	open := m.launch
	if open == nil {
		open = startDetached
	}
	app := m.folderApp()
	if err := open(dir, argv); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			m.status = app + " not found in PATH"
			return m, m.notify(toneErr, app+" not found in PATH")
		}
		m.status = m.errLine(err)
		return m, m.notify(toneErr, app+": "+err.Error())
	}
	m.status = app + " " + config.HomePath(dir)
	return m, m.notify(toneOK, "opened in "+app)
}

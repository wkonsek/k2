package ui

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const clipTimeout = 3 * time.Second

type clipTool struct {
	name string
	args []string
	env  string
}

var clipTools = []clipTool{
	{name: "wl-copy", args: nil, env: "WAYLAND_DISPLAY"},
	{name: "xclip", args: []string{"-selection", "clipboard"}, env: "DISPLAY"},
	{name: "xsel", args: []string{"--clipboard", "--input"}, env: "DISPLAY"},
}

const methodOSC52 = "osc52"

type clipEnv struct {
	getenv   func(string) string
	lookPath func(string) (string, error)
	run      func(path string, args []string, stdin string) error
	writeTTY func(seq string) error
}

func realClipEnv() clipEnv {
	return clipEnv{getenv: os.Getenv, lookPath: exec.LookPath, run: runClipTool, writeTTY: clipboardWrite}
}

func copyToClipboard(env clipEnv, mode, text string) (string, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = "auto"
	}
	if mode == methodOSC52 {
		return methodOSC52, sendOSC52(env, text)
	}

	for _, t := range clipTools {
		if t.name != mode {
			continue
		}
		path, err := env.lookPath(t.name)
		if err != nil {
			return t.name, fmt.Errorf("%s is not installed (ui.clipboard: %s)", t.name, mode)
		}
		if err := env.run(path, t.args, text); err != nil {
			return t.name, fmt.Errorf("%s: %w", t.name, err)
		}
		return t.name, nil
	}
	if mode != "auto" {

		return "", fmt.Errorf("unknown ui.clipboard %q", mode)
	}

	var errs []string
	for _, t := range clipTools {
		if env.getenv(t.env) == "" {
			continue
		}
		path, err := env.lookPath(t.name)
		if err != nil {
			continue
		}
		if err := env.run(path, t.args, text); err != nil {
			errs = append(errs, t.name+": "+err.Error())
			continue
		}
		return t.name, nil
	}
	if err := sendOSC52(env, text); err != nil {
		if len(errs) > 0 {
			return methodOSC52, fmt.Errorf("%s; OSC 52: %w", strings.Join(errs, "; "), err)
		}
		return methodOSC52, err
	}
	return methodOSC52, nil
}

func sendOSC52(env clipEnv, text string) error {
	seq := osc52(text)
	if env.getenv("TMUX") != "" {
		seq = tmuxPassthrough(seq)
	}
	return env.writeTTY(seq)
}

func osc52(text string) string {
	return "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(text)) + "\a"
}

func tmuxPassthrough(seq string) string {
	return "\x1bPtmux;" + strings.ReplaceAll(seq, "\x1b", "\x1b\x1b") + "\x1b\\"
}

func runClipTool(path string, args []string, stdin string) error {
	errFile, err := os.CreateTemp("", "k2-clip-*.err")
	if err != nil {
		return err
	}
	defer os.Remove(errFile.Name())
	defer errFile.Close()
	cmd := exec.Command(path, args...)
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Stdout = nil
	cmd.Stderr = errFile
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
	case <-time.After(clipTimeout):
		_ = cmd.Process.Kill()
		<-done
		return errors.New("timed out")
	}
	if err != nil {
		msg, _ := os.ReadFile(errFile.Name())
		if text := strings.TrimSpace(string(bytes.ToValidUTF8(msg, nil))); text != "" {
			return errors.New(strings.TrimSpace(text))
		}
		return err
	}
	return nil
}

var clipboardWrite = func(seq string) error {
	f, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {

		if _, werr := os.Stderr.WriteString(seq); werr != nil {
			return fmt.Errorf("no terminal: %w", err)
		}
		return nil
	}
	defer f.Close()
	_, err = f.WriteString(seq)
	return err
}

var clipboardRun = func(mode, text string) (string, error) {
	return copyToClipboard(realClipEnv(), mode, text)
}

type clipDoneMsg struct {
	text   string
	method string
	err    error
}

func (m model) copyCmd(text string) tea.Cmd { return m.copyCmdAs(text, text) }

func (m model) copyCmdAs(text, label string) tea.Cmd {
	mode := m.cfg.UI.Clipboard
	return func() tea.Msg {
		method, err := clipboardRun(mode, text)
		return clipDoneMsg{text: label, method: method, err: err}
	}
}

func (m model) applyClip(msg clipDoneMsg) (model, tea.Cmd) {
	switch {
	case msg.err != nil:
		m.status = "copy failed: " + m.errLine(msg.err)
		return m, m.notify(toneErr, m.status)
	case msg.method == methodOSC52:
		m.status = "copied " + msg.text + " via OSC 52 (terminal must support it; set ui.clipboard or install wl-clipboard / xclip)"
		return m, m.notify(toneWarn, "copied "+msg.text+" via OSC 52 (terminal must support it)")
	default:
		m.status = "copied " + msg.text + " (" + msg.method + ")"
		return m, m.notify(toneOK, "copied "+msg.text+" ("+msg.method+")")
	}
}

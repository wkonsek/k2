package main

import (
	"errors"
	"fmt"
	"os"
	"runtime/debug"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"k2/internal/kube"
	"k2/internal/shell"
	"k2/internal/ui"
)

var version = "dev"

const release = "v0.0.0"

func versionString() string {
	if version != "dev" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return release + "-dev"
	}
	v, dirty := release+"-dev", false
	for _, s := range info.Settings {
		switch {
		case s.Key == "vcs.revision" && len(s.Value) >= 7:
			v += "-" + s.Value[:7]
		case s.Key == "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if dirty {
		v += "-dirty"
	}
	return v
}

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--check-config" || os.Args[1] == "--check") {
		out, code := checkConfig()
		fmt.Print(out)
		os.Exit(code)
	}
	if out, exit := cliAction(os.Args[1:], versionString()); exit {
		fmt.Print(out)
		return
	}
	os.Exit(run(os.Stdout, os.Stderr))
}

func run(stdout, stderr *os.File) int {

	os.Stdout = stderr
	defer func() { os.Stdout = stdout }()

	runner := shell.Exec{}
	opts := []tea.ProgramOption{tea.WithAltScreen()}

	if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
		defer tty.Close()
		opts = append(opts, tea.WithInput(tty), tea.WithOutput(tty))
		lipgloss.SetColorProfile(termenv.NewOutput(tty).EnvColorProfile())
	}

	notice, warn := firstStart()
	profile, perr := profileArg(os.Args[1:])
	if perr != nil {
		fmt.Fprintln(stderr, perr.Error())
		return 1
	}
	uiOpts := ui.Options{Runner: runner, Version: versionString(), Notice: notice, Profile: profile}
	if warn != "" {
		uiOpts.Warnings = []string{warn}
	}
	model, crash := ui.Guard(ui.New(uiOpts))
	p := tea.NewProgram(model, opts...)
	crash.Attach(p)
	final, err := p.Run()
	if crash.Crashed() {
		reportCrash(stderr, crash.Summary(), crash.Stack)
		return 2
	}
	if err != nil {
		if errors.Is(err, tea.ErrProgramPanic) {

			fmt.Fprintln(stderr, "k2: crashed (program experienced a panic); nothing was exported")
			return 2
		}
		fmt.Fprintf(stderr, "k2: %v\n", err)
		return 1
	}
	s := ui.Finish(final)
	if s.Kubeconfig == "" {
		return 0
	}
	if s.Context != "" {

		kube.New(runner).PersistSelection(s.Kubeconfig, s.Context, s.Namespace)
	}
	fmt.Fprint(stdout, exportLine(s.Kubeconfig))
	return 0
}

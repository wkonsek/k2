package main

import (
	"fmt"
	"os"
	"strings"

	"k2"
	"k2/internal/config"
)

// only a single export line gets eval'd. a panic used to dump bubble tea's
// "Caught panic" text on stdout and the shell tried to run it.
const shellWrapper = `  k2() {
    if (( $# )); then
      command k2 "$@"
      return
    fi
    local out
    out="$(command k2)" || return
    if [[ $out == 'export KUBECONFIG='* && $out != *$'\n'* ]]; then
      eval "$out"
    fi
  }`

const usage = `k2 - a terminal UI for working with several Kubernetes clusters side by side

Usage:
  k2                  start the UI
  k2 --fresh          one home pane; the saved tabs and panes are not restored
  k2 --profile NAME   open that profile (panes, contexts, views)
  k2 --version        print the version (also -v, version)
  k2 --help           print this help (also -h)
  k2 --init-config    write the commented example config to ~/.k2/config.yaml if it
                      is missing (k2 also does this on its first start); with --force
                      and an existing config, write config.yaml.new beside it instead
  k2 --check-config   print every problem in ~/.k2/config.yaml (also --check).
                      exit 1 if something was skipped; k2 still starts with the default
                      for that entry

On exit k2 takes the FOCUSED pane: it sets current-context and the namespace
of that pane in the pane's kubeconfig file, then prints a single line,
export KUBECONFIG="<that kubeconfig>", and nothing else.

Use it through a shell function so the export line is evaluated, e.g. in ~/.zshrc
(it evaluates only a clean exit's export line, never an error or crash text):

` + shellWrapper + `

Keys: c context, C kubeconfig, n namespace, K kustomize, U kubectl, f forwards,
d describe, ctrl+f or F port-forward (pods/services), ctrl+t theme, w wrap (viewers), : command line, q back, Q quit.
Panes and tabs: the prefix ctrl+a, then (tmux keys) " or h split down, % or v split right,
o next pane, ; last pane, arrows move (repeat), x close pane, z zoom (also Z), c or t new tab,
& or w close tab, , rename tab, n/p next/previous tab (also ] and [ alone), 1..9 tab N.
ctrl+a ctrl+a sends a literal ctrl+a to the in-pane vim / shell.
Every key is configurable in ~/.k2/config.yaml; see CHEATSHEET.md for the full list and README.md.
`

func startArgs(args []string) (profile string, fresh bool, err error) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			break
		}
		if a == "--fresh" {
			fresh = true
			continue
		}
		if name, ok := strings.CutPrefix(a, "--profile="); ok {
			if name == "" {
				return "", false, fmt.Errorf("k2: --profile needs a name")
			}
			profile = name
			continue
		}
		if a == "--profile" {
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return "", false, fmt.Errorf("k2: --profile needs a name")
			}
			profile = args[i+1]
			i++
		}
	}
	return profile, fresh, nil
}

func cliAction(args []string, version string) (string, bool) {
	if len(args) == 0 {
		return "", false
	}
	switch args[0] {
	case "--version", "-v", "version":
		return fmt.Sprintf("k2 %s\n", version), true
	case "--help", "-h":
		return usage, true
	case "--init-config":
		return initConfig(args[1:]), true
	case "--check-config", "--check":
		out, _ := checkConfig()
		return out, true
	}
	return "", false
}

func initConfig(rest []string) string {
	force := len(rest) > 0 && (rest[0] == "--force" || rest[0] == "-f")
	r, err := config.InitFile(k2.ConfigExample, force)
	switch {
	case err != nil:
		return fmt.Sprintf("k2: cannot write the example config: %v\n", err)
	case r.Written && r.Exists:
		return fmt.Sprintf("kept %s; wrote the example beside it: %s\n", config.Tilde(config.Path()), config.Tilde(r.Path))
	case r.Written:
		return fmt.Sprintf("wrote %s (every option commented; edit it to change k2)\n", config.Tilde(r.Path))
	}
	return fmt.Sprintf("%s already exists, left unchanged (--force writes config.yaml.new beside it)\n", config.Tilde(r.Path))
}

func firstStart() (notice string, warning string) {
	r, err := config.EnsureFile(k2.ConfigExample)
	if err != nil {
		return "", "config: could not create " + config.Tilde(config.Path()) + ": " + err.Error()
	}
	if r.Written {
		return "created " + config.Tilde(r.Path) + " (all options commented)", ""
	}
	return "", ""
}

func checkConfig() (string, int) {
	path := config.Path()
	legacy := strings.TrimSuffix(path, ".yaml")
	used := path
	switch {
	case fileExists(path):
	case legacy != path && fileExists(legacy):
		used = legacy
	default:
		return fmt.Sprintf("%s does not exist (k2 uses defaults until the first start)\n", config.Tilde(path)), 0
	}
	_, warns := config.Load()
	if len(warns) == 0 {
		return fmt.Sprintf("%s\nok\n", config.Tilde(used)), 0
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", config.Tilde(used))
	for _, w := range warns {
		b.WriteString(w)
		if !strings.HasSuffix(w, "\n") {
			b.WriteByte('\n')
		}
	}
	word := "problem"
	if len(warns) != 1 {
		word = "problems"
	}
	fmt.Fprintf(&b, "%d %s. k2 still starts and keeps the default for each of these.\n", len(warns), word)
	return b.String(), 1
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

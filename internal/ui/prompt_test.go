package ui

import (
	"strings"
	"testing"

	"k2/internal/theme"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestKubectlRoles(t *testing.T) {
	line := "get pods -n shop -o=yaml web --watch"
	roles := kubectlRoles(line)
	at := func(word string) tokenRole { return roles[len([]rune(line[:strings.Index(line, word)]))] }
	want := map[string]tokenRole{"get": roleVerb, "pods": roleResource, "-n": roleFlag, "shop": roleValue, "-o=": roleFlag, "yaml": roleValue, "web": roleText, "--watch": roleFlag}
	for word, r := range want {
		if got := at(word); got != r {
			t.Errorf("%q: role %d, want %d", word, got, r)
		}
	}
}

func kubectlView(t *testing.T, cfgTheme, ctx string, prod ...string) model {
	t.Helper()
	cfg := testEnv(t)
	cfg.Theme = cfgTheme
	cfg.Prod = prod
	m := newTestModel(t, cfg, nil)
	m.kubeconfig, m.context, m.namespace = "/kc", ctx, "shop"
	return press(t, m, "U")
}

func TestKubectlPromptColorsOnEveryTheme(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	defer useTheme(theme.Nord())
	for _, name := range theme.Names() {
		m := kubectlView(t, name, "kind-dev")
		if m.stage != "kcmd" {
			t.Fatalf("%s: U opened %q", name, m.stage)
		}
		for _, r := range "get Quotas -o wide --sort-by=FO" {
			m = press(t, m, string(r))
		}
		out := m.View()
		view := ansi.Strip(out)
		if !strings.Contains(view, "◉ kind-dev · shop  ❯ kubectl get Quotas -o wide --sort-by=FO") {
			t.Fatalf("%s: prompt:\n%s", name, view)
		}
		for _, want := range []string{
			roleStyle(roleVerb).Render("get"),
			roleStyle(roleResource).Render("Quotas"),
			roleStyle(roleFlag).Render("-o"),
			roleStyle(roleValue).Render("wide"),
			roleStyle(roleFlag).Render("--sort-by="),
		} {
			if !strings.Contains(out, want) {
				t.Errorf("%s: missing styled %q", name, ansi.Strip(want))
			}
		}
	}
}

func TestKubectlPromptProdChip(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	m := kubectlView(t, "nord", "kind-prod", "kind-prod")
	out := m.View()
	if !strings.Contains(ansi.Strip(out), "☠ kind-prod · shop  ❯ kubectl") {
		t.Fatalf("prod prompt:\n%s", ansi.Strip(out))
	}
	if !strings.Contains(out, lipgloss.NewStyle().Foreground(th.Danger).Bold(true).Render("☠ kind-prod")) {
		t.Fatal("prod chip is not red")
	}
}

func TestKubectlPromptScrolls(t *testing.T) {
	m := kubectlView(t, "nord", "kind-dev")
	m.sh.SetValue("get pods " + strings.Repeat("x", 200) + "END")
	m.sh.CursorEnd()
	line := ansi.Strip(m.kubectlPrompt(60))
	if !strings.HasSuffix(strings.TrimRight(line, " "), "xEND") || lipgloss.Width(line) > 60 {
		t.Fatalf("scrolled prompt %q (%d cells)", line, lipgloss.Width(line))
	}
	m.sh.SetCursor(0)
	if line := ansi.Strip(m.kubectlPrompt(60)); !strings.Contains(line, "❯ kubectl get pods x") {
		t.Fatalf("cursor at start: %q", line)
	}
	narrow := strings.Split(ansi.Strip(m.kubectlPrompt(30)), "\n")
	if len(narrow) != 2 || !strings.HasPrefix(narrow[0], "◉ kind-dev") || !strings.HasPrefix(narrow[1], "❯ kubectl ") {
		t.Fatalf("narrow prompt %q", narrow)
	}
}

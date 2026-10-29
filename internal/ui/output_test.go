package ui

import (
	"strings"
	"testing"
	"time"

	"k2/internal/kustomize"
	"k2/internal/shell"
	"k2/internal/theme"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func kustCluster() *shell.Fake {
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		switch r.Args[0] {
		case "kustomize":
			return shell.OK("kind: Namespace\n---\nkind: Deployment\n---\nkind: Service\n")
		case "apply":
			return shell.OK("namespace/shop unchanged\ndeployment.apps/web created\nservice/web configured\n")
		case "diff":
			return shell.Result{Stdout: []byte(kustDiff), Code: 1, Err: errExit1}
		}
		return shell.Fail(1, "not in this test")
	}}
}

var errExit1 = shellErr("exit status 1")

type shellErr string

func (e shellErr) Error() string { return string(e) }

const kustDiff = `diff -u -N /tmp/LIVE-1/apps.v1.Deployment.shop.web /tmp/MERGED-1/apps.v1.Deployment.shop.web
--- /tmp/LIVE-1/apps.v1.Deployment.shop.web	2026-10-22 10:00:00 +0000
+++ /tmp/MERGED-1/apps.v1.Deployment.shop.web	2026-10-22 10:00:00 +0000
@@ -6,7 +6,7 @@
-  replicas: 1
+  replicas: 2`

func kustModel(t *testing.T, f *shell.Fake, verb string) model {
	t.Helper()
	m := newTestModel(t, testEnv(t), f)
	m.kubeconfig, m.context, m.kroot = "/kc", "kind-dev", "/k"
	m.subject, m.stage = verb, "kout"
	return m
}

func TestKustomizeApplyProgressAndSummary(t *testing.T) {
	t0 := time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)
	fixClock(t, t0)
	m := kustModel(t, kustCluster(), "apply")
	m.startLoad("applying", false)
	msg := m.kustomizeCmd("apply", "/k/overlays/dev")()
	var steps []string
	for {
		if out, ok := msg.(kOutMsg); ok {
			fixClock(t, t0.Add(3400*time.Millisecond))
			nm, cmd := m.Update(out)
			m = nm.(model)
			if cmd == nil {
				t.Fatal("no toast after apply")
			}
			break
		}
		nm, next := m.Update(msg)
		m = nm.(model)
		steps = append(steps, loadText(m.load))
		if !strings.Contains(ansi.Strip(m.View()), "━") && m.load.done > 0 {
			t.Fatalf("no progress bar at %s", loadText(m.load))
		}
		msg = next()
	}
	if strings.Join(steps, "|") != "applying 0/3 · 0.0s|applying 1/3 · 0.0s|applying 2/3 · 0.0s|applying 3/3 · 0.0s" {
		t.Fatalf("progress steps %q", steps)
	}
	view := ansi.Strip(m.View())
	for _, want := range []string{
		"✓ exit 0 · done · 3.4s",
		"· namespace/shop unchanged", "+ deployment.apps/web created", "~ service/web configured",
		"╭─ apply · overlays/dev · kind-dev ─",
		"│ + 1 created   ~ 1 configured   · 1 unchanged",
		"│ 3 objects · 3.4s",
		"── exit 0 · done ──",
		"✓ applied · 3 objects",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in\n%s", want, view)
		}
	}
	if m.status != "exit 0 · done · 3.4s" || m.load.active() {
		t.Fatalf("status %q load %+v", m.status, m.load)
	}
}

func TestKustomizeFailureWithoutTotal(t *testing.T) {
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		return shell.Result{Stderr: []byte("error: accumulating resources: boom"), Code: 1, Err: errExit1}
	}}
	m := kustModel(t, f, "delete")
	m = settle(t, m, tea.Batch(m.startLoad("deleting", false), m.kustomizeCmd("delete", "/elsewhere/app")))
	view := ansi.Strip(m.View())
	for _, want := range []string{"✗ exit 1 · failed", "✗ error: accumulating resources: boom", "│ ✗ 1 error ", "│ ✗ delete failed — see error above", "nothing deleted", "── exit 1 · failed ──", "✗ delete failed · exit 1", "delete · /elsewhere/app · kind-dev"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in\n%s", want, view)
		}
	}
}

func TestColoredDiffIsKept(t *testing.T) {
	line := "\x1b[32m+ one document added:\x1b[0m"
	if got := colorOut(line, "kust"); got != line {
		t.Fatalf("ansi rewritten: %q", got)
	}
	if !strings.Contains(colorOut("+ replicas: 2", "kust"), "+ replicas: 2") {
		t.Fatal("plain addition lost")
	}
}

func TestKustomizeDiffRendering(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	defer useTheme(theme.Nord())
	for _, name := range theme.Names() {
		cfg := testEnv(t)
		cfg.Theme = name
		m := newTestModel(t, cfg, kustCluster())
		m.kubeconfig, m.context, m.kroot, m.subject, m.stage = "/kc", "kind-dev", "/k", "diff", "kout"
		m = settle(t, m, tea.Batch(m.startLoad("diffing", false), m.kustomizeCmd("diff", "/k/dev")))
		out := m.View()
		view := ansi.Strip(out)
		if strings.Contains(view, "/tmp/LIVE") || strings.Contains(view, "+++") || !strings.Contains(view, "▸ Deployment shop/web  (apps/v1)") {
			t.Fatalf("%s: diff not collapsed:\n%s", name, view)
		}
		for _, want := range []string{
			titleStyle.Render("▸ Deployment shop/web  (apps/v1)"),
			sgr(th.OK) + "+  replicas: 2",
			sgr(th.Danger) + "-  replicas: 1",
			warnStyle.Render("1 resource differs · 0.0s"),
			"! exit 1 · differences",
		} {
			if !strings.Contains(out, want) && !strings.Contains(view, want) {
				t.Fatalf("%s: missing %q", name, want)
			}
		}
		if m.status != "exit 1 · differences · 0.0s" {
			t.Fatalf("%s: status %q", name, m.status)
		}
	}
}

func TestApplyLineColorsOnEveryTheme(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	defer useTheme(theme.Nord())
	for _, name := range theme.Names() {
		p, _ := theme.Lookup(name)
		useTheme(p)
		cases := map[string]string{
			"deployment.apps/web created":    sgr(p.OK) + "+ deployment.apps/web created",
			"service/web configured":         sgr(p.Warn) + "~ service/web configured",
			"configmap/cfg unchanged":        sgr(p.Muted) + "· configmap/cfg unchanged",
			"error: no objects passed":       sgr(p.Danger) + "✗ error: no objects passed",
			"Warning: deprecated apiVersion": sgr(p.Warn) + "! Warning: deprecated apiVersion",
		}
		for line, want := range cases {
			if got := colorResult(line); !strings.HasPrefix(got, want) {
				t.Errorf("%s: %q rendered %q, want prefix %q", name, line, got, want)
			}
		}
		box := colorBox("│ + 2 created   · 1 unchanged │")
		if !strings.Contains(box, sgr(p.OK)+"+ 2 created") || !strings.Contains(box, sgr(p.Muted)+"· 1 unchanged") {
			t.Errorf("%s: box counts %q", name, box)
		}
		if got := colorExit("── exit 2 · failed ──"); !strings.Contains(got, sgr(p.Danger)+"exit 2 · failed") {
			t.Errorf("%s: exit line %q", name, got)
		}
	}
}

func TestSummaryOnProdIsRed(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	cfg := testEnv(t)
	cfg.Prod = []string{"kind-prod"}
	m := newTestModel(t, cfg, kustCluster())
	m.kubeconfig, m.context, m.kroot, m.subject, m.stage = "/kc", "kind-prod", "/k", "apply", "kout"
	m = settle(t, m, tea.Batch(m.startLoad("applying", false), m.kustomizeCmd("apply", "/k/prod")))
	if !strings.Contains(m.logText, "╭─ apply · prod · ☠ kind-prod ─") {
		t.Fatalf("summary:\n%s", m.logText)
	}
	if !strings.Contains(m.View(), titleStyle.Foreground(th.Danger).Render("apply · prod · ☠ kind-prod")) {
		t.Fatal("prod summary title is not red")
	}
}

func TestKubectlShape(t *testing.T) {
	cases := map[string][2]string{
		"get po -o yaml":                     {"get", "yaml"},
		"kubectl -n shop get po -oyaml":      {"get", "yaml"},
		"get po --output=json":               {"get", "json"},
		"diff -f x.yaml":                     {"diff", ""},
		"":                                   {"", ""},
		"apply -f web.yaml --dry-run=client": {"apply", ""},
	}
	for line, want := range cases {
		if v, f := kubectlShape(line); v != want[0] || f != want[1] {
			t.Errorf("kubectlShape(%q) = %q %q, want %q", line, v, f, want)
		}
	}
}

func TestKubectlDiffOutput(t *testing.T) {
	m := kustModel(t, nil, "kubectl")
	m.showOutput(kOutMsg{text: kustDiff, code: 1, line: "diff -f web.yaml"})
	if !strings.Contains(m.logText, kustomize.DiffHeaderPrefix+"Deployment shop/web") || !strings.HasSuffix(m.logText, "── exit 1 · differences ──") || m.ran.mode != "diff" || m.ran.outcome != "differences" {
		t.Fatalf("kubectl diff:\n%s", m.logText)
	}
	m.subject = "diff"
	m.showOutput(kOutMsg{text: "domain: example\n-   prefix: legacy\n\n  [25 lines unchanged)]\n", target: "helm-gateway-app.yml", ctx: "prod"})
	if m.ran.outcome != "differences" || strings.Contains(m.logText, "no differences") || !strings.Contains(m.logText, "differences found") || !strings.HasSuffix(m.logText, "── exit 0 · differences ──") {
		t.Fatalf("dyff exit 0: outcome %q\n%s", m.ran.outcome, m.logText)
	}
	m.subject = "kubectl"
	m.showOutput(kOutMsg{text: "a: 1\n- b", line: "get cm x -o yaml"})
	if m.ran.mode != "yaml" {
		t.Fatalf("mode %q", m.ran.mode)
	}
}

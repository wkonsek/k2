package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"k2/internal/shell"
	"k2/internal/theme"
)

const describeText = `Name:             web-1
Namespace:        shop
Status:           Pending
Containers:
  web:
    State:          Waiting
      Reason:       ContainerCreating
Events:
  Type     Reason            Age   From               Message
  ----     ------            ----  ----               -------
  Normal   Scheduled         2m    default-scheduler  Successfully assigned shop/web-1 to node-a
  Warning  FailedMount       1m    kubelet            MountVolume.SetUp failed for volume "certs" :
                                                       secret "certs" not found
  Warning  Unhealthy         30s   kubelet            Readiness probe failed
  Warning  FailedScheduling  10s   default-scheduler  0/1 nodes are available`

func describeCluster() *shell.Fake {
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		switch {
		case len(r.Args) > 1 && r.Args[0] == "get" && r.Args[1] == "pods":
			return shell.OK(podsJSON)
		case len(r.Args) > 0 && r.Args[0] == "describe":
			return shell.OK(describeText)
		}
		return shell.Fail(1, "not in this test")
	}}
}

func TestDescribePod(t *testing.T) {
	f := describeCluster()
	m := newTestModel(t, testEnv(t), f)
	m.kubeconfig, m.context = "/kc", "kind-dev"
	n, c := m.openPods()
	m = settle(t, n.(model), c)
	m = press(t, m, "down")
	sel := m.content.Index()
	m = pressSettle(t, m, "d")
	if m.stage != "describe" || !strings.Contains(m.logText, "FailedMount") || m.subject != "shop/web-1" {
		t.Fatalf("stage %q subject %q text %q", m.stage, m.subject, m.logText)
	}
	var args []string
	for _, r := range f.Calls() {
		if len(r.Args) > 0 && r.Args[0] == "describe" {
			args = r.Args
		}
	}
	if strings.Join(args, " ") != "describe pod web-1 -n shop --kubeconfig /kc --context kind-dev" {
		t.Fatalf("describe args %v", args)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "describe shop/web-1") || !strings.Contains(view, "Events:") {
		t.Fatalf("view:\n%s", view)
	}

	m = press(t, m, "j")
	if m.logAt == 0 && !m.logStick {
		t.Fatal("j did not scroll")
	}
	for _, back := range []string{"esc", "q", "H"} {
		m2 := press(t, m, back)
		if m2.stage != "workpods" || m2.content.Index() != sel {
			t.Fatalf("%s: stage %q index %d", back, m2.stage, m2.content.Index())
		}
	}
}

func TestDescribeResourceAndError(t *testing.T) {
	m := newTestModel(t, testEnv(t), &shell.Fake{Handler: func(r shell.Request) shell.Result {
		return shell.Fail(1, `Error from server (NotFound): deployments.apps "web" not found`)
	}})
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	_, items, _ := packTable([]string{"NAME", "AGE"}, [][]string{{"web", "1d"}}, [][2]string{{"shop", "web"}}, nil, -1, 60, "shop")
	m.content.SetItems(items)
	m.stage, m.kind = "res", "deployments"
	if kind, ns, name, ok := m.selectedRef(); !ok || kind != "deployments" || ns != "shop" || name != "web" {
		t.Fatalf("ref %s %s %s %v", kind, ns, name, ok)
	}
	m = pressSettle(t, m, "d")
	if m.stage != "res" || !strings.Contains(m.status, "not found") {
		t.Fatalf("stage %q status %q", m.stage, m.status)
	}
}

func TestDescribeEventColors(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	defer useTheme(theme.Nord())
	for _, name := range theme.Names() {
		p, _ := theme.Lookup(name)
		useTheme(p)
		lines := strings.Split(describeText, "\n")
		out := colorDescribe(lines)
		find := func(sub string) string {
			for i, l := range lines {
				if strings.Contains(l, sub) {
					return out[i]
				}
			}
			t.Fatalf("no line %q", sub)
			return ""
		}
		for sub, c := range map[string]lipgloss.Color{
			"Successfully assigned": p.Muted, "MountVolume": p.Danger, `secret "certs"`: p.Danger,
			"Readiness probe": p.Warn, "0/1 nodes": p.Danger, "Events:": p.Accent,
		} {

			code := strings.TrimSuffix(strings.TrimPrefix(sgr(c), "\x1b["), "m")
			if got := find(sub); !strings.Contains(got, code+"m") {
				t.Errorf("%s: %q drawn %q, want %q", name, sub, got, sgr(c))
			}
		}
		if got := find("Name:"); !strings.HasPrefix(got, keyStyle.Render("Name:")) {
			t.Errorf("%s: key not highlighted: %q", name, got)
		}
		if got := find("Reason:       ContainerCreating"); strings.Contains(got, sgr(p.Danger)) || strings.Contains(got, sgr(p.Warn)) {
			t.Errorf("%s: line outside Events colored as an event: %q", name, got)
		}
		for i := range out {
			if ansi.Strip(out[i]) != lines[i] {
				t.Fatalf("text changed: %q", out[i])
			}
		}
	}
	if !eventIsError("ImagePullBackOff") || !eventIsError("FailedMount") || eventIsError("Unhealthy") || eventIsError("Pulling") {
		t.Fatal("eventIsError")
	}
}

package ui

import (
	"os"
	"strings"
	"testing"

	"k2/internal/shell"

	"github.com/charmbracelet/x/ansi"
)

func deleteCluster() *shell.Fake {
	base := actionCluster()
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		if len(r.Args) >= 2 && r.Args[0] == "get" && r.Args[1] == "configmaps" {
			return shell.OK(`{"items":[{"metadata":{"name":"cfg","namespace":"shop","creationTimestamp":"2026-10-22T10:00:00Z"},"data":{"a":"1"}}]}`)
		}
		if len(r.Args) > 0 && r.Args[0] == "delete" {
			return shell.OK("deleted")
		}
		if len(r.Args) > 0 && r.Args[0] == "kustomize" {
			return shell.OK("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: a\n---\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: b\n---\napiVersion: v1\nkind: Service\nmetadata:\n  name: c\n")
		}
		return base.Handler(r)
	}}
}

func mutations(f *shell.Fake) []string {
	var out []string
	for _, verb := range []string{"delete", "apply", "patch", "scale", "rollout", "create", "label", "annotate"} {
		out = append(out, calls(f, verb)...)
	}
	return out
}

type popupCase struct {
	name   string
	setup  func(t *testing.T, f *shell.Fake, prod bool) model
	keys   []string
	title  string
	want   []string
	danger bool
}

func popupCases() []popupCase {
	pods := func(t *testing.T, f *shell.Fake, prod bool) model {
		m := actionModel(t, f)
		if prod {
			m.cfg.Prod = []string{"kind-dev"}
		}
		return selectRow(t, m, "web-1")
	}
	fwds := func(t *testing.T, f *shell.Fake, prod bool) model {
		m := actionModel(t, f)
		if prod {
			m.cfg.Prod = []string{"kind-dev"}
		}
		m.forwards = append(m.forwards, forwardFor(os.Getpid(), "8080", "80", "svc", "web"))
		next, cmd := m.runCommand("pf")
		return settle(t, next.(model), cmd)
	}
	kust := func(t *testing.T, f *shell.Fake, prod bool) model {
		m := actionModel(t, f)
		if prod {
			m.cfg.Prod = []string{"kind-dev"}
		}
		_ = m.showKList("/k", []string{"/k/overlays/dev"})
		return m
	}
	return []popupCase{
		{"delete pod", pods, []string{"ctrl+d"}, "✗ delete pod", []string{"delete pod/web-1?", "pod shop/web-1", "context kind-dev"}, true},
		{"delete resource", func(t *testing.T, f *shell.Fake, prod bool) model {
			m := pods(t, f, prod)
			return selectRow(t, open(t, m, "cm"), "cfg")
		}, []string{"ctrl+d"}, "✗ delete configmaps", []string{"delete configmaps/cfg?", "configmaps shop/cfg"}, true},
		{"stop forward", fwds, []string{"ctrl+d"}, "■ stop port-forward", []string{"localhost:8080 → svc/web:80", "shop · kind-dev"}, true},
		{"stop all forwards", fwds, []string{"D"}, "■ stop all port-forwards", []string{"stop and remove all 1 forward?", "localhost:8080 → svc/web:80"}, true},
		{"kustomize delete", kust, []string{"d"}, "✗ kustomize delete", []string{"delete overlays/dev on kind-dev?", "/k/overlays/dev", "resources:"}, true},
		{"kustomize apply", kust, []string{"a"}, "⇪ kustomize apply", []string{"apply overlays/dev on kind-dev?", "/k/overlays/dev"}, true},
		{"close pane", pods, []string{"ctrl+a", "v", "ctrl+a", "x"}, "✗ close pane", []string{"close pane?"}, false},
		{"close tab", pods, []string{"ctrl+a", "t", "ctrl+a", "w"}, "✗ close tab", []string{"close tab 2?", "tab 2 · 1 pane"}, false},
		{"rollout restart", func(t *testing.T, f *shell.Fake, prod bool) model {
			return open(t, pods(t, f, prod), "deploy")
		}, []string{"r"}, "↻ rollout restart", []string{"rollout restart"}, false},
	}
}

func TestDestructivePopups(t *testing.T) {
	trueColor(t)
	for _, c := range popupCases() {
		for _, prod := range []bool{false, true} {
			for _, cancel := range []string{"n", "esc", "enter"} {
				f := deleteCluster()
				m := c.setup(t, f, prod)
				m = press(t, m, c.keys...)
				if m.dlg == nil || m.dlg.ask == "" || m.dlg.title != c.title {
					t.Fatalf("%s: no popup (dlg %+v status %q)", c.name, m.dlg, m.status)
				}
				if c.danger != m.dlg.danger {
					t.Fatalf("%s: danger %v", c.name, m.dlg.danger)
				}
				view := m.View()
				plain := ansi.Strip(view)
				for _, w := range append([]string{c.title, "y / n", "default no"}, c.want...) {
					if !strings.Contains(plain, w) {
						t.Fatalf("%s: popup lacks %q:\n%s", c.name, w, plain)
					}
				}
				if prod != strings.Contains(plain, skull+" "+c.title) {
					t.Fatalf("%s prod=%v: skull wrong:\n%s", c.name, prod, plain)
				}
				red := colorBefore(view, m.dlg.ask) == sgr(th.Danger)
				if red != (c.danger || prod) {
					t.Fatalf("%s prod=%v: question red=%v", c.name, prod, red)
				}

				if strings.Contains(m.content.Title, "?") {
					t.Fatalf("%s: inline prompt in the title %q", c.name, m.content.Title)
				}
				before := len(mutations(f))
				panes, tabs, fw := len(m.panes), len(m.tabs), len(m.forwards)
				m = pressSettle(t, m, cancel)
				if cancel == "enter" {

					if m.dlg == nil {
						t.Fatalf("%s: enter closed the popup", c.name)
					}
					m = pressSettle(t, m, "n")
				}
				if m.dlg != nil || len(mutations(f)) != before || len(m.panes) != panes || len(m.tabs) != tabs || len(m.forwards) != fw || m.stage == "kout" {
					t.Fatalf("%s: %s did not cancel cleanly: dlg %v calls %v", c.name, cancel, m.dlg, mutations(f))
				}
			}
		}
	}
}

func TestDestructivePopupsYes(t *testing.T) {
	f := deleteCluster()
	m := pressSettle(t, selectRow(t, actionModel(t, f), "web-1"), "ctrl+d", "y")
	if d := calls(f, "delete"); len(d) != 1 || !strings.HasPrefix(d[0], "delete pod web-1") && !strings.Contains(d[0], "web-1") {
		t.Fatalf("pod delete %v", d)
	}
	if m.load.active() || !strings.Contains(m.toast.text, "deleted web-1") {
		t.Fatalf("after delete: load %v toast %q", m.load.active(), m.toast.text)
	}
	f = deleteCluster()
	m = selectRow(t, open(t, actionModel(t, f), "cm"), "cfg")
	m = pressSettle(t, m, "ctrl+d", "y")
	if d := calls(f, "delete"); len(d) != 1 || !strings.HasPrefix(d[0], "delete configmaps cfg -n shop --kubeconfig /kc --context kind-dev") {
		t.Fatalf("resource delete %v", d)
	}
	if m.load.active() {
		t.Fatal("spinner left running")
	}

	f = deleteCluster()
	m = actionModel(t, f)
	_ = m.showKList("/k", []string{"/k/overlays/dev"})
	next, cmd := m.Update(key(t, "d"))
	m = next.(model)
	if cmd == nil {
		t.Fatal("no count command")
	}
	next, _ = m.Update(cmd())
	m = next.(model)
	if !strings.Contains(ansi.Strip(m.View()), "resources: 3 objects") {
		t.Fatalf("count missing:\n%s", ansi.Strip(m.View()))
	}

	m2 := m.setKustCount(kcountMsg{seq: m.dlg.seq + 1, n: 9, ok: true})
	if strings.Contains(ansi.Strip(m2.View()), "9 objects") {
		t.Fatal("stale count applied")
	}
	m = press(t, m, "y")
	if m.stage != "kout" || m.subject != "delete" || m.dlg != nil {
		t.Fatalf("y: stage %q subject %q", m.stage, m.subject)
	}

	m = actionModel(t, deleteCluster())
	m.forwards = append(m.forwards, forwardFor(-1, "8080", "80", "svc", "web"), forwardFor(-2, "9090", "90", "", "db-0"))
	next, c := m.runCommand("pf")
	m = settle(t, next.(model), c)
	m = press(t, m, "ctrl+d", "y")
	if len(m.forwards) != 1 {
		t.Fatalf("forwards %d", len(m.forwards))
	}
	m = press(t, m, "D", "y")
	if len(m.forwards) != 0 {
		t.Fatalf("forwards %d after D y", len(m.forwards))
	}
}

func TestPopupSwallowsKeys(t *testing.T) {
	f := deleteCluster()
	m := press(t, selectRow(t, actionModel(t, f), "web-1"), "ctrl+d")
	for _, k := range []string{"j", "ctrl+d", "r", "S", "x", "enter", "ctrl+a"} {
		m = pressSettle(t, m, k)
		if m.dlg == nil || m.dlg.title != "✗ delete pod" {
			t.Fatalf("%s changed the popup: %+v", k, m.dlg)
		}
	}
	if len(mutations(f)) != 0 {
		t.Fatalf("calls %v", mutations(f))
	}
}

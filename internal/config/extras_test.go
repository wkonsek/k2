package config

import (
	"strings"
	"testing"
)

func TestExtraKeyConflicts(t *testing.T) {
	cases := []struct{ yaml, warn string }{
		{"keys: {xray: d}", "keys.xray: d is already used by describe (using X)"},
		{"keys: {multi_logs: s}", "keys.multi_logs: s is already used by shell (using M)"},
		{"keys: {favorites: c}", "keys.favorites: c is already used by context (using b)"},
		{"keys: {xray_toggle: y}", "keys.xray_toggle: y is already used by yaml (using  )"},
		{"keys: {helm_values: t}", "keys.helm_values: t is already used by helm_history (using v)"},
		{"keys: {all_namespaces: ctrl+d}", "keys.all_namespaces: ctrl+d is already used by delete (using 0)"},
		{"keys: {log_filter: w}", "keys.log_filter: w is already used by wrap (using /)"},
		{"keys: {fav_add: x}", "keys.fav_add: x is already used by fav_remove (using a)"},
		{"keys: {fav_up: enter}", "keys.fav_up: enter is already used by submit (using K)"},

		{"keys: {helm_notes: n}", "keys.helm_notes: n is already used by namespace (using N)"},
	}
	for _, c := range cases {
		_, w := Parse([]byte(c.yaml))
		if len(w) != 1 || !strings.Contains(w[0], c.warn) {
			t.Errorf("%s: %v", c.yaml, w)
		}
	}

	for _, y := range []string{"keys: {helm_notes: d}", "keys: {yaml: e}", "keys: {fav_add: d}", "keys: {log_filter: x}",
		"keys: {xray: y}", "keys: {helm_rollback: s}", "keys: {favorites: B}"} {
		if _, w := Parse([]byte(y)); len(w) != 0 {
			t.Errorf("%s: %v", y, w)
		}
	}
	k := DefaultKeys()
	if !k.Xray.Has("X") || !k.MultiLogs.Has("M") || !k.Favorites.Has("b") || !k.XrayToggle.Has(" ") || !k.HelmManifest.Has("M") || !k.AllNamespaces.Has("0") {
		t.Fatalf("defaults %+v", k)
	}

	c, w := Parse([]byte("keys: {xray_toggle: tab}"))
	if len(w) != 0 || !c.Keys.XrayToggle.Has("tab") {
		t.Fatalf("tab: %v", w)
	}
}

func TestHelmAndLogSettings(t *testing.T) {
	c, w := Parse([]byte(`
logs: {multi_max_lines: 9000, multi_every: 5, multi_max_pods: 50, multi_all_containers: false}
helm: {binary: /opt/helm/bin/helm}
favorites:
  - {context: kind-dev, namespace: shop, view: ":deploy"}
  - {name: lab, context: kind-lab, kubeconfig: ~/.kube/lab}
`))
	if len(w) != 0 {
		t.Fatalf("warnings %v", w)
	}
	if c.Logs.MultiMaxLines != 9000 || c.Logs.MultiEvery != 5 || c.Logs.MultiMaxPods != 50 || c.Logs.MultiAllContainers || c.Logs.Tail != 80 {
		t.Fatalf("logs %+v", c.Logs)
	}
	if c.Helm.Binary != "/opt/helm/bin/helm" {
		t.Fatalf("helm %+v", c.Helm)
	}
	if len(c.Favorites) != 2 || c.Favorites[0].View != "deploy" || c.Favorites[1].Name != "lab" || !strings.HasSuffix(c.Favorites[1].Kubeconfig, "/.kube/lab") || strings.HasPrefix(c.Favorites[1].Kubeconfig, "~") {
		t.Fatalf("favorites %+v", c.Favorites)
	}
	if c, w := Parse([]byte("helm: {binary: ~}")); len(w) != 0 || c.Helm.Binary != "" {
		t.Fatalf("helm off: %v %q", w, c.Helm.Binary)
	}
	if c, w := Parse([]byte("favorites:")); len(w) != 0 || c.Favorites != nil {
		t.Fatalf("empty favorites: %v", w)
	}
	c, w = Parse([]byte(`
logs: {multi_every: 0, multi_max_lines: 10, multi_all_containers: maybe, nope: 1}
helm: {binary: [a], extra: 1}
favorites:
  - {namespace: shop}
  - plain
  - {context: ok, color: red}
`))
	joined := strings.Join(w, "\n")
	for _, frag := range []string{"logs.multi_every: 0 is out of range 1..60", "logs.multi_max_lines: 10 is out of range", "logs.multi_all_containers:", `logs: unknown setting "nope"`,
		"helm.binary: expected a single value", `helm: unknown setting "extra"`, "an entry without context", "not a mapping", `unknown field "color"`} {
		if !strings.Contains(joined, frag) {
			t.Errorf("missing %q in:\n%s", frag, joined)
		}
	}
	d := Default()
	if c.Logs != d.Logs || c.Helm != d.Helm || len(c.Favorites) != 1 || c.Favorites[0].Context != "ok" {
		t.Fatalf("fallbacks %+v %+v %+v", c.Logs, c.Helm, c.Favorites)
	}
	if c, w := Parse([]byte("favorites: {context: x}")); len(w) != 1 || c.Favorites != nil {
		t.Fatalf("not a list: %v", w)
	}
}

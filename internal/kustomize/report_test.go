package kustomize

import (
	"strings"
	"testing"
	"time"

	"k2/internal/shell"
)

func TestClassify(t *testing.T) {
	cases := map[string]Class{
		"deployment.apps/web created":                       Created,
		"service/web configured":                            Configured,
		"configmap/cfg unchanged":                           Unchanged,
		"deployment.apps/web serverside-applied":            ServerSide,
		`deployment.apps "web" deleted`:                     Deleted,
		"configmap/old pruned":                              Pruned,
		"Warning: resource deployments/web is missing …":    Warning,
		"error: no objects passed to apply":                 Error,
		`Error from server (NotFound): "web" deleted`:       Error,
		"The Deployment \"web\" is invalid: spec: Required": Error,
		"the server could not find the requested resource":  Error,
		"unable to recognize \"x.yaml\": no matches":        Error,
		"":                                    Other,
		"no differences":                      Other,
		"  namespace/shop unchanged  ":        Unchanged,
		"some line that ends in created data": Other,

		`deployment.apps "web" deleted from shop namespace`:          Deleted,
		`pod "web-1" force deleted from shop namespace`:              Deleted,
		`service "web" deleted from shop namespace (server dry run)`: Deleted,
		`configmap "cfg" deleted (dry run)`:                          Deleted,
		`namespace "shop" deleted`:                                   Deleted,
		`Error from server (NotFound): configmaps "cfg" not found`:   Error,
		`a line about the default namespace`:                         Other,
	}
	for line, want := range cases {
		if got := Classify(line); got != want {
			t.Errorf("Classify(%q) = %d, want %d", line, got, want)
		}
	}
}

func TestCountsAndParts(t *testing.T) {
	c := Count("a/x created\nb/y created\nc/z unchanged\nWarning: w\nerror: e\nd/q configured")
	if c.Objects() != 4 {
		t.Fatalf("objects = %d", c.Objects())
	}
	got := strings.Join(c.Parts(), " | ")
	if got != "+ 2 created | ~ 1 configured | · 1 unchanged | ! 1 warning | ✗ 1 error" {
		t.Fatalf("parts %q", got)
	}
}

func TestBuildCountsObjects(t *testing.T) {
	f := &shell.Fake{Handler: func(req shell.Request) shell.Result {
		return shell.OK("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: a\n---\napiVersion: apps/v1\nkind: Deployment\nspec:\n  template:\n    kind: nested\n")
	}}
	if n, ok := Build(f, "/k/dev"); n != 2 || !ok {
		t.Fatalf("Build = %d %v", n, ok)
	}
	if f.Calls()[0].Args[0] != "kustomize" || f.Calls()[0].Args[1] != "/k/dev" {
		t.Fatalf("args %v", f.Calls()[0].Args)
	}
	bad := &shell.Fake{Handler: func(shell.Request) shell.Result { return shell.Fail(1, "boom") }}
	if _, ok := Build(bad, "/k"); ok {
		t.Fatal("a failing build must not give a total")
	}
}

func TestRunStreamSeesLines(t *testing.T) {
	f := &shell.Fake{Handler: func(shell.Request) shell.Result { return shell.OK("a/x created\nb/y unchanged\n") }}
	var got []string
	text, code := RunStream(f, "apply", "/k", "/kc", "dev", func(l string) { got = append(got, l) })
	if code != 0 || text != "a/x created\nb/y unchanged" || len(got) != 2 {
		t.Fatalf("%q %d %q", text, code, got)
	}
}

func TestReportBoxKeepsExitLine(t *testing.T) {
	r := Report{Verb: "apply", Target: "overlays/dev", Context: "kind-dev",
		Text: "ns/shop unchanged\ndeployment.apps/web created\nservice/web configured", Took: 3400 * time.Millisecond}
	out := r.String()
	lines := strings.Split(out, "\n")
	if lines[0] != "ns/shop unchanged" || lines[len(lines)-1] != "── exit 0 · done ──" {
		t.Fatalf("report:\n%s", out)
	}
	for _, want := range []string{"╭─ apply · overlays/dev · kind-dev ", "│ + 1 created   ~ 1 configured   · 1 unchanged", "│ 3 objects · 3.4s", "╰─"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in\n%s", want, out)
		}
	}

	var w int
	for _, l := range lines {
		if strings.HasPrefix(l, "╭") || strings.HasPrefix(l, "│") || strings.HasPrefix(l, "╰") {
			if n := len([]rune(l)); w == 0 {
				w = n
			} else if n != w {
				t.Fatalf("ragged box row %q (%d vs %d)", l, n, w)
			}
		}
	}
	failed := Report{Verb: "delete", Text: "error: boom", Code: 1, Took: 12 * time.Second}.String()
	if !strings.HasSuffix(failed, "── exit 1 · failed ──") || !strings.Contains(failed, "✗ 1 error ") || !strings.Contains(failed, "nothing deleted · 12s") || strings.Contains(failed, "0 objects") {
		t.Fatalf("failed report:\n%s", failed)
	}
}

const rawDiff = `diff -u -N /tmp/LIVE-1234/apps.v1.Deployment.shop.web /tmp/MERGED-5678/apps.v1.Deployment.shop.web
--- /tmp/LIVE-1234/apps.v1.Deployment.shop.web	2026-10-22 10:00:00.000000000 +0000
+++ /tmp/MERGED-5678/apps.v1.Deployment.shop.web	2026-10-22 10:00:00.000000000 +0000
@@ -6,7 +6,7 @@
-  replicas: 1
+  replicas: 2
diff -u -N /tmp/LIVE-1234/v1.ConfigMap.shop.web.config /tmp/MERGED-5678/v1.ConfigMap.shop.web.config
--- /tmp/LIVE-1234/v1.ConfigMap.shop.web.config	2026-10-22 10:00:00.000000000 +0000
+++ /tmp/MERGED-5678/v1.ConfigMap.shop.web.config	2026-10-22 10:00:00.000000000 +0000
@@ -1,3 +1,4 @@
+  extra: "1"`

func TestCollapseDiff(t *testing.T) {
	got := CollapseDiff(rawDiff)
	want := `▸ Deployment shop/web  (apps/v1)
@@ -6,7 +6,7 @@
-  replicas: 1
+  replicas: 2
▸ ConfigMap shop/web.config  (v1)
@@ -1,3 +1,4 @@
+  extra: "1"`
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	if h := DiffHeaders(got); len(h) != 2 {
		t.Fatalf("headers %q", h)
	}

	plain := "---\n--- /tmp/LIVE-1/v1.Service.shop.web\n+++ /tmp/MERGED-1/v1.Service.shop.web\n-a\n+b"
	if got := CollapseDiff(plain); got != "---\n▸ Service shop/web  (v1)\n-a\n+b" {
		t.Fatalf("plain: %q", got)
	}
}

func TestResourceName(t *testing.T) {
	cases := map[string]string{
		"apps.v1.Deployment.shop.web":                          "Deployment shop/web  (apps/v1)",
		"v1.Namespace..shop":                                   "Namespace shop  (v1)",
		"rbac.authorization.k8s.io.v1.ClusterRole..admin.view": "ClusterRole admin.view  (rbac.authorization.k8s.io/v1)",
		"operators.coreos.com.v1alpha1.Subscription.olm.etcd":  "Subscription olm/etcd  (operators.coreos.com/v1alpha1)",
		"weird-name": "weird-name",
	}
	for in, want := range cases {
		if got := ResourceName(in); got != want {
			t.Errorf("ResourceName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDiffReport(t *testing.T) {
	out := Report{Verb: "diff", Text: rawDiff, Code: 1, Took: 800 * time.Millisecond}.String()
	if !strings.Contains(out, "│ 2 resources differ · 0.8s") || !strings.HasSuffix(out, "── exit 1 · differences ──") || strings.Contains(out, "/tmp/LIVE") {
		t.Fatalf("diff report:\n%s", out)
	}
	none := Report{Verb: "diff", Text: "no differences", Took: time.Second}.String()
	if !strings.HasPrefix(none, "╭─ diff") || !strings.Contains(none, "no differences · 1.0s") {
		t.Fatalf("no-diff report:\n%s", none)
	}
	dyff := "\x1b[33m± value change in multiline text (0 inserts, 2 deletions)\x1b[0m\n  domain: example\n\x1b[31m-   prefix: legacy\x1b[0m\n\n  [25 lines unchanged)]\n"
	colored := Report{Verb: "diff", Target: "helm-gateway-app.yml", Text: dyff, Code: 0, Took: 2500 * time.Millisecond}.String()
	if strings.Contains(colored, "no differences") || !strings.Contains(colored, "differences found · 2.5s") || !strings.HasSuffix(colored, "── exit 0 · differences ──") {
		t.Fatalf("dyff exit 0 still looked empty:\n%s", colored)
	}
}

func TestSettles(t *testing.T) {
	for line, want := range map[string]bool{
		`deployment.apps "web" deleted from shop namespace`:        true,
		`namespace "shop" deleted`:                                 true,
		"deployment.apps/web created":                              true,
		`Error from server (NotFound): configmaps "cfg" not found`: true,
		"error: unable to recognize \"x.yaml\"":                    false,
		"Warning: deleting cluster-scoped resources":               false,
		"": false,
	} {
		if got := Settles(line); got != want {
			t.Errorf("Settles(%q) = %v", line, got)
		}
	}
}

func TestSummaryCountsNewDeleteFormat(t *testing.T) {
	out := "namespace \"shop\" deleted\ndeployment.apps \"web\" deleted from shop namespace\nservice \"web\" deleted from shop namespace\n"
	if got := Summary(out); got != "\n\ndeleted 3" {
		t.Fatalf("summary %q", got)
	}
	if n := Count(out).Objects(); n != 3 {
		t.Fatalf("objects %d", n)
	}
}

func TestReportFailedWording(t *testing.T) {
	warn := "# Warning: 'vars' is deprecated. Please use 'replacements' instead."
	e1 := "# error: accumulating components: accumulateDirectory: boom"
	cases := []struct {
		r       Report
		want    []string
		mustNot []string
	}{
		{Report{Verb: "diff", Text: warn + "\n" + e1, Code: 2}, []string{"✗ diff failed — see error above", "! 1 warning   ✗ 1 error", "── exit 2 · failed ──"}, []string{"no differences", "1 errors", "1 warnings"}},
		{Report{Verb: "diff", Text: "error: a\nerror: b", Code: 1 + 1}, []string{"diff failed — see errors above", "✗ 2 errors"}, []string{"no differences"}},
		{Report{Verb: "diff", Text: "", Code: 0}, []string{"no differences"}, []string{"failed"}},
		{Report{Verb: "diff", Text: "something", Code: 1}, []string{"differences found"}, []string{"no differences", "failed"}},
		{Report{Verb: "apply", Text: e1, Code: 1}, []string{"✗ apply failed — see error above", "nothing applied"}, []string{"0 objects", "applied 0"}},
		{Report{Verb: "apply", Text: "deployment.apps/web created\nerror: x", Code: 1}, []string{"apply failed", "+ 1 created", "1 object applied before it failed"}, []string{"0 objects"}},
		{Report{Verb: "apply", Text: "deployment.apps/web created\nservice/web created", Code: 0}, []string{"+ 2 created", "2 objects"}, []string{"failed"}},
		{Report{Verb: "delete", Text: "Error from server (Forbidden): nope", Code: 1}, []string{"✗ delete failed — see error above", "nothing deleted"}, []string{"0 objects"}},
		{Report{Verb: "delete", Text: `deployment.apps "web" deleted`, Code: 0}, []string{"− 1 deleted", "1 object "}, []string{"failed", "1 objects"}},
	}
	for _, c := range cases {
		got := c.r.String()
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s exit %d: lacks %q:\n%s", c.r.Verb, c.r.Code, w, got)
			}
		}
		for _, w := range c.mustNot {
			if strings.Contains(got, w) {
				t.Errorf("%s exit %d: says %q:\n%s", c.r.Verb, c.r.Code, w, got)
			}
		}
	}
	if Classify(warn) != Warning || Classify(e1) != Error {
		t.Fatal("kustomize's # comments are not classified")
	}
}

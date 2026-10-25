package kube

import (
	"math/rand"
	"strings"
	"testing"
)

func lcs(a, b []string) int {
	tab := make([][]int, len(a)+1)
	for i := range tab {
		tab[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				tab[i][j] = tab[i+1][j+1] + 1
			} else {
				tab[i][j] = max(tab[i+1][j], tab[i][j+1])
			}
		}
	}
	return tab[0][0]
}

func TestLineDiffRandom(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	words := []string{"a", "b", "c", "d", "e"}
	gen := func() []string {
		out := make([]string, r.Intn(25))
		for i := range out {
			out[i] = words[r.Intn(len(words))]
		}
		return out
	}
	for n := 0; n < 500; n++ {
		a, b := gen(), gen()
		var ra, rb []string
		same := 0
		for _, l := range LineDiff(a, b) {
			if l.Kind != DiffAdd {
				ra = append(ra, l.Text)
			}
			if l.Kind != DiffDel {
				rb = append(rb, l.Text)
			}
			if l.Kind == DiffSame {
				same++
			}
		}
		if strings.Join(ra, ",") != strings.Join(a, ",") || strings.Join(rb, ",") != strings.Join(b, ",") {
			t.Fatalf("%v → %v: rebuilt %v → %v", a, b, ra, rb)
		}
		if same != lcs(a, b) {
			t.Fatalf("%v → %v: kept %d, LCS %d", a, b, same, lcs(a, b))
		}
	}
}

func TestLineDiffLarge(t *testing.T) {
	a := make([]string, 50000)
	for i := range a {
		a[i] = "line " + string(rune('a'+i%26)) + strings.Repeat("x", i%7)
	}
	b := append([]string(nil), a...)
	b[25000] = "changed"
	d := LineDiff(a, b)
	edits := 0
	for _, l := range d {
		if l.Kind != DiffSame {
			edits++
		}
	}
	if edits != 2 || len(d) != 50001 {
		t.Fatalf("edits %d len %d", edits, len(d))
	}
	x, y := make([]string, 3000), make([]string, 3000)
	for i := range x {
		x[i], y[i] = "old"+string(rune(i)), "new"+string(rune(i))
	}
	if d := LineDiff(x, y); len(d) != 6000 {
		t.Fatalf("rewrite %d", len(d))
	}
}

func TestUnified(t *testing.T) {
	a := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n"
	b := "1\n2\n3\nfour\n5\n6\n7\n8\n9\n10\n11\n"
	want := "@@ -1,10 +1,11 @@\n 1\n 2\n 3\n-4\n+four\n 5\n 6\n 7\n 8\n 9\n 10\n+11\n"
	if got := Unified(a, b, 3); got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}

	long := strings.Repeat("x\n", 20)
	if got := Unified("a\n"+long+"b\n", "A\n"+long+"B\n", 2); strings.Count(got, "@@ -") != 2 || !strings.Contains(got, "@@ -1,3 +1,3 @@") || !strings.Contains(got, "@@ -20,3 +20,3 @@") {
		t.Fatalf("hunks\n%s", got)
	}
	if Unified("same\n", "same\n", 3) != "" {
		t.Fatal("equal texts")
	}
}

func TestRevisionDiff(t *testing.T) {
	old := HelmRelease{Name: "web", Namespace: "shop", Revision: 1, Chart: "nginx-1.0.0", Config: map[string]any{"replicas": 1},
		Manifest: "---\n# Source: nginx/templates/deploy.yaml\napiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\n  labels:\n    name: not-this\nspec:\n  replicas: 1\n" +
			"---\nkind: Service\nmetadata:\n  name: web\nspec:\n  port: 80\n" +
			"---\nkind: ConfigMap\nmetadata:\n  name: old-cm\ndata:\n  a: b\n"}
	cur := HelmRelease{Name: "web", Namespace: "shop", Revision: 3, Chart: "nginx-1.1.0", Config: map[string]any{"replicas": 3},
		Manifest: "---\n# Source: nginx/templates/deploy.yaml\napiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\n  labels:\n    name: not-this\nspec:\n  replicas: 3\n" +
			"---\nkind: Service\nmetadata:\n  name: web\nspec:\n  port: 80\n" +
			"---\nkind: Secret\nmetadata:\n  name: new-secret\n"}
	d := RevisionDiff(old, cur)
	for _, want := range []string{
		"# shop/web: revision 1 (nginx-1.0.0) → revision 3 (nginx-1.1.0)",
		"# manifest: 1 changed, 1 added, 1 removed, 1 unchanged",
		"=== ~ values (user-supplied)\n@@ -1,1 +1,1 @@\n-replicas: 1\n+replicas: 3\n",
		"=== ~ Deployment/web\n", "-  replicas: 1\n+  replicas: 3\n",
		"=== + Secret/new-secret (added)\n+kind: Secret\n",
		"=== - ConfigMap/old-cm (removed)\n-kind: ConfigMap\n",
	} {
		if !strings.Contains(d, want) {
			t.Fatalf("diff lacks %q:\n%s", want, d)
		}
	}
	if strings.Contains(d, "Service/web") {
		t.Fatalf("unchanged resource shown:\n%s", d)
	}
	if !strings.Contains(RevisionDiff(cur, cur), "values (user-supplied): unchanged") {
		t.Fatal("equal values")
	}
}

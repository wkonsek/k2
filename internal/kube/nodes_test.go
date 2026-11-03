package kube

import (
	"fmt"
	"strings"
	"testing"
)

func rke2Nodes() string {
	var items []string
	for i, n := range []string{"master1", "master2", "master3", "worker1", "worker2", "worker3"} {
		labels := `"kubernetes.io/hostname":"` + n + `"`
		if strings.HasPrefix(n, "master") {
			labels += `,"node-role.kubernetes.io/control-plane":"true","node-role.kubernetes.io/etcd":"true","node-role.kubernetes.io/master":"true"`
		}
		spec := `{}`
		ready := "True"
		switch i {
		case 4:
			spec = `{"unschedulable":true}`
		case 5:
			ready = "Unknown"
		}
		items = append(items, fmt.Sprintf(`{"metadata":{"name":%q,"creationTimestamp":"2020-01-01T00:00:00Z","labels":{%s}},
			"spec":%s,"status":{"conditions":[{"type":"MemoryPressure","status":"False"},{"type":"Ready","status":%q}],
			"nodeInfo":{"kubeletVersion":"v1.34.2+rke2r1"}}}`, n, labels, spec, ready))
	}
	return `{"items":[` + strings.Join(items, ",") + `]}`
}

func TestNodeTable(t *testing.T) {
	for _, all := range []bool{false, true} {
		tb := TableFor("nodes", objs(t, rke2Nodes()), all)
		if got := strings.Join(tb.Headers, " "); got != "NAME STATUS ROLES AGE VERSION" {
			t.Fatalf("all=%v headers %q", all, got)
		}
		if tb.ColorCol != 1 || len(tb.Rows) != 6 {
			t.Fatalf("color %d rows %d", tb.ColorCol, len(tb.Rows))
		}
		want := [][]string{
			{"master1", "Ready", "control-plane,etcd,master"},
			{"master2", "Ready", "control-plane,etcd,master"},
			{"master3", "Ready", "control-plane,etcd,master"},
			{"worker1", "Ready", "<none>"},
			{"worker2", "Ready,SchedulingDisabled", "<none>"},
			{"worker3", "Unknown", "<none>"},
		}
		for i, r := range tb.Rows {
			if len(r) != len(tb.Headers) {
				t.Fatalf("row %d has %d cells: %v", i, len(r), r)
			}
			if strings.Join(r[:3], " ") != strings.Join(want[i], " ") || r[3] == "" || r[4] != "v1.34.2+rke2r1" {
				t.Fatalf("row %d = %v", i, r)
			}
			if tb.Refs[i] != [2]string{"", want[i][0]} {
				t.Fatalf("ref %v", tb.Refs[i])
			}
		}
	}
	for _, k := range []string{"no", "node", "Nodes"} {
		if got := TableFor(k, objs(t, rke2Nodes()), true).Headers; len(got) != 5 || got[2] != "ROLES" {
			t.Fatalf("%s headers %v", k, got)
		}
	}
}

func TestNodeRoles(t *testing.T) {
	cases := map[string]string{
		`{}`:                              "<none>",
		`{"kubernetes.io/role":"worker"}`: "worker",
		`{"node-role.kubernetes.io/worker":"","kubernetes.io/role":"worker"}`:         "worker",
		`{"node-role.kubernetes.io/etcd":"true","node-role.kubernetes.io/master":""}`: "etcd,master",
	}
	for labels, want := range cases {
		o := objs(t, `{"items":[{"metadata":{"name":"n","labels":`+labels+`}}]}`)[0]
		if got := NodeRoles(o); got != want {
			t.Errorf("%s: %q, want %q", labels, got, want)
		}
	}
	if NodeStatus(objs(t, `{"items":[{"metadata":{"name":"n"},"status":{"conditions":[{"type":"Ready","status":"False"}]}}]}`)[0]) != "NotReady" {
		t.Fatal("not ready")
	}
}

func TestClusterScopedNoNamespace(t *testing.T) {
	js := `{"items":[{"metadata":{"name":"a b, c","creationTimestamp":"2020-01-01T00:00:00Z"}},
		{"metadata":{"name":"<none>","creationTimestamp":"2020-01-01T00:00:00Z"}}]}`
	for _, k := range []string{"namespaces", "storageclasses", "pv", "persistentvolumes", "clusterroles", "clusterrolebindings",
		"crds", "customresourcedefinitions", "ingressclasses", "priorityclasses", "csidrivers", "apiservices"} {
		tb := TableFor(k, objs(t, js), true)
		if tb.Headers[0] == "NAMESPACE" {
			t.Fatalf("%s: %v", k, tb.Headers)
		}
		if strings.Join(tb.Rows[0][:1], "") != "<none>" || tb.Rows[1][0] != "a b, c" {
			t.Fatalf("%s rows %v", k, tb.Rows)
		}
		for _, r := range tb.Rows {
			if len(r) != len(tb.Headers) {
				t.Fatalf("%s row %v vs %v", k, r, tb.Headers)
			}
		}
	}
}

func TestNamespacedKeepsNamespace(t *testing.T) {
	js := `{"items":[{"metadata":{"name":"web","namespace":"shop","creationTimestamp":"2020-01-01T00:00:00Z"}}]}`
	for _, k := range []string{"configmaps", "deployments", "pods", "secrets", "events", "jobs"} {
		tb := TableFor(k, objs(t, js), true)
		if tb.Headers[0] != "NAMESPACE" || tb.Rows[0][0] != "shop" || len(tb.Rows[0]) != len(tb.Headers) {
			t.Fatalf("%s: %v %v", k, tb.Headers, tb.Rows)
		}
		if one := TableFor(k, objs(t, js), false); one.Headers[0] == "NAMESPACE" {
			t.Fatalf("%s single ns: %v", k, one.Headers)
		}
	}
}

func TestParseTopNodes(t *testing.T) {
	out := "master1   250m   6%   2048Mi   25%\nworker1   <unknown>   <unknown>   <unknown>   <unknown>\nworker2   1   12%   3Gi   40%\n\nbad\n"
	got := ParseTopNodes(out)
	if len(got) != 2 || got["master1"] != (Usage{CPU: "250m", Mem: "2048Mi"}) || got["worker2"] != (Usage{CPU: "1", Mem: "3Gi"}) {
		t.Fatalf("%v", got)
	}
	if args := strings.Join(TopNodesArgs("/kc", "rke2"), " "); args != "top nodes --no-headers --request-timeout=5s --kubeconfig /kc --context rke2" {
		t.Fatal(args)
	}
	ex := TableFor("nodes", objs(t, `{"items":[{"metadata":{"name":"n"},"status":{"allocatable":{"cpu":"7910m","memory":"8Gi"}}}]}`), false).Extra[0]
	if ex["cpuLimit"] != "7910" || ex["memLimit"] != "8589934592" {
		t.Fatalf("extra %v", ex)
	}
}

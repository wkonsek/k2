package kube

import (
	"reflect"
	"strings"
	"testing"

	"k2/internal/shell"
)

func TestParseTopAndArgs(t *testing.T) {
	got := ParseTop("web-1   3m   45Mi\nW1022 warning line\ndb-0  1   1Gi\n", "shop")
	want := map[string]Usage{"shop/web-1": {"3m", "45Mi"}, "shop/db-0": {"1", "1Gi"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ns: %v", got)
	}
	got = ParseTop("shop   web-1   3m   45Mi\nkube-system  coredns-x  2m  12Mi\n", "")
	if got["kube-system/coredns-x"] != (Usage{"2m", "12Mi"}) || len(got) != 2 {
		t.Fatalf("-A: %v", got)
	}
	if a := strings.Join(TopPodsArgs("/kc", "dev", ""), " "); a != "top pods --no-headers --request-timeout=5s -A --kubeconfig /kc --context dev" {
		t.Fatalf("args %q", a)
	}
	f := &shell.Fake{Handler: func(shell.Request) shell.Result {
		return shell.Fail(1, "Error from server (Forbidden): pods.metrics.k8s.io is forbidden")
	}}
	if _, err := New(f).TopPods("/kc", "dev", "shop"); err == nil || !strings.Contains(err.Error(), "Forbidden") {
		t.Fatalf("err %v", err)
	}
}

func TestQuantities(t *testing.T) {
	for in, want := range map[string]float64{"250m": 250, "1": 1000, "0.5": 500, "1500000n": 1.5, "100u": 0.1} {
		if v, ok := ParseCPU(in); !ok || v != want {
			t.Errorf("ParseCPU(%q) = %v %v", in, v, ok)
		}
	}
	for in, want := range map[string]float64{"45Mi": 45 << 20, "1Gi": 1 << 30, "128M": 128e6, "1000": 1000, "2Ki": 2048} {
		if v, ok := ParseMem(in); !ok || v != want {
			t.Errorf("ParseMem(%q) = %v %v", in, v, ok)
		}
	}
	for _, bad := range []string{"N/A", "-", "…", "", "-3m"} {
		if _, ok := ParseCPU(bad); ok {
			t.Errorf("ParseCPU(%q) ok", bad)
		}
		if _, ok := ParseMem(bad); ok {
			t.Errorf("ParseMem(%q) ok", bad)
		}
	}
}

func TestPodLimitsExtra(t *testing.T) {
	objs, err := ParseObjects([]byte(`{"items":[
 {"metadata":{"name":"a","namespace":"x"},"spec":{"containers":[
   {"name":"c1","resources":{"limits":{"cpu":"200m","memory":"64Mi"}}},
   {"name":"c2","resources":{"limits":{"cpu":1,"memory":"64Mi"}}}]},"status":{}},
 {"metadata":{"name":"b","namespace":"x"},"spec":{"containers":[
   {"name":"c1","resources":{"limits":{"cpu":"200m"}}},{"name":"c2"}]},"status":{}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	tb := PodTable(objs, false)
	if tb.Extra[0]["cpuLimit"] != "1200" || tb.Extra[0]["memLimit"] != "134217728" {
		t.Fatalf("a: %v", tb.Extra[0])
	}
	if tb.Extra[1]["cpuLimit"] != "" || tb.Extra[1]["memLimit"] != "" {
		t.Fatalf("b: %v", tb.Extra[1])
	}
}

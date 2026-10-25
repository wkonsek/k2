package kube

import (
	"strings"
	"testing"
)

func TestXraySummaryTroubleFilter(t *testing.T) {
	roots := BuildXray(xrayObjs(t), "deploy", "")
	if len(roots) != 1 {
		t.Fatalf("roots %d", len(roots))
	}
	web := roots[0]
	sum := Summary(web)
	for _, want := range []string{"ready", "2 rs", "2 pods", "restarts", "1 svc", "1 ing"} {
		if !strings.Contains(sum, want) {
			t.Errorf("summary %q lacks %q", sum, want)
		}
	}
	if !Trouble(web) {
		t.Fatalf("web has a crash-looping pod: %q", sum)
	}

	ok := &Node{Kind: "pod", Name: "api-1", Status: "Running", Ready: "1/1", Children: []*Node{{Kind: "container", Name: "app"}}}
	if Trouble(ok) || Summary(ok) != "1/1 ready · 1 container · 0 restarts" {
		t.Fatalf("healthy pod: %v %q", Trouble(ok), Summary(ok))
	}
	for _, bad := range []*Node{
		{Kind: "pod", Status: "Running", Ready: "0/1"},
		{Kind: "pod", Status: "Running", Ready: "1/1", Restarts: 2},
		{Kind: "pod", Status: "Pending", Ready: "0/1"},
		{Kind: "deploy", Children: []*Node{{Kind: "rs", Children: []*Node{{Kind: "pod", Status: "CrashLoopBackOff"}}}}},
	} {
		if !Trouble(bad) {
			t.Errorf("not trouble: %+v", bad)
		}
	}
	if Trouble(&Node{Kind: "pod", Status: "Completed", Ready: "0/1"}) {
		t.Error("a completed pod is fine")
	}

	all := BuildXray(xrayObjs(t), "deploy", "")
	got := FilterXray(all, "7D9F-A")
	rows := render(Flatten(got, nil))
	for _, want := range []string{"Deployment web", "ReplicaSet web-7d9f", "Pod web-7d9f-a", "container"} {
		if !strings.Contains(rows, want) {
			t.Fatalf("filtered tree lacks %q:\n%s", want, rows)
		}
	}
	for _, gone := range []string{"web-old", "web-7d9f-b", "Service"} {
		if strings.Contains(rows, gone) {
			t.Fatalf("filtered tree keeps %q:\n%s", gone, rows)
		}
	}
	if len(FilterXray(all, "nothing-like-it")) != 0 || len(FilterXray(all, "")) != 1 {
		t.Fatal("empty / no-match filters")
	}

	if len(Flatten(all, nil)) <= len(Flatten(got, nil)) {
		t.Fatal("the filter changed the tree it was given")
	}
}

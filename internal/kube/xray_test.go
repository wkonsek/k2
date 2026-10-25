package kube

import (
	"os"
	"strings"
	"testing"

	"k2/internal/shell"
)

func xrayObjs(t *testing.T) []Object {
	t.Helper()
	data, err := os.ReadFile("testdata/xray.json")
	if err != nil {
		t.Fatal(err)
	}
	objs, err := ParseObjects(data)
	if err != nil {
		t.Fatal(err)
	}
	return objs
}

func render(rows []XrayRow) string {
	var b strings.Builder
	for _, r := range rows {
		b.WriteString(r.Prefix + r.Node.Label() + " " + r.Node.Name)
		if r.Node.Init {
			b.WriteString(" (init)")
		}
		if r.Node.Status != "" {
			b.WriteString(" " + r.Node.Status)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func TestXrayDeployment(t *testing.T) {
	roots := BuildXray(xrayObjs(t), "deploy", "")
	if len(roots) != 1 {
		t.Fatalf("roots %d", len(roots))
	}
	got := render(Flatten(roots, nil))
	want := `Deployment web Progressing
├ Service web ClusterIP
│ └ Ingress web
├ ReplicaSet web-7d9f Progressing
│ ├ Pod web-7d9f-a CrashLoopBackOff
│ │ ├ container app Running
│ │ ├ container side CrashLoopBackOff
│ │ ├ container migrate (init) Completed
│ │ ├ ConfigMap web-config
│ │ ├ Secret db-creds
│ │ ├ Secret web-tls
│ │ └ PVC web-data
│ └ Pod web-7d9f-b Running
│   └ container app Running
└ ReplicaSet web-old ScaledDown
`
	if got != want {
		t.Fatalf("tree:\n%s\nwant:\n%s", got, want)
	}
	pod := roots[0].Children[1].Children[0]
	if pod.Ready != "1/2" || pod.Restarts != 3 || pod.Children[0].Restarts != 3 {
		t.Fatalf("pod %+v", pod)
	}
}

func TestXrayCollapseAndOtherKinds(t *testing.T) {
	objs := xrayObjs(t)
	roots := BuildXray(objs, "deploy", "web")
	rs := roots[0].Children[1]
	rows := Flatten(roots, map[string]bool{rs.ID(): true})
	if strings.Contains(render(rows), "Pod web-7d9f-a") || !strings.Contains(render(rows), "ReplicaSet web-7d9f") {
		t.Fatalf("collapsed:\n%s", render(rows))
	}
	for _, r := range rows {
		if r.Node == rs && !r.Kids {
			t.Fatal("a collapsed node still has kids")
		}
	}
	if got := render(Flatten(BuildXray(objs, "cronjob", ""), nil)); !strings.Contains(got, "└ Job report-1 Failed\n  └ Pod report-1-x Failed") {
		t.Fatalf("cronjob:\n%s", got)
	}
	if got := render(Flatten(BuildXray(objs, "svc", "web"), nil)); !strings.HasPrefix(got, "Service web ClusterIP\n├ Ingress web\n├ Pod web-7d9f-a") {
		t.Fatalf("service:\n%s", got)
	}
	if got := BuildXray(objs, "sts", ""); len(got) != 1 || got[0].Children[0].Name != "db-0" {
		t.Fatalf("sts %+v", got)
	}
	if got := BuildXray(objs, "deploy", "nope"); len(got) != 0 {
		t.Fatal("name filter")
	}
}

func TestXrayKind(t *testing.T) {
	for in, want := range map[string]string{"deploy": "deploy", "deployments": "deploy", "sts": "sts", "ds": "ds", "svc": "svc", "services": "svc", "cj": "cronjob", "po": "pod", "job": "job"} {
		if got, ok := XrayKind(in); !ok || got != want {
			t.Errorf("%s: %s %v", in, got, ok)
		}
	}
	if _, ok := XrayKind("configmaps"); ok {
		t.Error("configmaps have no tree")
	}
}

func TestXrayObjectsOneCall(t *testing.T) {
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		data, _ := os.ReadFile("testdata/xray.json")
		return shell.OK(string(data))
	}}
	objs, err := New(f).XrayObjects("/kc", "ctx", "shop")
	if err != nil || len(objs) == 0 || len(f.Calls()) != 1 {
		t.Fatalf("%v %d calls", err, len(f.Calls()))
	}
	if line := f.Calls()[0].Line(); !strings.Contains(line, "get pods,replicasets,deployments,statefulsets,daemonsets,jobs,cronjobs,services,ingresses -o json") || !strings.Contains(line, "-n shop") {
		t.Fatal(line)
	}
}

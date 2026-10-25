package forward

import (
	"os"
	"strings"
	"testing"
	"time"

	"k2/internal/shell"
)

func TestWhereShowsSourceContext(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cases := map[Forward]string{
		{Kubeconfig: home + "/.kube/lab-cluster", Context: "lab"}: "~/.kube/lab-cluster · lab",
		{Context: "lab"}:        "lab",
		{Kubeconfig: "/etc/kc"}: "/etc/kc",
		{}:                      "?",
	}
	for f, want := range cases {
		if got := Where(f); got != want {
			t.Errorf("Where(%+v) = %q, want %q", f, got, want)
		}
	}
}

func TestAge(t *testing.T) {
	if Age(time.Now().Add(-30*time.Second)) != "30s" || Age(time.Now().Add(-5*time.Minute)) != "5m" ||
		Age(time.Now().Add(-(2*time.Hour+3*time.Minute))) != "2h3m" || Age(time.Now().Add(time.Hour)) != "0s" {
		t.Fatal("Age")
	}
}

func TestAliveAndAnyDead(t *testing.T) {
	me := os.Getpid()
	if !Alive(me) || Alive(0) || Alive(-1) {
		t.Fatal("Alive")
	}
	if AnyDead([]Forward{{PID: me}}) || !AnyDead([]Forward{{PID: me}, {PID: 0}}) {
		t.Fatal("AnyDead")
	}
}

func TestPIDsOnPort(t *testing.T) {
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		switch r.Name {
		case "ss":
			return shell.OK(`LISTEN 0 4096 127.0.0.1:8080 0.0.0.0:* users:(("kubectl",pid=4242,fd=8))`)
		case "ps":
			return shell.OK("  PID ARGS\n 4242 kubectl port-forward -n a pod/web 8080:80\n 5000 kubectl port-forward pod/x 18080:80\n 6000 kubectl port-forward pod/y 9000:8080\n 7000 kubectl port-forward pod/z 8080\n")
		}
		return shell.Fail(1, "")
	}}
	got := PIDsOnPort(f, "8080")
	if len(got) != 3 || got[0] != 4242 || got[1] != 4242 || got[2] != 7000 {
		t.Fatalf("got %v", got)
	}
}

func TestSpawnReportsStartFailure(t *testing.T) {
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		if r.Name != "sh" || !strings.Contains(strings.Join(r.Args, " "), "pod/web 8080:80 --kubeconfig /kc --context c") {
			return shell.Fail(1, "unexpected "+r.Line())
		}
		return shell.Fail(127, "sh: kubectl: not found")
	}}
	_, err := Spawn(f, t.TempDir(), "/kc", "c", "shop/web", "8080", "80")
	if err == nil || !strings.Contains(err.Error(), "kubectl: not found") {
		t.Fatalf("err %v", err)
	}
}

func TestSpawnTargetKinds(t *testing.T) {
	var lines []string
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		lines = append(lines, strings.Join(r.Args, " "))
		return shell.Fail(1, "stop here")
	}}
	for _, kind := range []string{"svc", "deploy", "sts", "", "pod"} {
		_, _ = SpawnTarget(f, t.TempDir(), "/kc", "c", "shop", kind, "web", "8080", "80")
	}
	for i, want := range []string{"-n shop svc/web 8080:80", "-n shop deploy/web 8080:80", "-n shop sts/web 8080:80", "-n shop pod/web 8080:80", "-n shop pod/web 8080:80"} {
		if !strings.Contains(lines[i], want) {
			t.Errorf("%d: %q lacks %q", i, lines[i], want)
		}
	}
	if Target(Forward{Pod: "web"}) != "pod/web" || Target(Forward{Pod: "web", Kind: "svc"}) != "svc/web" {
		t.Fatal("Target")
	}
	lines = nil
	_, _ = Resume(f, t.TempDir(), []Forward{{Namespace: "shop", Pod: "web", Kind: "svc", Local: "8080", Remote: "80", PID: -1}})
	if len(lines) != 1 || !strings.Contains(lines[0], "svc/web 8080:80") {
		t.Fatalf("resume %v", lines)
	}
	if kindField("pod") != "" || kindField("svc") != "svc" {
		t.Fatal("kindField")
	}
}

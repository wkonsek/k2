package forward

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"k2/internal/shell"
)

func liveProc(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Skip("no sleep binary:", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
	go func() { _, _ = cmd.Process.Wait() }()
	return cmd.Process.Pid
}

type fakeCluster struct {
	pods    map[string]bool
	owners  map[string]string
	spawn   func() shell.Result
	spawns  []string
	listing string
	log     string
}

func (c *fakeCluster) runner() *shell.Fake {
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		args := strings.Join(r.Args, " ")
		switch {
		case r.Name == "sh":
			for i, a := range r.Args {
				if a == "port-forward" {
					c.spawns = append(c.spawns, r.Args[i+3]+" "+r.Args[i+4])
				}
			}
			if c.log != "" {
				for _, e := range r.Env {
					if path, ok := strings.CutPrefix(e, "K2_LOG="); ok {
						_ = os.WriteFile(path, []byte(c.log), 0o644)
					}
				}
			}
			return c.spawn()
		case r.Name == "kubectl" && strings.HasPrefix(args, "get deployments,statefulsets,pods "):
			return shell.OK(c.listing)
		case r.Name == "kubectl" && strings.HasPrefix(args, "get pod ") && strings.Contains(args, "deletionTimestamp"):
			name := r.Args[2]
			if c.pods[name] {
				return shell.OK("")
			}
			return shell.Fail(1, `Error from server (NotFound): pods "`+name+`" not found`)
		case r.Name == "kubectl" && strings.HasPrefix(args, "get "):
			return shell.OK(c.owners[r.Args[1]+"/"+r.Args[2]])
		}
		return shell.Fail(1, "unexpected "+r.Line())
	}}
}

func init() { stopWait = 0 }

func TestOwnerOf(t *testing.T) {
	c := &fakeCluster{owners: map[string]string{
		"pod/web-5d9-abc": "ReplicaSet/web-5d9", "rs/web-5d9": "Deployment/web",
		"pod/db-0": "StatefulSet/db", "pod/agent-x": "DaemonSet/agent", "pod/bare": "",
		"pod/custom": "Rollout/r",
	}}
	f := c.runner()
	for pod, want := range map[string]string{"web-5d9-abc": "deploy/web", "db-0": "sts/db", "agent-x": "ds/agent", "bare": "", "custom": ""} {
		if got := OwnerOf(f, "/kc", "c", "shop", pod); got != want {
			t.Errorf("%s: %q want %q", pod, got, want)
		}
	}
}

func TestRestartTargets(t *testing.T) {
	pid := liveProc(t)
	c := &fakeCluster{pods: map[string]bool{"web-1": true}, spawn: func() shell.Result { return shell.OK(strconv.Itoa(pid) + "\n") }}
	r := c.runner()
	dir := t.TempDir()

	nf, note, err := Restart(r, dir, Forward{Namespace: "shop", Pod: "web", Kind: "svc", Local: "8080", Remote: "80", PID: -1, Auto: "on"})
	if err != nil || note != "" || nf.PID != pid || Target(nf) != "svc/web" || nf.Local != "8080" || nf.Auto != "on" {
		t.Fatalf("svc: %+v %q %v", nf, note, err)
	}

	nf, note, err = Restart(r, dir, Forward{Namespace: "shop", Pod: "web-1", Local: "8081", Remote: "80", PID: -1, Owner: "deploy/web"})
	if err != nil || note != "" || Target(nf) != "pod/web-1" || nf.Owner != "deploy/web" {
		t.Fatalf("pod: %+v %q %v", nf, note, err)
	}

	nf, note, err = Restart(r, dir, Forward{Namespace: "shop", Pod: "web-2", Local: "8082", Remote: "80", PID: -1, Owner: "deploy/web"})
	if err != nil || note != "pod gone → forwarding via deploy/web" || Target(nf) != "deploy/web" || nf.Owner != "deploy/web" {
		t.Fatalf("fallback: %+v %q %v", nf, note, err)
	}

	n := len(c.spawns)
	_, _, err = Restart(r, dir, Forward{Namespace: "shop", Pod: "lonely", Local: "8083", Remote: "80", PID: -1})
	if err == nil || !strings.HasPrefix(err.Error(), "pod lonely not found; no owner to fall back to") || len(c.spawns) != n {
		t.Fatalf("no owner: %v spawns %d", err, len(c.spawns)-n)
	}
	want := []string{"svc/web 8080:80", "pod/web-1 8081:80", "deploy/web 8082:80"}
	for i, w := range want {
		if c.spawns[i] != w {
			t.Errorf("spawn %d: %q want %q", i, c.spawns[i], w)
		}
	}
}

func TestRestartLiveAndBusyPort(t *testing.T) {
	old := liveProc(t)
	c := &fakeCluster{spawn: func() shell.Result {
		return shell.Fail(1, "Unable to listen on port 8080: bind: address already in use")
	}}
	_, _, err := Restart(c.runner(), t.TempDir(), Forward{Namespace: "shop", Pod: "web", Kind: "svc", Local: "8080", Remote: "80", PID: old})
	if err == nil || !strings.Contains(err.Error(), "local port 8080 is busy") {
		t.Fatalf("busy: %v", err)
	}
	for i := 0; i < 50 && Alive(old); i++ {
		stopPoll()
	}
	if Alive(old) {
		t.Fatal("the live forward was not stopped first")
	}
}

func stopPoll() { exec.Command("sleep", "0.02").Run() }

package forward

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"k2/internal/shell"
)

func init() { spawnWait = 50e6 }

func TestSignal(t *testing.T) {
	for line, want := range map[string]string{
		"error: lost connection to pod": "lost connection to pod",
		`E1023 20:41:07.123456   4242 portforward.go:413] an error occurred forwarding 8083 -> 8083: error forwarding port 8083 to pod 1f2e3d, uid : pod not found`:                         "pod gone",
		`E1023 20:41:07.123456   4242 portforward.go:351] error creating error stream for port 8083 -> 8083: Error from server (NotFound): pods "kafka-connect-7bf45b66c5-cwpzj" not found`: "pod gone",
		`unable to do port forwarding: socat not found`: "port forwarding failed",
		`E1023 20:41:07.123456   4242 portforward.go:413] an error occurred forwarding 8083 -> 8083: error forwarding port 8083 to pod 1f2e, uid : failed to connect to localhost:8083 inside namespace "x", IPv4: dial tcp4 127.0.0.1:8083: connect: connection refused`: "remote port refused",
		`E1023 20:41:07.123456   4242 portforward.go:413] an error occurred forwarding 8083 -> 8083: error forwarding port 8083 to pod 1f2e, uid : network namespace for sandbox "abc" is closed`:                                                                         "forward error",
		"error upgrading connection: unable to upgrade connection: container not running": "connection upgrade failed",

		`I1023 20:41:07.123456   4242 round_trippers.go:553] POST https://127.0.0.1:6443/api/v1/namespaces/cluster1/pods/web-1/portforward 101 Switching Protocols in 12 milliseconds`: "",
		`I1023 20:41:07.123456   4242 round_trippers.go:553] GET https://127.0.0.1:6443/api/v1/namespaces/cluster1/pods/missing 404 Not Found in 3 milliseconds`:                       "",
		"Forwarding from 127.0.0.1:8083 -> 8083": "",
		"Handling connection for 8083":           "",
		"":                                       "",
	} {
		if got := Signal(line); got != want {
			t.Errorf("%q: %q want %q", line, got, want)
		}
	}
}

func TestScanLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pf.log")
	write := func(s string) {
		fh, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = fh.WriteString(s)
		_ = fh.Close()
	}
	if s := ScanLog("", 0); s != (Scan{}) {
		t.Fatalf("no log: %+v", s)
	}
	if s := ScanLog(path, 0); s != (Scan{}) {
		t.Fatalf("missing log: %+v", s)
	}
	write("I1023 20:41:07.000001   42 round_trippers.go:553] POST https://k/api/v1/namespaces/cluster1/pods/kafka-connect-6f7db8b4cb-ff4sp/portforward 101 Switching Protocols in 9 milliseconds\n" +
		"Forwarding from 127.0.0.1:8083 -> 8083\nHandling connection for 8083\nerror: lost con")
	s := ScanLog(path, 0)
	if s.Via != "kafka-connect-6f7db8b4cb-ff4sp" || s.Reason != "" {
		t.Fatalf("first scan: %+v", s)
	}
	write("nection to pod\nE1023 20:41:08.000001   42 portforward.go:413] an error occurred forwarding 8083 -> 8083: pod not found\n")
	s2 := ScanLog(path, s.Offset)
	if s2.Reason != "lost connection to pod" || s2.Line != "error: lost connection to pod" || s2.Via != "" {
		t.Fatalf("second scan: %+v", s2)
	}
	if s3 := ScanLog(path, s2.Offset); s3.Reason != "" || s3.Offset != s2.Offset {
		t.Fatalf("nothing new: %+v", s3)
	}
	_ = os.WriteFile(path, []byte("error: lost connection to pod\n"), 0o644)
	if s4 := ScanLog(path, s2.Offset); s4.Reason != "lost connection to pod" {
		t.Fatalf("replaced file: %+v", s4)
	}
}

func TestExitReason(t *testing.T) {
	dir := t.TempDir()
	log := func(s string) string {
		p := filepath.Join(dir, strconv.Itoa(len(s))+".log")
		_ = os.WriteFile(p, []byte(s), 0o644)
		return p
	}
	if r, _ := ExitReason(log("Forwarding from 127.0.0.1:8083 -> 8083\nerror: lost connection to pod\n")); r != "lost connection to pod" {
		t.Fatalf("signal: %q", r)
	}
	if r, l := ExitReason(log("I1023 20:41:07.000001   42 x.go:1] GET https://k 200 OK\nerror: something odd\n")); r != "kubectl exited: error: something odd" || l != "error: something odd" {
		t.Fatalf("last line: %q %q", r, l)
	}
	if r, _ := ExitReason(filepath.Join(dir, "none")); r != "kubectl exited" {
		t.Fatalf("no log: %q", r)
	}

	if msg := startError("I1023 20:41:07.000001   42 x.go:1] GET https://k 200 OK\nUnable to listen on port 8083\n"); msg != "Unable to listen on port 8083" {
		t.Fatalf("start error %q", msg)
	}
}

func probeFake(objs map[string]string, fail map[string]bool, calls *[]string) *shell.Fake {
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		ns := ""
		var refs []string
		for i, a := range r.Args {
			if a == "-n" {
				ns = r.Args[i+1]
			}
			if strings.Contains(a, "/") && i > 0 && r.Args[i-1] != "--kubeconfig" && !strings.HasPrefix(a, "-") {
				refs = append(refs, a)
			}
		}
		*calls = append(*calls, ns+": "+strings.Join(refs, " "))
		if fail[ns] {
			return shell.Fail(1, "Unable to connect to the server: dial tcp: i/o timeout")
		}
		var items []string
		for _, ref := range refs {
			if o, ok := objs[ns+"/"+ref]; ok {
				items = append(items, o)
			}
		}
		switch len(items) {
		case 0:
			return shell.OK("")
		case 1:
			return shell.OK(items[0])
		}
		return shell.OK(`{"kind":"List","items":[` + strings.Join(items, ",") + `]}`)
	}}
}

func podJSON(name, phase, deleting string) string {
	del := ""
	if deleting != "" {
		del = `,"deletionTimestamp":"` + deleting + `"`
	}
	return `{"kind":"Pod","metadata":{"name":"` + name + `"` + del + `},"status":{"phase":"` + phase + `"}}`
}

func TestProbe(t *testing.T) {
	a, b, c, d, e, g := liveProc(t), liveProc(t), liveProc(t), liveProc(t), liveProc(t), liveProc(t)
	objs := map[string]string{
		"cluster1/pod/ok":                             podJSON("ok", "Running", ""),
		"cluster1/pod/leaving":                        podJSON("leaving", "Running", "2026-10-23T20:40:00Z"),
		"cluster1/pod/kafka-connect-6f7db8b4cb-ff4sp": podJSON("kafka-connect-6f7db8b4cb-ff4sp", "Running", ""),
		"shop/deploy/web":                             `{"kind":"Deployment","metadata":{"name":"web"},"status":{"readyReplicas":0}}`,
	}
	var calls []string
	fs := []Forward{
		{Namespace: "cluster1", Pod: "ok", PID: a, Kubeconfig: "/kc", Context: "kind"},
		{Namespace: "cluster1", Pod: "kafka-connect-7bf45b66c5-cwpzj", PID: b, Kubeconfig: "/kc", Context: "kind"},
		{Namespace: "cluster1", Pod: "leaving", PID: c, Kubeconfig: "/kc", Context: "kind"},
		{Namespace: "cluster1", Pod: "kafka-connect", Kind: "deploy", Via: "kafka-connect-6f7db8b4cb-ff4sp", PID: d, Kubeconfig: "/kc", Context: "kind"},
		{Namespace: "shop", Pod: "web", Kind: "deploy", PID: e, Kubeconfig: "/kc", Context: "kind"},
		{Namespace: "shop", Pod: "web", Kind: "svc", PID: g, Kubeconfig: "/kc", Context: "kind"},
		{Namespace: "cluster1", Pod: "dead-already", PID: deadPIDForTest},
		{Namespace: "down", Pod: "x", PID: a, Kubeconfig: "/kc", Context: "kind"},
	}
	dead := Probe(probeFake(objs, map[string]bool{"down": true}, &calls), fs)
	want := map[int]string{b: "pod gone", c: "pod terminating", e: "no ready pods"}
	if len(dead) != len(want) {
		t.Fatalf("dead %v", dead)
	}
	for pid, w := range want {
		if dead[pid] != w {
			t.Errorf("pid %d: %q want %q", pid, dead[pid], w)
		}
	}
	if len(calls) != 3 || calls[0] != "cluster1: pod/kafka-connect-6f7db8b4cb-ff4sp pod/kafka-connect-7bf45b66c5-cwpzj pod/leaving pod/ok" || calls[1] != "shop: deploy/web" {
		t.Fatalf("calls %q", calls)
	}

	calls = nil
	dead = Probe(probeFake(map[string]string{"cluster1/pod/done": podJSON("done", "Succeeded", "")}, nil, &calls),
		[]Forward{{Namespace: "cluster1", Pod: "done", PID: a}})
	if dead[a] != "pod Succeeded" {
		t.Fatalf("finished pod: %v", dead)
	}
}

const deadPIDForTest = 1 << 22

func TestGuessOwner(t *testing.T) {
	guess := func(listing, pod string) (string, error) {
		c := &fakeCluster{listing: listing}
		return GuessOwner(c.runner(), "/kc", "kind", "cluster1", pod)
	}
	listing := "Deployment/kafka-connect\nDeployment/kafka\nStatefulSet/kafka\nPod/kafka-connect-6f7db8b4cb-ff4sp\nPod/kafka-connect-6f7db8b4cb-khf7b\nPod/kafka-0\n"
	if got, err := guess(listing, "kafka-connect-7bf45b66c5-cwpzj"); err != nil || got != "deploy/kafka-connect" {
		t.Fatalf("deploy: %q %v", got, err)
	}
	if got, err := guess(listing, "kafka-1"); err != nil || got != "sts/kafka" {
		t.Fatalf("sts: %q %v", got, err)
	}

	if _, err := guess("Deployment/kafka-connect\n", "kafka-connect-7bf45b66c5-cwpzj"); err == nil || !strings.Contains(err.Error(), "no deployment or statefulset in cluster1 has pods named like it") {
		t.Fatalf("no pods: %v", err)
	}

	amb := "Deployment/db\nStatefulSet/db-7bf45b66c5\nPod/db-6f7db8b4cb-aaaaa\nPod/db-7bf45b66c5-0\n"
	if _, err := guess(amb, "db-7bf45b66c5-12345"); err == nil || !strings.Contains(err.Error(), "deploy/db, sts/db-7bf45b66c5 all match") {
		t.Fatalf("ambiguous: %v", err)
	}
}

func TestRestartAfterRollingRestart(t *testing.T) {
	pid := liveProc(t)
	c := &fakeCluster{
		pods:   map[string]bool{"kafka-connect-7bf45b66c5-cwpzj": true},
		owners: map[string]string{"pod/kafka-connect-7bf45b66c5-cwpzj": "ReplicaSet/kafka-connect-7bf45b66c5", "rs/kafka-connect-7bf45b66c5": "Deployment/kafka-connect"},
		spawn:  func() shell.Result { return shell.OK(strconv.Itoa(pid) + "\n") },
		log:    "I1023 20:41:07.000001   42 round_trippers.go:553] POST https://k/api/v1/namespaces/cluster1/pods/kafka-connect-6f7db8b4cb-ff4sp/portforward 101 Switching Protocols in 9 milliseconds\nForwarding from 127.0.0.1:8083 -> 8083\n",
	}
	r := c.runner()
	owner := OwnerOf(r, "/kc", "kind-kindest", "cluster1", "kafka-connect-7bf45b66c5-cwpzj")
	if owner != "deploy/kafka-connect" {
		t.Fatalf("recorded owner %q (must be the Deployment, not the ReplicaSet)", owner)
	}
	c.pods = map[string]bool{}
	f := Forward{Namespace: "cluster1", Pod: "kafka-connect-7bf45b66c5-cwpzj", Local: "8083", Remote: "8083", PID: -1, Kubeconfig: "/kc", Context: "kind-kindest", Owner: owner, Reason: "pod gone"}
	nf, note, err := Restart(r, t.TempDir(), f)
	if err != nil || note != "pod gone → forwarding via deploy/kafka-connect" {
		t.Fatalf("restart: %v %q", err, note)
	}
	if c.spawns[0] != "deploy/kafka-connect 8083:8083" || nf.Reason != "" || nf.Note != note || nf.Owner != owner || nf.Log == "" {
		t.Fatalf("forward %+v spawns %v", nf, c.spawns)
	}
	if Display(nf) != "deploy/kafka-connect → pod/kafka-connect-6f7db8b4cb-ff4sp" {
		t.Fatalf("display %q", Display(nf))
	}
	if !Up(nf) || Up(Forward{PID: pid, Reason: "pod gone"}) {
		t.Fatal("Up must need a live process and no reason")
	}
}

func TestRestartWithoutRecordedOwner(t *testing.T) {
	pid := liveProc(t)
	c := &fakeCluster{
		pods:    map[string]bool{"web-5d9f8c7b6-abcde": true},
		owners:  map[string]string{"pod/web-5d9f8c7b6-abcde": "ReplicaSet/web-5d9f8c7b6", "rs/web-5d9f8c7b6": "Deployment/web"},
		spawn:   func() shell.Result { return shell.OK(strconv.Itoa(pid) + "\n") },
		listing: "Deployment/kafka-connect\nPod/kafka-connect-6f7db8b4cb-ff4sp\n",
	}
	r := c.runner()
	nf, note, err := Restart(r, t.TempDir(), Forward{Namespace: "shop", Pod: "web-5d9f8c7b6-abcde", Local: "8080", Remote: "80", PID: -1})
	if err != nil || note != "" || nf.Owner != "deploy/web" || Target(nf) != "pod/web-5d9f8c7b6-abcde" {
		t.Fatalf("pod exists: %+v %q %v", nf, note, err)
	}
	nf, note, err = Restart(r, t.TempDir(), Forward{Namespace: "cluster1", Pod: "kafka-connect-7bf45b66c5-cwpzj", Local: "8083", Remote: "8083", PID: -1})
	if err != nil || note != "guessed owner deploy/kafka-connect · pod gone → forwarding via deploy/kafka-connect" || Target(nf) != "deploy/kafka-connect" || nf.Owner != "deploy/kafka-connect" {
		t.Fatalf("guessed: %+v %q %v", nf, note, err)
	}
}

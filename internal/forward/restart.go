package forward

import (
	"fmt"
	"strings"
	"time"

	"k2/internal/shell"
)

var ownerKinds = map[string]string{
	"Deployment": "deploy", "StatefulSet": "sts", "DaemonSet": "ds",
	"ReplicaSet": "rs", "Job": "job", "ReplicationController": "rc",
}

const controllerJSONPath = `jsonpath={range .metadata.ownerReferences[?(@.controller==true)]}{.kind}/{.name}{end}`

func controllerOf(r shell.Runner, kc, ctx, ns, kind, name string) (string, string) {
	out, err := shell.Output(r, "kubectl", "get", kind, name, "-n", ns,
		"--kubeconfig", kc, "--context", ctx, "-o", controllerJSONPath)
	if err != nil {
		return "", ""
	}
	k, n, ok := strings.Cut(strings.TrimSpace(string(out)), "/")
	if !ok || n == "" || strings.Contains(n, "/") {
		return "", ""
	}
	return k, n
}

func OwnerOf(r shell.Runner, kc, ctx, ns, pod string) string {
	kind, name := controllerOf(r, kc, ctx, ns, "pod", pod)
	if kind == "ReplicaSet" {
		if k, n := controllerOf(r, kc, ctx, ns, "rs", name); k == "Deployment" {
			kind, name = k, n
		}
	}
	if short, ok := ownerKinds[kind]; ok {
		return short + "/" + name
	}
	return ""
}

func podState(r shell.Runner, f Forward) (bool, error) {
	res := r.Run(shell.Request{Name: "kubectl", Args: []string{"get", "pod", f.Pod, "-n", nsOr(f.Namespace),
		"--kubeconfig", f.Kubeconfig, "--context", f.Context, "-o", "jsonpath={.metadata.deletionTimestamp}"}})
	if res.Err == nil {
		return strings.TrimSpace(string(res.Stdout)) == "", nil
	}
	msg := strings.TrimSpace(string(res.Stderr) + " " + string(res.Stdout))
	if strings.Contains(msg, "NotFound") || strings.Contains(msg, "not found") {
		return false, nil
	}
	if msg == "" {
		msg = res.Err.Error()
	}
	return false, fmt.Errorf("%s", msg)
}

func nsOr(ns string) string {
	if ns == "" {
		return "default"
	}
	return ns
}

var stopWait = 300 * time.Millisecond

func Restart(r shell.Runner, logDir string, f Forward) (Forward, string, error) {
	if Alive(f.PID) {
		Stop(f.PID)
		time.Sleep(stopWait)
	}
	kind, name, note, owner := KindOr(f.Kind), f.Pod, "", f.Owner
	if kind == "pod" {
		exists, err := podState(r, f)
		switch {
		case err != nil:
			return Forward{}, "", err
		case exists && owner == "":
			owner = OwnerOf(r, f.Kubeconfig, f.Context, nsOr(f.Namespace), f.Pod)
		case !exists:
			guessed := ""
			if owner == "" {
				g, err := GuessOwner(r, f.Kubeconfig, f.Context, nsOr(f.Namespace), f.Pod)
				if err != nil {
					return Forward{}, "", err
				}
				owner, guessed = g, "guessed owner "+g+" · "
			}
			kind, name, _ = strings.Cut(owner, "/")
			note = guessed + "pod gone → forwarding via " + owner
		}
	}
	nf, err := SpawnTarget(r, logDir, f.Kubeconfig, f.Context, nsOr(f.Namespace), kind, name, f.Local, f.Remote)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "address already in use") {
			return Forward{}, "", fmt.Errorf("local port %s is busy (another process listens there): %s", f.Local, shell.OneLine(err.Error()))
		}
		return Forward{}, "", err
	}
	nf.Namespace = f.Namespace
	nf.Owner, nf.Auto, nf.Note = owner, f.Auto, note
	return nf, note, nil
}

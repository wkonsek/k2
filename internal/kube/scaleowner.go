package kube

import (
	"fmt"
	"strings"

	"k2/internal/shell"
)

const ownerPath = `jsonpath={range .metadata.ownerReferences[?(@.controller==true)]}{.kind}/{.name}{end}`

func ownerOf(r shell.Runner, kc, ctx, ns, kind, name string) (string, string) {
	out, err := shell.Output(r, "kubectl", "get", kind, name, "-n", ns, "--kubeconfig", kc, "--context", ctx, "-o", ownerPath)
	if err != nil {
		return "", ""
	}
	k, n, ok := strings.Cut(strings.TrimSpace(string(out)), "/")
	if !ok || n == "" || strings.Contains(n, "/") {
		return "", ""
	}
	return k, n
}

// ScalableOwner is the deployment or statefulset that owns a pod, plus its
// current replica count. A pod owned by a replicaset is followed up to the
// deployment. Anything else (a job, a bare pod) is an error.
func ScalableOwner(r shell.Runner, kc, ctx, ns, pod string) (kind, name, replicas string, err error) {
	okind, name := ownerOf(r, kc, ctx, ns, "pod", pod)
	switch okind {
	case "ReplicaSet":
		if k, n := ownerOf(r, kc, ctx, ns, "rs", name); k == "Deployment" {
			kind, name = "deploy", n
		}
	case "StatefulSet":
		kind = "sts"
	case "Deployment":
		kind = "deploy"
	}
	if kind == "" {
		return "", "", "", fmt.Errorf("%s is not owned by a deployment or statefulset", pod)
	}
	out, err := shell.Output(r, "kubectl", "get", kind, name, "-n", ns, "--kubeconfig", kc, "--context", ctx, "-o", "jsonpath={.spec.replicas}")
	if err != nil {
		return "", "", "", fmt.Errorf("%s", shell.FailText(out, err))
	}
	replicas = strings.TrimSpace(string(out))
	if replicas == "" {
		replicas = "1"
	}
	return kind, name, replicas, nil
}

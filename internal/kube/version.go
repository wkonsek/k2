package kube

import (
	"encoding/json"
	"strings"
	"time"

	"k2/internal/shell"
)

const InfoTimeout = 5 * time.Second

type Versions struct {
	Client    string
	Server    string
	OpenShift string
}

func (c Client) ClusterVersions(kubeconfig, context string) Versions {
	target := targetArgs(kubeconfig, context)
	timeout := "--request-timeout=" + InfoTimeout.String()
	args := append([]string{"version", "-o", "json", timeout}, target...)
	if context == "" {
		args = append(args, "--client")
	}
	out, _ := shell.Output(c.Run, "kubectl", args...)
	v := ParseVersion(out)
	if v.Server == "" || kubeconfig == "" || context == "" {
		return v
	}
	args = append([]string{"get", "clusterversion", "version", "-o", "jsonpath={.status.desired.version}", timeout}, target...)
	if ocp, err := shell.Output(c.Run, "kubectl", args...); err == nil {
		v.OpenShift = strings.TrimSpace(string(ocp))
	}
	return v
}

func ParseVersion(out []byte) Versions {
	var doc struct {
		Client struct {
			GitVersion string `json:"gitVersion"`
		} `json:"clientVersion"`
		Server struct {
			GitVersion string `json:"gitVersion"`
		} `json:"serverVersion"`
	}

	_ = json.NewDecoder(strings.NewReader(string(out))).Decode(&doc)
	return Versions{Client: doc.Client.GitVersion, Server: doc.Server.GitVersion}
}

func targetArgs(kubeconfig, context string) []string {
	var out []string
	if kubeconfig != "" {
		out = append(out, "--kubeconfig", kubeconfig)
	}
	if context != "" {
		out = append(out, "--context", context)
	}
	return out
}

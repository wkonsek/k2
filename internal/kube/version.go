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
	Reachable bool
}

func (c Client) ClusterVersions(kubeconfig, context string) Versions {
	target := targetArgs(kubeconfig, context)
	args := append([]string{"version", "-o", "json"}, target...)
	if context == "" {
		args = append(args, "--client")
	}
	res := c.Run.Run(shell.Request{Name: "kubectl", Args: args})
	v := ParseVersion(res.Stdout)
	if v.Server == "" && context != "" {
		raw := append([]string{"get", "--raw", "/version"}, target...)
		rawRes := c.Run.Run(shell.Request{Name: "kubectl", Args: raw})
		if s := parseRawVersion(rawRes.Stdout); s != "" {
			v.Server = s
			res = rawRes
		} else if !httpAnswered(res) {
			res = rawRes
		}
	}
	v.Reachable = versionReachable(res, v)
	if v.Server == "" || kubeconfig == "" || context == "" {
		return v
	}
	args = append([]string{"get", "clusterversion", "version", "-o", "jsonpath={.status.desired.version}"}, target...)
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

func parseRawVersion(out []byte) string {
	var doc struct {
		GitVersion string `json:"gitVersion"`
	}
	if json.NewDecoder(strings.NewReader(string(out))).Decode(&doc) != nil {
		return ""
	}
	return doc.GitVersion
}

func versionReachable(res shell.Result, v Versions) bool {
	if v.Server != "" || res.Err == nil {
		return true
	}
	return httpAnswered(res)
}

func httpAnswered(res shell.Result) bool {
	text := strings.ToLower(string(res.Stdout) + "\n" + string(res.Stderr))
	if res.Err != nil {
		text += "\n" + strings.ToLower(res.Err.Error())
	}
	for _, s := range []string{
		"unauthorized",
		"forbidden",
		"you must be logged in",
		"the server has asked for the client to provide credentials",
		"error from server",
		"status code",
		"(401)",
		"(403)",
		"error: the server",
	} {
		if strings.Contains(text, s) {
			return true
		}
	}
	return false
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

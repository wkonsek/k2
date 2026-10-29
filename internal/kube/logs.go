package kube

import (
	"fmt"
	"strings"
	"time"

	"k2/internal/shell"
)

type LogOpts struct {
	Container string
	Previous  bool
}

func (o LogOpts) args() []string {
	var a []string
	if o.Container != "" {
		a = append(a, "-c", o.Container)
	}
	if o.Previous {
		a = append(a, "--previous")
	}
	return a
}

func LogsArgs(name, namespace, kubeconfig, context string, since time.Time, tail int, opt LogOpts) []string {
	args := []string{"logs", name, "-n", namespace, "--kubeconfig", kubeconfig, "--context", context}
	args = append(args, opt.args()...)
	if !since.IsZero() {
		args = append(args, "--since-time="+since.UTC().Format(time.RFC3339))
	} else {
		args = append(args, fmt.Sprintf("--tail=%d", tail))
	}
	return args
}

func (c Client) Logs(name, namespace, kubeconfig, context string, since time.Time, tail int, opt LogOpts) (string, error) {
	out, err := shell.Combined(c.Run, "kubectl", LogsArgs(name, namespace, kubeconfig, context, since, tail, opt)...)
	text := strings.TrimSpace(string(out))
	if err != nil {
		return "", fmt.Errorf("%s", shell.FailText(out, err))
	}
	return text, nil
}

func (c Client) SinceLogs(name, namespace, kubeconfig, context string, since time.Time, opt LogOpts) ([]byte, error) {
	args := LogsArgs(name, namespace, kubeconfig, context, since, 0, opt)
	out, err := shell.Combined(c.Run, "kubectl", args...)
	if err != nil && strings.TrimSpace(string(out)) == "" {
		return nil, fmt.Errorf("%s", shell.FailText(out, err))
	}
	if err != nil && !strings.Contains(string(out), "\n") {
		return nil, fmt.Errorf("%s", shell.FailText(out, err))
	}
	return out, nil
}

func (c Client) AllLogs(name, namespace, kubeconfig, context string, opt LogOpts) ([]byte, error) {
	args := append([]string{"logs", name, "-n", namespace, "--kubeconfig", kubeconfig, "--context", context}, opt.args()...)
	out, err := shell.Combined(c.Run, "kubectl", args...)
	if err != nil && strings.TrimSpace(string(out)) == "" {
		return nil, err
	}
	if err != nil && !strings.Contains(string(out), "\n") {
		return nil, fmt.Errorf("%s", shell.FailText(out, err))
	}
	return out, nil
}

func AppendLogs(old, neu string) string {
	neu = strings.TrimSpace(neu)
	if neu == "" {
		return old
	}
	if old == "" {
		return neu
	}
	if strings.HasSuffix(old, neu) {
		return old
	}
	ol := strings.Split(old, "\n")
	nl := strings.Split(neu, "\n")
	for k := len(ol); k > 0; k-- {
		if k > len(nl) {
			continue
		}
		if strings.Join(ol[len(ol)-k:], "\n") == strings.Join(nl[:k], "\n") {
			nl = nl[k:]
			break
		}
	}
	if len(nl) == 0 {
		return old
	}
	return old + "\n" + strings.Join(nl, "\n")
}

func TrimLog(s string, max int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= max {
		return s
	}
	return strings.Join(lines[len(lines)-max:], "\n")
}

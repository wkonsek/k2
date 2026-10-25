package kube

import (
	"strings"

	"k2/internal/shell"
)

func BuildArgs(line, kc, ctx, ns, format string, wantJSON bool) []string {
	fields := strings.Fields(line)
	has := func(flags ...string) bool {
		for _, f := range fields {
			for _, flag := range flags {
				if f == flag || strings.HasPrefix(f, flag+"=") {
					return true
				}
			}
		}
		return false
	}
	args := append([]string{}, fields...)
	if kc != "" && !has("--kubeconfig") {
		args = append(args, "--kubeconfig", kc)
	}
	if ctx != "" && !has("--context") {
		args = append(args, "--context", ctx)
	}
	if !has("-n", "--namespace", "-A", "--all-namespaces") {
		if ns != "" {
			args = append(args, "-n", ns)
		} else if len(fields) > 0 {
			switch fields[0] {
			case "get", "top", "delete", "describe":
				if len(fields) <= 2 {
					args = append(args, "--all-namespaces")
				}
			}
		}
	}
	if wantJSON {
		format = "json"
	}
	if format != "" && !has("-o", "--output") {
		args = append(args, "-o", format)
	}
	return args
}

func (c Client) RunLine(line, kc, ctx, ns, format, jqExpr string) (string, int) {
	args := BuildArgs(line, kc, ctx, ns, format, jqExpr != "")
	res := c.Run.Run(shell.Request{Name: "kubectl", Args: args})
	code := 0
	if res.Err != nil {
		if res.Code > 0 {
			code = res.Code
		} else {
			code = 1
		}
	}
	out := string(res.Stdout)
	if jqExpr != "" && strings.TrimSpace(out) != "" {
		jq := c.Run.Run(shell.Request{Name: "jq", Args: []string{jqExpr}, Stdin: out, Combined: true})
		if jq.Err != nil {
			out = strings.TrimSpace(out) + "\n\njq: " + strings.TrimSpace(string(jq.Stdout))
			if code == 0 {
				code = 1
			}
		} else {
			out = string(jq.Stdout)
		}
	}
	text := strings.TrimSpace(out)
	if errText := strings.TrimSpace(string(res.Stderr)); errText != "" {
		if text != "" {
			text = errText + "\n" + text
		} else {
			text = errText
		}
	}
	if text == "" {
		text = "kubectl finished"
	}
	return text, code
}

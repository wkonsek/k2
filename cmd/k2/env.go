package main

import (
	"fmt"
	"os"
	"strings"

	"k2/internal/config"
	"k2/internal/kube"
	"k2/internal/shell"
	"k2/internal/state"
)

func isEnv(args []string) bool {
	return len(args) > 0 && (args[0] == "env" || args[0] == "-e")
}

func runEnv(args []string) int {
	out, errText, code := envFromDisk(args, func(kc, ctx, ns string) {
		kube.New(shell.Exec{}).PersistSelection(kc, ctx, ns)
	})
	if errText != "" {
		fmt.Fprint(os.Stderr, errText)
	}
	fmt.Fprint(os.Stdout, out)
	return code
}

func envFromDisk(args []string, persist func(kubeconfig, context, namespace string)) (out, errText string, code int) {
	cfg, _ := config.Load()
	return envAction(args, state.Load(cfg.StateDir), persist)
}

func envAction(args []string, st state.State, persist func(kubeconfig, context, namespace string)) (out, errText string, code int) {
	if len(args) != 1 {
		name := "env"
		if len(args) > 0 {
			name = args[0]
		}
		return "", fmt.Sprintf("k2: %s takes no arguments\n", name), 1
	}
	if strings.TrimSpace(st.Kubeconfig) == "" || st.Kubeconfig == "no kubeconfig" {
		return "", "k2: no kubeconfig remembered yet; start k2 and pick one first\n", 1
	}
	if st.Context != "" && persist != nil {
		persist(st.Kubeconfig, st.Context, st.Namespace)
	}
	return exportLine(st.Kubeconfig), "", 0
}

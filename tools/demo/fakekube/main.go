package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	name := filepath.Base(os.Args[0])
	args := os.Args[1:]

	if name != "kubectl" && name != "helm" {
		if len(args) == 0 {
			fmt.Fprintln(os.Stderr, "usage: fakekube init | kubectl ARGS… | helm ARGS…")
			os.Exit(2)
		}
		switch args[0] {
		case "init":
			if err := initWorld(); err != nil {
				fmt.Fprintln(os.Stderr, "fakekube init:", err)
				os.Exit(1)
			}
			return
		case "token":
			if err := setToken(args[1:]); err != nil {
				fmt.Fprintln(os.Stderr, "fakekube token:", err)
				os.Exit(1)
			}
			return
		case "kubectl", "helm":
			name, args = args[0], args[1:]
		default:
			fmt.Fprintln(os.Stderr, "fakekube: unknown command", args[0])
			os.Exit(2)
		}
	}
	if demoDir() == "" {
		fmt.Fprintln(os.Stderr, name+": K2_DEMO_DIR is not set (this is the k2 demo fake, see docs/demo/setup.sh)")
		os.Exit(1)
	}

	if os.Getenv("K2_DEMO_TRACE") != "" {
		if f, err := os.OpenFile(filepath.Join(demoDir(), "trace.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
			fmt.Fprintln(f, name+" "+strings.Join(args, " "))
			f.Close()
		}
	}
	var code int
	if name == "helm" {
		code = helmMain(args)
	} else {
		code = kubectlMain(args)
	}
	os.Exit(code)
}

func demoDir() string { return os.Getenv("K2_DEMO_DIR") }

func unknown(prog string, args []string) int {
	if f, err := os.OpenFile(filepath.Join(demoDir(), "unknown.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		fmt.Fprintln(f, prog+" "+strings.Join(args, " "))
		f.Close()
	}
	fmt.Fprintf(os.Stderr, "error: unknown command %q for %q\n", strings.Join(args, " "), prog)
	return 1
}

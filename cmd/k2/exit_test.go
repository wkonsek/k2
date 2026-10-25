package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportLine(t *testing.T) {
	cases := map[string]string{
		"/home/me/.kube/config":     `export KUBECONFIG="/home/me/.kube/config"` + "\n",
		"/k/my config":              `export KUBECONFIG="/k/my config"` + "\n",
		"/k/$(touch x)`id`\"\\.yml": `export KUBECONFIG="/k/\$(touch x)\` + "`id\\`" + `\"\\.yml"` + "\n",
	}
	for in, want := range cases {
		if got := exportLine(in); got != want {
			t.Errorf("exportLine(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReportCrash(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	var errOut bytes.Buffer
	reportCrash(&errOut, "runtime error: index out of range [-1] (in update)", []byte("goroutine 1 [running]:\nk2/internal/ui.rowProblems(...)\n"))
	line := errOut.String()
	if strings.Count(line, "\n") != 1 || !strings.Contains(line, "index out of range [-1]") || !strings.Contains(line, "~/.k2/crash.log") {
		t.Fatalf("stderr %q", line)
	}
	data, err := os.ReadFile(filepath.Join(home, ".k2", "crash.log"))
	if err != nil || !strings.Contains(string(data), "rowProblems") || !strings.Contains(string(data), "panic: runtime error") {
		t.Fatalf("crash.log %q %v", data, err)
	}
}

func TestWrapperDocsInSync(t *testing.T) {
	var lines []string
	for _, l := range strings.Split(shellWrapper, "\n") {
		lines = append(lines, strings.TrimPrefix(l, "  "))
	}
	want := strings.Join(lines, "\n")
	data, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), want) {
		t.Fatalf("README does not show the wrapper:\n%s", want)
	}
}

func TestWrapperEvaluatesOnlyCleanExports(t *testing.T) {
	cases := []struct {
		name, script string
		wantCode     int
		wantKC       string
		wantErr      string
	}{
		{"clean exit", `echo 'export KUBECONFIG="/tmp/picked config"'`, 0, "/tmp/picked config", ""},
		{"clean exit, nothing to export", `exit 0`, 0, "unchanged", ""},

		{"panic text on stdout", `printf 'Caught panic:\n\nruntime error: index out of range [-1]\n\nRestoring terminal...\n'; touch "$MARK"; exit 1`, 1, "unchanged", ""},
		{"crash exit 2", `echo 'k2: crashed: boom; stack trace in ~/.k2/crash.log' >&2; exit 2`, 2, "unchanged", "k2: crashed"},
		{"exit 0 but garbage", `echo 'rm -rf "$MARK"'`, 0, "unchanged", ""},
		{"exit 0, export plus more", `printf 'export KUBECONFIG=/x\ntouch "$MARK.2"\n'`, 0, "unchanged", ""},
		{"args run directly", ``, 0, "unchanged", ""},
	}
	shells := map[string][]string{}
	for _, sh := range []string{"zsh", "bash"} {
		if p, err := exec.LookPath(sh); err == nil {
			shells[sh] = []string{p}
		}
	}
	if len(shells) == 0 {
		t.Skip("neither zsh nor bash installed")
	}
	if _, ok := shells["zsh"]; !ok {
		t.Log("zsh not installed; the wrapper is only checked with bash")
	}
	var fn []string
	for _, l := range strings.Split(shellWrapper, "\n") {
		fn = append(fn, strings.TrimPrefix(l, "  "))
	}
	for sh, argv := range shells {
		for _, c := range cases {
			t.Run(sh+"/"+c.name, func(t *testing.T) {
				dir := t.TempDir()
				mark := filepath.Join(dir, "mark")

				if err := os.WriteFile(mark, nil, 0o644); err != nil {
					t.Fatal(err)
				}
				body := c.script
				if c.name == "args run directly" {
					body = `echo "k2 v9 $*"`
				}
				if err := os.WriteFile(filepath.Join(dir, "k2"), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
					t.Fatal(err)
				}
				call := "k2"
				if c.name == "args run directly" {
					call = "k2 --version"
				}
				script := strings.Join(fn, "\n") + "\n" + call + "\ncode=$?\nprint_kc() { printf '%s' \"$KUBECONFIG\" > \"$MARK.kc\"; }\nprint_kc\nexit $code\n"
				cmd := exec.Command(argv[0], "-c", script)
				if sh == "zsh" {
					cmd = exec.Command(argv[0], "-f", "-c", script)
				}
				cmd.Env = append(os.Environ(), "PATH="+dir+":/usr/bin:/bin", "KUBECONFIG=unchanged", "MARK="+mark)
				var stdout, stderr bytes.Buffer
				cmd.Stdout, cmd.Stderr = &stdout, &stderr
				err := cmd.Run()
				code := 0
				if ee, ok := err.(*exec.ExitError); ok {
					code = ee.ExitCode()
				} else if err != nil {
					t.Fatal(err)
				}
				if code != c.wantCode {
					t.Errorf("exit %d, want %d (stderr %q)", code, c.wantCode, stderr.String())
				}
				kc, _ := os.ReadFile(mark + ".kc")
				if string(kc) != c.wantKC {
					t.Errorf("KUBECONFIG %q, want %q", kc, c.wantKC)
				}
				if _, err := os.Stat(mark); err != nil {
					t.Error("garbage was evaluated: the mark file is gone")
				}
				if _, err := os.Stat(mark + ".2"); err == nil {
					t.Error("garbage after the export line was evaluated")
				}
				if strings.Contains(stderr.String(), "command not found") || strings.Contains(stderr.String(), "no matches found") {
					t.Errorf("the shell evaluated text: %q", stderr.String())
				}
				if c.wantErr != "" && !strings.Contains(stderr.String(), c.wantErr) {
					t.Errorf("stderr %q lacks %q", stderr.String(), c.wantErr)
				}
				if c.name == "args run directly" && !strings.Contains(stdout.String(), "k2 v9 --version") {
					t.Errorf("stdout %q", stdout.String())
				}
			})
		}
	}
}

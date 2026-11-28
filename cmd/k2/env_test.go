package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k2/internal/state"
)

func TestIsEnv(t *testing.T) {
	for _, a := range []string{"env", "-e"} {
		if !isEnv([]string{a}) || !isEnv([]string{a, "extra"}) {
			t.Errorf("%s should be env", a)
		}
	}
	for _, args := range [][]string{nil, {"--help"}, {"-v"}, {"kind-cache"}, {"--env"}, {"e"}, {"--e"}} {
		if isEnv(args) {
			t.Errorf("%v must not be env", args)
		}
	}
}

func TestEnvAction(t *testing.T) {
	kc := `/home/me/.kube/config`
	st := state.State{Kubeconfig: kc, Context: "kind-dev", Namespace: "shop"}
	var gotKC, gotCtx, gotNS string
	persist := func(kubeconfig, context, namespace string) {
		gotKC, gotCtx, gotNS = kubeconfig, context, namespace
	}

	out, errText, code := envAction([]string{"env"}, st, persist)
	if code != 0 || errText != "" || out != exportLine(kc) {
		t.Fatalf("env: %d %q %q", code, out, errText)
	}
	if gotKC != kc || gotCtx != "kind-dev" || gotNS != "shop" {
		t.Fatalf("persist %q %q %q", gotKC, gotCtx, gotNS)
	}

	gotKC, gotCtx, gotNS = "", "", ""
	out, errText, code = envAction([]string{"-e"}, st, persist)
	if code != 0 || errText != "" || out != exportLine(kc) || gotKC != kc {
		t.Fatalf("-e: %d %q %q persist %q", code, out, errText, gotKC)
	}

	spacey := `/k/my config`
	out, errText, code = envAction([]string{"env"}, state.State{Kubeconfig: spacey, Context: "c"}, persist)
	if code != 0 || out != exportLine(spacey) || errText != "" {
		t.Fatalf("space: %d %q %q", code, out, errText)
	}
}

func TestEnvActionNoContextSkipsPersist(t *testing.T) {
	called := false
	out, errText, code := envAction([]string{"env"}, state.State{Kubeconfig: "/kc"}, func(string, string, string) {
		called = true
	})
	if code != 0 || errText != "" || out != exportLine("/kc") || called {
		t.Fatalf("%d %q %q called=%v", code, out, errText, called)
	}
}

func TestEnvActionNothingRemembered(t *testing.T) {
	persist := func(string, string, string) {
		t.Fatal("persist must not run")
	}
	for _, st := range []state.State{{}, {Kubeconfig: "  "}, {Kubeconfig: "no kubeconfig", Context: "dev"}} {
		out, errText, code := envAction([]string{"env"}, st, persist)
		if code != 1 || out != "" || !strings.Contains(errText, "no kubeconfig remembered yet") || !strings.HasSuffix(errText, "\n") {
			t.Fatalf("st=%+v: %d %q %q", st, code, out, errText)
		}
		if strings.Contains(errText, "export KUBECONFIG") {
			t.Fatalf("error leaked an export: %q", errText)
		}
	}
}

func TestEnvActionExtraArgs(t *testing.T) {
	st := state.State{Kubeconfig: "/kc", Context: "c"}
	persist := func(string, string, string) {
		t.Fatal("persist must not run")
	}
	for _, args := range [][]string{{"env", "foo"}, {"-e", "--help"}, {"env", "-e"}} {
		out, errText, code := envAction(args, st, persist)
		if code != 1 || out != "" || !strings.Contains(errText, "takes no arguments") {
			t.Fatalf("%v: %d %q %q", args, code, out, errText)
		}
		if !strings.Contains(errText, args[0]) {
			t.Fatalf("error should name %s: %q", args[0], errText)
		}
	}
}

func TestEnvFromDisk(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	var got string
	persist := func(kc, _, _ string) { got = kc }

	out, errText, code := envFromDisk([]string{"env"}, persist)
	if code != 1 || out != "" || !strings.Contains(errText, "no kubeconfig remembered yet") || got != "" {
		t.Fatalf("empty: %d %q %q persist %q", code, out, errText, got)
	}

	kc := filepath.Join(home, ".kube", "prod.yaml")
	state.Save(filepath.Join(home, ".k2"), state.State{Kubeconfig: kc, Context: "prod", Namespace: "ops"})
	out, errText, code = envFromDisk([]string{"-e"}, persist)
	if code != 0 || errText != "" || out != exportLine(kc) || got != kc {
		t.Fatalf("saved: %d %q %q persist %q", code, out, errText, got)
	}
}

func TestEnvFromDiskUsesStateDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	custom := filepath.Join(home, "custom-state")
	kc := "/opt/clusters/dev.yaml"
	state.Save(custom, state.State{Kubeconfig: kc, Context: "dev"})
	if err := os.MkdirAll(filepath.Join(home, ".k2"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".k2", "config.yaml"), []byte("state_dir: "+custom+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var got string
	out, errText, code := envFromDisk([]string{"env"}, func(k, _, _ string) { got = k })
	if code != 0 || errText != "" || out != exportLine(kc) || got != kc {
		t.Fatalf("%d %q %q persist %q", code, out, errText, got)
	}
}

func TestEnvFromDiskCorruptState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	dir := filepath.Join(home, ".k2")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errText, code := envFromDisk([]string{"env"}, func(string, string, string) {
		t.Fatal("persist must not run")
	})
	if code != 1 || out != "" || !strings.Contains(errText, "no kubeconfig remembered yet") {
		t.Fatalf("%d %q %q", code, out, errText)
	}
}

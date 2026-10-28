package main

import (
	"strings"
	"testing"
)

func TestCLIAction(t *testing.T) {
	for _, a := range []string{"--version", "-v", "version"} {
		if out, exit := cliAction([]string{a}, "v9"); !exit || out != "k2 v9\n" {
			t.Errorf("%s: %q %v", a, out, exit)
		}
	}
	for _, a := range []string{"--help", "-h"} {
		out, exit := cliAction([]string{a}, "v9")
		if !exit || !strings.HasPrefix(out, "k2 - ") || !strings.Contains(out, "README.md") || !strings.Contains(out, `out="$(command k2)"`) {
			t.Errorf("%s: %q %v", a, out, exit)
		}
	}
	for _, args := range [][]string{nil, {"--whatever"}, {"pods"}, {"help-me"}} {
		if out, exit := cliAction(args, "v9"); exit || out != "" {
			t.Errorf("%v must start the UI, got %q", args, out)
		}
	}
}

func TestHelpDescribesExit(t *testing.T) {
	out, _ := cliAction([]string{"--help"}, "v")
	for _, want := range []string{"FOCUSED pane", "current-context", "namespace", `export KUBECONFIG="<that kubeconfig>"`} {
		if !strings.Contains(out, want) {
			t.Errorf("help lacks %q", want)
		}
	}
}

func TestStartArgs(t *testing.T) {
	profile, fresh, err := startArgs([]string{"--fresh"})
	if err != nil || profile != "" || !fresh {
		t.Fatalf("fresh: %q %v %v", profile, fresh, err)
	}
	profile, fresh, err = startArgs([]string{"--profile", "dev", "--fresh"})
	if err != nil || profile != "dev" || !fresh {
		t.Fatalf("both: %q %v %v", profile, fresh, err)
	}
	if _, _, err = startArgs([]string{"--profile"}); err == nil {
		t.Fatal("bare --profile")
	}
}
func TestReleaseString(t *testing.T) {
	if v := versionString(); version == "dev" && !strings.HasPrefix(v, release+"-dev") {
		t.Fatalf("versionString %q", v)
	}
}

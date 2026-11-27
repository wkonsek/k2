package state

import (
	"os"
	"strconv"
	"testing"
)

func TestKubectlHistoryRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if got := LoadKubectlHistory(dir); got != nil {
		t.Fatalf("missing file %v", got)
	}
	in := []string{"get pods -A", "apply -f ~/x.yaml", "get svc"}
	if err := SaveKubectlHistory(dir, in); err != nil {
		t.Fatal(err)
	}
	got := LoadKubectlHistory(dir)
	if len(got) != 3 || got[0] != in[0] || got[2] != in[2] {
		t.Fatalf("got %v", got)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != KubectlHistoryFile {
		t.Fatalf("leftover files %v", entries)
	}
}

func TestKubectlHistoryCap(t *testing.T) {
	dir := t.TempDir()
	var in []string
	for i := 0; i < 150; i++ {
		in = append(in, "get pod p"+strconv.Itoa(i))
	}
	if err := SaveKubectlHistory(dir, in); err != nil {
		t.Fatal(err)
	}
	got := LoadKubectlHistory(dir)
	if len(got) != KubectlHistoryCap || got[0] != "get pod p50" || got[99] != "get pod p149" {
		t.Fatalf("len %d first %q last %q", len(got), got[0], got[len(got)-1])
	}
}

func TestKubectlHistoryLoadDedupsAndSkipsBlank(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(KubectlHistoryPath(dir), []byte("get pods\nget pods\n\n  \nget svc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := LoadKubectlHistory(dir)
	if len(got) != 2 || got[0] != "get pods" || got[1] != "get svc" {
		t.Fatalf("got %q", got)
	}
}

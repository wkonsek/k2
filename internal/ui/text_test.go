package ui

import "testing"

func TestFitMiddle(t *testing.T) {
	if got := fitMiddle("abc", 10); got != "abc" {
		t.Fatalf("short: %q", got)
	}
	path := "/tmp/k2-helm-54192100/kafbat-ui-values.yaml"
	got := fitMiddle(path, 24)
	if got == path || !hasEllipsis(got) || len([]rune(got)) != 24 {
		t.Fatalf("got %q", got)
	}
	if got[:4] != "/tmp" || got[len(got)-5:] != ".yaml" {
		t.Fatalf("ends: %q", got)
	}
}

func hasEllipsis(s string) bool {
	for _, r := range s {
		if r == '…' {
			return true
		}
	}
	return false
}

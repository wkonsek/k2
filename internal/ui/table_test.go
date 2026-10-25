package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/x/ansi"

	"k2/internal/kube"
)

func TestRenderTableFitsWidth(t *testing.T) {
	headers := []string{"NAMESPACE", "NAME", "READY", "STATUS", "RESTARTS", "IP", "NODE", "AGE"}
	rows := [][]string{
		{"openshift-operator-lifecycle-manager", "catalog-operator-5d7f8c9b4-abcde", "1/1", "Running", "0", "10.128.0.12", "master-0.cluster.example.com", "12d"},
		{"shop", "web", "0/1", "CrashLoopBackOff", "7", "-", "-", "3m"},
	}
	for _, width := range []int{40, 60, 80, 120, 200} {
		header, colored, plain, widths, flex, _, ageIdx := renderTable(headers, rows, 3, width)
		if headers[flex] != "NAME" || headers[ageIdx] != "AGE" {
			t.Fatalf("flex/age columns %d %d", flex, ageIdx)
		}
		for i, line := range append([]string{header}, plain...) {
			if w := ansi.StringWidth(line); w > width {
				t.Errorf("width %d: line %d is %d wide: %q", width, i, w, line)
			}
		}
		for i := range colored {
			if ansi.Strip(colored[i]) != plain[i] {
				t.Errorf("colored and plain rows differ")
			}
		}
		if widths[ageIdx] < 3 {
			t.Errorf("AGE column squeezed to %d", widths[ageIdx])
		}
	}
}

func TestAgeColumnIsLive(t *testing.T) {
	born := time.Now().Add(-90 * time.Second)
	_, items, _ := packTable([]string{"NAME", "AGE"}, [][]string{{"web", "5s"}}, [][2]string{{"shop", "web"}}, []time.Time{born}, -1, 40, "shop")
	it := items[0].(item).aged()
	if !strings.Contains(it.plain, kube.Age(born)) || strings.Contains(it.plain, "5s") {
		t.Fatalf("age not refreshed: %q", it.plain)
	}
	if items[0].(item).id != "web" {
		t.Fatal("id in a single namespace is the bare name")
	}
	_, items, _ = packTable([]string{"NAMESPACE", "NAME", "AGE"}, [][]string{{"a", "web", "1s"}}, [][2]string{{"a", "web"}}, nil, -1, 40, "")
	if items[0].(item).id != "a/web" {
		t.Fatal("id across namespaces is namespace/name")
	}
}

func TestColorYAMLKeepsText(t *testing.T) {
	for _, line := range []string{"apiVersion: v1", "  - name: web", "    replicas: 3", "# comment", `    image: "nginx"`, "  enabled: true", "plain text"} {
		if got := ansi.Strip(colorYAML(line)); got != line {
			t.Errorf("colorYAML(%q) text = %q", line, got)
		}
	}
}

func TestColumnWidthsFollowContent(t *testing.T) {
	headers := []string{"NAMESPACE", "NAME", "READY", "STATUS", "RESTARTS", "IP", "NODE", "AGE"}
	rows := [][]string{
		{"cert-manager", "cert-manager-58b7ddd96b-6klxw", "1/1", "Running", "28", "10.244.0.16", "kindest-control-plane", "13d"},
		{"kube-system", "etcd-kindest-control-plane", "1/1", "Running", "28", "172.28.0.2", "kindest-control-plane", "13d"},
	}
	header, _, plain, widths, _, _, _ := renderTable(headers, rows, 3, 220)
	want := []int{12, 29, 5, 7, 8, 11, 21, 7}
	for i := range want {
		if widths[i] != want[i] {
			t.Fatalf("widths %v, want %v", widths, want)
		}
	}
	if !strings.HasPrefix(header, "NAMESPACE     NAME                           READY  STATUS ") {
		t.Fatalf("header %q", header)
	}
	if !strings.Contains(plain[0], "cert-manager-58b7ddd96b-6klxw  1/1    Running  28 ") {
		t.Fatalf("row %q", plain[0])
	}
	if w := ansi.StringWidth(plain[0]); w >= 150 {
		t.Fatalf("row padded to %d cells", w)
	}

	_, _, _, narrow, _, _, _ := renderTable(headers, rows, 3, 100)
	if want := []int{12, 24, 5, 7, 8, 11, 12, 7}; fmt.Sprint(narrow) != fmt.Sprint(want) {
		t.Fatalf("narrow widths %v, want %v", narrow, want)
	}

	_, _, _, tiny, _, _, _ := renderTable(headers, rows, 3, 40)
	if s := sum(tiny) + 14; s > 40 || tiny[7] < 3 || tiny[1] < 4 {
		t.Fatalf("tiny widths %v (%d)", tiny, s)
	}

	_, items, _ := packTable(headers, rows, [][2]string{{"cert-manager", "a"}, {"kube-system", "b"}}, nil, 3, 220, "")
	lst := list.New(items, rowDelegate{width: 80}, 80, 10)
	reflowList(&lst, 80)
	if it := lst.Items()[0].(item); strings.Contains(it.plain, "6klxw") {
		t.Fatalf("not narrowed: %q", it.plain)
	}
	reflowList(&lst, 220)
	if it := lst.Items()[0].(item); !strings.Contains(it.plain, "cert-manager-58b7ddd96b-6klxw  1/1") {
		t.Fatalf("not restored: %q", it.plain)
	}
}

func sum(xs []int) int {
	t := 0
	for _, x := range xs {
		t += x
	}
	return t
}

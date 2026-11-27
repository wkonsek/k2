package kube

import (
	"strings"
	"testing"
)

func TestParseDockerSubnets(t *testing.T) {
	got := ParseDockerSubnets("172.18.0.0/16 fc00:f853:ccd:e793::/64\n")
	if len(got) != 2 || got[0] != "172.18.0.0/16" || got[1] != "fc00:f853:ccd:e793::/64" {
		t.Fatalf("%q", got)
	}
	if ParseDockerSubnets("not a net") != nil {
		t.Fatal("junk")
	}
}

func TestMetalLBRange(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"172.18.0.0/16", "172.18.255.200-172.18.255.250"},
		{"10.0.0.0/16", "10.0.255.200-10.0.255.250"},
		{"192.168.1.0/24", "192.168.1.200-192.168.1.250"},
		{"10.42.0.0/20", "10.42.15.200-10.42.15.250"},
		{"10.0.0.0/28", "10.0.0.1-10.0.0.14"},
		{"10.1.1.0/25", "10.1.1.76-10.1.1.126"},
	}
	for _, c := range cases {
		got, err := MetalLBRange([]string{c.in})
		if err != nil || got != c.want {
			t.Errorf("%s: %q %v want %s", c.in, got, err, c.want)
		}
	}
	got, err := MetalLBRange([]string{"fc00:f853:ccd:e793::/64", "172.18.0.0/16"})
	if err != nil || got != "172.18.255.200-172.18.255.250" {
		t.Fatalf("skip v6: %q %v", got, err)
	}
	if _, err := MetalLBRange(nil); err == nil {
		t.Fatal("empty")
	}
	if _, err := MetalLBRange([]string{"fc00::/64"}); err == nil {
		t.Fatal("v6 only")
	}
}

func TestMetalLBManifest(t *testing.T) {
	got := MetalLBManifest("metallb-system", "k2-kind", "172.18.255.200-172.18.255.250")
	for _, want := range []string{
		"kind: IPAddressPool",
		"kind: L2Advertisement",
		"namespace: metallb-system",
		"name: k2-kind",
		"- 172.18.255.200-172.18.255.250",
		"- k2-kind",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in\n%s", want, got)
		}
	}
}

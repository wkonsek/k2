package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestProdMatch(t *testing.T) {
	eks := "arn:aws:eks:eu-west-1:123456789012:cluster/prod-x"
	cases := []struct {
		pattern, ctx string
		want         bool
	}{
		{"prod-eu-1", "prod-eu-1", true},
		{"prod-eu-1", "prod-eu-10", false},
		{"prod-eu-1", "PROD-EU-1", false},
		{"*prod*", "kind-prod", true},
		{"*prod*", "prod", true},
		{"*prod*", "kind-dev", false},
		{"prod-*", "prod-eu-1", true},
		{"prod-*", "kind-prod-eu", false},
		{"prod-eu-?", "prod-eu-2", true},
		{"prod-eu-?", "prod-eu-12", false},
		{"*prod*", eks, true},
		{"*cluster/prod-*", eks, true},
		{"arn:aws:eks:*:cluster/prod-?", eks, true},
		{"arn:aws:eks:*:cluster/dev-*", eks, false},
		{eks, eks, true},
		{"prod.eu", "prodXeu", false},
		{"re:^prod-(eu|us)-[0-9]+$", "prod-us-3", true},
		{"re:^prod-(eu|us)-[0-9]+$", "prod-ap-3", false},
		{"re:cluster/prod-", eks, true},
		{"re:[", "[", false},
		{"re:", "anything", false},
		{"", "", false},
		{"*", "", false},
	}
	for _, c := range cases {
		if got := ProdMatch(c.pattern, c.ctx); got != c.want {
			t.Errorf("ProdMatch(%q, %q) = %v, want %v", c.pattern, c.ctx, got, c.want)
		}
	}
}

func TestIsProdPatterns(t *testing.T) {
	if (Config{}).IsProd("prod-eu-1") {
		t.Fatal("empty list matched")
	}
	c := Config{Prod: []string{"prod-eu-1", "*-prod", "re:^live-"}}
	for ctx, want := range map[string]bool{"prod-eu-1": true, "kind-prod": true, "live-1": true, "kind-dev": false, "prod-eu-2": false} {
		if c.IsProd(ctx) != want {
			t.Errorf("IsProd(%q) != %v", ctx, want)
		}
	}
}

func TestParseProdPatterns(t *testing.T) {
	c, w := Parse([]byte("prod: [prod-eu-1, \"*prod*\", \"re:^live-[0-9]+$\", \"re:(\", \"re:\"]\n"))
	if !reflect.DeepEqual(c.Prod, []string{"prod-eu-1", "*prod*", "re:^live-[0-9]+$"}) {
		t.Fatalf("prod = %v", c.Prod)
	}
	if len(w) != 2 || !strings.Contains(w[0], `"re:(" is not a valid regex`) || !strings.Contains(w[1], "no expression") {
		t.Fatalf("warnings: %v", w)
	}
	if !c.IsProd("kind-prod") || !c.IsProd("live-7") || c.IsProd("kind-dev") {
		t.Fatal("IsProd")
	}
	c, w = Parse([]byte("prod: \"*prod*\"\n"))
	if len(w) != 0 || !c.IsProd("arn:aws:eks:eu-west-1:1:cluster/prod-x") {
		t.Fatalf("scalar pattern: %v %v", c.Prod, w)
	}
}

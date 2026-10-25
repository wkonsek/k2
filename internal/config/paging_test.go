package config

import (
	"strings"
	"testing"
)

func TestNoTmuxPrefixInDefaults(t *testing.T) {
	k := DefaultKeys()
	for _, f := range keyFields {
		for _, key := range *f.ptr(&k) {
			if key == "ctrl+b" {
				t.Errorf("keys.%s uses ctrl+b by default", f.name)
			}
			if key == "ctrl+f" && f.name != "port_forward" {
				t.Errorf("keys.%s uses ctrl+f by default (port_forward's key)", f.name)
			}
		}
	}

	if len(k.PageUp) != 2 || !k.PageUp.Has("pgup") || !k.PageUp.Has("{") || len(k.PageDown) != 2 || !k.PageDown.Has("pgdown") || !k.PageDown.Has("}") {
		t.Fatalf("paging %v %v", k.PageUp, k.PageDown)
	}

	cfg, w := Parse([]byte("keys: {page_up: [pgup, ctrl+b]}"))
	if len(w) != 0 || !cfg.Keys.PageUp.Has("ctrl+b") {
		t.Fatalf("ctrl+b by hand: %v %v", cfg.Keys.PageUp, w)
	}
}

func TestBracesConflict(t *testing.T) {
	_, w := Parse([]byte(`keys: {describe: "}"}`))
	if len(w) != 1 || !strings.Contains(w[0], "keys.describe: } is already used by page_down") {
		t.Fatalf("warnings %v", w)
	}
	k := DefaultKeys()
	if !k.NextTab.Has("direct ]") || !k.PrevTab.Has("direct [") {
		t.Fatalf("tabs %v %v", k.NextTab, k.PrevTab)
	}
}

func TestPageDownCtrlFConflicts(t *testing.T) {
	cfg, w := Parse([]byte("keys: {page_down: [pgdown, ctrl+f]}"))
	if len(w) != 1 || !strings.Contains(w[0], "keys.page_down: ctrl+f is already used by port_forward") {
		t.Fatalf("warnings %v", w)
	}
	if len(cfg.Keys.PageDown) != 2 || !cfg.Keys.PageDown.Has("}") || !cfg.Keys.PortForward.Has("ctrl+f") {
		t.Fatalf("page_down %v port_forward %v", cfg.Keys.PageDown, cfg.Keys.PortForward)
	}
}

func TestHealthEvery(t *testing.T) {
	if Default().PortForward.HealthEvery != 5 {
		t.Fatal("default")
	}
	if cfg, w := Parse([]byte("portforward: {health_every: 0}")); len(w) != 0 || cfg.PortForward.HealthEvery != 0 {
		t.Fatalf("0: %v %v", cfg.PortForward.HealthEvery, w)
	}
	if cfg, w := Parse([]byte("portforward: {health_every: 999}")); len(w) != 1 || cfg.PortForward.HealthEvery != 5 {
		t.Fatalf("999: %v %v", cfg.PortForward.HealthEvery, w)
	}
}

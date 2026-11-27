package kube

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPickLatestKindNodeTags(t *testing.T) {
	got := PickLatestKindNodeTags([]string{
		"latest",
		"v1.34.0",
		"v1.33.1",
		"v1.33.0",
		"v1.32.2",
		"v1.31.4",
		"v1.30.10",
		"v1.29.2",
		"v1.34.0-amd64",
		"v1.33.0-rc.1",
		"sha256-abc",
		"kindest/node:v1.32.0",
	}, 5)
	want := []string{
		"kindest/node:v1.34.0",
		"kindest/node:v1.33.1",
		"kindest/node:v1.32.2",
		"kindest/node:v1.31.4",
		"kindest/node:v1.30.10",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestParseKindNodeImagesJSON(t *testing.T) {
	raw := `{"results":[{"name":"latest"},{"name":"v1.34.0"},{"name":"v1.33.2"},{"name":"v1.33.1"},{"name":"v1.32.5"},{"name":"v1.31.9"},{"name":"v1.30.13"},{"name":"v1.34.0-amd64"}]}`
	got := ParseKindNodeImages([]byte(raw))
	if len(got) != 5 || got[0] != "kindest/node:v1.34.0" || got[1] != "kindest/node:v1.33.2" {
		t.Fatalf("%v", got)
	}
}

func TestFetchKindNodeImages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page_size") == "" {
			t.Errorf("missing page_size")
		}
		io.WriteString(w, `{"results":[{"name":"v1.34.0"},{"name":"v1.33.1"},{"name":"v1.32.2"},{"name":"v1.31.4"},{"name":"v1.30.10"}]}`)
	}))
	defer srv.Close()
	got, err := FetchKindNodeImages(srv.Client(), srv.URL+"?page_size=50&ordering=last_updated")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 || got[0] != "kindest/node:v1.34.0" {
		t.Fatalf("%v", got)
	}
}

func TestFetchKindNodeImagesOffline(t *testing.T) {
	client := &http.Client{Timeout: 50 * time.Millisecond}
	got, err := FetchKindNodeImages(client, "http://127.0.0.1:1/missing")
	if err == nil || got != nil {
		t.Fatalf("got %v err %v", got, err)
	}
}

func TestParseKindestNodeTag(t *testing.T) {
	if _, _, _, ok := ParseKindestNodeTag("v1.33.0-rc.1"); ok {
		t.Fatal("pre-release")
	}
	if _, _, _, ok := ParseKindestNodeTag("latest"); ok {
		t.Fatal("latest")
	}
	maj, min, pat, ok := ParseKindestNodeTag("v1.33.1")
	if !ok || maj != 1 || min != 33 || pat != 1 {
		t.Fatalf("%d %d %d %v", maj, min, pat, ok)
	}
}

package state

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"k2/internal/forward"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	dir := filepath.Join(t.TempDir(), "nested")
	st := State{
		Version:    Version,
		Kubeconfig: "/home/u/.kube/config", Context: "kind-dev", Namespace: "shop", Focus: 1,
		Tabs: []Tab{
			{Layout: Node{Pane: 0}},
			{Layout: Node{Split: "row", Kids: []Node{{Pane: 1}, {Split: "col", Kids: []Node{{Pane: 2}, {Pane: 3}}}}}, Focus: 3},
		},
		Tab: 1,
		Panes: []Pane{
			{Stage: "workpods", Kubeconfig: "/a", Context: "kind-dev", Namespace: "shop"},
			{Stage: "logs", LogPod: "olm/catalog-0", Kubeconfig: "/b", Context: "prod", Namespace: "olm"},
			{Stage: "nav"},
			{Stage: "res", Kind: "routes"},
		},
		Forwards:   []forward.Forward{{Namespace: "shop", Pod: "web", Local: "8080", Remote: "80", PID: 42, Started: time.Unix(1700000000, 0).UTC(), Kubeconfig: "/a", Context: "kind-dev"}},
		KustomRoot: "/src/deploy",
	}
	Save(dir, st)
	got := Load(dir)
	if !reflect.DeepEqual(got, st) {
		t.Fatalf("got %+v", got)
	}
}

func TestLoadMissingOrCorrupt(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	dir := t.TempDir()
	if !reflect.DeepEqual(Load(dir), State{}) {
		t.Fatal("missing file should give empty state")
	}
	_ = os.WriteFile(Path(dir), []byte("{not json"), 0o644)
	if !reflect.DeepEqual(Load(dir), State{}) {
		t.Fatal("corrupt file should give empty state")
	}
}

func TestLegacyFallback(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_STATE_HOME", xdg)
	_ = os.MkdirAll(filepath.Join(xdg, "k2"), 0o755)
	_ = os.WriteFile(filepath.Join(xdg, "k2", "state.json"), []byte(`{"context":"legacy"}`), 0o644)
	if Load(t.TempDir()).Context != "legacy" {
		t.Fatal("legacy state not read")
	}
}

func TestMigrateV02Fixture(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	dir := t.TempDir()
	b, err := os.ReadFile("testdata/state-old.json")
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(Path(dir), b, 0o644)
	st := Load(dir)
	if st.Version != Version || st.Tab != 0 || len(st.Tabs) != 1 || len(st.Panes) != 3 {
		t.Fatalf("got %+v", st)
	}
	want := Tab{Layout: Node{Split: "col", Kids: []Node{{Pane: 0}, {Pane: 1}, {Pane: 2}}}, Focus: 2}
	if !reflect.DeepEqual(st.Tabs[0], want) {
		t.Fatalf("tab %+v", st.Tabs[0])
	}
	if st.Panes[1].LogPod != "shop/web-1" || st.Panes[2].Context != "kind-prod" || len(st.Forwards) != 1 || st.Forwards[0].PID != 4242 || st.KustomRoot != "/src/deploy" {
		t.Fatalf("data lost: %+v", st)
	}

	Save(dir, st)
	if again := Load(dir); !reflect.DeepEqual(again, st) {
		t.Fatalf("second load %+v", again)
	}
}

func TestMigrateEdgeCases(t *testing.T) {
	one := Migrate(State{Panes: []Pane{{Stage: "nav"}}, Focus: 7})
	if !reflect.DeepEqual(one.Tabs, []Tab{{Layout: Node{Pane: 0}}}) {
		t.Fatalf("one pane: %+v", one.Tabs)
	}
	if none := Migrate(State{Context: "x"}); none.Tabs != nil || none.Version != Version {
		t.Fatalf("no panes: %+v", none)
	}
	cur := State{Version: Version, Panes: []Pane{{}}}
	if got := Migrate(cur); got.Tabs != nil {
		t.Fatal("a current state must not be touched")
	}
}

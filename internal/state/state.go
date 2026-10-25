package state

import (
	"encoding/json"
	"os"
	"path/filepath"

	"k2/internal/forward"
)

const Version = 2

const FileName = "state.json"

type State struct {
	Version    int               `json:"version"`
	Kubeconfig string            `json:"kubeconfig"`
	Context    string            `json:"context"`
	Namespace  string            `json:"namespace"`
	Focus      int               `json:"focus"`
	Panes      []Pane            `json:"panes"`
	Tabs       []Tab             `json:"tabs,omitempty"`
	Tab        int               `json:"tab"`
	Forwards   []forward.Forward `json:"forwards"`
	KustomRoot string            `json:"kustomRoot"`

	Favorites []Favorite `json:"favorites,omitempty"`
}

type Favorite struct {
	Kubeconfig string `json:"kubeconfig,omitempty"`
	Context    string `json:"context"`
	Namespace  string `json:"namespace,omitempty"`
	View       string `json:"view,omitempty"`
}

type Tab struct {
	Layout Node `json:"layout"`
	Focus  int  `json:"focus"`

	Name string `json:"name,omitempty"`
}

type Node struct {
	Split string `json:"split,omitempty"`
	Pane  int    `json:"pane"`
	Kids  []Node `json:"kids,omitempty"`
}

type Pane struct {
	Stage  string `json:"stage"`
	Kind   string `json:"kind"`
	LogPod string `json:"logPod"`

	Logs       string `json:"logs,omitempty"`
	LogsTitle  string `json:"logsTitle,omitempty"`
	Pod        string `json:"pod"`
	Kubeconfig string `json:"kubeconfig"`
	Context    string `json:"context"`
	Namespace  string `json:"namespace"`

	NsPrev string `json:"nsPrev,omitempty"`
}

func Path(dir string) string { return filepath.Join(dir, FileName) }

func legacyPath() string {
	if x := os.Getenv("XDG_STATE_HOME"); x != "" {
		return filepath.Join(x, "k2", "state.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "state", "k2", "state.json")
}

func Load(dir string) State {
	b, err := os.ReadFile(Path(dir))
	if err != nil {
		if legacy := legacyPath(); legacy != "" {
			b, err = os.ReadFile(legacy)
		}
	}
	if err != nil {
		return State{}
	}
	var st State
	if json.Unmarshal(b, &st) != nil {
		return State{}
	}
	return Migrate(st)
}

func Migrate(st State) State {
	if st.Version >= Version {
		return st
	}
	st.Tabs, st.Tab = nil, 0
	if n := len(st.Panes); n > 0 {
		root := Node{Split: "col"}
		for i := range st.Panes {
			root.Kids = append(root.Kids, Node{Pane: i})
		}
		if n == 1 {
			root = Node{Pane: 0}
		}
		focus := st.Focus
		if focus < 0 || focus >= n {
			focus = 0
		}
		st.Tabs = []Tab{{Layout: root, Focus: focus}}
	}
	st.Version = Version
	return st
}

func Save(dir string, st State) {
	st.Version = Version
	_ = os.MkdirAll(dir, 0o755)
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(Path(dir), b, 0o644)
}

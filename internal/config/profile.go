package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type ProfilePane struct {
	Kubeconfig string `yaml:"kubeconfig,omitempty"`
	Context    string `yaml:"context,omitempty"`
	Namespace  string `yaml:"namespace,omitempty"`
	View       string `yaml:"view,omitempty"`
	Target     string `yaml:"target,omitempty"`
	Selector   string `yaml:"selector,omitempty"`
}

// ProfileLayout is the pane tree. split right places children left to right,
// split down places them top to bottom, and pane is an index into Profile.Panes.
// A leaf has Pane set. A branch has Split and at least two Children.
type ProfileLayout struct {
	Split    string          `yaml:"split,omitempty"`
	Pane     *int            `yaml:"pane,omitempty"`
	Children []ProfileLayout `yaml:"children,omitempty"`
}

func (l ProfileLayout) Empty() bool {
	return l.Split == "" && l.Pane == nil && len(l.Children) == 0
}

type Profile struct {
	Split  string        `yaml:"split,omitempty"`
	Layout ProfileLayout `yaml:"layout,omitempty"`
	Panes  []ProfilePane `yaml:"panes,omitempty"`
}

func ValidProfileName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
		default:
			return false
		}
	}
	return true
}

func ProfileFile(dir, name string) string {
	return filepath.Join(dir, "profiles", name+".yaml")
}

func SaveProfile(dir, name string, prof Profile) error {
	if !ValidProfileName(name) {
		return fmt.Errorf("profile name must be letters, numbers, dot, dash or underscore")
	}
	if prof.Split == "" {
		prof.Split = "right"
		if prof.Layout.Split == "down" {
			prof.Split = "down"
		}
	}
	b, err := yaml.Marshal(prof)
	if err != nil {
		return err
	}
	path := ProfileFile(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func LoadProfileFile(dir, name string) (Profile, error) {
	b, err := os.ReadFile(ProfileFile(dir, name))
	if err != nil {
		return Profile{}, err
	}
	var prof Profile
	if err := yaml.Unmarshal(b, &prof); err != nil {
		return Profile{}, err
	}
	for i := range prof.Panes {
		prof.Panes[i].Kubeconfig = ExpandHome(prof.Panes[i].Kubeconfig)
	}
	return prof, nil
}

func (c Config) LookupProfile(name string) (Profile, error) {
	p, err := LoadProfileFile(c.StateDir, name)
	if err != nil {
		if os.IsNotExist(err) {
			return Profile{}, fmt.Errorf("no profile %q", name)
		}
		return Profile{}, err
	}
	return p, nil
}

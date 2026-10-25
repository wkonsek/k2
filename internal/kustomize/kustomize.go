package kustomize

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"k2/internal/shell"
)

func IsFile(name string) bool {
	switch strings.ToLower(name) {
	case "kustomization.yaml", "kustomization.yml", "kustomization":
		return true
	default:
		return false
	}
}

func FileIn(dir string) string {
	for _, name := range []string{"kustomization.yaml", "kustomization.yml", "Kustomization"} {
		p := filepath.Join(dir, name)
		st, err := os.Stat(p)
		if err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

type Entry struct {
	Name string
	Path string

	Kind string
}

var skipDirs = map[string]bool{".git": true, "node_modules": true, "vendor": true}

func List(dir string) ([]Entry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := []Entry{{Name: "..", Path: filepath.Dir(dir), Kind: "parent"}}
	var dirs []fs.DirEntry
	var others []Entry
	for _, e := range entries {
		name := e.Name()
		if skipDirs[name] {
			continue
		}
		if isDir(dir, e) {
			dirs = append(dirs, e)
			continue
		}
		if IsFile(name) {
			out = append(out, Entry{Name: name, Path: dir, Kind: "file"})
			continue
		}
		others = append(others, Entry{Name: name, Path: dir, Kind: "other"})
	}
	for _, e := range dirs {
		path := filepath.Join(dir, e.Name())
		kind := ""
		if FileIn(path) != "" {
			kind = "kust"
		}
		out = append(out, Entry{Name: e.Name(), Path: path, Kind: kind})
	}
	return append(out, others...), nil
}

func isDir(dir string, e fs.DirEntry) bool {
	if e.IsDir() {
		return true
	}
	if e.Type()&fs.ModeSymlink == 0 {
		return false
	}
	st, err := os.Stat(filepath.Join(dir, e.Name()))
	return err == nil && st.IsDir()
}

func IsYAML(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".yaml" || ext == ".yml"
}

func Find(root string) []string {
	if root == "" {
		return nil
	}
	skip := map[string]bool{".git": true, "node_modules": true, "vendor": true, ".terraform": true}
	var out []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && (skip[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			rel, _ := filepath.Rel(root, path)
			if rel != "." && strings.Count(rel, string(os.PathSeparator)) > 6 {
				return filepath.SkipDir
			}
			return nil
		}
		if IsFile(d.Name()) {
			out = append(out, filepath.Dir(path))
			if len(out) > 400 {
				return fs.SkipAll
			}
		}
		return nil
	})
	sort.Strings(out)
	return out
}

func Run(r shell.Runner, verb, dir, kc, ctx string) (string, int) {
	return RunStream(r, verb, dir, kc, ctx, nil)
}

func run(r shell.Runner, req shell.Request, verb string) (string, int) {
	res := r.Run(req)
	text := strings.TrimSpace(string(res.Stdout))
	errText := strings.TrimSpace(string(res.Stderr))
	if errText != "" {
		if text != "" {
			text += "\n" + errText
		} else {
			text = errText
		}
	}
	code := 0
	if res.Err != nil {
		if res.Code > 0 {
			code = res.Code
		} else {
			code = 1
			if text == "" {
				text = res.Err.Error()
			}
		}
	}
	if verb == "diff" && text == "" && code == 0 {
		text = "no differences"
	}
	if text == "" {
		text = verb + " finished"
	}
	return text, code
}

func Outcome(verb string, code int) string {
	switch {
	case code == 0:
		return "done"
	case verb == "diff" && code == 1:
		return "differences"
	default:
		return "failed"
	}
}

func Finish(text string, code int, verb string) string {
	text = strings.TrimRight(text, "\n")
	return text + Summary(text) + fmt.Sprintf("\n\n── exit %d · %s ──", code, Outcome(verb, code))
}

func Summary(text string) string {
	counts := map[string]int{}
	for _, line := range strings.Split(text, "\n") {
		if c := Classify(line); IsObject(c) {
			counts[names[c]]++
		}
	}
	if len(counts) == 0 {
		return ""
	}
	order := []string{"created", "configured", "unchanged", "deleted", "pruned", "serverside-applied"}
	var parts []string
	for _, k := range order {
		if n := counts[k]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", k, n))
		}
	}
	return "\n\n" + strings.Join(parts, "   ")
}

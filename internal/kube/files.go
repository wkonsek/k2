package kube

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func KubeFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if !LooksLikeKubeconfig(path) {
			continue
		}
		names = append(names, path)
	}
	sort.Slice(names, func(i, j int) bool {
		bi, bj := filepath.Base(names[i]) == "config", filepath.Base(names[j]) == "config"
		if bi != bj {
			return bi
		}
		return names[i] < names[j]
	})
	if len(names) == 0 {
		return nil, fmt.Errorf("no kubeconfig files in %s", dir)
	}
	return names, nil
}

func LooksLikeKubeconfig(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 8192)
	n, _ := f.Read(buf)
	s := string(buf[:n])
	return strings.Contains(s, "clusters:") || strings.Contains(s, "contexts:") || strings.Contains(s, "apiVersion:")
}

func (c Client) KubeconfigFromEnv() (file, context string, ok bool) {
	raw := strings.TrimSpace(os.Getenv("KUBECONFIG"))
	if raw == "" || strings.Contains(raw, ":") {
		return "", "", false
	}
	info, err := os.Stat(raw)
	if err != nil || info.IsDir() {
		return "", "", false
	}
	return raw, c.CurrentContext(raw), true
}

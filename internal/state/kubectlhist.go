package state

import (
	"os"
	"path/filepath"
	"strings"
)

const KubectlHistoryFile = "kubectl-history"

const KubectlHistoryCap = 100

func KubectlHistoryPath(dir string) string { return filepath.Join(dir, KubectlHistoryFile) }

func LoadKubectlHistory(dir string) []string {
	b, err := os.ReadFile(KubectlHistoryPath(dir))
	if err != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || (len(out) > 0 && out[len(out)-1] == line) {
			continue
		}
		out = append(out, line)
	}
	return capHistory(out)
}

func SaveKubectlHistory(dir string, hist []string) error {
	var b strings.Builder
	for _, line := range capHistory(hist) {
		line = strings.TrimSpace(strings.ReplaceAll(line, "\n", " "))
		if line != "" {
			b.WriteString(line + "\n")
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, KubectlHistoryFile+".*")
	if err != nil {
		return err
	}
	if _, err := tmp.WriteString(b.String()); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), KubectlHistoryPath(dir))
}

func capHistory(hist []string) []string {
	if len(hist) > KubectlHistoryCap {
		return append([]string(nil), hist[len(hist)-KubectlHistoryCap:]...)
	}
	return hist
}

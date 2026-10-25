package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"k2/internal/config"
)

func exportLine(kubeconfig string) string {
	var b strings.Builder
	b.WriteString(`export KUBECONFIG="`)
	for _, r := range kubeconfig {
		switch r {
		case '\\', '"', '$', '`':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteString("\"\n")
	return b.String()
}

func crashLogPath() string { return filepath.Join(config.DefaultDir(), "crash.log") }

func reportCrash(w io.Writer, summary string, stack []byte) {
	path := crashLogPath()
	text := fmt.Sprintf("k2 %s crashed at %s\npanic: %s\n\n%s", versionString(), time.Now().Format(time.RFC3339), summary, stack)
	err := os.MkdirAll(filepath.Dir(path), 0o755)
	if err == nil {
		err = os.WriteFile(path, []byte(text), 0o600)
	}
	if err != nil {

		fmt.Fprintf(w, "k2: crashed: %s (could not write %s: %v)\n", summary, config.HomePath(path), err)
		return
	}
	fmt.Fprintf(w, "k2: crashed: %s; stack trace in %s\n", summary, config.HomePath(path))
}

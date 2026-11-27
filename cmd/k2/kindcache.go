package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"k2/internal/config"
	"k2/internal/kube"
	"k2/internal/shell"
)

const kindCacheUsage = `Usage: k2 kind-cache clean cache|ca|all

Clears the registry proxy directory (kind.registry_proxy.cache_dir, default ~/.k2/kind-cache):
  cache   cached layers and manifests in <cache_dir>/cache; keeps the CA, so wired clusters keep trusting the proxy
  ca      the proxy CA cert and key in <cache_dir>/ca; the next proxy start makes a new one
  all     both

ca and all refuse while the proxy container is running: stop it first (s or ctrl+d on its card in :kind).
`

type kindCacheEnv struct {
	dir       string
	container string
	image     string
	running   func() bool
	docker    func(args ...string) (string, error)
}

func isKindCache(args []string) bool {
	return len(args) > 0 && args[0] == "kind-cache"
}

func runKindCache(args []string) int {
	out, code := kindCacheAction(args, defaultKindCacheEnv())
	if code == 0 {
		fmt.Fprint(os.Stdout, out)
	} else {
		fmt.Fprint(os.Stderr, out)
	}
	return code
}

func defaultKindCacheEnv() kindCacheEnv {
	cfg, _ := config.Load()
	r := cfg.Kind.RegistryProxy
	def := config.DefaultRegistryProxy()
	p := kube.RegistryProxy{Image: r.Image, Container: r.Container, CacheDir: r.CacheDir}
	if p.Image == "" {
		p.Image = def.Image
	}
	if p.Container == "" {
		p.Container = def.Container
	}
	if p.CacheDir == "" {
		p.CacheDir = def.CacheDir
	}
	env := kindCacheEnv{dir: p.CacheDir, container: p.Container, image: p.Image, running: func() bool { return false }}
	if _, err := exec.LookPath("docker"); err != nil {
		return env
	}
	env.running = func() bool {
		return kube.InspectRegistryProxy(shell.Exec{}, "docker", p).Running()
	}
	env.docker = func(args ...string) (string, error) {
		out, err := shell.Combined(shell.Exec{}, "docker", args...)
		text := strings.TrimSpace(string(out))
		if err != nil {
			if text == "" {
				return "", err
			}
			return text, errors.New(strings.Split(text, "\n")[0])
		}
		return text, nil
	}
	return env
}

func kindCacheAction(args []string, env kindCacheEnv) (string, int) {
	if len(args) > 0 && args[0] == "kind-cache" {
		args = args[1:]
	}
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		return kindCacheUsage, 0
	}
	if args[0] != "clean" {
		return fmt.Sprintf("k2 kind-cache: unknown command %q\n\n%s", args[0], kindCacheUsage), 2
	}
	if len(args) != 2 {
		return "k2 kind-cache clean: pick one of cache, ca or all\n\n" + kindCacheUsage, 2
	}
	var parts []string
	switch args[1] {
	case "cache":
		parts = []string{"cache"}
	case "ca":
		parts = []string{"ca"}
	case "all":
		parts = []string{"cache", "ca"}
	default:
		return fmt.Sprintf("k2 kind-cache clean: unknown target %q (cache, ca or all)\n\n%s", args[1], kindCacheUsage), 2
	}
	dir := env.dir
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return fmt.Sprintf("nothing to clear: %s does not exist\n", config.HomePath(dir)), 0
	}
	running := env.running != nil && env.running()
	if running && args[1] != "cache" {
		return fmt.Sprintf("k2: %s is running and uses %s; clearing it now would break the cache for wired clusters\n"+
			"stop it first (s or ctrl+d on its card in :kind, or docker stop %s), then run this again\n",
			env.container, config.HomePath(filepath.Join(dir, "ca")), env.container), 1
	}
	var b strings.Builder
	var total int64
	code := 0
	for _, part := range parts {
		path := filepath.Join(dir, part)
		viaExec := part == "cache" && running
		size, ok, denied := kube.DirUsage(path)
		if !ok {
			fmt.Fprintf(&b, "%s: nothing there (%s)\n", part, config.HomePath(path))
			continue
		}
		if denied {
			if n, dok := env.dockerSize(path, viaExec); dok && n > size {
				size = n
			}
		}
		left, err := clearDir(path)
		if err != nil && errors.Is(err, fs.ErrPermission) && env.docker != nil {
			if derr := env.dockerClean(path, viaExec); derr != nil {
				err = fmt.Errorf("%v; docker fallback: %v", err, derr)
			} else {
				err = nil
				left, _ = kube.DirSize(path)
			}
		}
		freed := max(size-left, 0)
		total += freed
		if err != nil {
			fmt.Fprintf(&b, "could not clear all of %s (%s freed, %s left): %v\n", part, kube.FormatBytes(freed), kube.FormatBytes(left), err)
			fmt.Fprintf(&b, "files written by the proxy container are owned by root; try: sudo rm -rf %s/*\n", path)
			code = 1
			continue
		}
		fmt.Fprintf(&b, "cleared %s (%s) · %s\n", part, kube.FormatBytes(freed), config.HomePath(path))
	}
	if len(parts) > 1 {
		fmt.Fprintf(&b, "freed %s\n", kube.FormatBytes(total))
	}
	if running {
		fmt.Fprintf(&b, "note: %s is running; it fills the cache again on the next pulls\n", env.container)
	}
	for _, part := range parts {
		if part == "ca" && code == 0 {
			fmt.Fprintf(&b, "note: the next proxy start makes a new CA; clusters wired to the old one need to be created again to use the cache\n")
		}
	}
	return b.String(), code
}

func clearDir(path string) (int64, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0, err
	}
	var first error
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(path, e.Name())); err != nil && first == nil {
			first = err
		}
	}
	left, _ := kube.DirSize(path)
	return left, first
}

func (e kindCacheEnv) dockerSize(path string, viaExec bool) (int64, bool) {
	if e.docker == nil {
		return 0, false
	}
	args := kube.HostDirDuArgs(e.image, path)
	if viaExec {
		args = kube.RegistryProxyDuArgs(e.container)
	}
	out, err := e.docker(args...)
	if err != nil {
		return 0, false
	}
	return kube.ParseDuK(out)
}

func (e kindCacheEnv) dockerClean(path string, viaExec bool) error {
	args := kube.HostDirCleanArgs(e.image, path)
	if viaExec {
		args = kube.RegistryProxyCleanExecArgs(e.container)
	}
	_, err := e.docker(args...)
	return err
}

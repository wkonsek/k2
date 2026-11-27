package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func kindCacheDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "kind-cache")
	for path, size := range map[string]int{
		"cache/layers/a": 2048,
		"cache/manifest": 1024,
		"ca/ca.crt":      512,
		"ca/ca.key":      256,
	} {
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, make([]byte, size), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func entries(t *testing.T, dir string) int {
	t.Helper()
	e, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("%s: %v", dir, err)
	}
	return len(e)
}

func stopped() bool { return false }

func TestKindCacheCleanCache(t *testing.T) {
	dir := kindCacheDir(t)
	out, code := kindCacheAction([]string{"kind-cache", "clean", "cache"}, kindCacheEnv{dir: dir, container: "k2-registry-proxy", running: stopped})
	if code != 0 || !strings.Contains(out, "cleared cache (3.0 KiB)") || strings.Contains(out, "cleared ca (") {
		t.Fatalf("%d %q", code, out)
	}
	if entries(t, filepath.Join(dir, "cache")) != 0 || entries(t, filepath.Join(dir, "ca")) != 2 {
		t.Fatal("cache must be empty and ca kept")
	}
}

func TestKindCacheCleanCA(t *testing.T) {
	dir := kindCacheDir(t)
	out, code := kindCacheAction([]string{"clean", "ca"}, kindCacheEnv{dir: dir, running: stopped})
	if code != 0 || !strings.Contains(out, "cleared ca (768 B)") || strings.Contains(out, "cleared cache") || !strings.Contains(out, "new CA") {
		t.Fatalf("%d %q", code, out)
	}
	if entries(t, filepath.Join(dir, "ca")) != 0 || entries(t, filepath.Join(dir, "cache")) != 2 {
		t.Fatal("ca must be empty and cache kept")
	}
}

func TestKindCacheCleanAll(t *testing.T) {
	dir := kindCacheDir(t)
	out, code := kindCacheAction([]string{"clean", "all"}, kindCacheEnv{dir: dir, running: stopped})
	if code != 0 || !strings.Contains(out, "cleared cache (3.0 KiB)") || !strings.Contains(out, "cleared ca (768 B)") || !strings.Contains(out, "freed 3.8 KiB") {
		t.Fatalf("%d %q", code, out)
	}
	if entries(t, filepath.Join(dir, "ca")) != 0 || entries(t, filepath.Join(dir, "cache")) != 0 {
		t.Fatal("both must be empty")
	}
	if st, err := os.Stat(filepath.Join(dir, "cache")); err != nil || !st.IsDir() {
		t.Fatal("cache dir itself must stay for the proxy mount")
	}
}

func TestKindCacheMissingDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nope")
	out, code := kindCacheAction([]string{"clean", "all"}, kindCacheEnv{dir: dir, running: stopped})
	if code != 0 || !strings.Contains(out, "nothing to clear") {
		t.Fatalf("%d %q", code, out)
	}
	dir = t.TempDir()
	out, code = kindCacheAction([]string{"clean", "cache"}, kindCacheEnv{dir: dir, running: stopped})
	if code != 0 || !strings.Contains(out, "cache: nothing there") {
		t.Fatalf("%d %q", code, out)
	}
}

func TestKindCacheUnknownArgs(t *testing.T) {
	dir := kindCacheDir(t)
	env := kindCacheEnv{dir: dir, running: stopped}
	for _, args := range [][]string{{"clean", "layers"}, {"clean"}, {"wipe", "all"}, {"clean", "cache", "ca"}} {
		out, code := kindCacheAction(args, env)
		if code != 2 || !strings.Contains(out, "Usage: k2 kind-cache clean cache|ca|all") {
			t.Fatalf("%v: %d %q", args, code, out)
		}
	}
	if entries(t, filepath.Join(dir, "cache")) != 2 || entries(t, filepath.Join(dir, "ca")) != 2 {
		t.Fatal("bad args must not delete anything")
	}
	if out, code := kindCacheAction([]string{"--help"}, env); code != 0 || !strings.Contains(out, "keeps the CA") {
		t.Fatalf("help %d %q", code, out)
	}
}

func TestKindCacheRunningProxy(t *testing.T) {
	dir := kindCacheDir(t)
	env := kindCacheEnv{dir: dir, container: "k2-registry-proxy", running: func() bool { return true }}
	for _, target := range []string{"ca", "all"} {
		out, code := kindCacheAction([]string{"clean", target}, env)
		if code != 1 || !strings.Contains(out, "k2-registry-proxy is running") || !strings.Contains(out, "stop it first") {
			t.Fatalf("%s: %d %q", target, code, out)
		}
	}
	if entries(t, filepath.Join(dir, "ca")) != 2 || entries(t, filepath.Join(dir, "cache")) != 2 {
		t.Fatal("refused clean must not delete anything")
	}
	out, code := kindCacheAction([]string{"clean", "cache"}, env)
	if code != 0 || !strings.Contains(out, "cleared cache") || !strings.Contains(out, "note: k2-registry-proxy is running") {
		t.Fatalf("cache while running: %d %q", code, out)
	}
	if entries(t, filepath.Join(dir, "ca")) != 2 {
		t.Fatal("ca must stay")
	}
}

func TestHelpListsKindCache(t *testing.T) {
	out, _ := cliAction([]string{"--help"}, "v")
	if !strings.Contains(out, "k2 kind-cache clean cache|ca|all") {
		t.Fatal("help lacks kind-cache")
	}
	if !isKindCache([]string{"kind-cache", "clean", "all"}) || isKindCache([]string{"--help"}) {
		t.Fatal("isKindCache")
	}
}

func lockedDir(t *testing.T, dir, part string) string {
	t.Helper()
	locked := filepath.Join(dir, part, "root-owned")
	if err := os.MkdirAll(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(locked, "blob"), make([]byte, 4096), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	return locked
}

func fakeDocker(t *testing.T, dir string, calls *[]string) func(args ...string) (string, error) {
	t.Helper()
	return func(args ...string) (string, error) {
		line := strings.Join(args, " ")
		*calls = append(*calls, line)
		host := ""
		switch {
		case strings.HasPrefix(line, "exec k2-registry-proxy sh -c rm -rf /docker_mirror_cache/"):
			host = filepath.Join(dir, "cache")
		case args[0] == "run" && strings.Contains(line, "--entrypoint sh"):
			host = strings.TrimSuffix(args[3], ":/target")
		default:
			return "", nil
		}
		e, _ := os.ReadDir(host)
		for _, x := range e {
			p := filepath.Join(host, x.Name())
			_ = filepath.WalkDir(p, func(q string, d os.DirEntry, err error) error {
				if d != nil && d.IsDir() {
					_ = os.Chmod(q, 0o755)
				}
				return nil
			})
			if err := os.RemoveAll(p); err != nil {
				return "", err
			}
		}
		return "", nil
	}
}

func TestKindCachePermissionDeniedFallsBackToDocker(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file modes")
	}
	dir := kindCacheDir(t)
	lockedDir(t, dir, "cache")
	var calls []string
	env := kindCacheEnv{dir: dir, container: "k2-registry-proxy", image: "rpardini/docker-registry-proxy:0.8.0", running: func() bool { return true }, docker: fakeDocker(t, dir, &calls)}
	out, code := kindCacheAction([]string{"clean", "cache"}, env)
	if code != 0 || !strings.Contains(out, "cleared cache (7.0 KiB)") {
		t.Fatalf("%d %q", code, out)
	}
	if len(calls) != 1 || !strings.HasPrefix(calls[0], "exec k2-registry-proxy sh -c rm -rf /docker_mirror_cache/") {
		t.Fatalf("running proxy should clean through docker exec: %v", calls)
	}
	if entries(t, filepath.Join(dir, "cache")) != 0 || entries(t, filepath.Join(dir, "ca")) != 2 {
		t.Fatal("cache must be empty and ca kept")
	}

	dir = kindCacheDir(t)
	lockedDir(t, dir, "ca")
	lockedDir(t, dir, "cache")
	calls = nil
	env = kindCacheEnv{dir: dir, container: "k2-registry-proxy", image: "rpardini/docker-registry-proxy:0.8.0", running: stopped, docker: fakeDocker(t, dir, &calls)}
	out, code = kindCacheAction([]string{"clean", "all"}, env)
	if code != 0 || !strings.Contains(out, "cleared cache (7.0 KiB)") || !strings.Contains(out, "cleared ca (4.8 KiB)") {
		t.Fatalf("%d %q", code, out)
	}
	want := []string{
		"run --rm -v " + filepath.Join(dir, "cache") + ":/target --entrypoint sh rpardini/docker-registry-proxy:0.8.0 -c rm -rf /target/..?* /target/.[!.]* /target/*",
		"run --rm -v " + filepath.Join(dir, "ca") + ":/target --entrypoint sh rpardini/docker-registry-proxy:0.8.0 -c rm -rf /target/..?* /target/.[!.]* /target/*",
	}
	if strings.Join(calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("stopped proxy should clean through docker run:\n%s", strings.Join(calls, "\n"))
	}
	if entries(t, filepath.Join(dir, "cache")) != 0 || entries(t, filepath.Join(dir, "ca")) != 0 {
		t.Fatal("both must be empty")
	}
}

func TestKindCachePermissionDeniedWithoutDocker(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file modes")
	}
	dir := kindCacheDir(t)
	lockedDir(t, dir, "cache")
	out, code := kindCacheAction([]string{"clean", "cache"}, kindCacheEnv{dir: dir, running: stopped})
	if code != 1 || !strings.Contains(out, "could not clear all of cache") || !strings.Contains(out, "sudo rm -rf") {
		t.Fatalf("%d %q", code, out)
	}
}

func TestKindCacheUnreadableSizeUsesDu(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file modes")
	}
	dir := kindCacheDir(t)
	locked := lockedDir(t, dir, "cache")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	var calls []string
	inner := fakeDocker(t, dir, &calls)
	docker := func(args ...string) (string, error) {
		if strings.Join(args, " ") == "exec k2-registry-proxy du -sk /docker_mirror_cache" {
			calls = append(calls, "du")
			return "2048\t/docker_mirror_cache", nil
		}
		return inner(args...)
	}
	env := kindCacheEnv{dir: dir, container: "k2-registry-proxy", image: "img", running: func() bool { return true }, docker: docker}
	out, code := kindCacheAction([]string{"clean", "cache"}, env)
	if code != 0 || !strings.Contains(out, "cleared cache (2.0 MiB)") || len(calls) == 0 || calls[0] != "du" {
		t.Fatalf("%d %q %v", code, out, calls)
	}
}

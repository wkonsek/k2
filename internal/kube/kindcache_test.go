package kube

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"k2/internal/shell"
)

func testProxy(dir string) RegistryProxy {
	return RegistryProxy{
		Image:      "rpardini/docker-registry-proxy:0.8.0",
		Container:  "k2-registry-proxy",
		CacheDir:   dir,
		Registries: []string{"docker.io", "registry.k8s.io", "gcr.io", "quay.io", "ghcr.io"},
	}
}

func TestRegistryProxyEnvDropsDockerHub(t *testing.T) {
	got := RegistryProxyEnv([]string{"docker.io", " registry.k8s.io ", "gcr.io", "gcr.io", "", "registry-1.docker.io", "quay.io"})
	if got != "registry.k8s.io gcr.io quay.io" {
		t.Fatalf("env %q", got)
	}
}

func TestRegistryProxyRunArgs(t *testing.T) {
	p := testProxy("/data/kc")
	want := []string{"run", "-d", "--name", "k2-registry-proxy", "--restart", "unless-stopped", "--network", "kind",
		"-e", "ENABLE_MANIFEST_CACHE=true", "-e", "REGISTRIES=registry.k8s.io gcr.io quay.io ghcr.io",
		"-e", "SEND_TIMEOUT=1200s", "-e", "CLIENT_BODY_TIMEOUT=1200s", "-e", "CLIENT_HEADER_TIMEOUT=1200s",
		"-e", "KEEPALIVE_TIMEOUT=1200s", "-e", "PROXY_READ_TIMEOUT=1200s", "-e", "PROXY_CONNECT_TIMEOUT=1200s",
		"-e", "PROXY_SEND_TIMEOUT=1200s", "-e", "PROXY_CONNECT_READ_TIMEOUT=1200s",
		"-e", "PROXY_CONNECT_CONNECT_TIMEOUT=1200s", "-e", "PROXY_CONNECT_SEND_TIMEOUT=1200s",
		"-e", "PROXY_REQUEST_BUFFERING=false",
		"-v", "/data/kc/cache:/docker_mirror_cache", "-v", "/data/kc/ca:/ca", "rpardini/docker-registry-proxy:0.8.0"}
	if got := RegistryProxyRunArgs(p); !reflect.DeepEqual(got, want) {
		t.Fatalf("args\n%v\n%v", got, want)
	}
	if p.URL() != "http://k2-registry-proxy:3128/" {
		t.Fatal(p.URL())
	}
	if got := RegistryProxyLogsArgs("x"); strings.Join(got, " ") != "logs --tail 200 -f x" {
		t.Fatal(got)
	}
	if got := RegistryProxyRemoveArgs("x"); strings.Join(got, " ") != "rm -f x" {
		t.Fatal(got)
	}
}

func TestRegistryProxyNoProxy(t *testing.T) {
	got := RegistryProxyNoProxy([]string{"172.18.0.0/16", "fc00:f853:ccd:e793::/64", "172.18.0.0/16"}, []string{"dev-control-plane", "dev-worker"})
	want := "localhost,127.0.0.1,.svc,.cluster.local,10.96.0.0/12,10.244.0.0/16,172.18.0.0/16,fc00:f853:ccd:e793::/64,dev-control-plane,dev-worker"
	if got != want {
		t.Fatalf("no_proxy\n%s\n%s", got, want)
	}
}

func TestRegistryProxyDropInAndNodeArgs(t *testing.T) {
	text := RegistryProxyDropInText("k2-registry-proxy", "localhost,.svc")
	want := "[Service]\nEnvironment=\"HTTP_PROXY=http://k2-registry-proxy:3128/\"\nEnvironment=\"HTTPS_PROXY=http://k2-registry-proxy:3128/\"\nEnvironment=\"NO_PROXY=localhost,.svc\"\n"
	if text != want {
		t.Fatalf("drop-in\n%s", text)
	}
	ca := strings.Join(NodeCAArgs("n1"), " ")
	if !strings.HasPrefix(ca, "exec -i n1 sh -c ") || !strings.Contains(ca, "cat > /usr/local/share/ca-certificates/docker_registry_proxy.crt") || !strings.Contains(ca, "update-ca-certificates") {
		t.Fatal(ca)
	}
	d := strings.Join(NodeDropInArgs("n1"), " ")
	for _, s := range []string{"mkdir -p /etc/systemd/system/containerd.service.d", "cat > /etc/systemd/system/containerd.service.d/http-proxy.conf", "systemctl daemon-reload", "systemctl restart containerd"} {
		if !strings.Contains(d, s) {
			t.Fatalf("%s lacks %s", d, s)
		}
	}
}

func TestParseRegistryProxyInspect(t *testing.T) {
	out := `[{"State":{"Status":"running","StartedAt":"2026-11-27T10:00:00.123456789Z"},"Config":{"Image":"rpardini/docker-registry-proxy:0.8.0","Env":["PATH=/usr/bin","REGISTRIES=registry.k8s.io quay.io"]},"NetworkSettings":{"Networks":{"kind":{},"bridge":{}}}}]`
	s, err := ParseRegistryProxyInspect(out)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Running() || !s.Exists() || s.StartedAt.IsZero() || s.Image == "" {
		t.Fatalf("%+v", s)
	}
	if !reflect.DeepEqual(s.Registries, []string{"registry.k8s.io", "quay.io"}) || !reflect.DeepEqual(s.Networks, []string{"bridge", "kind"}) || !s.OnNetwork("kind") {
		t.Fatalf("%+v", s)
	}
	s, _ = ParseRegistryProxyInspect(`[{"State":{"Status":"exited","StartedAt":"0001-01-01T00:00:00Z"}}]`)
	if s.Running() || !s.Exists() || !s.StartedAt.IsZero() {
		t.Fatalf("%+v", s)
	}
	if s, _ := ParseRegistryProxyInspect(""); s.Exists() {
		t.Fatalf("%+v", s)
	}
	if _, err := ParseRegistryProxyInspect("nope"); err == nil {
		t.Fatal("want error")
	}
}

func TestInspectRegistryProxyAbsentAndCacheSize(t *testing.T) {
	dir := t.TempDir()
	p := testProxy(dir)
	if err := os.MkdirAll(filepath.Join(p.CachePath(), "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.CachePath(), "a", "blob"), make([]byte, 1500), 0o644); err != nil {
		t.Fatal(err)
	}
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		return shell.Fail(1, "Error: No such object: k2-registry-proxy")
	}}
	s := InspectRegistryProxy(f, "docker", p)
	if s.Exists() || s.Err != nil || !s.CacheOK || s.CacheBytes != 1500 {
		t.Fatalf("%+v", s)
	}
	f.Handler = func(r shell.Request) shell.Result { return shell.Fail(1, "Cannot connect to the Docker daemon") }
	if s := InspectRegistryProxy(f, "docker", testProxy(filepath.Join(dir, "none"))); s.Exists() || s.Err == nil || s.CacheOK {
		t.Fatalf("%+v", s)
	}
}

type proxyDocker struct {
	state   string
	network bool
	calls   []string
	stdin   map[string]string
}

func (d *proxyDocker) handle(r shell.Request) shell.Result {
	line := r.Name + " " + strings.Join(r.Args, " ")
	d.calls = append(d.calls, line)
	if d.stdin == nil {
		d.stdin = map[string]string{}
	}
	if r.Stdin != "" {
		d.stdin[strings.Join(r.Args, " ")] = r.Stdin
	}
	args := r.Args
	switch {
	case r.Name == "kind" && strings.HasPrefix(strings.Join(args, " "), "get nodes"):
		return shell.OK("dev-control-plane\ndev-worker\n")
	case args[0] == "network" && args[1] == "inspect" && len(args) == 3:
		if !d.network {
			return shell.Fail(1, "Error: No such network: kind")
		}
		return shell.OK("[]")
	case args[0] == "network" && args[1] == "inspect":
		return shell.OK("172.18.0.0/16 fc00:f853:ccd:e793::/64 ")
	case args[0] == "network" && args[1] == "create":
		d.network = true
		return shell.OK("abc")
	case args[0] == "inspect":
		if d.state == "" {
			return shell.Fail(1, "Error: No such object: k2-registry-proxy")
		}
		return shell.OK(`[{"State":{"Status":"` + d.state + `"},"NetworkSettings":{"Networks":{"kind":{}}}}]`)
	case args[0] == "run":
		d.state = "running"
		return shell.OK("deadbeef")
	case args[0] == "start":
		d.state = "running"
		return shell.OK("k2-registry-proxy")
	case args[0] == "exec" && args[1] == "k2-registry-proxy":
		return shell.OK("-----BEGIN CERTIFICATE-----\nabc\n-----END CERTIFICATE-----\n")
	case args[0] == "exec":
		return shell.OK("")
	}
	return shell.OK("")
}

func countPrefix(calls []string, prefix string) int {
	n := 0
	for _, c := range calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

func TestEnsureRegistryProxyRunsWhenAbsent(t *testing.T) {
	RegistryProxyCAWait = time.Millisecond
	dir := t.TempDir()
	d := &proxyDocker{}
	f := &shell.Fake{Handler: d.handle}
	var lines []string
	ca, err := EnsureRegistryProxy(f, "docker", testProxy(dir), func(l string) { lines = append(lines, l) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ca), "BEGIN CERTIFICATE") {
		t.Fatalf("ca %q", ca)
	}
	for _, sub := range []string{"cache", "ca"} {
		if st, err := os.Stat(filepath.Join(dir, sub)); err != nil || !st.IsDir() {
			t.Fatalf("%s missing", sub)
		}
	}
	if countPrefix(d.calls, "docker network create kind") != 1 || countPrefix(d.calls, "docker run -d --name k2-registry-proxy") != 1 || countPrefix(d.calls, "docker start") != 0 {
		t.Fatalf("calls %v", d.calls)
	}
	if !strings.Contains(strings.Join(lines, "\n"), "$ docker run -d") {
		t.Fatalf("lines %v", lines)
	}
}

func TestEnsureRegistryProxyStartsStopped(t *testing.T) {
	RegistryProxyCAWait = time.Millisecond
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "ca"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ca", "ca.crt"), []byte("-----BEGIN CERTIFICATE-----\nhost\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d := &proxyDocker{state: "exited", network: true}
	f := &shell.Fake{Handler: d.handle}
	ca, err := EnsureRegistryProxy(f, "docker", testProxy(dir), nil)
	if err != nil || !strings.Contains(string(ca), "host") {
		t.Fatalf("%v %q", err, ca)
	}
	if countPrefix(d.calls, "docker start k2-registry-proxy") != 1 || countPrefix(d.calls, "docker run") != 0 || countPrefix(d.calls, "docker network create") != 0 {
		t.Fatalf("calls %v", d.calls)
	}
}

func TestEnsureRegistryProxyCATimeout(t *testing.T) {
	RegistryProxyCAWait, RegistryProxyCATries = time.Millisecond, 3
	t.Cleanup(func() { RegistryProxyCATries = 40 })
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		if r.Args[0] == "exec" {
			return shell.Fail(1, "no file")
		}
		if r.Args[0] == "inspect" {
			return shell.OK(`[{"State":{"Status":"running"},"NetworkSettings":{"Networks":{"kind":{}}}}]`)
		}
		return shell.OK("")
	}}
	if _, err := EnsureRegistryProxy(f, "docker", testProxy(t.TempDir()), nil); err == nil || !strings.Contains(err.Error(), "CA certificate") {
		t.Fatalf("err %v", err)
	}
	if _, err := EnsureRegistryProxy(f, "docker", RegistryProxy{}, nil); err == nil {
		t.Fatal("empty config should fail")
	}
}

func TestWireKindClusterProxy(t *testing.T) {
	RegistryProxyCAWait = time.Millisecond
	d := &proxyDocker{network: true}
	f := &shell.Fake{Handler: d.handle}
	var lines []string
	if err := WireKindClusterProxy(f, "docker", "kind", "dev", testProxy(t.TempDir()), func(l string) { lines = append(lines, l) }); err != nil {
		t.Fatal(err)
	}
	for _, node := range []string{"dev-control-plane", "dev-worker"} {
		ca := d.stdin[strings.Join(NodeCAArgs(node), " ")]
		if !strings.Contains(ca, "BEGIN CERTIFICATE") {
			t.Fatalf("%s ca %q", node, ca)
		}
		drop := d.stdin[strings.Join(NodeDropInArgs(node), " ")]
		if !strings.Contains(drop, "HTTPS_PROXY=http://k2-registry-proxy:3128/") || !strings.Contains(drop, "172.18.0.0/16") || !strings.Contains(drop, "dev-worker") || !strings.Contains(drop, "10.96.0.0/12") {
			t.Fatalf("%s drop-in %q", node, drop)
		}
	}
	if countPrefix(d.calls, "docker run -d") != 1 {
		t.Fatalf("calls %v", d.calls)
	}
	if !strings.Contains(lines[len(lines)-1], "dev-control-plane, dev-worker") {
		t.Fatalf("lines %v", lines)
	}
	f.Handler = func(r shell.Request) shell.Result { return shell.OK("") }
	if err := WireKindClusterProxy(f, "docker", "kind", "none", testProxy(t.TempDir()), nil); err == nil {
		t.Fatal("no nodes should fail")
	}
}

func TestParseKindNodeNames(t *testing.T) {
	got := ParseKindNodeNames("a-control-plane\n\nNo kind nodes found for cluster \"x\".\na-worker\n")
	if !reflect.DeepEqual(got, []string{"a-control-plane", "a-worker"}) {
		t.Fatal(got)
	}
}

func TestInspectRegistryProxyDuFallback(t *testing.T) {
	dir := t.TempDir()
	p := testProxy(dir)
	locked := filepath.Join(p.CachePath(), "locked")
	if err := os.MkdirAll(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(locked, "blob"), make([]byte, 4096), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	if os.Geteuid() != 0 {
		if n, ok, denied := DirUsage(p.CachePath()); !ok || !denied || n != 0 {
			t.Fatalf("usage %d %v %v", n, ok, denied)
		}
	}
	var du []string
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		line := strings.Join(r.Args, " ")
		switch {
		case r.Args[0] == "inspect":
			return shell.OK(`[{"State":{"Status":"running","StartedAt":"2026-11-27T08:00:00Z"},"Config":{"Env":[]},"NetworkSettings":{"Networks":{"kind":{}}}}]`)
		case strings.HasPrefix(line, "exec k2-registry-proxy du -sk /docker_mirror_cache"):
			du = append(du, line)
			return shell.OK("1258291\t/docker_mirror_cache\n")
		}
		return shell.Fail(1, "unexpected "+line)
	}}
	s := InspectRegistryProxy(f, "docker", p)
	if os.Geteuid() != 0 && len(du) != 1 {
		t.Fatalf("du calls %v", du)
	}
	if len(du) == 1 && (!s.CacheOK || s.CacheBytes != 1258291*1024 || FormatBytes(s.CacheBytes) != "1.2 GiB") {
		t.Fatalf("%+v %s", s, FormatBytes(s.CacheBytes))
	}
	if n, ok := ParseDuK("12\t/x\n"); !ok || n != 12*1024 {
		t.Fatalf("parse %d %v", n, ok)
	}
	if _, ok := ParseDuK("du: nope"); ok {
		t.Fatal("bad du output must not parse")
	}
}

func TestRegistryProxyCleanArgs(t *testing.T) {
	if got := strings.Join(RegistryProxyCleanExecArgs("k2-registry-proxy"), " "); got != "exec k2-registry-proxy sh -c rm -rf /docker_mirror_cache/..?* /docker_mirror_cache/.[!.]* /docker_mirror_cache/*" {
		t.Fatal(got)
	}
	if got := strings.Join(HostDirCleanArgs("img:1", "/h/ca"), " "); got != "run --rm -v /h/ca:/target --entrypoint sh img:1 -c rm -rf /target/..?* /target/.[!.]* /target/*" {
		t.Fatal(got)
	}
	if got := strings.Join(HostDirDuArgs("img:1", "/h/ca"), " "); got != "run --rm -v /h/ca:/target --entrypoint du img:1 -sk /target" {
		t.Fatal(got)
	}
}

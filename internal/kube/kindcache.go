package kube

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"k2/internal/shell"
)

const (
	RegistryProxyPort   = "3128"
	RegistryProxyNodeCA = "/usr/local/share/ca-certificates/docker_registry_proxy.crt"
	RegistryProxyDropIn = "/etc/systemd/system/containerd.service.d/http-proxy.conf"
	KindNetwork         = "kind"
)

var RegistryProxyCAWait = 500 * time.Millisecond
var RegistryProxyCATries = 40

type RegistryProxy struct {
	Image      string
	Container  string
	CacheDir   string
	Registries []string
}

type ProxyStatus struct {
	State      string
	StartedAt  time.Time
	Image      string
	Registries []string
	Networks   []string
	CacheBytes int64
	CacheOK    bool
	Err        error
}

func (s ProxyStatus) Exists() bool { return s.State != "" && s.State != "absent" }

func (s ProxyStatus) Running() bool { return s.State == "running" }

func (s ProxyStatus) OnNetwork(name string) bool {
	for _, n := range s.Networks {
		if n == name {
			return true
		}
	}
	return false
}

func (p RegistryProxy) CachePath() string { return filepath.Join(p.CacheDir, "cache") }

func (p RegistryProxy) CAPath() string { return filepath.Join(p.CacheDir, "ca") }

func (p RegistryProxy) URL() string { return "http://" + p.Container + ":" + RegistryProxyPort + "/" }

func dockerHubHost(r string) bool {
	switch strings.ToLower(r) {
	case "docker.io", "index.docker.io", "registry-1.docker.io":
		return true
	}
	return false
}

func RegistryProxyEnv(registries []string) string {
	var out []string
	seen := map[string]bool{}
	for _, r := range registries {
		r = strings.TrimSpace(r)
		if r == "" || dockerHubHost(r) || seen[r] {
			continue
		}
		seen[r] = true
		out = append(out, r)
	}
	return strings.Join(out, " ")
}

func RegistryProxyRunArgs(p RegistryProxy) []string {
	return []string{"run", "-d",
		"--name", p.Container,
		"--restart", "unless-stopped",
		"--network", KindNetwork,
		"-e", "ENABLE_MANIFEST_CACHE=true",
		"-e", "REGISTRIES=" + RegistryProxyEnv(p.Registries),
		"-e", "SEND_TIMEOUT=1200s",
		"-e", "CLIENT_BODY_TIMEOUT=1200s",
		"-e", "CLIENT_HEADER_TIMEOUT=1200s",
		"-e", "KEEPALIVE_TIMEOUT=1200s",
		"-e", "PROXY_READ_TIMEOUT=1200s",
		"-e", "PROXY_CONNECT_TIMEOUT=1200s",
		"-e", "PROXY_SEND_TIMEOUT=1200s",
		"-e", "PROXY_CONNECT_READ_TIMEOUT=1200s",
		"-e", "PROXY_CONNECT_CONNECT_TIMEOUT=1200s",
		"-e", "PROXY_CONNECT_SEND_TIMEOUT=1200s",
		"-e", "PROXY_REQUEST_BUFFERING=false",
		"-v", p.CachePath() + ":/docker_mirror_cache",
		"-v", p.CAPath() + ":/ca",
		p.Image,
	}
}

func RegistryProxyInspectArgs(container string) []string {
	return []string{"inspect", "--type", "container", container}
}

func RegistryProxyStartArgs(container string) []string { return []string{"start", container} }

func RegistryProxyStopArgs(container string) []string { return []string{"stop", container} }

func RegistryProxyRemoveArgs(container string) []string { return []string{"rm", "-f", container} }

func RegistryProxyLogsArgs(container string) []string {
	return []string{"logs", "--tail", "200", "-f", container}
}

func RegistryProxyConnectArgs(container string) []string {
	return []string{"network", "connect", KindNetwork, container}
}

func RegistryProxyCAExecArgs(container string) []string {
	return []string{"exec", container, "cat", "/ca/ca.crt"}
}

func KindNodesArgs(cluster string) []string { return []string{"get", "nodes", "--name", cluster} }

func ParseKindNodeNames(out string) []string {
	var names []string
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.Contains(l, " ") {
			continue
		}
		names = append(names, l)
	}
	return names
}

func NotFound(out string) bool {
	l := strings.ToLower(out)
	return strings.Contains(l, "no such object") || strings.Contains(l, "no such container") || strings.Contains(l, "not found")
}

type inspectDoc struct {
	State struct {
		Status    string
		StartedAt string
	}
	Config struct {
		Image string
		Env   []string
	}
	NetworkSettings struct {
		Networks map[string]json.RawMessage
	}
}

func ParseRegistryProxyInspect(out string) (ProxyStatus, error) {
	if strings.TrimSpace(out) == "" {
		return ProxyStatus{State: "absent"}, nil
	}
	var docs []inspectDoc
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &docs); err != nil {
		return ProxyStatus{}, err
	}
	if len(docs) == 0 {
		return ProxyStatus{State: "absent"}, nil
	}
	d := docs[0]
	s := ProxyStatus{State: strings.ToLower(strings.TrimSpace(d.State.Status)), Image: d.Config.Image}
	if s.State == "" {
		s.State = "unknown"
	}
	if t, err := time.Parse(time.RFC3339Nano, d.State.StartedAt); err == nil && t.Year() > 1 {
		s.StartedAt = t
	}
	for _, e := range d.Config.Env {
		if v, ok := strings.CutPrefix(e, "REGISTRIES="); ok {
			s.Registries = strings.Fields(v)
		}
	}
	for n := range d.NetworkSettings.Networks {
		s.Networks = append(s.Networks, n)
	}
	sort.Strings(s.Networks)
	return s, nil
}

func DirSize(path string) (int64, bool) {
	n, ok, _ := DirUsage(path)
	return n, ok
}

func DirUsage(path string) (total int64, ok bool, denied bool) {
	st, err := os.Stat(path)
	if err != nil || !st.IsDir() {
		return 0, false, errors.Is(err, fs.ErrPermission)
	}
	_ = filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrPermission) {
				denied = true
			}
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() {
			if info, ierr := d.Info(); ierr == nil {
				total += info.Size()
			} else if errors.Is(ierr, fs.ErrPermission) {
				denied = true
			}
		}
		return nil
	})
	return total, true, denied
}

const (
	RegistryProxyCacheMount = "/docker_mirror_cache"
	RegistryProxyCAMount    = "/ca"
	cleanMount              = "/target"
)

func RegistryProxyDuArgs(container string) []string {
	return []string{"exec", container, "du", "-sk", RegistryProxyCacheMount}
}

func ParseDuK(out string) (int64, bool) {
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return 0, false
	}
	var k int64
	if _, err := fmt.Sscan(fields[0], &k); err != nil || k < 0 {
		return 0, false
	}
	return k * 1024, true
}

func emptyDirScript(dir string) string {
	return "rm -rf " + dir + "/..?* " + dir + "/.[!.]* " + dir + "/*"
}

func RegistryProxyCleanExecArgs(container string) []string {
	return []string{"exec", container, "sh", "-c", emptyDirScript(RegistryProxyCacheMount)}
}

func HostDirDuArgs(image, path string) []string {
	return []string{"run", "--rm", "-v", path + ":" + cleanMount, "--entrypoint", "du", image, "-sk", cleanMount}
}

func HostDirCleanArgs(image, path string) []string {
	return []string{"run", "--rm", "-v", path + ":" + cleanMount, "--entrypoint", "sh", image, "-c", emptyDirScript(cleanMount)}
}

func ProxyCacheSize(run shell.Runner, docker, container string) (int64, bool) {
	out, err := shell.Combined(run, docker, RegistryProxyDuArgs(container)...)
	if err != nil {
		return 0, false
	}
	return ParseDuK(string(out))
}

func InspectRegistryProxy(run shell.Runner, docker string, p RegistryProxy) ProxyStatus {
	res := run.Run(shell.Request{Name: docker, Args: RegistryProxyInspectArgs(p.Container), Combined: true})
	out := string(res.Stdout) + string(res.Stderr)
	var s ProxyStatus
	if res.Err != nil {
		s = ProxyStatus{State: "absent"}
		if !NotFound(out) {
			s.Err = errors.New(strings.TrimSpace(firstLine(out, res.Err.Error())))
		}
	} else if parsed, err := ParseRegistryProxyInspect(string(res.Stdout)); err != nil {
		s = ProxyStatus{State: "absent", Err: err}
	} else {
		s = parsed
	}
	size, ok, denied := DirUsage(p.CachePath())
	s.CacheBytes, s.CacheOK = size, ok
	if s.Running() && (denied || !ok || size == 0) {
		if n, dok := ProxyCacheSize(run, docker, p.Container); dok {
			s.CacheBytes, s.CacheOK = n, true
		}
	}
	return s
}

func firstLine(s, alt string) string {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return alt
}

func dockerStep(run shell.Runner, docker string, args []string, stdin string, emit func(string)) (string, error) {
	if emit != nil {
		emit("$ docker " + strings.Join(args, " "))
	}
	res := run.Run(shell.Request{Name: docker, Args: args, Stdin: stdin, Combined: true})
	out := strings.TrimSpace(string(res.Stdout) + "\n" + string(res.Stderr))
	if emit != nil && out != "" {
		for _, l := range strings.Split(out, "\n") {
			if strings.TrimSpace(l) != "" {
				emit(l)
			}
		}
	}
	if res.Err != nil {
		return out, fmt.Errorf("docker %s: %s", args[0], firstLine(out, res.Err.Error()))
	}
	return out, nil
}

func EnsureKindNetwork(run shell.Runner, docker string, emit func(string)) error {
	if _, err := dockerStep(run, docker, []string{"network", "inspect", KindNetwork}, "", nil); err == nil {
		return nil
	}
	_, err := dockerStep(run, docker, []string{"network", "create", KindNetwork}, "", emit)
	return err
}

func EnsureRegistryProxy(run shell.Runner, docker string, p RegistryProxy, emit func(string)) ([]byte, error) {
	if p.Container == "" || p.Image == "" || p.CacheDir == "" {
		return nil, errors.New("registry proxy: image, container and cache_dir must be set")
	}
	for _, dir := range []string{p.CachePath(), p.CAPath()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	if err := EnsureKindNetwork(run, docker, emit); err != nil {
		return nil, err
	}
	s := InspectRegistryProxy(run, docker, p)
	switch {
	case !s.Exists():
		if _, err := dockerStep(run, docker, RegistryProxyRunArgs(p), "", emit); err != nil {
			return nil, err
		}
	case !s.Running():
		if _, err := dockerStep(run, docker, RegistryProxyStartArgs(p.Container), "", emit); err != nil {
			return nil, err
		}
	}
	if s.Exists() && !s.OnNetwork(KindNetwork) {
		if _, err := dockerStep(run, docker, RegistryProxyConnectArgs(p.Container), "", emit); err != nil {
			return nil, err
		}
	}
	return WaitRegistryProxyCA(run, docker, p, emit)
}

func WaitRegistryProxyCA(run shell.Runner, docker string, p RegistryProxy, emit func(string)) ([]byte, error) {
	host := filepath.Join(p.CAPath(), "ca.crt")
	tries := max(1, RegistryProxyCATries)
	for i := 0; i < tries; i++ {
		if i > 0 && RegistryProxyCAWait > 0 {
			time.Sleep(RegistryProxyCAWait)
		}
		if raw, err := os.ReadFile(host); err == nil && validCA(raw) {
			return raw, nil
		}
		res := run.Run(shell.Request{Name: docker, Args: RegistryProxyCAExecArgs(p.Container)})
		if res.Err == nil && validCA(res.Stdout) {
			return res.Stdout, nil
		}
	}
	if emit != nil {
		emit("registry proxy: no CA certificate after waiting")
	}
	return nil, errors.New("registry proxy: CA certificate not ready (" + host + ")")
}

func validCA(raw []byte) bool {
	return strings.Contains(string(raw), "BEGIN CERTIFICATE")
}

func RegistryProxyNoProxy(subnets, nodes []string) string {
	parts := []string{"localhost", "127.0.0.1", ".svc", ".cluster.local", "10.96.0.0/12", "10.244.0.0/16"}
	seen := map[string]bool{}
	for _, p := range parts {
		seen[p] = true
	}
	for _, list := range [][]string{subnets, nodes} {
		for _, s := range list {
			s = strings.TrimSpace(s)
			if s == "" || seen[s] {
				continue
			}
			seen[s] = true
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, ",")
}

func RegistryProxyDropInText(container, noProxy string) string {
	url := RegistryProxy{Container: container}.URL()
	return "[Service]\n" +
		"Environment=\"HTTP_PROXY=" + url + "\"\n" +
		"Environment=\"HTTPS_PROXY=" + url + "\"\n" +
		"Environment=\"NO_PROXY=" + noProxy + "\"\n"
}

func NodeCAArgs(node string) []string {
	return []string{"exec", "-i", node, "sh", "-c", "cat > " + RegistryProxyNodeCA + " && update-ca-certificates"}
}

func NodeDropInArgs(node string) []string {
	return []string{"exec", "-i", node, "sh", "-c",
		"mkdir -p " + filepath.Dir(RegistryProxyDropIn) + " && cat > " + RegistryProxyDropIn + " && systemctl daemon-reload && systemctl restart containerd"}
}

func WireKindClusterProxy(run shell.Runner, docker, kind, cluster string, p RegistryProxy, emit func(string)) error {
	if emit != nil {
		emit("$ " + kind + " " + strings.Join(KindNodesArgs(cluster), " "))
	}
	res := run.Run(shell.Request{Name: kind, Args: KindNodesArgs(cluster), Combined: true})
	if res.Err != nil {
		return fmt.Errorf("kind get nodes: %s", firstLine(string(res.Stdout)+string(res.Stderr), res.Err.Error()))
	}
	nodes := ParseKindNodeNames(string(res.Stdout))
	if len(nodes) == 0 {
		return errors.New("kind get nodes: no nodes for " + cluster)
	}
	ca, err := EnsureRegistryProxy(run, docker, p, emit)
	if err != nil {
		return err
	}
	out, err := dockerStep(run, docker, DockerNetworkInspectArgs(KindNetwork), "", emit)
	if err != nil {
		return err
	}
	dropIn := RegistryProxyDropInText(p.Container, RegistryProxyNoProxy(ParseDockerSubnets(out), nodes))
	for _, node := range nodes {
		if _, err := dockerStep(run, docker, NodeCAArgs(node), string(ca), emit); err != nil {
			return err
		}
		if _, err := dockerStep(run, docker, NodeDropInArgs(node), dropIn, emit); err != nil {
			return err
		}
	}
	if emit != nil {
		emit("registry cache wired on " + strings.Join(nodes, ", ") + " via " + p.Container)
	}
	return nil
}

package forward

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"k2/internal/config"
	"k2/internal/shell"
)

type Forward struct {
	Namespace  string    `json:"namespace"`
	Pod        string    `json:"pod"`
	Kind       string    `json:"kind,omitempty"`
	Local      string    `json:"local"`
	Remote     string    `json:"remote"`
	PID        int       `json:"pid"`
	Started    time.Time `json:"started"`
	Kubeconfig string    `json:"kubeconfig"`
	Context    string    `json:"context"`

	Owner string `json:"owner,omitempty"`

	Auto string `json:"auto,omitempty"`

	Log string `json:"log,omitempty"`

	Via string `json:"via,omitempty"`

	Reason string `json:"reason,omitempty"`

	Note string `json:"note,omitempty"`
}

func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

func Stop(pid int) {
	if pid <= 0 {
		return
	}
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	_ = syscall.Kill(pid, syscall.SIGTERM)
}

func Age(t time.Time) string {
	d := time.Since(t)
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

func Where(f Forward) string {
	file := config.HomePath(f.Kubeconfig)
	switch {
	case file != "" && f.Context != "":
		return file + " · " + f.Context
	case f.Context != "":
		return f.Context
	case file != "":
		return file
	default:
		return "?"
	}
}

func AnyDead(fs []Forward) bool {
	for _, f := range fs {
		if !Up(f) {
			return true
		}
	}
	return false
}

func Resume(r shell.Runner, logDir string, fs []Forward) ([]Forward, string) {
	cp := append([]Forward(nil), fs...)
	n := 0
	var note string
	for i, f := range cp {
		if Up(f) {
			continue
		}

		nf, _, err := Restart(r, logDir, f)
		if err != nil {
			note = shell.OneLine(err.Error())
			continue
		}
		cp[i] = nf
		n++
	}
	if note == "" && n > 0 {
		note = fmt.Sprintf("resumed %d forward", n)
		if n != 1 {
			note += "s"
		}
	}
	return cp, note
}

func KillPort(r shell.Runner, port string) {
	seen := map[int]bool{}
	for _, pid := range PIDsOnPort(r, port) {
		if pid > 0 && !seen[pid] {
			seen[pid] = true
			Stop(pid)
		}
	}
}

var pidArg = regexp.MustCompile(`pid=(\d+)`)

func PIDsOnPort(r shell.Runner, port string) []int {
	var pids []int
	if out, err := shell.Output(r, "ss", "-H", "-ltnp", "sport", "=", ":"+port); err == nil {
		text := string(out)
		if strings.Contains(text, "kubectl") || strings.Contains(text, "port-forward") {
			for _, m := range pidArg.FindAllStringSubmatch(text, -1) {
				if pid, err := strconv.Atoi(m[1]); err == nil {
					pids = append(pids, pid)
				}
			}
		}
	}
	out, err := shell.Output(r, "ps", "-eo", "pid,args")
	if err != nil {
		return pids
	}
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "port-forward") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 || !bindsLocal(fields[1:], port) {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err == nil {
			pids = append(pids, pid)
		}
	}
	return pids
}

func bindsLocal(args []string, port string) bool {
	for _, a := range args {
		if a == port || strings.HasPrefix(a, port+":") {
			return true
		}
	}
	return false
}

const spawnScript = `
trap '' HUP
if command -v setsid >/dev/null 2>&1; then
  setsid nohup "$@" </dev/null >>"$K2_LOG" 2>&1 &
else
  nohup "$@" </dev/null >>"$K2_LOG" 2>&1 &
fi
pid=$!
disown "$pid" 2>/dev/null || true
echo "$pid"
`

func Spawn(r shell.Runner, logDir, kc, ctx, podRef, local, remote string) (Forward, error) {
	parts := strings.SplitN(podRef, "/", 2)
	ns, name := "default", podRef
	if len(parts) == 2 {
		ns, name = parts[0], parts[1]
	}
	return SpawnTarget(r, logDir, kc, ctx, ns, "pod", name, local, remote)
}

func Target(f Forward) string { return KindOr(f.Kind) + "/" + f.Pod }

func KindOr(kind string) string {
	if kind == "" {
		return "pod"
	}
	return kind
}

func SpawnTarget(r shell.Runner, logDir, kc, ctx, ns, kind, name, local, remote string) (Forward, error) {
	kind = KindOr(kind)
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return Forward{}, err
	}
	logPath := filepath.Join(logDir, fmt.Sprintf("%d.log", time.Now().UnixNano()))
	if _, err := os.Create(logPath); err != nil {
		return Forward{}, err
	}
	res := r.Run(shell.Request{
		Name:     "sh",
		Combined: true,
		Args: []string{"-c", spawnScript, "sh", "kubectl", "port-forward",
			"-n", ns, kind + "/" + name, local + ":" + remote,
			"--kubeconfig", kc, "--context", ctx,

			"-v=6"},
		Env: []string{"K2_LOG=" + logPath},
	})
	out, err := res.Stdout, res.Err
	fields := strings.Fields(string(out))
	if err != nil || len(fields) == 0 {
		msg := strings.TrimSpace(string(out))
		if msg == "" && err != nil {
			msg = err.Error()
		}
		if msg == "" {
			msg = "could not start port-forward"
		}
		return Forward{}, fmt.Errorf("%s", shell.OneLine(msg))
	}
	pid, err := strconv.Atoi(fields[len(fields)-1])
	if err != nil {
		return Forward{}, fmt.Errorf("%s", shell.OneLine(string(out)))
	}
	time.Sleep(spawnWait)
	if !Alive(pid) {
		b, _ := os.ReadFile(logPath)
		return Forward{}, fmt.Errorf("%s", startError(string(b)))
	}
	f := Forward{
		Namespace: ns, Pod: name, Kind: kindField(kind), Local: local, Remote: remote,
		PID: pid, Started: time.Now(), Kubeconfig: kc, Context: ctx, Log: logPath,
	}
	if kind != "pod" {
		f.Via = ScanLog(logPath, 0).Via
	}
	return f, nil
}

var spawnWait = 400 * time.Millisecond

func kindField(kind string) string {
	if kind == "pod" {
		return ""
	}
	return kind
}

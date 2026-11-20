package kube

import (
	"context"
	"strings"
	"sync"
	"time"

	"k2/internal/shell"
)

var (
	FollowRetry    = 300 * time.Millisecond
	FollowRetryMax = 5 * time.Second
)

type FollowSpec struct {
	Args    func(since time.Time) []string
	Prefix  bool
	MaxIdle int
}

type Follower struct {
	mu   sync.Mutex
	last map[string]time.Time
	seen map[string]map[string]struct{}
}

func NewFollower() *Follower {
	return &Follower{last: map[string]time.Time{}, seen: map[string]map[string]struct{}{}}
}

func (f *Follower) accept(l LogLine, raw string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := l.Container + "\x00" + l.Pod
	last, ok := f.last[key]
	switch {
	case !ok || l.At.After(last):
		f.last[key] = l.At
		f.seen[key] = map[string]struct{}{raw: {}}
		return true
	case l.At.Equal(last):
		if _, dup := f.seen[key][raw]; dup {
			return false
		}
		f.seen[key][raw] = struct{}{}
		return true
	}
	return false
}

func (f *Follower) Since() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	var t time.Time
	for _, v := range f.last {
		if t.IsZero() || v.Before(t) {
			t = v
		}
	}
	return t.Truncate(time.Second)
}

func ParseStamped(raw string, prefixed bool) (LogLine, bool) {
	var l LogLine
	rest := raw
	if prefixed {
		if !strings.HasPrefix(rest, "[") {
			return l, false
		}
		end := strings.Index(rest, "] ")
		if end < 0 {
			return l, false
		}
		parts := strings.Split(rest[1:end], "/")
		if len(parts) == 3 && parts[0] == "pod" {
			l.Pod, l.Container = parts[1], parts[2]
		}
		rest = rest[end+2:]
	}
	stamp, text, _ := strings.Cut(rest, " ")
	at, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		return l, false
	}
	l.At, l.Text = at, text
	return l, true
}

func (c Client) Follow(ctx context.Context, f *Follower, spec FollowSpec, emit func(LogLine), note func(string)) {
	idle := 0
	for ctx.Err() == nil {
		accepted := 0
		errText := ""
		res := c.Run.Run(shell.Request{
			Name: "kubectl", Args: spec.Args(f.Since()), Ctx: ctx, Stream: true,
			OnLine: func(raw string) {
				l, ok := ParseStamped(raw, spec.Prefix)
				if !ok {
					if strings.TrimSpace(raw) != "" {
						errText = strings.TrimSpace(raw)
					}
					return
				}
				if f.accept(l, raw) {
					accepted++
					emit(l)
				}
			},
		})
		if ctx.Err() != nil {
			return
		}
		if note != nil {
			switch {
			case errText != "":
				note(errText)
			case res.Err != nil:
				note(res.Err.Error())
			default:
				note("")
			}
		}
		delay := FollowRetry
		if accepted > 0 {
			idle = 0
		} else {
			idle++
			if spec.MaxIdle > 0 && idle >= spec.MaxIdle {
				return
			}
			delay = min(FollowRetryMax, FollowRetry<<min(idle, 6))
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}
}

func FollowArgs(name, namespace, kubeconfig, context string, opt LogOpts, tail int, since time.Time) []string {
	return append(LogsArgs(name, namespace, kubeconfig, context, since, tail, opt), "-f", "--timestamps")
}

func FollowMultiArgs(pod, namespace, kubeconfig, context string, q LogQuery, since time.Time, tail int) []string {
	return append(MultiLogsArgs(pod, namespace, kubeconfig, context, q, since, tail), "-f")
}

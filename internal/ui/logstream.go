package ui

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"k2/internal/kube"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	batchWindow  = 100 * time.Millisecond
	firstWakeAge = time.Second
	parkedTTL    = 10 * time.Minute
)

type logStreams struct {
	mu     sync.Mutex
	all    map[string]*logStream
	closed bool
}

func newLogStreams() *logStreams { return &logStreams{all: map[string]*logStream{}} }

type logStream struct {
	id, key string
	k       kube.Client
	single  *singleSpec
	stern   *sternSpec

	mu        sync.Mutex
	running   bool
	ctx       context.Context
	cancel    context.CancelFunc
	parked    time.Time
	buf       []kube.LogLine
	capBuf    int
	lost      int
	note      string
	announced bool
	first     bool
	notify    chan struct{}
	fol       *kube.Follower
	pods      map[string]*podFollow
}

type podFollow struct {
	fol   *kube.Follower
	ended bool
	ran   bool
}

type singleSpec struct {
	name, ns, kc, ctx string
	opt               kube.LogOpts
	tail              int
}

type sternSpec struct {
	ns, kc, ctx string
	q           kube.LogQuery
	tail        int
}

type logWant struct {
	key    string
	single *singleSpec
	stern  *sternSpec
	capBuf int
	first  bool
}

type logWakeMsg struct{ s *logStream }

func (r *logStreams) stopAll() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	for _, s := range r.all {
		s.stop()
	}
}

func (r *logStreams) live(s *logStream) bool {
	if r == nil || s == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.all[s.id] == s && s.isRunning()
}

func (r *logStreams) get(id string) *logStream {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.all[id]
}

func (r *logStreams) reconcile(k kube.Client, want map[string]logWant) []tea.Cmd {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	var cmds []tea.Cmd
	for id, s := range r.all {
		w, ok := want[id]
		switch {
		case ok && w.key == s.key:
			continue
		case ok:
			s.stop()
			delete(r.all, id)
		case s.isRunning():
			s.stop()
		case time.Since(s.parkedAt()) > parkedTTL:
			delete(r.all, id)
		}
	}
	for id, w := range want {
		s := r.all[id]
		if s == nil {
			s = newLogStream(id, k, w)
			r.all[id] = s
		}
		if !s.isRunning() {
			s.start()
			cmds = append(cmds, s.wait())
		}
	}
	return cmds
}

func (r *logStreams) ensure(k kube.Client, id string, w logWant) []tea.Cmd {
	if r == nil {
		return nil
	}
	return r.reconcileOne(k, id, w)
}

func (r *logStreams) reconcileOne(k kube.Client, id string, w logWant) []tea.Cmd {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	s := r.all[id]
	if s != nil && s.key != w.key {
		s.stop()
		delete(r.all, id)
		s = nil
	}
	if s == nil {
		s = newLogStream(id, k, w)
		r.all[id] = s
	}
	if s.isRunning() {
		return nil
	}
	s.start()
	return []tea.Cmd{s.wait()}
}

func newLogStream(id string, k kube.Client, w logWant) *logStream {
	capBuf := w.capBuf
	if capBuf < 2000 {
		capBuf = 2000
	}
	return &logStream{id: id, key: w.key, k: k, single: w.single, stern: w.stern, capBuf: capBuf, first: w.first,
		notify: make(chan struct{}, 1), fol: kube.NewFollower(), pods: map[string]*podFollow{}}
}

func (s *logStream) isRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *logStream) parkedAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.parked
}

func (s *logStream) start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.running = true
	s.announced = false
	for _, p := range s.pods {
		p.ended = true
	}
	if sp := s.single; sp != nil {
		ctx, fol := s.ctx, s.fol
		spec := kube.FollowSpec{Args: func(since time.Time) []string {
			return kube.FollowArgs(sp.name, sp.ns, sp.kc, sp.ctx, sp.opt, sp.tail, since)
		}}
		go s.k.Follow(ctx, fol, spec, s.push, s.setNote)
	}
}

func (s *logStream) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	s.running = false
	s.parked = time.Now()
	s.cancel()
}

func (s *logStream) push(l kube.LogLine) {
	s.mu.Lock()
	s.buf = append(s.buf, l)
	if over := len(s.buf) - s.capBuf; over > 0 {
		s.buf = append([]kube.LogLine(nil), s.buf[over:]...)
		s.lost += over
	}
	s.mu.Unlock()
	s.signal()
}

func (s *logStream) setNote(n string) {
	s.mu.Lock()
	s.note = n
	s.mu.Unlock()
	if n != "" {
		s.signal()
	}
}

func (s *logStream) signal() {
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

func (s *logStream) drain() (lines []kube.LogLine, lost int, note string, first bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lines, lost, note, first = s.buf, s.lost, s.note, s.first
	s.buf, s.lost = nil, 0
	if len(lines) > 0 {
		s.first = false
	}
	return
}

func (s *logStream) wait() tea.Cmd {
	return func() tea.Msg {
		s.mu.Lock()
		ctx, announced := s.ctx, s.announced
		s.mu.Unlock()
		var timeout <-chan time.Time
		if !announced {
			timeout = time.After(firstWakeAge)
		}
		select {
		case <-s.notify:
			select {
			case <-time.After(batchWindow):
			case <-ctx.Done():
			}
		case <-timeout:
		case <-ctx.Done():
		}
		s.mu.Lock()
		s.announced = true
		s.mu.Unlock()
		return logWakeMsg{s: s}
	}
}

func (s *logStream) ensurePod(t mlogTarget) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running || s.stern == nil {
		return
	}
	key := t.ns + "/" + t.name
	p := s.pods[key]
	if p == nil {
		p = &podFollow{fol: kube.NewFollower(), ended: true}
		s.pods[key] = p
	}
	if !p.ended || (p.ran && !t.live) {
		return
	}
	p.ended, p.ran = false, true
	idle := 3
	if !t.live {
		idle = 1
	}
	sp, ctx, fol := s.stern, s.ctx, p.fol
	spec := kube.FollowSpec{Prefix: true, MaxIdle: idle, Args: func(since time.Time) []string {
		return kube.FollowMultiArgs(t.name, or(t.ns, sp.ns), sp.kc, sp.ctx, sp.q, since, sp.tail)
	}}
	go func() {
		s.k.Follow(ctx, fol, spec, func(l kube.LogLine) {
			if sp.ns == "" {
				l.Namespace = t.ns
			}
			s.push(l)
		}, s.setNote)
		s.mu.Lock()
		p.ended = true
		s.mu.Unlock()
	}()
}

func streamDuring(l load) bool {
	return !l.active() || l.what == "loading logs" || l.what == "reading logs"
}

func (m *model) streamWanted() map[string]logWant {
	want := map[string]logWant{}
	add := func(id, stage, pod, ns, kc, ctx string, vs viewState, text string, ed *editor, ld load) {
		if ed.live() || vs.logPager || !streamDuring(ld) {
			return
		}
		switch {
		case stage == "logs" && pod != "" && !vs.logPrev:
			want[id] = logWant{
				key:    logKey(pod, vs.logOpts()),
				single: &singleSpec{name: podName(pod, ns), ns: podNS(pod, ns), kc: kc, ctx: ctx, opt: vs.logOpts(), tail: m.cfg.Logs.Tail},
				capBuf: 4 * m.cfg.Logs.MaxLines,
				first:  text != "",
			}
		case stage == "mlogs" && vs.ml != nil:
			want[id] = logWant{
				key:    mlogKey(ns, vs.ml.q),
				stern:  &sternSpec{ns: ns, kc: kc, ctx: ctx, q: vs.ml.q, tail: sternTail(vs.ml.q, m.cfg.Logs.Tail)},
				capBuf: 4 * m.cfg.Logs.MultiMaxLines,
			}
		}
	}
	add(m.logID, m.stage, m.logPod, m.namespace, m.kubeconfig, m.context, m.vs, m.logText, m.ed, m.load)
	for i := range m.panes {
		if i == m.focus {
			continue
		}
		p := m.panes[i]
		add(p.logID, p.stage, p.logPod, p.namespace, p.kubeconfig, p.context, p.vs, p.logText, p.ed, p.load)
	}
	return want
}

func sternTail(q kube.LogQuery, tail int) int {
	if q.Tail > 0 {
		return q.Tail
	}
	return tail
}

func podName(ref, ns string) string {
	_, name := kube.SplitRef(ref, ns)
	return name
}

func podNS(ref, ns string) string {
	n, _ := kube.SplitRef(ref, ns)
	return n
}

func (m *model) syncLogStreams() []tea.Cmd {
	return m.streams.reconcile(m.k, m.streamWanted())
}

func (m *model) applyStreamWake(msg logWakeMsg) tea.Cmd {
	s := msg.s
	if !m.streams.live(s) {
		return nil
	}
	lines, lost, note, first := s.drain()
	var cmd tea.Cmd
	if s.stern != nil {
		cmd = m.applyMlog(mlogMsg{pane: s.id, key: s.key, lines: lines, stream: true, lost: lost})
	} else {
		texts := make([]string, len(lines))
		for i, l := range lines {
			texts[i] = l.Text
		}
		lm := logsMsg{id: s.id, pod: s.key, text: strings.Join(texts, "\n"), partial: true, stream: true, dedupe: first && len(lines) > 0, lost: lost}
		if note != "" && len(lines) == 0 {
			lm.err = errors.New(note)
		}
		m.applyLogs(lm)
	}
	return tea.Batch(cmd, s.wait())
}

func (m *model) stopLogStreams() { m.streams.stopAll() }

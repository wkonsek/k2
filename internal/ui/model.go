package ui

import (
	"fmt"
	"strconv"
	"time"

	"k2/internal/config"
	"k2/internal/forward"
	"k2/internal/kube"
	"k2/internal/layout"
	"k2/internal/shell"
	"k2/internal/state"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type navItem struct {
	id, icon, title, key string
}

func (n navItem) Title() string { return n.title }

func (n navItem) Description() string { return "" }

func (n navItem) FilterValue() string { return n.title }

type item struct {
	title, desc, id, plain         string
	born                           time.Time
	cells, heads                   []string
	widths                         []int
	natural                        []int
	ageIdx, colorCol, flex, dimCol int
	extra                          map[string]string
	ord                            int
	tones                          map[int]tone
	meters                         map[int]meter
}

type viewState struct {
	podSel    string
	podTitle  string
	sortCol   string
	sortDesc  bool
	parent    string
	parentSel string
	pick      string
	logCont   string
	logPrev   bool
	logMarks  []int
	wrapFlip  bool

	kpos map[string]kmark

	vback    string
	xray     *xrayView
	helmAll  bool
	helm     []kube.HelmRelease
	hist     *helmHist
	search   string
	searchAt int
	nsPrev   string
	ml       *mlogView
	jumpFrom string
	jumpSel  string
	xrayFrom string
	xrayKind string
	xraySel  string

	listFrom string
}

type kmark struct {
	name string
	idx  int
}

func (i item) Title() string { return i.title }

func (i item) Description() string { return i.desc }

func (i item) FilterValue() string {
	if i.plain != "" {
		return i.plain
	}
	return i.title + " " + i.desc
}

type resumedMsg struct {
	forwards []forward.Forward
	note     string
}

type fwdMsg struct {
	f       forward.Forward
	err     error
	retried bool
}

type nsMsg struct {
	ctx, ns string
}

type logsMsg struct {
	id, pod, text string
	err           error
	partial       bool
}

type viewMsg struct {
	id, text string
	err      error
	what     string
}

type rowsMsg struct {
	id, header string
	items      []list.Item
	err        error
}

type vimReadyMsg struct {
	pane string
	path string
	err  error
}

type lessDoneMsg struct{}

type vimDoneMsg struct{}

type resListMsg struct {
	ctx   string
	names []string
}

type yamlMsg struct {
	id, text string
	err      error
}

type kOutMsg struct {
	id     string
	text   string
	code   int
	line   string
	target string
	ctx    string
}

type progressMsg struct {
	id          string
	token       int
	done, total int
	what        string
	ch          <-chan tea.Msg
}

type editDoneMsg struct {
	what string
	err  error

	pane  string
	token int
}

type tickMsg time.Time

type model struct {
	nav         list.Model
	content     list.Model
	portInput   textinput.Model
	stage       string
	kubeconfig  string
	context     string
	namespace   string
	pod         string
	localPort   string
	remotePort  string
	suggestions []kube.PortSuggestion
	sugIndex    int
	forwards    []forward.Forward
	pfAuto      map[string]*pfAuto
	pfWatch     *pfWatch
	resuming    bool
	logPod      string
	logText     string
	logBusy     bool
	logSince    time.Time
	logAt       int
	logStick    bool
	logID       string
	vis         map[string]*visCache
	mlogStyles  map[color]lipgloss.Style
	mlogPaint   map[string]string
	width       int
	height      int
	quitting    bool
	status      string
	kind        string
	subject     string
	dback       string
	vs          viewState
	dlgSeq      int
	lastErr     lastErr
	dlg         *dialog
	pfOn        bool
	pfKind      string
	pfBusy      string
	pfMsg       string
	pfWait      bool
	ed          *editor
	prefixOn    bool
	copy        *copyState
	prefixTok   int
	repeatOn    bool
	repeatTok   int
	kroot       string
	kfile       string
	kback       string
	sh          textinput.Model
	jq          textinput.Model
	shFmt       int
	shHint      string
	shCycle     []string
	shCycleAt   int
	jqOn        bool
	panes       []paneSnap
	focus       int
	tabs        []tab
	tab         int
	paintBg     bool
	cmdOn       bool
	cmd         textinput.Model
	cmdHint     string
	cmdBase     string
	names       map[string]*nameEntry
	namesBusy   map[string]bool
	cmdCycle    []string
	cmdCycleAt  int
	resources   []string
	resContext  string
	restored    bool
	zoom        bool
	beat        int
	rowBusy     map[string]bool
	cfg         config.Config
	load        load
	ran         runInfo
	loadSeq     int
	spin        spinner.Model
	version     string
	cinfo       map[string]clusterInfo
	cinfoKey    string
	tok         tokenState
	themeName   string
	metrics     map[string]podMetrics
	metNoted    map[string]bool
	spinning    bool
	toast       toast
	toastSeq    int
	cfgNote     bool
	notice      string
	favs        []state.Favorite
	run         shell.Runner
	k           kube.Client
}

type Options struct {
	Runner shell.Runner

	Config   *config.Config
	Warnings []string

	Version string

	Notice string
}

func New(opts Options) tea.Model { return initialModel(opts) }

type Session struct {
	Kubeconfig, Context, Namespace string
}

func Finish(final tea.Model) Session {
	var m model
	if g, ok := final.(guarded); ok {

		if g.crash.Crashed() {
			return Session{}
		}
		final = g.inner
	}
	switch v := final.(type) {
	case model:
		m = v
	case *model:
		m = *v
	default:
		return Session{}
	}
	m.save()
	return Session{Kubeconfig: m.kubeconfig, Context: m.context, Namespace: m.namespace}
}

func initialModel(opts Options) model {
	if opts.Runner == nil {
		opts.Runner = shell.Exec{}
	}
	k := kube.New(opts.Runner)
	cfg, warnings := config.Config{}, opts.Warnings
	if opts.Config != nil {
		cfg = *opts.Config
	} else {
		var loaded []string
		cfg, loaded = config.Load()
		warnings = append(warnings, loaded...)
	}

	st := state.Load(cfg.StateDir)
	startKC, startCtx := st.Kubeconfig, st.Context
	if file, envCtx, ok := k.KubeconfigFromEnv(); ok && startKC == "" {
		startKC = file
		if envCtx != "" {
			startCtx = envCtx
		}
	}
	pal := cfg.PaletteNamed(cfg.ThemeFor(startKC, startCtx))
	useTheme(pal)
	if pal.Light && !cfg.UI.PaintBackground(true) {

		warnings = append(warnings, pal.Name+": light theme on a transparent background (ui.background: theme paints it)")
	}
	navItems := []list.Item{
		navItem{id: "kube", icon: "⧉", title: "Kubeconfig", key: cfg.Keys.Kubeconfig.Short()},
		navItem{id: "ctx", icon: "◉", title: "Context", key: cfg.Keys.Context.Short()},
		navItem{id: "proj", icon: "+", title: "Namespace", key: cfg.Keys.Namespace.Short()},
		navItem{id: "kust", icon: "k", title: "Kustomize", key: cfg.Keys.Kustomize.Short()},
		navItem{id: "sh", icon: ">", title: "Kubectl", key: cfg.Keys.Kubectl.Short()},
		navItem{id: "fwds", icon: "=", title: "Forwards", key: cfg.Keys.Forwards.Short()},
	}
	nav := list.New(navItems, list.NewDefaultDelegate(), 20, 8)
	nav.Title = "k2"
	nav.SetShowStatusBar(false)
	nav.SetFilteringEnabled(false)
	nav.SetShowHelp(false)
	nav.KeyMap = listKeyMap(cfg.Keys)

	delegate := contentDelegate(40)
	content := list.New(nil, delegate, 40, 12)
	content.SetShowStatusBar(false)
	content.SetFilteringEnabled(true)
	content.SetShowHelp(false)
	content.Styles.Title = titleStyle
	content.KeyMap = listKeyMap(cfg.Keys)

	ti := textinput.New()
	ti.Placeholder = "8080:80"
	ti.CharLimit = 32
	ti.Width = 24
	ti.PromptStyle = keyStyle
	ti.TextStyle = itemStyle

	cmd := textinput.New()
	cmd.Placeholder = "pods, secrets, svc…"
	cmd.CharLimit = 200
	cmd.Width = 28
	cmd.Prompt = ": "
	cmd.PromptStyle = keyStyle
	cmd.TextStyle = itemStyle

	sh := textinput.New()
	sh.Placeholder = "get pods"
	sh.CharLimit = 240
	sh.Width = 48
	sh.Prompt = "kubectl "
	sh.PromptStyle = keyStyle
	sh.TextStyle = itemStyle
	jq := textinput.New()
	jq.Placeholder = ".items[].metadata.name"
	jq.CharLimit = 200
	jq.Width = 48
	jq.Prompt = "jq "
	jq.PromptStyle = keyStyle
	jq.TextStyle = itemStyle

	kubeconfig, context := st.Kubeconfig, st.Context
	if file, envCtx, ok := k.KubeconfigFromEnv(); ok && kubeconfig == "" {
		kubeconfig = file
		if envCtx != "" {
			context = envCtx
		}
	}
	status := "pick an action"
	if context != "" {
		status = context
	}
	if len(warnings) > 0 {

		status = warnings[0]
		if len(warnings) > 1 {
			status += fmt.Sprintf("  (+%d more)", len(warnings)-1)
		}
	}
	m := model{
		nav:        nav,
		content:    content,
		portInput:  ti,
		cmd:        cmd,
		sh:         sh,
		jq:         jq,
		stage:      "nav",
		status:     status,
		width:      100,
		height:     28,
		kubeconfig: kubeconfig,
		context:    context,
		namespace:  st.Namespace,
		kroot:      st.KustomRoot,
		forwards:   st.Forwards,
		favs:       st.Favorites,
		resuming:   resumeAny(cfg, st.Forwards),
		logID:      newPaneID(),
		vis:        map[string]*visCache{},
		mlogStyles: map[color]lipgloss.Style{},
		mlogPaint:  map[string]string{},
		cfg:        cfg,
		cfgNote:    len(warnings) > 0,
		notice:     opts.Notice,
		paintBg:    cfg.UI.PaintBackground(pal.Light),
		themeName:  pal.Name,
		shFmt:      cfg.OutputIndex(),
		run:        opts.Runner,
		k:          k,
	}
	m.spin = newSpinner()
	m.version = opts.Version
	m.panes = []paneSnap{m.snap()}
	m.tabs = []tab{{root: layout.New(m.logID), focus: m.logID}}
	m.restorePanes(st)
	return m
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{tick(), readNamespaceCmd(m.k, m.kubeconfig, m.context)}
	if m.notice != "" {
		text := m.notice
		cmds = append(cmds, func() tea.Msg { return noticeMsg{text: text} })
	}
	if m.resuming {
		cmds = append(cmds, resumeForwards(m.k, m.logDir(), m.forwards, func(f forward.Forward) bool {
			return wantsAutoRestart(m.cfg, f)
		}))
	}
	cmds = append(cmds, m.resumeStern()...)
	return tea.Batch(cmds...)
}

func newPaneID() string {
	paneIDs++
	return strconv.Itoa(paneIDs)
}

var paneIDs int

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

type kScanMsg struct {
	root string
	dirs []string
}

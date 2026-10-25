package ui

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type paneTitle struct {
	num    int
	hot    bool
	config string
	ctx    string
	view   string
	muted  bool
}

func (m model) title(i int) paneTitle {
	kc, ctx, id := m.kubeconfig, m.context, m.logID
	if i != m.focus && i < len(m.panes) {
		s := m.panes[i]
		kc, ctx, id = s.kubeconfig, s.context, s.logID
	}
	return paneTitle{
		num:    m.tabPos(id) + 1,
		hot:    m.cfg.IsProd(ctx),
		config: shortKubeconfig(kc),
		ctx:    ctx,
		view:   m.paneLabel(i),
		muted:  m.cfg.UI.DimInactive && i != m.focus,
	}
}

func shortKubeconfig(kc string) string {
	if kc == "" {
		return ""
	}
	if home, err := os.UserHomeDir(); err == nil && filepath.Clean(kc) == filepath.Join(home, ".kube", "config") {
		return ""
	}
	base := filepath.Base(kc)
	if ext := filepath.Ext(base); ext != "" && ext != base {
		base = strings.TrimSuffix(base, ext)
	}
	return base
}

func (t paneTitle) render(width int, frame lipgloss.Style) string {
	if width < 1 {
		return ""
	}
	head := strconv.Itoa(t.num) + " "
	if t.hot {
		head += skull + " "
	}
	if t.ctx == "" {
		return frame.Render(clipLine(head+t.view, width))
	}
	ctxStyle := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	if t.muted {
		ctxStyle = dimStyle
	}
	if t.hot {
		ctxStyle = lipgloss.NewStyle().Foreground(th.Danger).Bold(true)
	}
	sep := " · "
	w := func(s string) int { return ansi.StringWidth(s) }
	plain := head + t.ctx + sep + t.view
	if t.config != "" && w(t.config+":"+plain) <= width {
		return frame.Render(head) + dimStyle.Render(t.config+":") + ctxStyle.Render(t.ctx) + frame.Render(sep+t.view)
	}
	if w(plain) <= width {
		return frame.Render(head) + ctxStyle.Render(t.ctx) + frame.Render(sep+t.view)
	}

	if rest := width - w(head+t.ctx+sep); rest >= 2 {
		return frame.Render(head) + ctxStyle.Render(t.ctx) + frame.Render(sep+clipLine(t.view, rest))
	}
	if rest := width - w(head); rest >= 1 {
		return frame.Render(head) + ctxStyle.Render(clipLine(t.ctx, rest))
	}
	return frame.Render(clipLine(strings.TrimSpace(head), width))
}

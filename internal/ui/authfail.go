package ui

import (
	"strings"

	"k2/internal/kube"
)

func (m model) loginHint() string {
	if s := m.cfg.Keys.Login.Short(); s != "" {
		return s + " or :login"
	}
	return ":login"
}

func (m model) tokenDead() (string, bool) {
	if !m.cfg.UI.TokenBar || m.tok.ctx != infoKey(m.kubeconfig, m.context) {
		return "", false
	}
	life := m.tok.life
	switch {
	case life.Invalid:
		return "token rejected", true
	case life.Expired && !life.Known:
		return "token expired", true
	case life.Known && !life.End.After(now()):
		return "token expired", true
	}
	return "", false
}

func (m model) failureText(f kube.Failure, text string) string {
	switch f {
	case kube.FailAuth:
		reason := kube.FailureReason(f, text)
		if why, ok := m.tokenDead(); ok && reason == "token expired or rejected" {
			reason = why
		}
		return "not logged in: " + reason + ", press " + m.loginHint()
	case kube.FailConn:
		return "cannot reach cluster: " + kube.FailureReason(f, text)
	}
	return kube.FailureReason(f, text)
}

func (m model) listFailure(text string) (string, bool) {
	f := kube.ClassifyFailure(text)
	if f != kube.FailAuth && f != kube.FailConn {
		return "", false
	}
	return m.failureText(f, text), true
}

func (m model) discoveryProblem() string {
	if len(m.apis) > 0 {
		return ""
	}
	if m.resContext == m.context && m.resFail != kube.FailNone {
		if m.resFail == kube.FailOther {
			return "cannot read the API resources: " + kube.FailureReason(m.resFail, m.resText)
		}
		return m.failureText(m.resFail, m.resText)
	}
	if why, ok := m.tokenDead(); ok {
		return "not logged in: " + why + ", press " + m.loginHint()
	}
	return ""
}

func (m *model) setDiscovery(msg resListMsg) {
	m.resources = msg.names
	m.apis = msg.apis
	m.resContext = msg.ctx
	m.resFail = msg.fail
	m.resText = strings.TrimSpace(msg.text)
}

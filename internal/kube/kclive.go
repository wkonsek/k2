package kube

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"k2/internal/shell"
)

const LiveTimeout = 3 * time.Second

type AccessState int

const (
	AccessUnknown AccessState = iota
	AccessOK
	AccessUnauthorized
	AccessForbidden
	AccessExecFailed
)

type Reach struct {
	OK     bool
	Code   int
	Reason string
}

type Access struct {
	State  AccessState
	User   string
	Reason string
}

type Live struct {
	Reach  Reach
	Access Access
	At     time.Time
}

type liveCreds struct {
	server   *url.URL
	tls      *tls.Config
	proxy    *url.URL
	bearer   string
	basicU   string
	basicP   string
	cert     *tls.Certificate
	execFail string
	anon     bool
}

func CheckContext(run shell.Runner, kubeconfigPath, contextName string) Live {
	live := Live{At: time.Now()}
	cr, err := loadLiveCreds(run, kubeconfigPath, contextName)
	if cr.server == nil {
		live.Reach = Reach{Reason: or(errText(err), "no server")}
		live.Access = Access{Reason: "api unreachable"}
		return live
	}
	live.Reach = probeReach(cr)
	if !live.Reach.OK {
		live.Access = Access{Reason: "api unreachable"}
		return live
	}
	live.Access = probeAccess(cr)
	return live
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return shortErr(err)
}

func loadLiveCreds(run shell.Runner, p, contextName string) (liveCreds, error) {
	var cr liveCreds
	kc, err := readKubeconfig(p)
	if err != nil {
		return cr, errors.New("kubeconfig unreadable")
	}
	if contextName == "" {
		contextName = kc.CurrentContext
	}
	var clusterName, userName string
	found := false
	for _, c := range kc.Contexts {
		if c.Name == contextName {
			clusterName, userName, found = c.Context.Cluster, c.Context.User, true
			break
		}
	}
	if !found {
		return cr, errors.New("context not found")
	}
	cl, ok := kc.cluster(clusterName)
	if !ok || strings.TrimSpace(cl.Server) == "" {
		return cr, errors.New("no server")
	}
	dir := filepath.Dir(p)
	server := strings.TrimSpace(cl.Server)
	if !strings.Contains(server, "://") {
		server = "https://" + server
	}
	cr.server, err = url.Parse(server)
	if err != nil || cr.server.Host == "" {
		cr.server = nil
		return cr, errors.New("bad server url")
	}
	cr.tls = &tls.Config{InsecureSkipVerify: cl.Insecure, ServerName: cl.TLSServerName, MinVersion: tls.VersionTLS12}
	if !cl.Insecure {
		ca, okCA := decodeB64(cl.CAData)
		if !okCA && cl.CA != "" {
			if b, err := os.ReadFile(resolvePath(dir, cl.CA)); err == nil {
				ca, okCA = b, true
			}
		}
		if okCA {
			pool := x509.NewCertPool()
			if pool.AppendCertsFromPEM(ca) {
				cr.tls.RootCAs = pool
			}
		}
	}
	if cl.ProxyURL != "" {
		cr.proxy, _ = url.Parse(cl.ProxyURL)
	}
	u, ok := kc.user(userName)
	if !ok {
		cr.anon = true
		return cr, nil
	}
	applyUserCreds(run, &cr, u, dir)
	return cr, nil
}

func applyUserCreds(run shell.Runner, cr *liveCreds, u kcUser, dir string) {
	if cb, ok := certBytes(u, dir); ok {
		kb, okKey := decodeB64(u.KeyData)
		if !okKey && u.Key != "" {
			if b, err := os.ReadFile(resolvePath(dir, u.Key)); err == nil {
				kb, okKey = b, true
			}
		}
		if okKey {
			if pair, err := tls.X509KeyPair(cb, kb); err == nil {
				cr.cert = &pair
			}
		}
	}
	if tok, _ := userToken(u, dir); tok != "" {
		cr.bearer = tok
		return
	}
	switch {
	case u.Exec != nil:
		runExecCreds(run, cr, u.Exec, dir)
	case u.AuthProvider != nil:
		cr.bearer = firstOf(u.AuthProvider.Config, "id-token", "access-token")
	case u.Username != "" && cr.cert == nil:
		cr.basicU, cr.basicP = u.Username, u.Password
	}
	if cr.bearer == "" && cr.basicU == "" && cr.cert == nil && cr.execFail == "" {
		cr.anon = true
	}
}

func runExecCreds(run shell.Runner, cr *liveCreds, e *kcExec, dir string) {
	if e.Command == "" || run == nil {
		cr.execFail = "no command"
		return
	}
	name := e.Command
	if strings.ContainsAny(name, `/\`) {
		name = resolvePath(dir, name)
	}
	info, _ := json.Marshal(map[string]any{
		"apiVersion": or(e.APIVersion, "client.authentication.k8s.io/v1"),
		"kind":       "ExecCredential",
		"spec":       map[string]any{"interactive": false},
	})
	env := []string{"KUBERNETES_EXEC_INFO=" + string(info)}
	for _, kv := range e.Env {
		env = append(env, kv.Name+"="+kv.Value)
	}
	ctx, cancel := context.WithTimeout(context.Background(), LiveTimeout)
	defer cancel()
	res := run.Run(shell.Request{Name: name, Args: e.Args, Env: env, Ctx: ctx})
	if ctx.Err() != nil {
		cr.execFail = "timeout"
		return
	}
	if res.Err != nil {
		cr.execFail = "exit " + fmt.Sprint(res.Code)
		return
	}
	var out struct {
		Status struct {
			Token    string `json:"token"`
			CertData string `json:"clientCertificateData"`
			KeyData  string `json:"clientKeyData"`
		} `json:"status"`
	}
	if json.Unmarshal(res.Stdout, &out) != nil {
		cr.execFail = "bad output"
		return
	}
	if out.Status.CertData != "" && out.Status.KeyData != "" {
		if pair, err := tls.X509KeyPair([]byte(out.Status.CertData), []byte(out.Status.KeyData)); err == nil {
			cr.cert = &pair
		}
	}
	cr.bearer = strings.TrimSpace(out.Status.Token)
	if cr.bearer == "" && cr.cert == nil {
		cr.execFail = "no credentials"
	}
}

func (cr liveCreds) client(withCert bool) *http.Client {
	cfg := cr.tls.Clone()
	if withCert && cr.cert != nil {
		cfg.Certificates = []tls.Certificate{*cr.cert}
	}
	tr := &http.Transport{
		TLSClientConfig:       cfg,
		DisableKeepAlives:     true,
		TLSHandshakeTimeout:   LiveTimeout,
		ResponseHeaderTimeout: LiveTimeout,
		Proxy:                 http.ProxyFromEnvironment,
	}
	if cr.proxy != nil {
		tr.Proxy = http.ProxyURL(cr.proxy)
	}
	return &http.Client{
		Transport:     tr,
		Timeout:       LiveTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func (cr liveCreds) endpoint(p string) string {
	u := *cr.server
	u.Path = strings.TrimRight(u.Path, "/") + p
	u.RawQuery = ""
	return u.String()
}

func probeReach(cr liveCreds) Reach {
	ctx, cancel := context.WithTimeout(context.Background(), LiveTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cr.endpoint("/version"), nil)
	if err != nil {
		return Reach{Reason: "bad url"}
	}
	resp, err := cr.client(false).Do(req)
	if err != nil {
		return Reach{Reason: shortErr(err)}
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	return Reach{OK: true, Code: resp.StatusCode}
}

func (cr liveCreds) authorize(req *http.Request) {
	switch {
	case cr.bearer != "":
		req.Header.Set("Authorization", "Bearer "+cr.bearer)
	case cr.basicU != "":
		req.SetBasicAuth(cr.basicU, cr.basicP)
	}
}

func probeAccess(cr liveCreds) Access {
	if cr.execFail != "" {
		return Access{State: AccessExecFailed, Reason: cr.execFail}
	}
	ctx, cancel := context.WithTimeout(context.Background(), LiveTimeout)
	defer cancel()
	cl := cr.client(true)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cr.endpoint("/api"), nil)
	if err != nil {
		return Access{Reason: "bad url"}
	}
	cr.authorize(req)
	resp, err := cl.Do(req)
	if err != nil {
		return Access{Reason: shortErr(err)}
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return Access{State: AccessUnauthorized, Reason: anonReason(cr)}
	case resp.StatusCode == http.StatusForbidden:
		return Access{State: AccessForbidden, Reason: anonReason(cr)}
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return Access{State: AccessOK, User: whoAmI(ctx, cl, cr)}
	}
	return Access{Reason: fmt.Sprintf("http %d", resp.StatusCode)}
}

func anonReason(cr liveCreds) string {
	if cr.anon {
		return "no credentials"
	}
	return ""
}

func whoAmI(ctx context.Context, cl *http.Client, cr liveCreds) string {
	body := `{"apiVersion":"authentication.k8s.io/v1","kind":"SelfSubjectReview"}`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cr.endpoint("/apis/authentication.k8s.io/v1/selfsubjectreviews"), strings.NewReader(body))
	if err == nil {
		req.Header.Set("Content-Type", "application/json")
		cr.authorize(req)
		if data, ok := doJSON(cl, req); ok {
			var out struct {
				Status struct {
					UserInfo struct {
						Username string `json:"username"`
					} `json:"userInfo"`
				} `json:"status"`
			}
			if json.Unmarshal(data, &out) == nil && out.Status.UserInfo.Username != "" {
				return out.Status.UserInfo.Username
			}
		}
	}
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, cr.endpoint("/apis/user.openshift.io/v1/users/~"), nil)
	if err != nil {
		return ""
	}
	cr.authorize(req)
	if data, ok := doJSON(cl, req); ok {
		var out struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
		}
		if json.Unmarshal(data, &out) == nil {
			return out.Metadata.Name
		}
	}
	return ""
}

func doJSON(cl *http.Client, req *http.Request) ([]byte, bool) {
	resp, err := cl.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return bytes.TrimSpace(data), err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300
}

func shortErr(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	var (
		dns  *net.DNSError
		ua   x509.UnknownAuthorityError
		host x509.HostnameError
		inv  x509.CertificateInvalidError
		op   *net.OpError
	)
	switch {
	case errors.Is(err, context.DeadlineExceeded), isTimeout(err):
		return "timeout"
	case errors.As(err, &dns):
		if dns.IsNotFound {
			return "dns: no such host"
		}
		return "dns error"
	case errors.As(err, &ua):
		return "tls: unknown authority"
	case errors.As(err, &host):
		return "tls: hostname mismatch"
	case errors.As(err, &inv):
		if inv.Reason == x509.Expired {
			return "tls: certificate expired"
		}
		return "tls: invalid certificate"
	case errors.As(err, &op):
		if strings.Contains(op.Err.Error(), "refused") {
			return "connection refused"
		}
		if strings.Contains(op.Err.Error(), "unreachable") {
			return "network unreachable"
		}
	}
	s := strings.TrimSpace(err.Error())
	if i := strings.LastIndex(s, ": "); i >= 0 && len(s) > 40 {
		s = s[i+2:]
	}
	if len(s) > 40 {
		s = s[:39] + "…"
	}
	return s
}

func isTimeout(err error) bool {
	var t interface{ Timeout() bool }
	return errors.As(err, &t) && t.Timeout()
}

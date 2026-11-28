package kube

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type AuthKind string

const (
	AuthNone     AuthKind = "none"
	AuthToken    AuthKind = "token"
	AuthCert     AuthKind = "cert"
	AuthExec     AuthKind = "exec"
	AuthProvider AuthKind = "auth-provider"
	AuthBasic    AuthKind = "basic"
)

type ValidState int

const (
	ValidUnknown ValidState = iota
	ValidOK
	ValidExpired
)

type Validity struct {
	State ValidState
	At    time.Time
}

type AuthInfo struct {
	Kind      AuthKind
	OpenShift bool
	Command   string
	Provider  string
	Present   bool
	Note      string
	Valid     Validity
}

type KubeconfigContext struct {
	Name      string
	Cluster   string
	Server    string
	Namespace string
	User      string
	Current   bool
	Auth      AuthInfo
}

type KubeconfigInfo struct {
	Path     string
	Current  string
	Contexts []KubeconfigContext
}

type kcFile struct {
	CurrentContext string `yaml:"current-context"`
	Clusters       []struct {
		Name    string `yaml:"name"`
		Cluster kcCluster
	} `yaml:"clusters"`
	Contexts []struct {
		Name    string `yaml:"name"`
		Context struct {
			Cluster   string `yaml:"cluster"`
			User      string `yaml:"user"`
			Namespace string `yaml:"namespace"`
		} `yaml:"context"`
	} `yaml:"contexts"`
	Users []struct {
		Name string `yaml:"name"`
		User kcUser
	} `yaml:"users"`
}

type kcCluster struct {
	Server        string `yaml:"server"`
	CAData        string `yaml:"certificate-authority-data"`
	CA            string `yaml:"certificate-authority"`
	Insecure      bool   `yaml:"insecure-skip-tls-verify"`
	TLSServerName string `yaml:"tls-server-name"`
	ProxyURL      string `yaml:"proxy-url"`
}

type kcUser struct {
	Token        string `yaml:"token"`
	TokenFile    string `yaml:"tokenFile"`
	CertData     string `yaml:"client-certificate-data"`
	Cert         string `yaml:"client-certificate"`
	KeyData      string `yaml:"client-key-data"`
	Key          string `yaml:"client-key"`
	Username     string `yaml:"username"`
	Password     string `yaml:"password"`
	Exec         *kcExec
	AuthProvider *struct {
		Name   string            `yaml:"name"`
		Config map[string]string `yaml:"config"`
	} `yaml:"auth-provider"`
}

type kcExec struct {
	APIVersion string `yaml:"apiVersion"`
	Command    string `yaml:"command"`
	Args       []string
	Env        []struct {
		Name  string `yaml:"name"`
		Value string `yaml:"value"`
	}
}

func readKubeconfig(p string) (kcFile, error) {
	var kc kcFile
	data, err := os.ReadFile(p)
	if err != nil {
		return kc, err
	}
	err = yaml.Unmarshal(data, &kc)
	return kc, err
}

func (kc kcFile) cluster(name string) (kcCluster, bool) {
	for _, c := range kc.Clusters {
		if c.Name == name {
			return c.Cluster, true
		}
	}
	return kcCluster{}, false
}

func (kc kcFile) user(name string) (kcUser, bool) {
	for _, u := range kc.Users {
		if u.Name == name {
			return u.User, true
		}
	}
	return kcUser{}, false
}

func InspectKubeconfig(p string) (KubeconfigInfo, error) {
	kc, err := readKubeconfig(p)
	if err != nil {
		return KubeconfigInfo{Path: p}, err
	}
	dir := filepath.Dir(p)
	info := KubeconfigInfo{Path: p, Current: kc.CurrentContext}
	for _, c := range kc.Contexts {
		ctx := KubeconfigContext{
			Name: c.Name, Cluster: c.Context.Cluster, User: c.Context.User,
			Namespace: c.Context.Namespace, Current: c.Name == kc.CurrentContext,
		}
		if cl, ok := kc.cluster(c.Context.Cluster); ok {
			ctx.Server = cl.Server
		}
		if u, ok := kc.user(c.Context.User); ok {
			ctx.Auth = describeAuth(u, dir)
		} else {
			ctx.Auth = AuthInfo{Kind: AuthNone, Note: "user not found"}
		}
		info.Contexts = append(info.Contexts, ctx)
	}
	return info, nil
}

func resolvePath(dir, p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(dir, p)
}

func decodeB64(s string) ([]byte, bool) {
	s = strings.Join(strings.Fields(s), "")
	if s == "" {
		return nil, false
	}
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return b, true
	}
	if b, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(s, "=")); err == nil {
		return b, true
	}
	return nil, false
}

func userToken(u kcUser, dir string) (string, string) {
	if t := strings.TrimSpace(u.Token); t != "" {
		return t, ""
	}
	if u.TokenFile == "" {
		return "", ""
	}
	data, err := os.ReadFile(resolvePath(dir, u.TokenFile))
	if err != nil {
		return "", "tokenFile unreadable"
	}
	return strings.TrimSpace(string(data)), ""
}

func CertNotAfter(pemData []byte) (time.Time, bool) {
	for len(pemData) > 0 {
		var blk *pem.Block
		blk, pemData = pem.Decode(pemData)
		if blk == nil {
			return time.Time{}, false
		}
		if blk.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(blk.Bytes)
		if err != nil {
			return time.Time{}, false
		}
		return cert.NotAfter, true
	}
	return time.Time{}, false
}

func validityAt(t time.Time) Validity {
	if t.IsZero() {
		return Validity{}
	}
	if !t.After(time.Now()) {
		return Validity{State: ValidExpired, At: t}
	}
	return Validity{State: ValidOK, At: t}
}

func tokenValidity(tok string) Validity {
	life := JWTLife(Token{secret: tok})
	if !life.Known {
		return Validity{}
	}
	return validityAt(life.End)
}

func certBytes(u kcUser, dir string) ([]byte, bool) {
	if b, ok := decodeB64(u.CertData); ok {
		return b, true
	}
	if u.Cert != "" {
		if b, err := os.ReadFile(resolvePath(dir, u.Cert)); err == nil {
			return b, true
		}
	}
	return nil, false
}

func keyPresent(u kcUser, dir string) bool {
	if _, ok := decodeB64(u.KeyData); ok {
		return true
	}
	if u.Key != "" {
		_, err := os.Stat(resolvePath(dir, u.Key))
		return err == nil
	}
	return false
}

func describeAuth(u kcUser, dir string) AuthInfo {
	switch tok, note := userToken(u, dir); {
	case tok != "":
		return AuthInfo{Kind: AuthToken, OpenShift: strings.HasPrefix(tok, "sha256~"), Present: true, Valid: tokenValidity(tok)}
	case u.Token != "" || u.TokenFile != "":
		if note == "" {
			note = "token empty"
		}
		return AuthInfo{Kind: AuthToken, Note: note}
	}
	switch {
	case u.CertData != "" || u.Cert != "":
		info := AuthInfo{Kind: AuthCert}
		pemData, ok := certBytes(u, dir)
		if !ok {
			info.Note = "certificate unreadable"
			return info
		}
		info.Present = true
		if !keyPresent(u, dir) {
			info.Note = "key missing"
		}
		if end, ok := CertNotAfter(pemData); ok {
			info.Valid = validityAt(end)
		}
		return info
	case u.Exec != nil:
		return AuthInfo{Kind: AuthExec, Command: path.Base(filepath.ToSlash(u.Exec.Command)), Present: u.Exec.Command != ""}
	case u.AuthProvider != nil:
		info := AuthInfo{Kind: AuthProvider, Provider: u.AuthProvider.Name}
		cfg := u.AuthProvider.Config
		if tok := firstOf(cfg, "id-token", "access-token"); tok != "" {
			info.Present = true
			info.Valid = tokenValidity(tok)
			if info.Valid.State == ValidUnknown {
				if t, err := time.Parse(time.RFC3339, cfg["expiry"]); err == nil {
					info.Valid = validityAt(t)
				}
			}
		}
		return info
	case u.Username != "" || u.Password != "":
		return AuthInfo{Kind: AuthBasic, Present: u.Password != ""}
	}
	return AuthInfo{Kind: AuthNone}
}

func firstOf(m map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(m[k]); v != "" {
			return v
		}
	}
	return ""
}

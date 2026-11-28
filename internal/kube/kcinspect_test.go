package kube

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testJWT(exp time.Time) string {
	claims := map[string]any{"sub": "alice"}
	if !exp.IsZero() {
		claims["exp"] = exp.Unix()
	}
	raw, _ := json.Marshal(claims)
	return "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString(raw) + ".sig"
}

func testCert(t *testing.T, notAfter time.Time) (certB64, keyB64 string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "alice"},
		NotBefore:    notAfter.Add(-48 * time.Hour),
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kb, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	c := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	k := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb})
	return base64.StdEncoding.EncodeToString(c), base64.StdEncoding.EncodeToString(k)
}

func writeKC(t *testing.T, server string, users map[string]string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("apiVersion: v1\nkind: Config\ncurrent-context: prod\nclusters:\n- name: prod-cluster\n  cluster:\n    server: " + server + "\n")
	b.WriteString("contexts:\n")
	names := []string{"prod", "staging", "dev", "ocp", "oidc", "legacy", "anon", "ghost"}
	for _, n := range names {
		if _, ok := users[n]; !ok && n != "ghost" {
			continue
		}
		fmt.Fprintf(&b, "- name: %s\n  context:\n    cluster: prod-cluster\n    user: %s-user\n", n, n)
		if n == "staging" {
			b.WriteString("    namespace: apps\n")
		}
	}
	b.WriteString("users:\n")
	for _, n := range names {
		if body, ok := users[n]; ok {
			fmt.Fprintf(&b, "- name: %s-user\n  user:\n%s", n, body)
		}
	}
	p := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(p, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func ctxByName(t *testing.T, info KubeconfigInfo, name string) KubeconfigContext {
	t.Helper()
	for _, c := range info.Contexts {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no context %s in %+v", name, info)
	return KubeconfigContext{}
}

func TestInspectAuthTypes(t *testing.T) {
	future := time.Now().Add(72 * time.Hour)
	past := time.Now().Add(-5 * time.Hour)
	certOK, keyOK := testCert(t, future)
	tokenFile := filepath.Join(t.TempDir(), "tok")
	if err := os.WriteFile(tokenFile, []byte(testJWT(future)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	users := map[string]string{
		"prod":    "    token: " + testJWT(future) + "\n",
		"staging": "    tokenFile: " + tokenFile + "\n",
		"dev":     "    client-certificate-data: " + certOK + "\n    client-key-data: " + keyOK + "\n",
		"ocp":     "    token: sha256~abcdefghijklmnop\n",
		"oidc":    "    auth-provider:\n      name: oidc\n      config:\n        id-token: " + testJWT(past) + "\n",
		"legacy":  "    exec:\n      apiVersion: client.authentication.k8s.io/v1\n      command: /opt/bin/get-token\n      args: [--cluster, prod]\n",
		"anon":    "    username: bob\n    password: hunter2\n",
	}
	p := writeKC(t, "https://api.prod.example.com:6443", users)
	info, err := InspectKubeconfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if info.Current != "prod" || !ctxByName(t, info, "prod").Current || ctxByName(t, info, "dev").Current {
		t.Fatalf("current marking: %+v", info)
	}

	prod := ctxByName(t, info, "prod")
	if prod.Server != "https://api.prod.example.com:6443" || prod.Cluster != "prod-cluster" || prod.User != "prod-user" || prod.Namespace != "" {
		t.Fatalf("prod: %+v", prod)
	}
	if prod.Auth.Kind != AuthToken || !prod.Auth.Present || prod.Auth.OpenShift || prod.Auth.Valid.State != ValidOK {
		t.Fatalf("prod auth: %+v", prod.Auth)
	}
	if ns := ctxByName(t, info, "staging"); ns.Namespace != "apps" || ns.Auth.Kind != AuthToken || ns.Auth.Valid.State != ValidOK {
		t.Fatalf("staging via tokenFile: %+v", ns)
	}
	if dev := ctxByName(t, info, "dev"); dev.Auth.Kind != AuthCert || !dev.Auth.Present || dev.Auth.Note != "" || dev.Auth.Valid.State != ValidOK {
		t.Fatalf("dev cert: %+v", dev.Auth)
	}
	if ocp := ctxByName(t, info, "ocp"); ocp.Auth.Kind != AuthToken || !ocp.Auth.OpenShift || ocp.Auth.Valid.State != ValidUnknown {
		t.Fatalf("ocp: %+v", ocp.Auth)
	}
	if oidc := ctxByName(t, info, "oidc"); oidc.Auth.Kind != AuthProvider || oidc.Auth.Provider != "oidc" || oidc.Auth.Valid.State != ValidExpired {
		t.Fatalf("oidc: %+v", oidc.Auth)
	}
	if legacy := ctxByName(t, info, "legacy"); legacy.Auth.Kind != AuthExec || legacy.Auth.Command != "get-token" || legacy.Auth.Valid.State != ValidUnknown {
		t.Fatalf("exec: %+v", legacy.Auth)
	}
	if anon := ctxByName(t, info, "anon"); anon.Auth.Kind != AuthBasic || !anon.Auth.Present {
		t.Fatalf("basic: %+v", anon.Auth)
	}
	if ghost := ctxByName(t, info, "ghost"); ghost.Auth.Kind != AuthNone || ghost.Auth.Note == "" {
		t.Fatalf("missing user: %+v", ghost.Auth)
	}
}

func TestInspectNoneAndMissing(t *testing.T) {
	p := writeKC(t, "https://api.dev.example.com", map[string]string{
		"prod":    "    {}\n",
		"staging": "    token: \"\"\n    tokenFile: /nonexistent/token\n",
		"dev":     "    client-certificate-data: " + base64.StdEncoding.EncodeToString([]byte("junk")) + "\n",
	})
	info, err := InspectKubeconfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if a := ctxByName(t, info, "prod").Auth; a.Kind != AuthNone || a.Present {
		t.Fatalf("none: %+v", a)
	}
	if a := ctxByName(t, info, "staging").Auth; a.Kind != AuthToken || a.Present || a.Note == "" {
		t.Fatalf("missing tokenFile: %+v", a)
	}
	if a := ctxByName(t, info, "dev").Auth; a.Kind != AuthCert || !a.Present || a.Note != "key missing" || a.Valid.State != ValidUnknown {
		t.Fatalf("junk cert: %+v", a)
	}
	if _, err := InspectKubeconfig(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("missing file must fail")
	}
}

func TestJWTExpiryDecode(t *testing.T) {
	future := time.Now().Add(2 * time.Hour).Truncate(time.Second)
	v := tokenValidity(testJWT(future))
	if v.State != ValidOK || !v.At.Equal(future) {
		t.Fatalf("future: %+v", v)
	}
	if v := tokenValidity(testJWT(time.Now().Add(-time.Minute))); v.State != ValidExpired {
		t.Fatalf("past: %+v", v)
	}
	for name, tok := range map[string]string{"no exp": testJWT(time.Time{}), "opaque": "plain-opaque", "bad b64": "a.!!!.c", "oc": "sha256~xyz"} {
		if v := tokenValidity(tok); v.State != ValidUnknown {
			t.Fatalf("%s: %+v", name, v)
		}
	}
}

func TestClientCertNotAfter(t *testing.T) {
	end := time.Now().Add(10 * 24 * time.Hour).Truncate(time.Second)
	c, _ := testCert(t, end)
	raw, _ := base64.StdEncoding.DecodeString(c)
	got, ok := CertNotAfter(raw)
	if !ok || !got.Equal(end) {
		t.Fatalf("not after %v ok %v want %v", got, ok, end)
	}
	if _, ok := CertNotAfter([]byte("not a cert")); ok {
		t.Fatal("garbage parsed")
	}
	expired, _ := testCert(t, time.Now().Add(-time.Hour))
	raw, _ = base64.StdEncoding.DecodeString(expired)
	at, _ := CertNotAfter(raw)
	if v := validityAt(at); v.State != ValidExpired {
		t.Fatalf("expired cert: %+v", v)
	}
}

func TestInspectDoesNotLeakSecrets(t *testing.T) {
	const secret = "sha256~very-secret-token-value"
	p := writeKC(t, "https://api.prod.example.com", map[string]string{
		"prod":   "    token: " + secret + "\n",
		"anon":   "    username: bob\n    password: very-secret-password\n",
		"legacy": "    exec:\n      command: get-token\n",
	})
	info, err := InspectKubeconfig(p)
	if err != nil {
		t.Fatal(err)
	}
	dump := fmt.Sprintf("%+v %#v", info, info)
	for _, s := range []string{secret, "very-secret-password"} {
		if strings.Contains(dump, s) {
			t.Fatalf("leaked %q in %s", s, dump)
		}
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func writeFile(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
}

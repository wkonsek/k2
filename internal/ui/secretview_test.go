package ui

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"k2/internal/kube"
	"k2/internal/shell"
	"k2/internal/theme"
)

func testCertPEM(t *testing.T, cn string, notAfter time.Time) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(7),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
		DNSNames:     []string{cn},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func secretDoc(name string, data map[string]string) string {
	enc := map[string]string{}
	for k, v := range data {
		enc[k] = base64.StdEncoding.EncodeToString([]byte(v))
	}
	raw, _ := json.Marshal(map[string]any{
		"metadata": map[string]any{"name": name, "namespace": "shop", "creationTimestamp": "2026-10-22T10:00:00Z"},
		"data":     enc,
	})
	return string(raw)
}

type secretCluster struct {
	data  map[string]map[string]string
	yamls int
}

func (c *secretCluster) fake() *shell.Fake {
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		j := strings.Join(r.Args, " ")
		switch {
		case strings.HasPrefix(j, "get secrets ") && strings.Contains(j, "-o yaml"):
			c.yamls++
			return shell.OK("apiVersion: v1\nkind: Secret\nmetadata:\n  name: " + r.Args[2] + "\ndata:\n  encoded: true\n")
		case strings.HasPrefix(j, "get secrets -o json"):
			var items []string
			for name, d := range c.data {
				items = append(items, secretDoc(name, d))
			}
			return shell.OK(`{"items":[` + strings.Join(items, ",") + `]}`)
		case strings.HasPrefix(j, "get secret ") && strings.Contains(j, "-o json"):
			name := r.Args[2]
			return shell.OK(secretDoc(name, c.data[name]))
		}
		return shell.Fail(1, "not in this test: "+j)
	}}
}

func openSecrets(t *testing.T, c *secretCluster) model {
	t.Helper()
	m := newTestModel(t, testEnv(t), c.fake())
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	next, cmd := m.openResource("secrets")
	m = settle(t, next.(model), cmd)
	if m.stage != "res" || len(m.content.Items()) == 0 {
		t.Fatalf("stage %q items %d status %q", m.stage, len(m.content.Items()), m.status)
	}
	return m
}

func selectSecret(t *testing.T, m model, name string) model {
	t.Helper()
	for i, raw := range m.content.Items() {
		if raw.(item).id == name {
			m.content.Select(i)
			return m
		}
	}
	t.Fatalf("no secret %q", name)
	return m
}

func TestSecretCycleEncodedDecodedCertificates(t *testing.T) {
	c := &secretCluster{data: map[string]map[string]string{
		"web-tls": {"tls.crt": testCertPEM(t, "shop.example.com", time.Now().Add(90*24*time.Hour+time.Hour)), "tls.key": "-----BEGIN PRIVATE KEY-----\nAAAA\n-----END PRIVATE KEY-----\n"},
	}}
	m := openSecrets(t, c)
	m = selectSecret(t, m, "web-tls")

	m = pressSettle(t, m, "x")
	if m.stage != "view" || m.secretMode() != "decoded" || !strings.Contains(m.logText, "BEGIN CERTIFICATE") || !strings.Contains(m.logText, "BEGIN PRIVATE KEY") {
		t.Fatalf("decoded: stage %q mode %q text %q", m.stage, m.secretMode(), m.logText)
	}
	if !strings.Contains(m.paneView(), "shop/web-tls · decoded") || !strings.Contains(ansi.Strip(m.legend()), "x certificates") {
		t.Fatalf("title %q legend %q", m.paneView(), ansi.Strip(m.legend()))
	}

	m = pressSettle(t, m, "x")
	if m.stage != "view" || m.secretMode() != "cert" {
		t.Fatalf("cert: stage %q mode %q", m.stage, m.secretMode())
	}
	for _, want := range []string{"tls.crt", "CN=shop.example.com", "expires in 90 days", "ECDSA P-256", "sha-256", "tls.key", "[private key hidden]"} {
		if !strings.Contains(m.logText, want) {
			t.Errorf("cert view missing %q:\n%s", want, m.logText)
		}
	}
	if strings.Contains(m.logText, "BEGIN") {
		t.Fatalf("cert view shows PEM:\n%s", m.logText)
	}
	if !strings.Contains(m.paneView(), "shop/web-tls · certificates") || !strings.Contains(ansi.Strip(m.legend()), "x encoded") {
		t.Fatalf("title %q legend %q", m.paneView(), ansi.Strip(m.legend()))
	}
	if strings.Contains(ansi.Strip(m.View()), "BEGIN PRIVATE KEY") {
		t.Fatal("private key drawn")
	}

	m = pressSettle(t, m, "x")
	if m.stage != "yaml" || m.secretMode() != "" || !strings.Contains(m.logText, "kind: Secret") || c.yamls != 1 {
		t.Fatalf("encoded: stage %q mode %q text %q", m.stage, m.secretMode(), m.logText)
	}
	if !strings.Contains(m.paneView(), "shop/web-tls · encoded") {
		t.Fatalf("title %q", m.paneView())
	}

	m = pressSettle(t, m, "x")
	if m.stage != "view" || m.secretMode() != "decoded" {
		t.Fatalf("back to decoded: stage %q mode %q", m.stage, m.secretMode())
	}

	m = pressSettle(t, m, "esc")
	if m.stage != "res" || m.vs.secret != "" {
		t.Fatalf("esc: stage %q mode %q", m.stage, m.vs.secret)
	}
}

func TestSecretWithoutCertsTogglesEncodedDecoded(t *testing.T) {
	c := &secretCluster{data: map[string]map[string]string{"db": {"password": "hunter2"}}}
	m := selectSecret(t, openSecrets(t, c), "db")
	m = pressSettle(t, m, "x")
	if m.secretMode() != "decoded" || !strings.Contains(m.logText, "hunter2") {
		t.Fatalf("mode %q text %q", m.secretMode(), m.logText)
	}
	if strings.Contains(ansi.Strip(m.legend()), "x certificates") || !strings.Contains(ansi.Strip(m.legend()), "x encoded") {
		t.Fatalf("legend %q", ansi.Strip(m.legend()))
	}
	m = pressSettle(t, m, "x")
	if m.stage != "yaml" || m.secretMode() != "" {
		t.Fatalf("stage %q mode %q", m.stage, m.secretMode())
	}
	m = pressSettle(t, m, "x")
	if m.stage != "view" || m.secretMode() != "decoded" {
		t.Fatalf("stage %q mode %q", m.stage, m.secretMode())
	}
}

func TestSecretXFromEncodedYAML(t *testing.T) {
	c := &secretCluster{data: map[string]map[string]string{"db": {"password": "hunter2"}}}
	m := selectSecret(t, openSecrets(t, c), "db")
	m = pressSettle(t, m, "enter")
	if m.stage != "yaml" {
		t.Fatalf("stage %q", m.stage)
	}
	m = pressSettle(t, m, "x")
	if m.stage != "view" || !strings.Contains(m.logText, "hunter2") || m.secretMode() != "decoded" {
		t.Fatalf("stage %q mode %q text %q", m.stage, m.secretMode(), m.logText)
	}
}

func TestSecretXIgnoredOutsideSecrets(t *testing.T) {
	c := &secretCluster{data: map[string]map[string]string{"db": {"password": "hunter2"}}}
	m := newTestModel(t, testEnv(t), c.fake())
	m.stage, m.kind, m.subject, m.logText = "yaml", "deployments", "shop/web", "kind: Deployment"
	m.vs.watchKind, m.vs.watchName, m.vs.watchNS = "deployments", "web", "shop"
	m = press(t, m, "x")
	if m.stage != "yaml" || m.logText != "kind: Deployment" {
		t.Fatalf("stage %q text %q", m.stage, m.logText)
	}
}

func TestSecretWatchRefreshesEveryMode(t *testing.T) {
	soon := time.Now().Add(10*24*time.Hour + time.Hour)
	c := &secretCluster{data: map[string]map[string]string{"web-tls": {"tls.crt": testCertPEM(t, "one.example.com", soon)}}}
	m := selectSecret(t, openSecrets(t, c), "web-tls")
	m = pressSettle(t, m, "enter")
	m = pressSettle(t, m, "r")
	if !m.vs.watchOn || m.stage != "yaml" {
		t.Fatalf("watch %v stage %q", m.vs.watchOn, m.stage)
	}

	m = pressSettle(t, m, "x")
	if m.stage != "view" || !m.vs.watchOn || m.secretMode() != "decoded" {
		t.Fatalf("watch must survive x: stage %q on %v mode %q", m.stage, m.vs.watchOn, m.secretMode())
	}
	if !strings.Contains(ansi.Strip(m.legend()), "r watching") {
		t.Fatalf("legend %q", ansi.Strip(m.legend()))
	}

	before := m.logText
	c.data["web-tls"] = map[string]string{"tls.crt": testCertPEM(t, "two.example.com", soon)}
	next, cmd := m.Update(tickMsg{})
	next, cmd = next.(model).Update(tickMsg{})
	m = settle(t, next.(model), cmd)
	if m.logText == before || !strings.Contains(m.logText, "BEGIN CERTIFICATE") || m.secretMode() != "decoded" {
		t.Fatalf("decoded refresh: mode %q text %q", m.secretMode(), m.logText)
	}

	m = pressSettle(t, m, "x")
	if m.secretMode() != "cert" || !strings.Contains(m.logText, "CN=two.example.com") {
		t.Fatalf("cert: mode %q text %q", m.secretMode(), m.logText)
	}
	c.data["web-tls"] = map[string]string{"tls.crt": testCertPEM(t, "three.example.com", soon)}
	next, cmd = m.Update(tickMsg{})
	next, cmd = next.(model).Update(tickMsg{})
	m = settle(t, next.(model), cmd)
	if m.secretMode() != "cert" || !strings.Contains(m.logText, "CN=three.example.com") || strings.Contains(m.logText, "BEGIN") {
		t.Fatalf("cert refresh: mode %q text %q", m.secretMode(), m.logText)
	}

	m = pressSettle(t, m, "x")
	if m.stage != "yaml" || !m.vs.watchOn {
		t.Fatalf("encoded: stage %q watch %v", m.stage, m.vs.watchOn)
	}
	m = press(t, m, "r")
	if m.vs.watchOn {
		t.Fatal("r stops the watch")
	}
}

func TestColorCertHighlightsExpiry(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	defer useTheme(theme.Nord())
	p, _ := theme.Lookup("nord")
	useTheme(p)
	lines := []string{
		"tls.crt",
		"    validity      expired 9 days ago",
		"    validity      expires in 12 days",
		"    validity      expires today",
		"    validity      expires in 400 days",
		"    validity      not valid yet",
		"    subject       CN=x",
	}
	out := colorCert(lines)
	code := func(c lipgloss.Color) string { return strings.TrimSuffix(strings.TrimPrefix(sgr(c), "\x1b["), "m") }
	want := []lipgloss.Color{p.Danger, p.Warn, p.Warn, p.OK, p.Warn}
	for i, c := range want {
		got := out[i+1]
		if !strings.Contains(got, code(c)) {
			t.Errorf("line %d: drawn %q, want %q", i+1, got, sgr(c))
		}
		for _, other := range []lipgloss.Color{p.Danger, p.Warn, p.OK} {
			if other != c && strings.Contains(got[strings.Index(got, "validity")+len("validity"):], code(other)) {
				t.Errorf("line %d: also drawn in %q", i+1, sgr(other))
			}
		}
	}
	for i := range out {
		if ansi.Strip(out[i]) != lines[i] {
			t.Fatalf("text changed: %q vs %q", ansi.Strip(out[i]), lines[i])
		}
	}
	if kube.CertSoonDays != 30 {
		t.Fatalf("soon threshold %d", kube.CertSoonDays)
	}
}

func TestColorCertContinuationLines(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	lines := []string{
		"tls.crt",
		"  subject       CN=x",
		"  alt names     DNS:a.example",
		"                DNS:www.shop.acme.example",
		"  sha-256       AA:BB",
		"                CC:DD",
	}
	out := colorCert(lines)
	for _, i := range []int{3, 5} {
		if n := strings.Count(out[i], "\x1b[0m"); n != 1 {
			t.Errorf("line %d drawn in %d runs: %q", i, n, out[i])
		}
		if ansi.Strip(out[i]) != lines[i] {
			t.Errorf("text changed: %q", ansi.Strip(out[i]))
		}
	}
	if n := strings.Count(out[1], "\x1b[0m"); n != 2 {
		t.Errorf("label and value are separate runs: %q", out[1])
	}
}

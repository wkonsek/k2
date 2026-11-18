package kube

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"strings"
	"testing"
	"time"
)

type certOpts struct {
	cn        string
	dns       []string
	ips       []string
	emails    []string
	uri       string
	notBefore time.Time
	notAfter  time.Time
	ca        bool
	pathLen   int
	rsaBits   int
	ed        bool
}

func makeCert(t *testing.T, o certOpts) (der []byte, pemText string) {
	t.Helper()
	var pub, priv any
	switch {
	case o.ed:
		p, k, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		pub, priv = p, k
	case o.rsaBits > 0:
		k, err := rsa.GenerateKey(rand.Reader, o.rsaBits)
		if err != nil {
			t.Fatal(err)
		}
		pub, priv = &k.PublicKey, k
	default:
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		pub, priv = &k.PublicKey, k
	}
	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(0x0a1b2c),
		Subject:               pkix.Name{CommonName: o.cn, Organization: []string{"Acme"}},
		NotBefore:             o.notBefore,
		NotAfter:              o.notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		DNSNames:              o.dns,
		EmailAddresses:        o.emails,
		IsCA:                  o.ca,
	}
	if o.ca {
		tpl.KeyUsage = x509.KeyUsageCertSign
		tpl.MaxPathLen = o.pathLen
		tpl.MaxPathLenZero = o.pathLen == 0
	}
	for _, ip := range o.ips {
		tpl.IPAddresses = append(tpl.IPAddresses, net.ParseIP(ip))
	}
	if o.uri != "" {
		u, err := url.Parse(o.uri)
		if err != nil {
			t.Fatal(err)
		}
		tpl.URIs = []*url.URL{u}
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, pub, priv)
	if err != nil {
		t.Fatal(err)
	}
	return der, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func secretJSON(t *testing.T, data map[string]string) []byte {
	t.Helper()
	enc := map[string]string{}
	for k, v := range data {
		enc[k] = base64.StdEncoding.EncodeToString([]byte(v))
	}
	raw, err := json.Marshal(map[string]any{"data": enc})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

const privateKeyPEM = "-----BEGIN PRIVATE KEY-----\nTUlHSEFnRUFNQk1HQnlxR1NNNDlBZ0VHQ0NxR1NNNDk=\n-----END PRIVATE KEY-----\n"

func TestCertDetails(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	der, text := makeCert(t, certOpts{
		cn: "shop.example.com", dns: []string{"shop.example.com", "*.shop.example.com"}, ips: []string{"10.0.0.7"},
		emails: []string{"ops@example.com"}, uri: "spiffe://cluster/ns/shop",
		notBefore: now.Add(-48 * time.Hour), notAfter: now.Add(90*24*time.Hour + time.Hour), rsaBits: 2048,
	})
	v, err := SecretViewsAt(secretJSON(t, map[string]string{"tls.crt": text}), now)
	if err != nil || !v.HasCerts {
		t.Fatalf("%v hasCerts=%v", err, v.HasCerts)
	}
	sum := sha256.Sum256(der)
	parts := make([]string, 16)
	for i, b := range sum[:16] {
		parts[i] = fmt.Sprintf("%02X", b)
	}
	fp := strings.Join(parts, ":")
	for _, want := range []string{
		"tls.crt\n",
		"subject       CN=shop.example.com,O=Acme",
		"issuer        CN=shop.example.com,O=Acme",
		"serial        0a:1b:2c",
		"not before    2026-11-16 12:00:00 UTC",
		"not after     2027-02-16 13:00:00 UTC",
		"validity      expires in 90 days",
		"alt names     DNS:shop.example.com",
		"DNS:*.shop.example.com",
		"IP:10.0.0.7",
		"email:ops@example.com",
		"URI:spiffe://cluster/ns/shop",
		"public key    RSA 2048 bit",
		"signature     SHA256-RSA",
		"ca            no",
		"key usage     Digital Signature, Key Encipherment",
		"ext key usage Server Auth, Client Auth",
		"sha-256       " + fp,
	} {
		if !strings.Contains(v.Certs, want) {
			t.Errorf("missing %q in:\n%s", want, v.Certs)
		}
	}
	if strings.Contains(v.Certs, "BEGIN CERTIFICATE") {
		t.Fatal("cert view must not repeat the PEM")
	}
	if !strings.Contains(v.Decoded, "BEGIN CERTIFICATE") {
		t.Fatal("decoded view keeps the PEM text")
	}
}

func TestCertKeyAlgorithms(t *testing.T) {
	now := time.Now()
	for want, o := range map[string]certOpts{
		"public key    ECDSA P-256":  {cn: "a"},
		"public key    Ed25519":      {cn: "b", ed: true},
		"public key    RSA 1024 bit": {cn: "c", rsaBits: 1024},
	} {
		o.notBefore, o.notAfter = now.Add(-time.Hour), now.Add(time.Hour)
		_, text := makeCert(t, o)
		v, _ := SecretViewsAt(secretJSON(t, map[string]string{"x": text}), now)
		if !strings.Contains(v.Certs, want) {
			t.Errorf("want %q in\n%s", want, v.Certs)
		}
	}
}

func TestCertAuthority(t *testing.T) {
	now := time.Now()
	_, text := makeCert(t, certOpts{cn: "root", ca: true, pathLen: 0, notBefore: now.Add(-time.Hour), notAfter: now.Add(time.Hour)})
	v, _ := SecretViewsAt(secretJSON(t, map[string]string{"ca.crt": text}), now)
	if !strings.Contains(v.Certs, "ca            yes (path length 0)") || !strings.Contains(v.Certs, "key usage     Certificate Sign") {
		t.Fatalf("%s", v.Certs)
	}
	_, text = makeCert(t, certOpts{cn: "root", ca: true, pathLen: -1, notBefore: now.Add(-time.Hour), notAfter: now.Add(time.Hour)})
	v, _ = SecretViewsAt(secretJSON(t, map[string]string{"ca.crt": text}), now)
	if !strings.Contains(v.Certs, "ca            yes\n") {
		t.Fatalf("%s", v.Certs)
	}
}

func TestCertChain(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	_, leaf := makeCert(t, certOpts{cn: "leaf.example.com", dns: []string{"leaf.example.com"}, notBefore: now.Add(-time.Hour), notAfter: now.Add(49 * time.Hour)})
	_, root := makeCert(t, certOpts{cn: "Root CA", ca: true, notBefore: now.Add(-time.Hour), notAfter: now.Add(24*time.Hour*400 + time.Hour)})
	_, third := makeCert(t, certOpts{cn: "Extra", notBefore: now.Add(-time.Hour), notAfter: now.Add(time.Hour)})
	v, _ := SecretViewsAt(secretJSON(t, map[string]string{"tls.crt": leaf + root + third}), now)
	for _, want := range []string{
		"tls.crt · 3 certificates",
		"certificate 1 of 3", "certificate 2 of 3", "certificate 3 of 3",
		"CN=leaf.example.com,O=Acme", "CN=Root CA,O=Acme", "CN=Extra,O=Acme",
		"expires in 2 days", "expires in 400 days",
	} {
		if !strings.Contains(v.Certs, want) {
			t.Errorf("missing %q in:\n%s", want, v.Certs)
		}
	}
	if n := strings.Count(v.Certs, "subject "); n != 3 {
		t.Fatalf("subjects %d", n)
	}
}

func TestCertDER(t *testing.T) {
	now := time.Now()
	der, _ := makeCert(t, certOpts{cn: "der.example.com", notBefore: now.Add(-time.Hour), notAfter: now.Add(24 * time.Hour * 10)})
	enc := base64.StdEncoding.EncodeToString(der)
	raw, _ := json.Marshal(map[string]any{"data": map[string]string{"ca.der": enc}})
	v, err := SecretViewsAt(raw, now)
	if err != nil || !v.HasCerts || !strings.Contains(v.Certs, "CN=der.example.com") {
		t.Fatalf("%v %v\n%s", err, v.HasCerts, v.Certs)
	}
	if !strings.Contains(v.Decoded, "[binary") {
		t.Fatalf("decoded view stays as before: %s", v.Decoded)
	}
}

func TestCertViewKeepsNonCertValues(t *testing.T) {
	now := time.Now()
	v, err := SecretViewsAt(secretJSON(t, map[string]string{"user": "admin", "multi": "a\nb\n", "empty": ""}), now)
	if err != nil || v.HasCerts {
		t.Fatalf("%v %v", err, v.HasCerts)
	}
	if v.Certs != v.Decoded {
		t.Fatalf("no certs: cert view must equal decoded view\n%s\n---\n%s", v.Certs, v.Decoded)
	}
	if !strings.Contains(v.Certs, "user\n  admin") || !strings.Contains(v.Certs, "multi\n  a\n  b") || !strings.Contains(v.Certs, "empty\n  [empty]") {
		t.Fatalf("%s", v.Certs)
	}
}

func TestCertViewHidesPrivateKeys(t *testing.T) {
	now := time.Now()
	_, crt := makeCert(t, certOpts{cn: "web", notBefore: now.Add(-time.Hour), notAfter: now.Add(time.Hour)})
	v, _ := SecretViewsAt(secretJSON(t, map[string]string{"tls.crt": crt, "tls.key": privateKeyPEM, "note": "hello"}), now)
	if strings.Contains(v.Certs, "PRIVATE KEY") || strings.Contains(v.Certs, "TUlHSEFn") {
		t.Fatalf("private key leaked in cert view:\n%s", v.Certs)
	}
	if !strings.Contains(v.Certs, "tls.key\n  [private key hidden]") || !strings.Contains(v.Certs, "note\n  hello") {
		t.Fatalf("%s", v.Certs)
	}
	if !strings.Contains(v.Decoded, "BEGIN PRIVATE KEY") {
		t.Fatal("decoded view is unchanged")
	}
	only, _ := SecretViewsAt(secretJSON(t, map[string]string{"tls.key": privateKeyPEM}), now)
	if only.HasCerts {
		t.Fatal("a private key alone is not a certificate")
	}
}

func TestCertExpiry(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		after time.Time
		want  string
	}{
		{now.Add(-9*24*time.Hour - time.Hour), "expired 9 days ago"},
		{now.Add(-25 * time.Hour), "expired 1 day ago"},
		{now.Add(-time.Hour), "expired today"},
		{now.Add(time.Hour), "expires today"},
		{now.Add(36 * time.Hour), "expires in 1 day"},
		{now.Add(29*24*time.Hour + time.Hour), "expires in 29 days"},
		{now.Add(365 * 24 * time.Hour), "expires in 365 days"},
	} {
		if got := CertLife(c.after, now); got != c.want {
			t.Errorf("CertLife(%v) = %q, want %q", c.after.Sub(now), got, c.want)
		}
	}
	_, text := makeCert(t, certOpts{cn: "old", notBefore: now.Add(-400 * 24 * time.Hour), notAfter: now.Add(-3 * 24 * time.Hour)})
	v, _ := SecretViewsAt(secretJSON(t, map[string]string{"ca.crt": text}), now)
	if !strings.Contains(v.Certs, "validity      expired 3 days ago") {
		t.Fatalf("%s", v.Certs)
	}
	_, text = makeCert(t, certOpts{cn: "future", notBefore: now.Add(24 * time.Hour), notAfter: now.Add(48 * time.Hour)})
	v, _ = SecretViewsAt(secretJSON(t, map[string]string{"ca.crt": text}), now)
	if !strings.Contains(v.Certs, "validity      not valid yet") {
		t.Fatalf("%s", v.Certs)
	}
}

func TestCertBrokenBlock(t *testing.T) {
	bad := "-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\n"
	v, _ := SecretViewsAt(secretJSON(t, map[string]string{"tls.crt": bad}), time.Now())
	if !v.HasCerts || !strings.Contains(v.Certs, "[cannot parse certificate]") {
		t.Fatalf("%s", v.Certs)
	}
}

func TestSecretViewsErrors(t *testing.T) {
	if _, err := SecretViewsAt([]byte("nope"), time.Now()); err == nil {
		t.Fatal("want error")
	}
	v, err := SecretViewsAt([]byte(`{"data":{}}`), time.Now())
	if err != nil || v.Decoded != "(no data)" || v.Certs != "(no data)" || v.HasCerts {
		t.Fatalf("%v %+v", err, v)
	}
	raw, _ := json.Marshal(map[string]any{"data": map[string]string{"x": "%%%"}})
	v, _ = SecretViewsAt(raw, time.Now())
	if !strings.Contains(v.Certs, "[not base64]") {
		t.Fatalf("%s", v.Certs)
	}
}

package ui

import (
	"strings"
	"testing"

	"k2/internal/kube"
	"k2/internal/shell"
)

const certAPIs = `pods                               po           v1                                true   Pod
certificatesigningrequests         csr          certificates.k8s.io/v1            false  CertificateSigningRequest
certificates                       cert,certs   cert-manager.io/v1                true   Certificate
certificaterequests                cr,crs       cert-manager.io/v1                true   CertificateRequest
`

const k8sCSRsOnly = `pods                               po           v1                                true   Pod
certificatesigningrequests         csr          certificates.k8s.io/v1            false  CertificateSigningRequest
`

const demoCerts = `{"items":[
 {"apiVersion":"cert-manager.io/v1","kind":"Certificate",
  "metadata":{"name":"shop-web","namespace":"shop","creationTimestamp":"2026-01-01T00:00:00Z"},
  "spec":{"secretName":"web-tls","issuerRef":{"name":"shop-ca","kind":"Issuer"}},
  "status":{"conditions":[{"type":"Ready","status":"True","message":"Certificate is up to date"}]}},
 {"apiVersion":"cert-manager.io/v1","kind":"Certificate",
  "metadata":{"name":"payments-api","namespace":"payments","creationTimestamp":"2026-01-01T00:00:00Z"},
  "spec":{"secretName":"api-tls","issuerRef":{"name":"public-ca","kind":"ClusterIssuer"}},
  "status":{"conditions":[{"type":"Ready","status":"False","message":"Issuer not ready"}]}}
]}`

const demoCertReqs = `{"items":[
 {"apiVersion":"cert-manager.io/v1","kind":"CertificateRequest",
  "metadata":{"name":"shop-web-1","namespace":"shop","creationTimestamp":"2026-01-01T00:00:00Z"},
  "spec":{"username":"system:serviceaccount:cert-manager:cert-manager","issuerRef":{"name":"shop-ca","kind":"Issuer"}},
  "status":{"conditions":[{"type":"Approved","status":"True"},{"type":"Denied","status":"False"},{"type":"Ready","status":"True","message":"Certificate issued"}]}}
]}`

const demoK8sCSRs = `{"items":[
 {"apiVersion":"certificates.k8s.io/v1","kind":"CertificateSigningRequest",
  "metadata":{"name":"lab-node-a","creationTimestamp":"2026-01-01T00:00:00Z"},
  "spec":{"signerName":"kubernetes.io/kubelet-serving","username":"system:node:lab-node-a"}}
]}`

func certCluster(apis string, byKind map[string]shell.Result) *shell.Fake {
	if apis == "" {
		apis = certAPIs
	}
	return &shell.Fake{Handler: func(r shell.Request) shell.Result {
		if len(r.Args) > 0 && r.Args[0] == "api-resources" {
			return shell.OK(apis)
		}
		if len(r.Args) > 1 && r.Args[0] == "get" {
			if res, ok := byKind[r.Args[1]]; ok {
				return res
			}
			return shell.OK(`{"items":[]}`)
		}
		return shell.Fail(1, "not in this test")
	}}
}

func certModel(t *testing.T, f *shell.Fake, apis string, aliases map[string]string) model {
	t.Helper()
	if apis == "" {
		apis = certAPIs
	}
	if f == nil {
		f = certCluster(apis, nil)
	}
	cfg := testEnv(t)
	if aliases != nil {
		cfg.Aliases = aliases
	}
	m := newTestModel(t, cfg, f)
	m.kubeconfig, m.context, m.namespace = "/kc", "kind-dev", "shop"
	m.apis = kube.ParseAPIList(apis)
	m.resources = kube.ResourceNames(m.apis)
	m.cmdOn = true
	m.cmd.SetValue("")
	m.cmd.Focus()
	return m
}

func TestCSRAliasOpensCertManagerRequests(t *testing.T) {
	f := certCluster(certAPIs, map[string]shell.Result{
		"certificaterequests.cert-manager.io": shell.OK(demoCertReqs),
		"certificatesigningrequests":          shell.OK(demoK8sCSRs),
	})
	m := open(t, certModel(t, f, certAPIs, nil), "csr")
	if m.stage != "res" || m.kind != "certificaterequests.cert-manager.io" {
		t.Fatalf("csr -> %s %s (%s)", m.stage, m.kind, m.status)
	}
	it := m.content.Items()[0].(item)
	if strings.Join(it.heads, " ") != "NAME APPROVED DENIED READY ISSUER REQUESTOR STATUS AGE" || it.cells[0] != "shop-web-1" || it.cells[4] != "shop-ca" {
		t.Fatalf("csr row %v %v", it.heads, it.cells)
	}
	gets := calls(f, "get")
	if len(gets) != 1 || !strings.HasPrefix(gets[0], "get certificaterequests.cert-manager.io ") {
		t.Fatalf("gets %v", gets)
	}
}

func TestCSRAliasOverrideAndFullName(t *testing.T) {
	f := certCluster(certAPIs, map[string]shell.Result{
		"certificaterequests.cert-manager.io": shell.OK(demoCertReqs),
		"certificatesigningrequests":          shell.OK(demoK8sCSRs),
	})
	m := open(t, certModel(t, f, certAPIs, map[string]string{"csr": "certificatesigningrequests"}), "csr")
	if m.kind != "certificatesigningrequests" {
		t.Fatalf("override: kind %q status %q", m.kind, m.status)
	}
	gets := calls(f, "get")
	if len(gets) == 0 {
		t.Fatalf("override gets %v", gets)
	}
	for _, g := range gets {
		if !strings.HasPrefix(g, "get certificatesigningrequests ") || strings.Contains(g, "certificaterequests") {
			t.Fatalf("override gets %v", gets)
		}
	}

	f2 := certCluster(certAPIs, map[string]shell.Result{"certificatesigningrequests": shell.OK(demoK8sCSRs)})
	m = open(t, certModel(t, f2, certAPIs, nil), "certificatesigningrequests")
	if m.kind != "certificatesigningrequests" || m.stage != "res" {
		t.Fatalf("full name: %s %s (%s)", m.stage, m.kind, m.status)
	}
}

func TestCSRMissingCertManager(t *testing.T) {
	f := certCluster(k8sCSRsOnly, map[string]shell.Result{"certificatesigningrequests": shell.OK(demoK8sCSRs)})
	m := open(t, certModel(t, f, k8sCSRsOnly, nil), "csr")
	if m.stage == "res" || !strings.Contains(m.status, "no cert-manager") {
		t.Fatalf("missing cert-manager: stage %q kind %q status %q", m.stage, m.kind, m.status)
	}
	if strings.Contains(m.status, "unknown resource") {
		t.Fatalf("should not look unknown: %q", m.status)
	}
	m = open(t, certModel(t, f, k8sCSRsOnly, nil), "certificatesigningrequests")
	if m.kind != "certificatesigningrequests" || m.stage != "res" {
		t.Fatalf("k8s csr still reachable: %s %s (%s)", m.stage, m.kind, m.status)
	}
}

func TestCertsTableShowsIssuer(t *testing.T) {
	f := certCluster(certAPIs, map[string]shell.Result{"certs": shell.OK(demoCerts), "certificates": shell.OK(demoCerts)})
	m := open(t, certModel(t, f, certAPIs, nil), "certs")
	if m.stage != "res" || (m.kind != "certs" && m.kind != "certificates" && m.kind != "certificates.cert-manager.io") {
		t.Fatalf("certs -> %s %s (%s)", m.stage, m.kind, m.status)
	}
	if len(m.content.Items()) != 2 {
		t.Fatalf("rows %d status %q", len(m.content.Items()), m.status)
	}
	byName := map[string]item{}
	for _, raw := range m.content.Items() {
		it := raw.(item)
		byName[it.cells[0]] = it
		if strings.Join(it.heads, " ") != "NAME READY SECRET ISSUER STATUS AGE" {
			t.Fatalf("heads %v", it.heads)
		}
	}
	if it := byName["shop-web"]; it.cells[1] != "True" || it.cells[2] != "web-tls" || it.cells[3] != "shop-ca" {
		t.Fatalf("shop-web %v", it.cells)
	}
	if it := byName["payments-api"]; it.cells[3] != "ClusterIssuer/public-ca" || it.cells[1] != "False" {
		t.Fatalf("payments-api %v", it.cells)
	}
}

func TestCSRIsABuiltinCandidate(t *testing.T) {
	m := certModel(t, nil, certAPIs, nil)
	got := strings.Join(m.cmdCandidates("cs"), " ")
	if !strings.Contains(got, "csr") {
		t.Fatalf("candidates %s", got)
	}
}

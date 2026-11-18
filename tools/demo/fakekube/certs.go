package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"time"
)

type certSpec struct {
	cn       string
	org      string
	dns      []string
	ips      []string
	notAfter time.Time
	age      time.Duration
}

func pemBlock(kind string, der []byte) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: der}))
}

func demoChain(now time.Time, s certSpec) (chain, key string) {
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Acme Demo Root CA", Organization: []string{"Acme"}},
		NotBefore:             now.Add(-2 * 365 * 24 * time.Hour),
		NotAfter:              now.Add(8 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}
	caDER, _ := x509.CreateCertificate(rand.Reader, caTpl, caTpl, &caKey.PublicKey, caKey)
	caCert, _ := x509.ParseCertificate(caDER)
	leafKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 100))
	tpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: s.cn, Organization: []string{s.org}},
		NotBefore:             now.Add(-s.age),
		NotAfter:              s.notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		DNSNames:              s.dns,
	}
	for _, ip := range s.ips {
		tpl.IPAddresses = append(tpl.IPAddresses, net.ParseIP(ip))
	}
	leafDER, _ := x509.CreateCertificate(rand.Reader, tpl, caCert, &leafKey.PublicKey, caKey)
	keyDER, _ := x509.MarshalPKCS8PrivateKey(leafKey)
	return pemBlock("CERTIFICATE", leafDER) + pemBlock("CERTIFICATE", caDER), pemBlock("PRIVATE KEY", keyDER)
}

package kube

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"strings"
	"time"
)

const CertSoonDays = 30

var keyUsageNames = []struct {
	use  x509.KeyUsage
	name string
}{
	{x509.KeyUsageDigitalSignature, "Digital Signature"},
	{x509.KeyUsageContentCommitment, "Content Commitment"},
	{x509.KeyUsageKeyEncipherment, "Key Encipherment"},
	{x509.KeyUsageDataEncipherment, "Data Encipherment"},
	{x509.KeyUsageKeyAgreement, "Key Agreement"},
	{x509.KeyUsageCertSign, "Certificate Sign"},
	{x509.KeyUsageCRLSign, "CRL Sign"},
	{x509.KeyUsageEncipherOnly, "Encipher Only"},
	{x509.KeyUsageDecipherOnly, "Decipher Only"},
}

var extKeyUsageNames = map[x509.ExtKeyUsage]string{
	x509.ExtKeyUsageAny:                            "Any",
	x509.ExtKeyUsageServerAuth:                     "Server Auth",
	x509.ExtKeyUsageClientAuth:                     "Client Auth",
	x509.ExtKeyUsageCodeSigning:                    "Code Signing",
	x509.ExtKeyUsageEmailProtection:                "Email Protection",
	x509.ExtKeyUsageIPSECEndSystem:                 "IPSec End System",
	x509.ExtKeyUsageIPSECTunnel:                    "IPSec Tunnel",
	x509.ExtKeyUsageIPSECUser:                      "IPSec User",
	x509.ExtKeyUsageTimeStamping:                   "Time Stamping",
	x509.ExtKeyUsageOCSPSigning:                    "OCSP Signing",
	x509.ExtKeyUsageMicrosoftServerGatedCrypto:     "Microsoft Server Gated Crypto",
	x509.ExtKeyUsageNetscapeServerGatedCrypto:      "Netscape Server Gated Crypto",
	x509.ExtKeyUsageMicrosoftCommercialCodeSigning: "Microsoft Commercial Code Signing",
	x509.ExtKeyUsageMicrosoftKernelCodeSigning:     "Microsoft Kernel Code Signing",
}

type certEntry struct {
	cert *x509.Certificate
}

func certEntries(raw []byte) (entries []certEntry, private bool) {
	rest := raw
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		switch {
		case block.Type == "CERTIFICATE" || block.Type == "TRUSTED CERTIFICATE":
			c, _ := x509.ParseCertificate(block.Bytes)
			entries = append(entries, certEntry{cert: c})
		case strings.Contains(block.Type, "PRIVATE KEY"):
			private = true
		}
	}
	if len(entries) == 0 && !private && len(raw) > 0 && raw[0] == 0x30 {
		if c, err := x509.ParseCertificate(raw); err == nil {
			entries = append(entries, certEntry{cert: c})
		}
	}
	return entries, private
}

func CertDetails(c *x509.Certificate, now time.Time) []string {
	var lines []string
	add := func(label string, values ...string) {
		for i, v := range values {
			l := ""
			if i == 0 {
				l = label
			}
			lines = append(lines, fmt.Sprintf("%-14s%s", l, v))
		}
	}
	add("subject", orNone(c.Subject.String()))
	add("issuer", orNone(c.Issuer.String()))
	add("serial", serialLines(c)...)
	add("not before", certStamp(c.NotBefore))
	add("not after", certStamp(c.NotAfter))
	add("validity", validityText(c, now))
	if names := altNames(c); len(names) > 0 {
		add("alt names", names...)
	}
	add("public key", keyText(c))
	add("signature", c.SignatureAlgorithm.String())
	add("ca", caText(c))
	if u := keyUsageText(c.KeyUsage); u != "" {
		add("key usage", u)
	}
	if u := extKeyUsageText(c.ExtKeyUsage); u != "" {
		add("ext key usage", u)
	}
	sum := sha256.Sum256(c.Raw)
	add("sha-256", hexLines(sum[:], "%02X")...)
	return lines
}

func CertLife(notAfter, now time.Time) string {
	d := notAfter.Sub(now)
	if d < 0 {
		days := int(-d / (24 * time.Hour))
		switch days {
		case 0:
			return "expired today"
		case 1:
			return "expired 1 day ago"
		}
		return fmt.Sprintf("expired %d days ago", days)
	}
	days := int(d / (24 * time.Hour))
	switch days {
	case 0:
		return "expires today"
	case 1:
		return "expires in 1 day"
	}
	return fmt.Sprintf("expires in %d days", days)
}

func validityText(c *x509.Certificate, now time.Time) string {
	if now.Before(c.NotBefore) {
		return "not valid yet"
	}
	return CertLife(c.NotAfter, now)
}

func certStamp(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05 UTC")
}

func orNone(s string) string {
	if s == "" {
		return "(empty)"
	}
	return s
}

func serialLines(c *x509.Certificate) []string {
	b := c.SerialNumber.Bytes()
	if len(b) == 0 {
		return []string{"00"}
	}
	return hexLines(b, "%02x")
}

func altNames(c *x509.Certificate) []string {
	var out []string
	for _, n := range c.DNSNames {
		out = append(out, "DNS:"+n)
	}
	for _, ip := range c.IPAddresses {
		out = append(out, "IP:"+net.IP(ip).String())
	}
	for _, e := range c.EmailAddresses {
		out = append(out, "email:"+e)
	}
	for _, u := range c.URIs {
		out = append(out, "URI:"+u.String())
	}
	return out
}

func keyText(c *x509.Certificate) string {
	switch k := c.PublicKey.(type) {
	case *rsa.PublicKey:
		return fmt.Sprintf("RSA %d bit", k.N.BitLen())
	case *ecdsa.PublicKey:
		return fmt.Sprintf("ECDSA %s", k.Curve.Params().Name)
	case ed25519.PublicKey:
		return "Ed25519"
	}
	return c.PublicKeyAlgorithm.String()
}

func caText(c *x509.Certificate) string {
	if !c.BasicConstraintsValid || !c.IsCA {
		return "no"
	}
	if c.MaxPathLen > 0 || (c.MaxPathLen == 0 && c.MaxPathLenZero) {
		return fmt.Sprintf("yes (path length %d)", c.MaxPathLen)
	}
	return "yes"
}

func keyUsageText(u x509.KeyUsage) string {
	var out []string
	for _, k := range keyUsageNames {
		if u&k.use != 0 {
			out = append(out, k.name)
		}
	}
	return strings.Join(out, ", ")
}

func extKeyUsageText(us []x509.ExtKeyUsage) string {
	var out []string
	for _, u := range us {
		if n, ok := extKeyUsageNames[u]; ok {
			out = append(out, n)
		} else {
			out = append(out, fmt.Sprintf("usage %d", int(u)))
		}
	}
	return strings.Join(out, ", ")
}

func hexLines(sum []byte, format string) []string {
	var lines []string
	for i := 0; i < len(sum); i += 16 {
		end := min(i+16, len(sum))
		parts := make([]string, 0, 16)
		for _, b := range sum[i:end] {
			parts = append(parts, fmt.Sprintf(format, b))
		}
		lines = append(lines, strings.Join(parts, ":"))
	}
	return lines
}

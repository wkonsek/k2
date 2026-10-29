package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func writeDemoKubeconfig(now time.Time) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, ".kube", "config")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	cfg := map[string]any{
		"apiVersion": "v1", "kind": "Config", "preferences": map[string]any{},
		"current-context": "kind-dev",
		"clusters": []any{
			cluster("kind-dev", "https://127.0.0.1:41235"),
			cluster("staging-eu-1", "https://api.staging-eu-1.example.com"),
			cluster("prod-eu-1", "https://api.prod-eu-1.example.com"),
		},
		"contexts": []any{
			context("kind-dev", "shop"),
			context("staging-eu-1", "payments"),
			context("prod-eu-1", "shop"),
		},
		"users": []any{
			map[string]any{"name": "kind-dev", "user": map[string]any{
				"client-certificate-data": b64("-----BEGIN CERTIFICATE-----\ndemo placeholder, not a certificate\n-----END CERTIFICATE-----\n"),
				"client-key-data":         b64("-----BEGIN PRIVATE KEY-----\ndemo placeholder, not a key\n-----END PRIVATE KEY-----\n")}},
			map[string]any{"name": "staging-eu-1", "user": map[string]any{"token": demoJWT(now.Add(-2*time.Hour-48*time.Minute), now.Add(5*time.Hour+12*time.Minute))}},
			map[string]any{"name": "prod-eu-1", "user": map[string]any{"token": demoJWT(now.Add(-9*time.Hour-20*time.Minute), now.Add(2*time.Hour+40*time.Minute))}},
		},
	}
	return writeKubeconfig(path, cfg)
}

func cluster(n, server string) map[string]any {
	return map[string]any{"name": n, "cluster": map[string]any{"server": server, "insecure-skip-tls-verify": true}}
}

func context(n, ns string) map[string]any {
	return map[string]any{"name": n, "context": map[string]any{"cluster": n, "user": n, "namespace": ns}}
}

func demoJWT(iat, exp time.Time) string {
	enc := func(v any) string {
		b, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(b)
	}
	head := enc(map[string]any{"alg": "RS256", "typ": "JWT", "kid": "k2-demo"})
	body := enc(map[string]any{"iss": "https://oidc.example.com", "sub": "dev@example.com", "aud": "kubernetes",
		"iat": iat.Unix(), "nbf": iat.Unix(), "exp": exp.Unix(), "groups": []string{"platform"}})
	return fmt.Sprintf("%s.%s.%s", head, body, base64.RawURLEncoding.EncodeToString([]byte("k2 demo signature, not valid")))
}

func setToken(args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("usage: fakekube token CONTEXT LEFT LIFETIME")
	}
	left, err := time.ParseDuration(args[1])
	if err != nil {
		return err
	}
	life, err := time.ParseDuration(args[2])
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, ".kube", "config")
	cfg, err := readKubeconfig(path)
	if err != nil {
		return err
	}
	now := time.Now()
	users, _ := cfg["users"].([]any)
	for _, u := range users {
		e, _ := u.(map[string]any)
		if e["name"] == args[0] {
			e["user"] = map[string]any{"token": demoJWT(now.Add(left-life), now.Add(left))}
			return writeKubeconfig(path, cfg)
		}
	}
	return fmt.Errorf("no user %s", args[0])
}

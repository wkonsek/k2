package kube

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"k2/internal/shell"
)

var dropMeta = []string{"uid", "resourceVersion", "creationTimestamp", "managedFields", "ownerReferences", "namespace",
	"generation", "selfLink", "generateName", "deletionTimestamp", "deletionGracePeriodSeconds", "finalizers"}

var secretName = regexp.MustCompile(`^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$`)

func ValidSecretName(name string) error {
	if name == "" {
		return fmt.Errorf("type a name")
	}
	if len(name) > 253 || !secretName.MatchString(name) {
		return fmt.Errorf("%q: lowercase letters, digits, - and . only", name)
	}
	return nil
}

func helmAnnotation(k string) bool {
	return strings.HasPrefix(k, "meta.helm.sh/") || strings.HasPrefix(k, "helm.sh/")
}

func helmLabel(k string, v any) bool {
	if k == "helm.sh/chart" {
		return true
	}
	s, _ := v.(string)
	return k == "app.kubernetes.io/managed-by" && strings.EqualFold(s, "helm")
}

func SecretCopy(raw []byte, name string) ([]byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("cannot read secret")
	}
	if k, _ := doc["kind"].(string); k != "" && k != "Secret" {
		return nil, fmt.Errorf("not a secret: %s", k)
	}
	meta, _ := doc["metadata"].(map[string]any)
	if meta == nil {
		meta = map[string]any{}
	}
	for _, k := range dropMeta {
		delete(meta, k)
	}
	meta["name"] = name
	if ann, ok := meta["annotations"].(map[string]any); ok {
		delete(ann, "kubectl.kubernetes.io/last-applied-configuration")
		for k := range ann {
			if helmAnnotation(k) {
				delete(ann, k)
			}
		}
		if len(ann) == 0 {
			delete(meta, "annotations")
		}
	}
	if labels, ok := meta["labels"].(map[string]any); ok {
		for k, v := range labels {
			if helmLabel(k, v) {
				delete(labels, k)
			}
		}
		if len(labels) == 0 {
			delete(meta, "labels")
		}
	}
	doc["metadata"] = meta
	doc["apiVersion"], doc["kind"] = "v1", "Secret"
	delete(doc, "status")
	return json.Marshal(doc)
}

func (c Client) SecretExists(kubeconfig, context, namespace, name string) (bool, error) {
	out, err := shell.Combined(c.Run, "kubectl", "get", "secret", name, "-n", namespace, "--ignore-not-found", "-o", "name",
		"--kubeconfig", kubeconfig, "--context", context)
	if err != nil {
		return false, fmt.Errorf("%s", shell.FailText(out, err))
	}
	return strings.TrimSpace(string(out)) != "", nil
}

func (c Client) CopySecret(kubeconfig, context, fromNS, name, toNS, toName string, replace bool) error {
	got := c.Run.Run(shell.Request{Name: "kubectl", Args: []string{"get", "secret", name, "-n", fromNS, "-o", "json",
		"--kubeconfig", kubeconfig, "--context", context}})
	if got.Err != nil {
		return fmt.Errorf("%s", shell.FailText(append(got.Stderr, got.Stdout...), got.Err))
	}
	doc, err := SecretCopy(got.Stdout, toName)
	if err != nil {
		return err
	}
	verb := "create"
	if replace {
		verb = "replace"
	}
	res := c.Run.Run(shell.Request{Name: "kubectl", Args: []string{verb, "-f", "-", "-n", toNS, "--kubeconfig", kubeconfig, "--context", context},
		Stdin: string(doc), Combined: true})
	if res.Err != nil {
		return fmt.Errorf("%s", shell.FailText(res.Stdout, res.Err))
	}
	return nil
}

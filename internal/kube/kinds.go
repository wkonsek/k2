package kube

import "strings"

var kindNames = map[string]string{
	"po": "pods", "pod": "pods", "pods": "pods",
	"deploy": "deployments", "deployment": "deployments", "deployments": "deployments", "dp": "deployments",
	"sts": "statefulsets", "statefulset": "statefulsets", "statefulsets": "statefulsets",
	"ds": "daemonsets", "daemonset": "daemonsets", "daemonsets": "daemonsets",
	"rs": "replicasets", "replicaset": "replicasets", "replicasets": "replicasets",
	"job": "jobs", "jobs": "jobs",
	"cj": "cronjobs", "cronjob": "cronjobs", "cronjobs": "cronjobs",
	"ev": "events", "event": "events", "events": "events",
	"secret": "secrets", "secrets": "secrets",
	"sub": "subscriptions", "subs": "subscriptions", "subscription": "subscriptions", "subscriptions": "subscriptions",
	"csv": "clusterserviceversions", "csvs": "clusterserviceversions", "clusterserviceversion": "clusterserviceversions", "clusterserviceversions": "clusterserviceversions",
	"ip": "installplans", "installplan": "installplans", "installplans": "installplans",
	"catsrc": "catalogsources", "catalogsource": "catalogsources", "catalogsources": "catalogsources",
	"og": "operatorgroups", "operatorgroup": "operatorgroups", "operatorgroups": "operatorgroups",
	"crd": "customresourcedefinitions", "crds": "customresourcedefinitions", "customresourcedefinition": "customresourcedefinitions", "customresourcedefinitions": "customresourcedefinitions",
	"route": "routes", "routes": "routes",
}

var kindGroups = map[string]string{
	"deployments": "apps", "statefulsets": "apps", "daemonsets": "apps", "replicasets": "apps",
	"jobs": "batch", "cronjobs": "batch",
	"events":        "",
	"subscriptions": "operators.coreos.com", "clusterserviceversions": "operators.coreos.com",
	"installplans": "operators.coreos.com", "catalogsources": "operators.coreos.com",
	"operatorgroups":            "operators.coreos.com",
	"customresourcedefinitions": "apiextensions.k8s.io",
	"routes":                    "route.openshift.io",
}

func BaseKind(kind string) string {
	k := strings.ToLower(strings.TrimSpace(kind))
	name, group, _ := strings.Cut(k, ".")
	base, ok := kindNames[name]
	if !ok {
		return k
	}
	if group != "" {
		want, known := kindGroups[base]
		if known && want != "" && group != want && !(base == "events" && group == "events.k8s.io") {
			return k
		}
	}
	return base
}

func KnownKind(name string) bool {
	_, ok := kindNames[strings.ToLower(name)]
	return ok
}

var Builtin = map[string]string{
	"dp":   "deployments",
	"sts":  "statefulsets",
	"ds":   "daemonsets",
	"rs":   "replicasets",
	"cm":   "configmaps",
	"sec":  "secrets",
	"svc":  "services",
	"ing":  "ingresses",
	"pvc":  "persistentvolumeclaims",
	"cj":   "cronjobs",
	"ev":   "events",
	"crd":  "customresourcedefinitions",
	"sub":  "subscriptions.operators.coreos.com",
	"csv":  "clusterserviceversions.operators.coreos.com",
	"ip":   "installplans.operators.coreos.com",
	"og":   "operatorgroups.operators.coreos.com",
	"cs":   "catalogsources.operators.coreos.com",
	"rt":   "routes.route.openshift.io",
	"proj": "projects",
}

func MissingAPI(kind, msg string) string {
	l := strings.ToLower(msg)
	if !strings.Contains(l, "doesn't have a resource type") && !strings.Contains(l, "does not have a resource type") &&
		!strings.Contains(l, "no matches for kind") && !strings.Contains(l, "could not find the requested resource") {
		return ""
	}
	return kind + " is not installed on this cluster" + apiHint(kind)
}

func apiHint(kind string) string {
	k := strings.ToLower(kind)
	switch BaseKind(k) {
	case "subscriptions", "clusterserviceversions", "installplans", "catalogsources", "operatorgroups":
		return " (no OLM)"
	case "routes":
		return " (not OpenShift)"
	}
	if strings.HasSuffix(k, ".operators.coreos.com") {
		return " (no OLM)"
	}
	if strings.HasSuffix(k, ".openshift.io") || k == "projects" {
		return " (not OpenShift)"
	}
	return ""
}

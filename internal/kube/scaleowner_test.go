package kube

import (
	"strings"
	"testing"

	"k2/internal/shell"
)

func TestScalableOwnerFollowsReplicaSetToDeployment(t *testing.T) {
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		line := strings.Join(r.Args, " ")
		switch {
		case strings.HasPrefix(line, "get pod kafka-connect-abc"):
			return shell.OK("ReplicaSet/kafka-connect-6d4f")
		case strings.HasPrefix(line, "get rs kafka-connect-6d4f"):
			return shell.OK("Deployment/kafka-connect")
		case strings.Contains(line, "jsonpath={.spec.replicas}"):
			return shell.OK("3")
		}
		return shell.Fail(1, "unexpected "+line)
	}}
	kind, name, reps, err := ScalableOwner(f, "/kc", "ctx", "shop", "kafka-connect-abc")
	if err != nil || kind != "deploy" || name != "kafka-connect" || reps != "3" {
		t.Fatalf("%s %s %s %v", kind, name, reps, err)
	}
}

func TestScalableOwnerStatefulSet(t *testing.T) {
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		line := strings.Join(r.Args, " ")
		if strings.HasPrefix(line, "get pod db-0") {
			return shell.OK("StatefulSet/db")
		}
		if strings.Contains(line, "jsonpath={.spec.replicas}") {
			return shell.OK("2")
		}
		return shell.Fail(1, line)
	}}
	kind, name, reps, err := ScalableOwner(f, "/kc", "ctx", "shop", "db-0")
	if err != nil || kind != "sts" || name != "db" || reps != "2" {
		t.Fatalf("%s %s %s %v", kind, name, reps, err)
	}
}

func TestScalableOwnerRejectsAJob(t *testing.T) {
	f := &shell.Fake{Handler: func(r shell.Request) shell.Result {
		return shell.OK("Job/pi")
	}}
	if _, _, _, err := ScalableOwner(f, "/kc", "ctx", "shop", "pi-x7"); err == nil || !strings.Contains(err.Error(), "not owned") {
		t.Fatal(err)
	}
}

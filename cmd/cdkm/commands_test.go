package main

import (
	"strings"
	"testing"

	"github.com/nguyenquangkhai/cdk-manager/internal/adapter"
	"github.com/nguyenquangkhai/cdk-manager/internal/config"
	"github.com/nguyenquangkhai/cdk-manager/internal/target"
)

func TestBuildCDKJobs(t *testing.T) {
	c := &config.Config{
		Accounts: map[string]config.Account{
			"prod-us": {Profile: "prod-us", Region: "us-east-1", Tags: []string{"prod"}},
		},
		Groups: map[string]config.Group{"prod": {Tags: []string{"prod"}}},
		Stacks: config.Stacks{Shared: []string{"AppStack"}},
	}
	tgts, _ := target.Resolve(c, target.Selector{Group: "prod"})
	jobs := buildCDKJobs(c, tgts, adapter.OpDeploy, nil, "never")

	if len(jobs) != 1 {
		t.Fatalf("got %d jobs", len(jobs))
	}
	got := strings.Join(jobs[0].Command.Args, " ")
	if !strings.Contains(got, "deploy") || !strings.Contains(got, "--output cdk.out/prod-us") || !strings.Contains(got, "AppStack") {
		t.Fatalf("argv = %v", jobs[0].Command.Args)
	}
}

func TestResolveVersionUsesLdflags(t *testing.T) {
	old := version
	defer func() { version = old }()
	version = "1.2.3"
	if got := resolveVersion(); got != "1.2.3" {
		t.Fatalf("resolveVersion() = %q, want 1.2.3 (ldflags value wins)", got)
	}
}

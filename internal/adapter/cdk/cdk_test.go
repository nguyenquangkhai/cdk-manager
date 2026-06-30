package cdk

import (
	"strings"
	"testing"

	"github.com/nguyenquangkhai/cdk-manager/internal/adapter"
	"github.com/nguyenquangkhai/cdk-manager/internal/target"
)

func tgt() target.Target {
	return target.Target{
		Name: "dev-eu", Profile: "dev-eu", Region: "eu-west-1",
		Context: map[string]string{"env": "dev"},
	}
}

func TestBuildDeployArgs(t *testing.T) {
	c := New()
	cmd := c.Build(tgt(), adapter.OpDeploy, []string{"AppStack"}, "never")

	if cmd.Name != "cdk" {
		t.Fatalf("name = %q", cmd.Name)
	}
	if cmd.Env["AWS_PROFILE"] != "dev-eu" || cmd.Env["AWS_REGION"] != "eu-west-1" {
		t.Fatalf("env = %v", cmd.Env)
	}
	got := strings.Join(cmd.Args, " ")
	for _, want := range []string{
		"deploy", "--output cdk.out/dev-eu", "--require-approval never",
		"-c env=dev", "AppStack",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("args %q missing %q", got, want)
		}
	}
}

func TestBuildDeployAllStacks(t *testing.T) {
	cmd := New().Build(tgt(), adapter.OpDeploy, nil, "never")
	if !strings.Contains(strings.Join(cmd.Args, " "), "--all") {
		t.Errorf("expected --all when no stacks, got %v", cmd.Args)
	}
}

func TestBuildDestroyForce(t *testing.T) {
	cmd := New().Build(tgt(), adapter.OpDestroy, []string{"AppStack"}, "")
	got := strings.Join(cmd.Args, " ")
	if !strings.Contains(got, "destroy") || !strings.Contains(got, "--force") {
		t.Errorf("destroy args = %v", cmd.Args)
	}
	if strings.Contains(got, "--require-approval") {
		t.Errorf("destroy must not pass --require-approval: %v", cmd.Args)
	}
}

func TestParseStatus(t *testing.T) {
	c := New()
	cases := map[string]adapter.State{
		"dev-eu: synthesizing CloudFormation template": adapter.StateSynth,
		"AppStack: deploying...":                        adapter.StateDeploy,
		"unrelated noise":                               "",
	}
	for line, want := range cases {
		got, ok := c.ParseStatus(line)
		if want == "" {
			if ok {
				t.Errorf("%q: expected no match", line)
			}
			continue
		}
		if !ok || got != want {
			t.Errorf("%q: got %q ok=%v want %q", line, got, ok, want)
		}
	}
}

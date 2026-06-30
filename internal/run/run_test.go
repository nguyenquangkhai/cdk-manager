package run

import (
	"reflect"
	"testing"

	"github.com/nguyenquangkhai/cdk-manager/internal/target"
)

func TestBuildCommandSubstitutes(t *testing.T) {
	tg := target.Target{
		Name: "dev-eu", Profile: "dev-eu", Region: "eu-west-1",
		Account: "111", Context: map[string]string{"env": "dev"},
	}
	cmd := BuildCommand(tg, ".cdkm/out/dev-eu",
		[]string{"terraform", "apply", "-var", "region={{region}}", "-var", "e={{context.env}}", "{{outdir}}"})

	want := []string{"apply", "-var", "region=eu-west-1", "-var", "e=dev", ".cdkm/out/dev-eu"}
	if cmd.Name != "terraform" {
		t.Fatalf("name = %q", cmd.Name)
	}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("args = %v want %v", cmd.Args, want)
	}
	if cmd.Env["AWS_PROFILE"] != "dev-eu" || cmd.Env["AWS_REGION"] != "eu-west-1" {
		t.Fatalf("env = %v", cmd.Env)
	}
}

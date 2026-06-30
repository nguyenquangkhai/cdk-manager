package cdk

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nguyenquangkhai/cdk-manager/internal/adapter"
	"github.com/nguyenquangkhai/cdk-manager/internal/target"
)

type CDK struct{}

func New() *CDK { return &CDK{} }

func (c *CDK) OutputDir(t target.Target) string {
	return "cdk.out/" + t.Name
}

func (c *CDK) Build(t target.Target, op adapter.Operation, stacks []string, requireApproval string) adapter.Command {
	args := []string{string(op)}

	// Unconditional --output (synth also takes --output)
	args = append(args, "--output", c.OutputDir(t))

	switch op {
	case adapter.OpDeploy:
		if requireApproval != "" {
			args = append(args, "--require-approval", requireApproval)
		}
	case adapter.OpDestroy:
		args = append(args, "--force")
	}

	// Deterministic context ordering for stable argv (testable).
	keys := make([]string, 0, len(t.Context))
	for k := range t.Context {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "-c", fmt.Sprintf("%s=%s", k, t.Context[k]))
	}

	if len(stacks) == 0 {
		args = append(args, "--all")
	} else {
		args = append(args, stacks...)
	}

	return adapter.Command{
		Name: "cdk",
		Args: args,
		Env: map[string]string{
			"AWS_PROFILE": t.Profile,
			"AWS_REGION":  t.Region,
		},
	}
}

func (c *CDK) ParseStatus(line string) (adapter.State, bool) {
	l := strings.ToLower(line)
	switch {
	case strings.Contains(l, "synthesiz"):
		return adapter.StateSynth, true
	case strings.Contains(l, "deploy"):
		return adapter.StateDeploy, true
	default:
		return "", false
	}
}

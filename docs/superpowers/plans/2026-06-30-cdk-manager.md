# cdkm — Multi-Account CDK Orchestrator Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A single Go binary `cdkm` that fans CDK operations (deploy/destroy/diff/synth) across many AWS accounts in parallel, with isolated `cdk.out` per account, group/tag targeting, safety gates, and a live status table.

**Architecture:** Layered. A CDK-agnostic core engine resolves targets from config and runs a per-target command in a bounded goroutine pool, injecting per-target env + template vars and capturing logs. A CDK adapter (first-class) builds `cdk` argv, isolates output, parses status, and enforces safety gates. A generic `run` passthrough is the escape hatch. We shell out to the `cdk` CLI (Approach A) — no AWS SDK in v1.

**Tech Stack:** Go 1.22+, cobra (CLI), yaml.v3 (config), bubbletea + lipgloss (UI), golang.org/x/sync (errgroup/semaphore). Tests use a fake `cdk` script on `PATH`.

## Global Constraints

- Module path: `github.com/nguyenquangkhai/cdk-manager` (copy verbatim into go.mod and all imports).
- Go version floor: **1.22**.
- License: **Apache-2.0**. SPDX header optional; LICENSE file required.
- Config file name: **`cdkm.yaml`**. State/log dir: **`.cdkm/`** in cwd.
- Default concurrency: **4**. Default result gate: **`--continue`**.
- No AWS SDK dependency in v1 — shell out to `cdk` only.
- Every command shells out using the binary named `cdk` resolved from `PATH` (tests override via a fake `cdk` earlier on `PATH`).
- Commit after every task. Conventional Commits style (`feat:`, `test:`, `chore:`, `docs:`, `ci:`).

---

### Task 1: Repo bootstrap + OSS scaffolding

**Files:**
- Create: `go.mod`
- Create: `LICENSE` (Apache-2.0 full text)
- Create: `.gitignore`
- Create: `README.md`
- Create: `CONTRIBUTING.md`
- Create: `CODE_OF_CONDUCT.md` (Contributor Covenant 2.1)
- Create: `SECURITY.md`
- Create: `CHANGELOG.md`
- Create: `.editorconfig`
- Create: `.golangci.yml`
- Create: `.github/ISSUE_TEMPLATE/bug_report.md`
- Create: `.github/ISSUE_TEMPLATE/feature_request.md`
- Create: `.github/PULL_REQUEST_TEMPLATE.md`
- Create: `.github/CODEOWNERS`
- Create: `cmd/cdkm/main.go` (minimal entrypoint that prints version)

**Interfaces:**
- Produces: a buildable module; `go build ./...` succeeds; `cdkm --version`-style print works (full cobra wiring lands in Task 9).

- [ ] **Step 1: Init git + module**

```bash
cd /Users/khainguyen/Projects/cdk-manager
git init
go mod init github.com/nguyenquangkhai/cdk-manager
```

- [ ] **Step 2: Write `.gitignore`**

```
# Build
/dist/
cdkm
*.exe

# Runtime
.cdkm/
cdk.out/

# Go
*.test
*.out
```

- [ ] **Step 3: Add LICENSE (Apache-2.0)**

Fetch the canonical text:

```bash
curl -fsSL https://www.apache.org/licenses/LICENSE-2.0.txt -o LICENSE
```

Expected: `LICENSE` exists, first line contains `Apache License`.

- [ ] **Step 4: Minimal entrypoint**

`cmd/cdkm/main.go`:

```go
package main

import "fmt"

// version is overridden at build time via -ldflags.
var version = "dev"

func main() {
	fmt.Printf("cdkm %s\n", version)
}
```

- [ ] **Step 5: Write README, CONTRIBUTING, CODE_OF_CONDUCT, SECURITY, CHANGELOG, .editorconfig, .golangci.yml, GitHub templates, CODEOWNERS**

`README.md` (skeleton — expand at Task 12):

```markdown
# cdkm

Fan AWS CDK operations across many accounts in parallel — from your laptop.

`cdkm deploy --group prod` synthesizes and deploys to every account in the
`prod` group concurrently, each in an isolated `cdk.out/<account>`, with a live
status table and safety gates on destroy.

## Status

Early development. See `docs/superpowers/specs/` for the design.

## License

Apache-2.0
```

`CONTRIBUTING.md`:

```markdown
# Contributing

## Build & test

    go build ./...
    go test ./...
    go vet ./...
    golangci-lint run

## Workflow

- Branch from `main`.
- Conventional Commits (`feat:`, `fix:`, `docs:`, ...).
- One logical change per PR; include tests.
```

`CODE_OF_CONDUCT.md`: paste Contributor Covenant 2.1 (https://www.contributor-covenant.org/version/2/1/code_of_conduct/), set contact to the maintainer email.

`SECURITY.md`:

```markdown
# Security Policy

Report vulnerabilities privately to the maintainer via GitHub Security Advisories
(Security tab → Report a vulnerability). Do not open public issues for security bugs.
```

`CHANGELOG.md`:

```markdown
# Changelog

All notable changes follow [Keep a Changelog](https://keepachangelog.com/) and SemVer.

## [Unreleased]
- Initial scaffolding.
```

`.editorconfig`:

```ini
root = true

[*]
charset = utf-8
end_of_line = lf
insert_final_newline = true
trim_trailing_whitespace = true

[*.go]
indent_style = tab

[*.{yml,yaml,md}]
indent_style = space
indent_size = 2
```

`.golangci.yml`:

```yaml
linters:
  enable:
    - gofmt
    - govet
    - revive
    - errcheck
    - staticcheck
    - ineffassign
    - unused
```

`.github/ISSUE_TEMPLATE/bug_report.md`, `feature_request.md`, `PULL_REQUEST_TEMPLATE.md`: standard short templates. `.github/CODEOWNERS`:

```
* @nguyenquangkhai
```

- [ ] **Step 6: Verify build**

Run: `go build ./... && go run ./cmd/cdkm`
Expected: prints `cdkm dev`.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "chore: bootstrap module and OSS scaffolding"
```

---

### Task 2: Config loading + validation

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`
- Create: `internal/config/testdata/valid.yaml`

**Interfaces:**
- Produces:
  ```go
  package config

  type Account struct {
      Profile string            `yaml:"profile"`
      Region  string            `yaml:"region"`
      Tags    []string          `yaml:"tags"`
      Context map[string]string `yaml:"context"`
  }
  type Group struct {
      Tags     []string `yaml:"tags"`
      Accounts []string `yaml:"accounts"`
  }
  type Stacks struct {
      Shared     []string            `yaml:"shared"`
      PerAccount map[string][]string `yaml:"perAccount"`
  }
  type Defaults struct {
      Concurrency     int    `yaml:"concurrency"`
      RequireApproval string `yaml:"requireApproval"`
  }
  type Config struct {
      Defaults Defaults           `yaml:"defaults"`
      Accounts map[string]Account `yaml:"accounts"`
      Groups   map[string]Group   `yaml:"groups"`
      Stacks   Stacks             `yaml:"stacks"`
  }

  func Load(path string) (*Config, error) // reads + unmarshals + Validate
  func (c *Config) Validate() error
  ```
- Validation: each group resolves to ≥1 known account; every `perAccount` key is a known account; `Defaults.Concurrency` defaults to 4 if 0.

- [ ] **Step 1: Write the failing test**

`internal/config/config_test.go`:

```go
package config

import "testing"

func TestLoadValid(t *testing.T) {
	c, err := Load("testdata/valid.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Defaults.Concurrency != 4 {
		t.Errorf("concurrency = %d, want 4 (default applied)", c.Defaults.Concurrency)
	}
	if _, ok := c.Accounts["dev-eu"]; !ok {
		t.Errorf("missing account dev-eu")
	}
	if got := c.Accounts["dev-eu"].Context["env"]; got != "dev" {
		t.Errorf("dev-eu context env = %q, want dev", got)
	}
}

func TestValidateUnknownPerAccount(t *testing.T) {
	c := &Config{
		Accounts: map[string]Account{"a": {Profile: "a", Region: "r"}},
		Stacks:   Stacks{PerAccount: map[string][]string{"ghost": {"S"}}},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for unknown perAccount key, got nil")
	}
}

func TestValidateEmptyGroup(t *testing.T) {
	c := &Config{
		Accounts: map[string]Account{"a": {Profile: "a", Region: "r", Tags: []string{"dev"}}},
		Groups:   map[string]Group{"prod": {Tags: []string{"prod"}}},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for group resolving to zero accounts, got nil")
	}
}
```

`internal/config/testdata/valid.yaml`:

```yaml
defaults:
  requireApproval: never
accounts:
  dev-eu:
    profile: dev-eu
    region: eu-west-1
    tags: [dev, eu]
    context: { env: dev }
  prod-us:
    profile: prod-us
    region: us-east-1
    tags: [prod, us]
    context: { env: prod }
groups:
  prod: { tags: [prod] }
  core: { accounts: [dev-eu, prod-us] }
stacks:
  shared: [NetworkStack, AppStack]
  perAccount:
    prod-us: [ProdOnlyStack]
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/...`
Expected: FAIL — package/functions undefined.

- [ ] **Step 3: Write minimal implementation**

`internal/config/config.go`:

```go
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Account struct {
	Profile string            `yaml:"profile"`
	Region  string            `yaml:"region"`
	Tags    []string          `yaml:"tags"`
	Context map[string]string `yaml:"context"`
}

type Group struct {
	Tags     []string `yaml:"tags"`
	Accounts []string `yaml:"accounts"`
}

type Stacks struct {
	Shared     []string            `yaml:"shared"`
	PerAccount map[string][]string `yaml:"perAccount"`
}

type Defaults struct {
	Concurrency     int    `yaml:"concurrency"`
	RequireApproval string `yaml:"requireApproval"`
}

type Config struct {
	Defaults Defaults           `yaml:"defaults"`
	Accounts map[string]Account `yaml:"accounts"`
	Groups   map[string]Group   `yaml:"groups"`
	Stacks   Stacks             `yaml:"stacks"`
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if c.Defaults.Concurrency == 0 {
		c.Defaults.Concurrency = 4
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) Validate() error {
	if len(c.Accounts) == 0 {
		return fmt.Errorf("config has no accounts")
	}
	for name := range c.Stacks.PerAccount {
		if _, ok := c.Accounts[name]; !ok {
			return fmt.Errorf("stacks.perAccount references unknown account %q", name)
		}
	}
	for gname, g := range c.Groups {
		n := 0
		for _, a := range g.Accounts {
			if _, ok := c.Accounts[a]; !ok {
				return fmt.Errorf("group %q references unknown account %q", gname, a)
			}
			n++
		}
		if len(g.Tags) > 0 {
			for _, acc := range c.Accounts {
				if hasAnyTag(acc.Tags, g.Tags) {
					n++
				}
			}
		}
		if n == 0 {
			return fmt.Errorf("group %q resolves to zero accounts", gname)
		}
	}
	return nil
}

func hasAnyTag(have, want []string) bool {
	set := make(map[string]struct{}, len(have))
	for _, t := range have {
		set[t] = struct{}{}
	}
	for _, w := range want {
		if _, ok := set[w]; ok {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Add dep + run tests**

Run: `go get gopkg.in/yaml.v3 && go test ./internal/config/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config go.mod go.sum
git commit -m "feat: config loading and validation"
```

---

### Task 3: Target model + selector resolution

**Files:**
- Create: `internal/target/target.go`
- Test: `internal/target/target_test.go`

**Interfaces:**
- Consumes: `config.Config`, `config.Account`, `config.Stacks`.
- Produces:
  ```go
  package target

  type Target struct {
      Name    string            // account key, e.g. "dev-eu"
      Profile string
      Region  string
      Account string            // AWS account id if present in context["account"], else ""
      Context map[string]string // copied from account.Context
  }

  type Selector struct {
      All     bool
      Group   string
      Account string
      Tag     string
  }

  // Resolve returns targets sorted by Name. Error if selector matches nothing
  // or names an unknown group/account.
  func Resolve(c *config.Config, s Selector) ([]Target, error)

  // Stacks returns the effective stack list for a target: shared + perAccount[name].
  // Empty slice means "all stacks" (caller maps to cdk --all).
  func Stacks(c *config.Config, t Target) []string
  ```

- [ ] **Step 1: Write the failing test**

`internal/target/target_test.go`:

```go
package target

import (
	"testing"

	"github.com/nguyenquangkhai/cdk-manager/internal/config"
)

func fixture() *config.Config {
	return &config.Config{
		Accounts: map[string]config.Account{
			"dev-eu":  {Profile: "dev-eu", Region: "eu-west-1", Tags: []string{"dev", "eu"}, Context: map[string]string{"env": "dev"}},
			"prod-us": {Profile: "prod-us", Region: "us-east-1", Tags: []string{"prod", "us"}},
		},
		Groups: map[string]config.Group{
			"prod": {Tags: []string{"prod"}},
			"core": {Accounts: []string{"dev-eu", "prod-us"}},
		},
		Stacks: config.Stacks{
			Shared:     []string{"NetworkStack", "AppStack"},
			PerAccount: map[string][]string{"prod-us": {"ProdOnlyStack"}},
		},
	}
}

func TestResolveAll(t *testing.T) {
	got, err := Resolve(fixture(), Selector{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "dev-eu" || got[1].Name != "prod-us" {
		t.Fatalf("got %+v", got)
	}
}

func TestResolveByTag(t *testing.T) {
	got, _ := Resolve(fixture(), Selector{Tag: "prod"})
	if len(got) != 1 || got[0].Name != "prod-us" {
		t.Fatalf("got %+v", got)
	}
}

func TestResolveByGroupAccounts(t *testing.T) {
	got, _ := Resolve(fixture(), Selector{Group: "core"})
	if len(got) != 2 {
		t.Fatalf("got %+v", got)
	}
}

func TestResolveUnknownGroup(t *testing.T) {
	if _, err := Resolve(fixture(), Selector{Group: "ghost"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestStacksMerge(t *testing.T) {
	c := fixture()
	prod := Target{Name: "prod-us"}
	got := Stacks(c, prod)
	want := []string{"NetworkStack", "AppStack", "ProdOnlyStack"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/target/...`
Expected: FAIL — undefined.

- [ ] **Step 3: Write minimal implementation**

`internal/target/target.go`:

```go
package target

import (
	"fmt"
	"sort"

	"github.com/nguyenquangkhai/cdk-manager/internal/config"
)

type Target struct {
	Name    string
	Profile string
	Region  string
	Account string
	Context map[string]string
}

type Selector struct {
	All     bool
	Group   string
	Account string
	Tag     string
}

func newTarget(name string, a config.Account) Target {
	return Target{
		Name:    name,
		Profile: a.Profile,
		Region:  a.Region,
		Account: a.Context["account"],
		Context: a.Context,
	}
}

func Resolve(c *config.Config, s Selector) ([]Target, error) {
	names := map[string]struct{}{}
	switch {
	case s.All:
		for n := range c.Accounts {
			names[n] = struct{}{}
		}
	case s.Account != "":
		if _, ok := c.Accounts[s.Account]; !ok {
			return nil, fmt.Errorf("unknown account %q", s.Account)
		}
		names[s.Account] = struct{}{}
	case s.Tag != "":
		for n, a := range c.Accounts {
			for _, t := range a.Tags {
				if t == s.Tag {
					names[n] = struct{}{}
				}
			}
		}
	case s.Group != "":
		g, ok := c.Groups[s.Group]
		if !ok {
			return nil, fmt.Errorf("unknown group %q", s.Group)
		}
		for _, n := range g.Accounts {
			names[n] = struct{}{}
		}
		for n, a := range c.Accounts {
			for _, gt := range g.Tags {
				for _, at := range a.Tags {
					if gt == at {
						names[n] = struct{}{}
					}
				}
			}
		}
	default:
		return nil, fmt.Errorf("no selector provided (use --all/--group/--account/--tag)")
	}

	if len(names) == 0 {
		return nil, fmt.Errorf("selector matched zero accounts")
	}
	out := make([]Target, 0, len(names))
	for n := range names {
		out = append(out, newTarget(n, c.Accounts[n]))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func Stacks(c *config.Config, t Target) []string {
	out := append([]string{}, c.Stacks.Shared...)
	out = append(out, c.Stacks.PerAccount[t.Name]...)
	return out
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/target/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/target
git commit -m "feat: target model and selector resolution"
```

---

### Task 4: Adapter interface + CDK argv builder

**Files:**
- Create: `internal/adapter/adapter.go`
- Create: `internal/adapter/cdk/cdk.go`
- Test: `internal/adapter/cdk/cdk_test.go`

**Interfaces:**
- Consumes: `target.Target`.
- Produces:
  ```go
  package adapter

  type Operation string
  const (
      OpDeploy  Operation = "deploy"
      OpDestroy Operation = "destroy"
      OpDiff    Operation = "diff"
      OpSynth   Operation = "synth"
  )

  type Command struct {
      Name string            // binary, e.g. "cdk"
      Args []string          // argv after binary
      Env  map[string]string // extra env (AWS_PROFILE, AWS_REGION)
      Dir  string            // working dir ("" = inherit)
  }

  type State string
  const (
      StatePending State = "pending"
      StateRunning State = "running"
      StateSynth   State = "synth"
      StateDeploy  State = "deploy"
      StateDone    State = "done"
      StateFailed  State = "failed"
  )

  type Adapter interface {
      Build(t target.Target, op Operation, stacks []string, requireApproval string) Command
      OutputDir(t target.Target) string
      ParseStatus(line string) (State, bool) // ok=false if line maps to no state
  }
  ```
  CDK adapter (`cdk.New()`):
  - `OutputDir` → `cdk.out/<target.Name>`.
  - `Build` argv: `cdk <op> [--output cdk.out/<name>] [--require-approval V (deploy only)] [--force (destroy)] [-c k=v ...] [stacks... | --all]`, with `--profile` and region via env.

- [ ] **Step 1: Write the failing test**

`internal/adapter/cdk/cdk_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapter/...`
Expected: FAIL — undefined.

- [ ] **Step 3: Write minimal implementation**

`internal/adapter/adapter.go`:

```go
package adapter

import "github.com/nguyenquangkhai/cdk-manager/internal/target"

type Operation string

const (
	OpDeploy  Operation = "deploy"
	OpDestroy Operation = "destroy"
	OpDiff    Operation = "diff"
	OpSynth   Operation = "synth"
)

type Command struct {
	Name string
	Args []string
	Env  map[string]string
	Dir  string
}

type State string

const (
	StatePending State = "pending"
	StateRunning State = "running"
	StateSynth   State = "synth"
	StateDeploy  State = "deploy"
	StateDone    State = "done"
	StateFailed  State = "failed"
)

type Adapter interface {
	Build(t target.Target, op Operation, stacks []string, requireApproval string) Command
	OutputDir(t target.Target) string
	ParseStatus(line string) (State, bool)
}
```

`internal/adapter/cdk/cdk.go`:

```go
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

	if op != adapter.OpSynth {
		args = append(args, "--output", c.OutputDir(t))
	} else {
		args = append(args, "--output", c.OutputDir(t))
	}

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
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/adapter/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapter
git commit -m "feat: adapter interface and CDK argv builder"
```

---

### Task 5: Engine — concurrency pool, process spawn, log capture

**Files:**
- Create: `internal/engine/engine.go`
- Test: `internal/engine/engine_test.go`
- Create: `internal/engine/testdata/fakecdk.sh`

**Interfaces:**
- Consumes: `adapter.Command`, `adapter.State`, `target.Target`.
- Produces:
  ```go
  package engine

  type Result struct {
      Target   string
      State    adapter.State // StateDone or StateFailed
      ExitCode int
      Err      error
      LogPath  string
      Elapsed  time.Duration
  }

  type Update struct {
      Target string
      State  adapter.State
      Line   string // last meaningful log line
  }

  type Job struct {
      Target  target.Target
      Command adapter.Command
  }

  type Options struct {
      Concurrency int
      FailFast    bool
      LogDir      string             // e.g. ".cdkm/logs"
      DryRun      bool               // if true, do not exec; emit planned argv
      OnUpdate    func(Update)       // nil-safe progress callback
      Parse       func(string) (adapter.State, bool)
  }

  // Run executes all jobs respecting Options. Returns one Result per job.
  func Run(ctx context.Context, jobs []Job, opts Options) []Result
  ```
- Behavior: bounded by `Concurrency` (semaphore). Each job's stdout+stderr streamed to `LogDir/<target>.log`; each line passed to `opts.Parse`; on a recognized state, `opts.OnUpdate` fired. `FailFast` cancels the context after the first failure so pending jobs don't start. `DryRun` writes the planned command to the log and returns `StateDone` without exec.

- [ ] **Step 1: Write the fake cdk + failing test**

`internal/engine/testdata/fakecdk.sh`:

```bash
#!/usr/bin/env bash
# Fake cdk for tests. Behavior controlled by env FAKE_MODE.
echo "synthesizing CloudFormation template"
echo "deploying $*"
if [ "$FAKE_MODE" = "fail" ]; then
  echo "error: boom" >&2
  exit 1
fi
echo "done"
exit 0
```

`internal/engine/engine_test.go`:

```go
package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nguyenquangkhai/cdk-manager/internal/adapter"
	"github.com/nguyenquangkhai/cdk-manager/internal/target"
)

func fakeJob(name, mode string) Job {
	abs, _ := filepath.Abs("testdata/fakecdk.sh")
	return Job{
		Target: target.Target{Name: name},
		Command: adapter.Command{
			Name: "bash",
			Args: []string{abs, "AppStack"},
			Env:  map[string]string{"FAKE_MODE": mode},
		},
	}
}

func parse(line string) (adapter.State, bool) {
	l := strings.ToLower(line)
	if strings.Contains(l, "synthesiz") {
		return adapter.StateSynth, true
	}
	if strings.Contains(l, "deploy") {
		return adapter.StateDeploy, true
	}
	return "", false
}

func TestRunSuccess(t *testing.T) {
	dir := t.TempDir()
	res := Run(context.Background(),
		[]Job{fakeJob("a", "ok"), fakeJob("b", "ok")},
		Options{Concurrency: 2, LogDir: dir, Parse: parse})

	if len(res) != 2 {
		t.Fatalf("got %d results", len(res))
	}
	for _, r := range res {
		if r.State != adapter.StateDone {
			t.Errorf("%s state = %s, want done", r.Target, r.State)
		}
		b, _ := os.ReadFile(filepath.Join(dir, r.Target+".log"))
		if !strings.Contains(string(b), "synthesizing") {
			t.Errorf("%s log missing output: %s", r.Target, b)
		}
	}
}

func TestRunFailureReported(t *testing.T) {
	dir := t.TempDir()
	res := Run(context.Background(),
		[]Job{fakeJob("a", "fail")},
		Options{Concurrency: 1, LogDir: dir, Parse: parse})
	if res[0].State != adapter.StateFailed || res[0].ExitCode != 1 {
		t.Fatalf("got %+v, want failed exit 1", res[0])
	}
}

func TestDryRunNoExec(t *testing.T) {
	dir := t.TempDir()
	res := Run(context.Background(),
		[]Job{fakeJob("a", "fail")}, // would fail if executed
		Options{Concurrency: 1, LogDir: dir, DryRun: true, Parse: parse})
	if res[0].State != adapter.StateDone {
		t.Fatalf("dry-run should report done, got %s", res[0].State)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "a.log"))
	if !strings.Contains(string(b), "DRY-RUN") {
		t.Errorf("dry-run log missing plan: %s", b)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `chmod +x internal/engine/testdata/fakecdk.sh && go test ./internal/engine/...`
Expected: FAIL — undefined.

- [ ] **Step 3: Write minimal implementation**

`internal/engine/engine.go`:

```go
package engine

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nguyenquangkhai/cdk-manager/internal/adapter"
	"github.com/nguyenquangkhai/cdk-manager/internal/target"
)

type Result struct {
	Target   string
	State    adapter.State
	ExitCode int
	Err      error
	LogPath  string
	Elapsed  time.Duration
}

type Update struct {
	Target string
	State  adapter.State
	Line   string
}

type Job struct {
	Target  target.Target
	Command adapter.Command
}

type Options struct {
	Concurrency int
	FailFast    bool
	LogDir      string
	DryRun      bool
	OnUpdate    func(Update)
	Parse       func(string) (adapter.State, bool)
}

func Run(ctx context.Context, jobs []Job, opts Options) []Result {
	if opts.Concurrency < 1 {
		opts.Concurrency = 1
	}
	if opts.Parse == nil {
		opts.Parse = func(string) (adapter.State, bool) { return "", false }
	}
	_ = os.MkdirAll(opts.LogDir, 0o755)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sem := make(chan struct{}, opts.Concurrency)
	results := make([]Result, len(jobs))
	var wg sync.WaitGroup

	for i, job := range jobs {
		wg.Add(1)
		go func(i int, job Job) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[i] = Result{Target: job.Target.Name, State: adapter.StateFailed, Err: ctx.Err()}
				return
			}
			defer func() { <-sem }()

			r := runOne(ctx, job, opts)
			results[i] = r
			if r.State == adapter.StateFailed && opts.FailFast {
				cancel()
			}
		}(i, job)
	}
	wg.Wait()
	return results
}

func runOne(ctx context.Context, job Job, opts Options) Result {
	start := time.Now()
	logPath := filepath.Join(opts.LogDir, job.Target.Name+".log")
	lf, err := os.Create(logPath)
	if err != nil {
		return Result{Target: job.Target.Name, State: adapter.StateFailed, Err: err, LogPath: logPath}
	}
	defer lf.Close()

	emit := func(state adapter.State, line string) {
		if opts.OnUpdate != nil {
			opts.OnUpdate(Update{Target: job.Target.Name, State: state, Line: line})
		}
	}

	if opts.DryRun {
		plan := fmt.Sprintf("DRY-RUN %s %s\n", job.Command.Name, strings.Join(job.Command.Args, " "))
		_, _ = lf.WriteString(plan)
		emit(adapter.StateDone, "dry-run")
		return Result{Target: job.Target.Name, State: adapter.StateDone, LogPath: logPath, Elapsed: time.Since(start)}
	}

	cmd := exec.CommandContext(ctx, job.Command.Name, job.Command.Args...)
	cmd.Dir = job.Command.Dir
	cmd.Env = os.Environ()
	for k, v := range job.Command.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	emit(adapter.StateRunning, "")
	if err := cmd.Start(); err != nil {
		return Result{Target: job.Target.Name, State: adapter.StateFailed, Err: err, LogPath: logPath, Elapsed: time.Since(start)}
	}

	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		sc := bufio.NewScanner(pr)
		for sc.Scan() {
			line := sc.Text()
			_, _ = lf.WriteString(line + "\n")
			if st, ok := opts.Parse(line); ok {
				emit(st, line)
			}
		}
	}()

	runErr := cmd.Wait()
	_ = pw.Close()
	<-scanDone

	res := Result{Target: job.Target.Name, LogPath: logPath, Elapsed: time.Since(start)}
	if runErr != nil {
		res.State = adapter.StateFailed
		res.Err = runErr
		if ee, ok := runErr.(*exec.ExitError); ok {
			res.ExitCode = ee.ExitCode()
		} else {
			res.ExitCode = -1
		}
		emit(adapter.StateFailed, runErr.Error())
		return res
	}
	res.State = adapter.StateDone
	emit(adapter.StateDone, "done")
	return res
}
```

Add the missing import `"io"` to the import block.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/engine/...`
Expected: PASS (success, failure, dry-run).

- [ ] **Step 5: Add fail-fast test + verify**

Append to `engine_test.go`:

```go
func TestFailFastCancelsPending(t *testing.T) {
	dir := t.TempDir()
	jobs := []Job{fakeJob("a", "fail"), fakeJob("b", "ok"), fakeJob("c", "ok")}
	res := Run(context.Background(), jobs,
		Options{Concurrency: 1, FailFast: true, LogDir: dir, Parse: parse})
	failed := 0
	for _, r := range res {
		if r.State == adapter.StateFailed {
			failed++
		}
	}
	if failed == 0 {
		t.Fatal("expected at least one failure recorded")
	}
}
```

Run: `go test ./internal/engine/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/engine
git commit -m "feat: concurrency engine with log capture and dry-run"
```

---

### Task 6: Report — summary table + exit code + state file

**Files:**
- Create: `internal/report/report.go`
- Test: `internal/report/report_test.go`

**Interfaces:**
- Consumes: `engine.Result`, `adapter.State`.
- Produces:
  ```go
  package report

  // Summarize renders a plain-text table of results to w and returns the
  // process exit code: 0 if all done, 1 if any failed.
  func Summarize(w io.Writer, results []engine.Result) int

  // SaveState writes results as JSON to path (e.g. ".cdkm/state.json").
  func SaveState(path string, results []engine.Result) error
  ```

- [ ] **Step 1: Write the failing test**

`internal/report/report_test.go`:

```go
package report

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/nguyenquangkhai/cdk-manager/internal/adapter"
	"github.com/nguyenquangkhai/cdk-manager/internal/engine"
)

func TestSummarizeExitCode(t *testing.T) {
	var buf bytes.Buffer
	code := Summarize(&buf, []engine.Result{
		{Target: "a", State: adapter.StateDone, Elapsed: time.Second},
		{Target: "b", State: adapter.StateFailed, ExitCode: 1, Elapsed: 2 * time.Second},
	})
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	out := buf.String()
	if !strings.Contains(out, "a") || !strings.Contains(out, "b") {
		t.Errorf("summary missing targets: %s", out)
	}
	if !strings.Contains(out, "failed") {
		t.Errorf("summary missing failed marker: %s", out)
	}
}

func TestSummarizeAllPass(t *testing.T) {
	var buf bytes.Buffer
	code := Summarize(&buf, []engine.Result{{Target: "a", State: adapter.StateDone}})
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/report/...`
Expected: FAIL — undefined.

- [ ] **Step 3: Write minimal implementation**

`internal/report/report.go`:

```go
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/nguyenquangkhai/cdk-manager/internal/adapter"
	"github.com/nguyenquangkhai/cdk-manager/internal/engine"
)

func Summarize(w io.Writer, results []engine.Result) int {
	code := 0
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "TARGET\tSTATE\tELAPSED\tEXIT")
	for _, r := range results {
		if r.State == adapter.StateFailed {
			code = 1
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\n", r.Target, r.State, r.Elapsed.Round(1e6), r.ExitCode)
	}
	tw.Flush()
	return code
}

func SaveState(path string, results []engine.Result) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/report/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/report
git commit -m "feat: result summary and state persistence"
```

---

### Task 7: Safety gates — typed destroy confirmation

**Files:**
- Create: `internal/safety/safety.go`
- Test: `internal/safety/safety_test.go`

**Interfaces:**
- Consumes: `target.Target`, `adapter.Operation`.
- Produces:
  ```go
  package safety

  // ConfirmDestroy prompts via r/w. The user must type selectorLabel exactly.
  // Returns nil if confirmed, error otherwise. Skipped (nil) for non-destroy ops.
  func ConfirmDestroy(r io.Reader, w io.Writer, op adapter.Operation, selectorLabel string, targets []target.Target) error
  ```

- [ ] **Step 1: Write the failing test**

`internal/safety/safety_test.go`:

```go
package safety

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nguyenquangkhai/cdk-manager/internal/adapter"
	"github.com/nguyenquangkhai/cdk-manager/internal/target"
)

func tgts() []target.Target { return []target.Target{{Name: "prod-us"}} }

func TestConfirmDestroyMatch(t *testing.T) {
	var out bytes.Buffer
	err := ConfirmDestroy(strings.NewReader("prod\n"), &out, adapter.OpDestroy, "prod", tgts())
	if err != nil {
		t.Fatalf("expected confirm, got %v", err)
	}
}

func TestConfirmDestroyMismatch(t *testing.T) {
	var out bytes.Buffer
	err := ConfirmDestroy(strings.NewReader("nope\n"), &out, adapter.OpDestroy, "prod", tgts())
	if err == nil {
		t.Fatal("expected error on mismatch")
	}
}

func TestConfirmSkippedForDeploy(t *testing.T) {
	var out bytes.Buffer
	if err := ConfirmDestroy(strings.NewReader(""), &out, adapter.OpDeploy, "prod", tgts()); err != nil {
		t.Fatalf("deploy should not require confirm: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/safety/...`
Expected: FAIL — undefined.

- [ ] **Step 3: Write minimal implementation**

`internal/safety/safety.go`:

```go
package safety

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/nguyenquangkhai/cdk-manager/internal/adapter"
	"github.com/nguyenquangkhai/cdk-manager/internal/target"
)

func ConfirmDestroy(r io.Reader, w io.Writer, op adapter.Operation, selectorLabel string, targets []target.Target) error {
	if op != adapter.OpDestroy {
		return nil
	}
	fmt.Fprintf(w, "About to DESTROY %d target(s):\n", len(targets))
	for _, t := range targets {
		fmt.Fprintf(w, "  - %s (%s / %s)\n", t.Name, t.Profile, t.Region)
	}
	fmt.Fprintf(w, "Type %q to confirm: ", selectorLabel)

	sc := bufio.NewScanner(r)
	sc.Scan()
	got := strings.TrimSpace(sc.Text())
	if got != selectorLabel {
		return fmt.Errorf("confirmation %q did not match %q; aborting", got, selectorLabel)
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/safety/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/safety
git commit -m "feat: typed destroy confirmation gate"
```

---

### Task 8: Template substitution for generic `run`

**Files:**
- Create: `internal/run/run.go`
- Test: `internal/run/run_test.go`

**Interfaces:**
- Consumes: `target.Target`, `adapter.Command`.
- Produces:
  ```go
  package run

  // BuildCommand templates argv tokens for a target. Supported vars:
  // {{profile}} {{region}} {{account}} {{target}} {{outdir}} and {{context.KEY}}.
  // outDir is the per-target generic isolation dir (e.g. ".cdkm/out/<name>").
  func BuildCommand(t target.Target, outDir string, argv []string) adapter.Command
  ```

- [ ] **Step 1: Write the failing test**

`internal/run/run_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/run/...`
Expected: FAIL — undefined.

- [ ] **Step 3: Write minimal implementation**

`internal/run/run.go`:

```go
package run

import (
	"strings"

	"github.com/nguyenquangkhai/cdk-manager/internal/adapter"
	"github.com/nguyenquangkhai/cdk-manager/internal/target"
)

func BuildCommand(t target.Target, outDir string, argv []string) adapter.Command {
	repl := map[string]string{
		"{{profile}}": t.Profile,
		"{{region}}":  t.Region,
		"{{account}}": t.Account,
		"{{target}}":  t.Name,
		"{{outdir}}":  outDir,
	}
	for k, v := range t.Context {
		repl["{{context."+k+"}}"] = v
	}
	subst := func(s string) string {
		for k, v := range repl {
			s = strings.ReplaceAll(s, k, v)
		}
		return s
	}

	out := make([]string, len(argv))
	for i, a := range argv {
		out[i] = subst(a)
	}
	return adapter.Command{
		Name: out[0],
		Args: out[1:],
		Env: map[string]string{
			"AWS_PROFILE": t.Profile,
			"AWS_REGION":  t.Region,
		},
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/run/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/run
git commit -m "feat: template substitution for generic run"
```

---

### Task 9: UI — bubbletea live table + non-TTY fallback

**Files:**
- Create: `internal/ui/ui.go`
- Test: `internal/ui/ui_test.go`

**Interfaces:**
- Consumes: `engine.Update`, `engine.Result`, `adapter.State`.
- Produces:
  ```go
  package ui

  // Reporter receives progress updates during a run. Two implementations:
  // a bubbletea TUI (TTY) and a plain prefixed-line logger (non-TTY).
  type Reporter interface {
      Update(u engine.Update)
      Done(results []engine.Result)
  }

  // New returns a TUI reporter if w is a terminal, else a plain reporter.
  func New(w io.Writer, isTTY bool) Reporter

  // PlainReporter is exported for testing.
  type PlainReporter struct{ /* writer + mutex */ }
  ```
- v1 keeps the TUI minimal; the **PlainReporter is the tested path** (deterministic). The bubbletea model wraps the same state; if it proves heavy, the plain reporter is always the fallback.

- [ ] **Step 1: Write the failing test (plain reporter)**

`internal/ui/ui_test.go`:

```go
package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nguyenquangkhai/cdk-manager/internal/adapter"
	"github.com/nguyenquangkhai/cdk-manager/internal/engine"
)

func TestPlainReporter(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false) // non-TTY -> plain
	r.Update(engine.Update{Target: "dev-eu", State: adapter.StateSynth, Line: "synthesizing"})
	r.Update(engine.Update{Target: "dev-eu", State: adapter.StateDeploy, Line: "deploying"})
	r.Done([]engine.Result{{Target: "dev-eu", State: adapter.StateDone}})

	out := buf.String()
	if !strings.Contains(out, "[dev-eu]") {
		t.Errorf("missing target prefix: %s", out)
	}
	if !strings.Contains(out, "synthesizing") {
		t.Errorf("missing update line: %s", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/...`
Expected: FAIL — undefined.

- [ ] **Step 3: Write minimal implementation**

`internal/ui/ui.go`:

```go
package ui

import (
	"fmt"
	"io"
	"sync"

	"github.com/nguyenquangkhai/cdk-manager/internal/engine"
)

type Reporter interface {
	Update(u engine.Update)
	Done(results []engine.Result)
}

type PlainReporter struct {
	mu sync.Mutex
	w  io.Writer
}

func (p *PlainReporter) Update(u engine.Update) {
	p.mu.Lock()
	defer p.mu.Unlock()
	line := u.Line
	if line == "" {
		line = string(u.State)
	}
	fmt.Fprintf(p.w, "[%s] %s\n", u.Target, line)
}

func (p *PlainReporter) Done(results []engine.Result) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, r := range results {
		fmt.Fprintf(p.w, "[%s] %s\n", r.Target, r.State)
	}
}

// New returns a reporter. v1: always PlainReporter; the bubbletea TUI is wired
// in cmd when isTTY is true (see Task 10). Kept simple + testable here.
func New(w io.Writer, isTTY bool) Reporter {
	return &PlainReporter{w: w}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/ui/...`
Expected: PASS.

- [ ] **Step 5: Add bubbletea TUI model (no unit test; smoke only)**

Create `internal/ui/tui.go` with a bubbletea `Model` holding `map[string]engine.Update`, rendering a lipgloss table on each `Update` message, ending on `Done`. Wire it in Task 10 only when `isTTY`. Add deps:

```bash
go get github.com/charmbracelet/bubbletea github.com/charmbracelet/lipgloss
```

Keep `New` returning `PlainReporter` until the TUI model compiles and renders; switch `New` to return the TUI reporter when `isTTY` is true once verified by manual smoke (`go run ./cmd/cdkm deploy ...` against the fake cdk).

- [ ] **Step 6: Commit**

```bash
git add internal/ui go.mod go.sum
git commit -m "feat: progress reporter with plain and TUI modes"
```

---

### Task 10: CLI wiring (cobra) — deploy/destroy/diff/synth/run/list/status

**Files:**
- Modify: `cmd/cdkm/main.go`
- Create: `cmd/cdkm/root.go`
- Create: `cmd/cdkm/commands.go`
- Test: `cmd/cdkm/commands_test.go`

**Interfaces:**
- Consumes: every package above — `config.Load`, `target.Resolve`/`Stacks`, `cdk.New`, `engine.Run`, `report.Summarize`/`SaveState`, `safety.ConfirmDestroy`, `run.BuildCommand`, `ui.New`.
- Produces: the `cdkm` command tree. Each cdk op resolves targets → builds jobs via the CDK adapter → confirms (destroy) → runs the engine → summarizes → exits with the engine's code.

- [ ] **Step 1: Write the failing test (target→job assembly)**

Factor job assembly into a testable function. `cmd/cdkm/commands_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/cdkm/...`
Expected: FAIL — `buildCDKJobs` undefined.

- [ ] **Step 3: Implement command tree + helpers**

`cmd/cdkm/commands.go`:

```go
package main

import (
	"github.com/nguyenquangkhai/cdk-manager/internal/adapter"
	"github.com/nguyenquangkhai/cdk-manager/internal/adapter/cdk"
	"github.com/nguyenquangkhai/cdk-manager/internal/config"
	"github.com/nguyenquangkhai/cdk-manager/internal/engine"
	"github.com/nguyenquangkhai/cdk-manager/internal/target"
)

func buildCDKJobs(c *config.Config, tgts []target.Target, op adapter.Operation, cliStacks []string, requireApproval string) []engine.Job {
	a := cdk.New()
	jobs := make([]engine.Job, 0, len(tgts))
	for _, t := range tgts {
		stacks := cliStacks
		if len(stacks) == 0 {
			stacks = target.Stacks(c, t)
		}
		jobs = append(jobs, engine.Job{Target: t, Command: a.Build(t, op, stacks, requireApproval)})
	}
	return jobs
}
```

`cmd/cdkm/root.go`: build the cobra root with a persistent `--config` (default `cdkm.yaml`), the selector flags (`--all/--group/--account/--tag`), and shared run flags (`--concurrency/--dry-run/--fail-fast/--require-approval`). Add subcommands `deploy/destroy/diff/synth/run/list/status`. Each cdk subcommand:

```go
// pseudocode body shared by deploy/destroy/diff/synth
cfg := mustLoad(configPath)
sel := selectorFromFlags()
tgts := must(target.Resolve(cfg, sel))
if op == adapter.OpDestroy {
    label := selectorLabel(sel) // group/account/tag/"all"
    if err := safety.ConfirmDestroy(os.Stdin, os.Stdout, op, label, tgts); err != nil {
        fatal(err)
    }
}
jobs := buildCDKJobs(cfg, tgts, op, cliStacks, requireApproval(cfg))
rep := ui.New(os.Stdout, isatty(os.Stdout))
results := engine.Run(ctx, jobs, engine.Options{
    Concurrency: concurrency(cfg), FailFast: failFast, DryRun: dryRun,
    LogDir: ".cdkm/logs", Parse: cdk.New().ParseStatus,
    OnUpdate: rep.Update,
})
rep.Done(results)
_ = report.SaveState(".cdkm/state.json", results)
os.Exit(report.Summarize(os.Stdout, results))
```

`list` prints resolved targets + effective stacks (no exec). `status` reads `.cdkm/state.json` and prints it. `run` resolves targets and builds jobs via `run.BuildCommand(t, ".cdkm/out/"+t.Name, argv)` then runs the engine with `Parse=nil`.

`cmd/cdkm/main.go` becomes:

```go
package main

func main() { Execute() } // Execute defined in root.go
```

Add cobra:

```bash
go get github.com/spf13/cobra
```

- [ ] **Step 4: Run unit test + build**

Run: `go test ./cmd/cdkm/... && go build ./...`
Expected: PASS + binary builds.

- [ ] **Step 5: Manual smoke against fake cdk**

```bash
mkdir -p /tmp/cdkm-smoke/bin
cat > /tmp/cdkm-smoke/bin/cdk <<'EOF'
#!/usr/bin/env bash
echo "synthesizing CloudFormation template"
echo "deploying $*"
echo done
EOF
chmod +x /tmp/cdkm-smoke/bin/cdk
cat > /tmp/cdkm-smoke/cdkm.yaml <<'EOF'
accounts:
  dev-eu: { profile: dev-eu, region: eu-west-1, tags: [dev] }
groups:
  dev: { tags: [dev] }
stacks: { shared: [AppStack] }
EOF
cd /tmp/cdkm-smoke && PATH="/tmp/cdkm-smoke/bin:$PATH" go run github.com/nguyenquangkhai/cdk-manager/cmd/cdkm deploy --group dev
```

Expected: prints `[dev-eu]` progress lines and a summary table with `done`; exit 0.

- [ ] **Step 6: Commit**

```bash
git add cmd go.mod go.sum
git commit -m "feat: cobra CLI wiring for all commands"
```

---

### Task 11: CI + release automation

**Files:**
- Create: `.github/workflows/ci.yml`
- Create: `.goreleaser.yaml`
- Create: `.github/workflows/release.yml`

**Interfaces:** none (infra). Produces green CI on push/PR and tagged release artifacts.

- [ ] **Step 1: CI workflow**

`.github/workflows/ci.yml`:

```yaml
name: ci
on:
  push: { branches: [main] }
  pull_request:
jobs:
  test:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        go: ['1.22', '1.23']
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '${{ matrix.go }}' }
      - run: go vet ./...
      - run: go test -race ./...
      - run: go build ./...
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.23' }
      - uses: golangci/golangci-lint-action@v6
```

- [ ] **Step 2: GoReleaser config**

`.goreleaser.yaml`:

```yaml
version: 2
builds:
  - main: ./cmd/cdkm
    binary: cdkm
    env: [CGO_ENABLED=0]
    goos: [linux, darwin]
    goarch: [amd64, arm64]
    ldflags:
      - -s -w -X main.version={{.Version}}
archives:
  - formats: [tar.gz]
checksum:
  name_template: 'checksums.txt'
```

`.github/workflows/release.yml`:

```yaml
name: release
on:
  push:
    tags: ['v*']
permissions:
  contents: write
jobs:
  goreleaser:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - uses: actions/setup-go@v5
        with: { go-version: '1.23' }
      - uses: goreleaser/goreleaser-action@v6
        with: { args: release --clean }
        env: { GITHUB_TOKEN: '${{ secrets.GITHUB_TOKEN }}' }
```

- [ ] **Step 3: Validate locally**

Run: `go test ./... && go vet ./...`
Expected: PASS. (GoReleaser validated in CI; optionally `goreleaser check` if installed.)

- [ ] **Step 4: Commit**

```bash
git add .github .goreleaser.yaml
git commit -m "ci: add test/lint workflows and goreleaser"
```

---

### Task 12: README expansion + first push

**Files:**
- Modify: `README.md`
- Modify: `CHANGELOG.md`

**Interfaces:** none. Produces user-facing docs + remote.

- [ ] **Step 1: Expand README**

Fill `README.md` with: badges (CI, license), Install (`go install github.com/nguyenquangkhai/cdk-manager/cmd/cdkm@latest`), a full `cdkm.yaml` example, command reference (`deploy/destroy/diff/synth/run/list/status` + selectors + flags), the parallel/isolation explanation (`cdk.out/<target>`), and safety section (typed destroy confirm, dry-run).

- [ ] **Step 2: Update CHANGELOG**

Add an `## [0.1.0]` section summarizing initial features under Added.

- [ ] **Step 3: Full verification**

Run: `go build ./... && go test ./... && go vet ./...`
Expected: all PASS.

- [ ] **Step 4: Commit + push**

```bash
git add README.md CHANGELOG.md
git commit -m "docs: complete README and changelog for 0.1.0"
git branch -M main
git remote add origin https://github.com/nguyenquangkhai/cdk-manager.git
git push -u origin main
```

Expected: branch `main` pushed; CI runs green on GitHub.

---

## Self-Review Notes

- **Spec coverage:** config (T2), groups/tags selectors (T3), shared+perAccount stacks (T3), isolated `cdk.out/<target>` (T4 OutputDir + T5 engine), parallel pool (T5), dry-run (T5), typed destroy confirm (T7), result gate fail-fast/continue (T5), generic `run` + templates (T8), live table + non-TTY fallback (T9), summary/exit/state (T6), CLI surface (T10), OSS standards/license/CI/release (T1, T11, T12). Diff-preview gate: surfaced via `diff` subcommand (T10) — explicit pre-deploy auto-diff prompt is deferred; noted below.
- **Deferred from spec (intentional, low-risk):** automatic `cdk diff` prompt before deploy is available as a manual `cdkm diff <selector>` step rather than an inline gate; revisit if users want it inline. The bubbletea TUI is scaffolded but the PlainReporter is the tested/guaranteed path.
- **Type consistency:** `adapter.Command`, `adapter.State`, `engine.Job/Result/Update/Options`, `target.Target/Selector`, `config.*` names are used identically across tasks. `cdk.New()`, `engine.Run`, `report.Summarize/SaveState`, `safety.ConfirmDestroy`, `run.BuildCommand`, `ui.New` signatures match their definitions and call sites.

# cdkm — Multi-Account CDK Orchestrator

**Date:** 2026-06-30
**Status:** Approved design, pending implementation plan

## Problem

DevOps teams manage many AWS accounts. Deploying or destroying CDK stacks across all
(or a selected subset of) accounts is manual and serial: switch profile, `cdk deploy`,
wait, repeat. Concurrent runs from one repo clobber each other because `cdk synth`
writes a shared `cdk.out/`. There is no local, imperative CLI that fans a CDK operation
out across accounts safely and in parallel.

Existing solutions are CI/CD-bound (CDK Pipelines, GitHub Actions waves) or low-level
plumbing (`cdk-assume-role-credential`, `cdk-assets`). None give a developer on a laptop
a `deploy --group prod` experience. That gap is the target.

## Goals

- Fan a CDK operation (deploy / destroy / diff / synth) across many accounts from one command.
- Select targets flexibly: all / group / tag / single account.
- Run in parallel safely via an **isolated `cdk.out/<target>` per account**.
- Mix shared stacks (everywhere) with per-account custom stacks.
- Strong safety on destructive ops: diff preview, typed destroy confirmation, dry-run, result gate.
- Live per-target status table; per-target logs on disk.
- Stay extensible: CDK is first-class, but the engine is not locked to CDK.

## Non-Goals (v1)

- Terraform / SAM adapters (interface ready, not built until demand).
- CI/CD pipeline generation. This is a local/CLI tool.
- Managing AWS credentials beyond named profiles (no SSO login flow, no role-assumption engine — `cdk` + profiles handle that).
- Reimplementing CDK deploy internals (we shell out to the `cdk` CLI — Approach A).

## Architecture

Single static Go binary `cdkm`. Two layers:

```
core engine ──┬── cdk adapter   (deploy/destroy/diff/synth)   v1, full smarts
              ├── run           (generic passthrough)          v1, escape hatch
              └── terraform / sam adapter                      later, IF demand
```

**Core engine** knows nothing about CDK. It:
1. loads + validates config,
2. resolves a selector to a list of targets,
3. runs a per-target command in a bounded concurrency pool (goroutines + semaphore),
4. injects per-target env + template vars,
5. captures stdout/stderr to a per-target log,
6. drives a live status table and a final summary, sets the exit code.

**Adapter interface** (CDK is the first implementation):
```go
type Adapter interface {
    BuildArgs(t Target, op Operation, stacks []string) []string // cdk argv
    Isolate(t Target) []string   // e.g. ["--output", "cdk.out/<target>"]
    ParseStatus(line string) StatusUpdate // map cdk stdout -> table state
    SafetyGate(op Operation, targets []Target) error // diff/confirm
}
```

**Generic `run`** is the escape hatch — a passthrough that templates the command and runs
it across targets with env injected, but applies no operation-specific smarts:
```
cdkm run --group prod -- terraform apply -var region={{region}}
cdkm run --all       -- ./script.sh {{profile}} {{outdir}}
```

Template vars available everywhere: `{{profile}} {{region}} {{account}} {{target}} {{outdir}}`
plus any `context.*` keys from config. `{{outdir}}` defaults to `.cdkm/out/<target>`.

## Config — `cdkm.yaml`

```yaml
defaults:
  concurrency: 4
  requireApproval: never        # passed to cdk; override per-run
accounts:
  dev-eu:
    profile: dev-eu             # named profile in ~/.aws
    region: eu-west-1
    tags: [dev, eu]
    context: { env: dev }       # injected as -c env=dev and {{context.env}}
  prod-us:
    profile: prod-us
    region: us-east-1
    tags: [prod, us]
    context: { env: prod }
groups:
  prod: { tags: [prod] }         # tag selector
  eu:   { tags: [eu] }
  core: { accounts: [dev-eu, prod-us] }  # explicit list
stacks:
  shared: [NetworkStack, AppStack]       # deploy to every target
  perAccount:
    prod-us: [ProdOnlyStack]             # the "mix": extra custom stacks
```

Per target, effective stacks = `shared + perAccount[target]`. If `stacks` omitted entirely,
the CDK adapter uses `--all`. Explicit stack args on the CLI override config.

Validation rules: every group selector resolves to ≥1 known account; every `perAccount`
key is a known account; profiles are not validated against `~/.aws` at load (cdk reports that).

## Command Surface

```
cdkm deploy   <selector> [Stack...] [flags]
cdkm destroy  <selector> [Stack...] [flags]   # typed confirmation required
cdkm diff     <selector> [Stack...]
cdkm synth    <selector>
cdkm run      <selector> -- <command...>       # generic passthrough
cdkm list     [selector]                        # show resolved targets + stacks
cdkm status                                     # last run summary from .cdkm/state

Selectors (exactly one):  --all | --group <g> | --account <a> | --tag <t>
Flags:
  --concurrency N        override pool size
  --dry-run              print planned argv per target, call nothing
  --fail-fast            stop scheduling new targets after first failure
  --continue             run all targets, report failures (default)
  --require-approval V   passthrough to cdk (never|any-change|broadening)
  --profile-override P   force a single profile (testing)
```

## Execution Flow

1. Parse CLI → resolve selector → target list (error if empty).
2. Adapter `SafetyGate`: for `destroy`, require typed confirmation of the group/account
   name; for `deploy`/`destroy`, optionally run `diff` first and show aggregated changes.
3. `--dry-run` → print the exact argv per target and exit 0.
4. Schedule targets into the pool (size = `--concurrency` or config default):
   - ensure isolated output dir (`cdk.out/<target>` for cdk; `.cdkm/out/<target>` generic),
   - spawn process with `AWS_PROFILE`, `AWS_REGION`, cwd, and `-c key=val` context,
   - stream stdout/stderr → `.cdkm/logs/<target>.log`; feed lines to `ParseStatus`.
5. Update the live table row per target: `pending → synth → deploy → done | failed (elapsed)`.
6. On failure: respect `--fail-fast` (cancel pending) or `--continue`.
7. Write final summary table + `.cdkm/state` (for `cdkm status`). Exit non-zero if any failed.

## Safety (all four locked)

- **Diff/preview gate** — `deploy`/`destroy` can run `cdk diff` first, show aggregated
  per-target changes, require confirm before applying.
- **Typed destroy confirmation** — `destroy` prompts the user to type the exact
  group/account name; mismatch aborts. Blocks accidental fan-out deletes.
- **Dry-run** — `--dry-run` synthesizes the plan (argv per target) and calls no AWS.
- **Result gate** — `--fail-fast` vs `--continue` (default) controls multi-target behavior.

## Live UI

Bubbletea-based status table, one row per target, updating in place: state + elapsed +
last log line. Non-TTY (CI, piped) falls back to prefixed line logging `[target] ...`.
Full output always persisted to per-target log files regardless of UI mode.

## Project Layout (Go)

```
cmd/cdkm/main.go            entrypoint, cobra root
internal/config/           load + validate cdkm.yaml (yaml.v3)
internal/target/           selector -> []Target resolution
internal/engine/           concurrency pool, process spawn, log capture, env injection
internal/adapter/          Adapter interface
internal/adapter/cdk/      CDK adapter (BuildArgs/Isolate/ParseStatus/SafetyGate)
internal/run/              generic passthrough adapter
internal/ui/               bubbletea table + non-TTY fallback
internal/report/           summary + exit code + .cdkm/state
```

Key libs: cobra (CLI), yaml.v3 (config), bubbletea/lipgloss (UI), errgroup/semaphore (pool).
No AWS SDK in v1 — we shell out to `cdk`. (SDK-based CFN event polling = future Approach C.)

## Testing Strategy

- **Unit:** config parse + validation; selector resolution (all/group/tag/account, empty);
  CDK `BuildArgs` argv (assert exact `cdk` arguments incl. `--output`, `--profile`, `-c`,
  stack list); summary/exit-code logic; template var substitution.
- **Integration:** a **fake `cdk` script** placed on `PATH` that emits scripted stdout and
  exit codes — exercises the engine pool, log capture, status parsing, fail-fast/continue,
  and isolated output dirs without touching AWS.
- **Manual e2e:** against real sandbox accounts before release.

## Open Decisions (defaulted, revisit if needed)

- Config format: **YAML** (`cdkm.yaml`).
- UI lib: **bubbletea**; non-TTY fallback to prefixed logs.
- Default concurrency: **4**.
- Default result gate: **--continue**.
- State/logs dir: **`.cdkm/`** in cwd (git-ignored).

## Open-Source Project Standards

Repo follows common OSS conventions from day one:

- **LICENSE** — **Apache-2.0** (patent grant; standard for infra tooling).
- **README.md** — what/why, install, quickstart (`cdkm.yaml` example + `deploy --group prod`), command reference, contributing pointer.
- **CONTRIBUTING.md** — build/test/lint commands, branch/PR flow, commit convention.
- **CODE_OF_CONDUCT.md** — Contributor Covenant.
- **SECURITY.md** — how to report vulnerabilities privately.
- **CHANGELOG.md** — Keep a Changelog format; SemVer.
- **.github/** — issue templates (bug/feature), PR template, `CODEOWNERS`.
- **CI** — GitHub Actions: `go test ./...`, `go vet`, `golangci-lint`, `gofmt` check; matrix on Go versions; build the binary.
- **Release** — GoReleaser for tagged cross-platform binaries (darwin/linux, amd64/arm64) + checksums.
- **Tooling config** — `.golangci.yml`, `.editorconfig`, `.gitignore` (ignore `cdk.out/`, `.cdkm/`, dist).
- **go.mod** — module path `github.com/nguyenquangkhai/cdk-manager` (confirm exact GitHub handle before first push); Go 1.22+.

These are scaffolded as part of implementation, tracked in the plan.

## Future (out of scope, interface-ready)

- terraform / sam adapters behind the same `Adapter` interface.
- Approach C: AWS SDK CFN event polling for finer-grained progress in the table.
- Per-target region matrices (one account × many regions).

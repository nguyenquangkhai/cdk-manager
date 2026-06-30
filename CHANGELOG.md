# Changelog

All notable changes follow [Keep a Changelog](https://keepachangelog.com/) and SemVer.

## [Unreleased]

## [0.2.0] - 2026-06-30

### Added
- `cdkm init` config generator: interactive picker with tag/group suggestions (reuses previously entered values to avoid typos); optional `--verify` to confirm credentials via `aws sts get-caller-identity` and auto-fill account IDs; `--stdout`, `--force`, and `--non-interactive` flags.
- Opt-in lefthook git hooks (gofmt/vet pre-commit, tests pre-push).
- Update check: `cdkm version --check` plus a conservative, cached,
  opt-out-able (`CDKM_NO_UPDATE_CHECK`) auto-warning when a newer release exists.

## [0.1.0] - 2026-06-30

### Added

- **Multi-account fan-out deploy:** Synthesize and deploy CDK stacks across many AWS accounts in parallel.
- **Group and tag selectors:** Target subsets of accounts via named groups or tags, in addition to `--all` and single `--account` selection.
- **Shared and per-account stacks:** Define stacks that run on all targets or only on specific accounts.
- **Isolated output directories:** Each account's CloudFormation output is stored in `cdk.out/<account>`, ensuring parallel operations don't interfere.
- **Parallel execution engine:** Configurable concurrency (default 4) with `--fail-fast` for early termination on errors.
- **Typed destroy confirmation:** Requires explicit confirmation of the operation scope (group/account name) before destroying stacks.
- **Dry-run mode:** Preview all CDK commands without execution via `--dry-run`.
- **Generic run passthrough:** Execute arbitrary commands across targets with template variable substitution (`{{profile}}`, `{{region}}`, `{{account}}`, `{{context.KEY}}`).
- **Live progress reporting:** Real-time table of job status on TTY; plain text fallback on non-TTY output.
- **Command suite:** `deploy`, `destroy`, `diff`, `synth`, `run`, `list`, and `status` subcommands with unified flags.
- **Per-target logging:** All output stored in `.cdkm/logs/<target>.log` for debugging and audit trails.
- **State persistence:** `.cdkm/state.json` records the result of each operation for later inspection via `cdkm status`.

### Note

Initial release. Design and implementation documented in ADRs and spec docs in the repository.

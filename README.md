# cdkm

Fan AWS CDK operations across many accounts in parallel — from your laptop.

`cdkm deploy --group prod` synthesizes and deploys to every account in the `prod` group concurrently, with each account's output isolated in `cdk.out/<account>`. Live per-target progress, safety gates on destroy, and a single CLI to rule them all.

## Install

**Prerequisites:** Go 1.24+ and `aws-cdk` (Node.js CDK CLI).

```bash
go install github.com/nguyenquangkhai/cdk-manager/cmd/cdkm@latest
```

Once built, download prebuilt binaries from [Releases](https://github.com/nguyenquangkhai/cdk-manager/releases).

## Getting Started

Run `cdkm init` in your CDK project root to scaffold `cdkm.yaml` from your `~/.aws` profiles:

```bash
cdkm init                     # interactive checkbox picker — space to toggle, type to filter, a to select all, enter to confirm
cdkm init --verify            # same, but first confirms credentials via `aws sts get-caller-identity` and fills account IDs
cdkm init --edit              # open a prefilled config in $VISUAL/$EDITOR/vi for full manual control before writing
cdkm init --non-interactive   # include all profiles with empty tags/groups (no prompts; useful in CI)
cdkm init --stdout            # print to stdout instead of writing cdkm.yaml
cdkm init --force             # overwrite an existing cdkm.yaml
```

### Interactive flow

When running on a TTY without `--non-interactive` or `--edit`, `cdkm init` launches a checkbox TUI:

1. **Select accounts** — use `space` to toggle, type to filter by name, `a` to toggle all, `enter` to confirm.
2. **Bulk tagging** — after selection, you are prompted for tag names. Enter a tag name, then pick which of the selected accounts receive it. Leave the tag name blank to finish.

### `--edit` mode

`cdkm init --edit` generates a prefilled YAML document containing every profile (with empty `tags` and `groups`) and opens it in your `$VISUAL` or `$EDITOR` (falls back to `vi`). Edit freely — add tags, groups, remove accounts — then save and quit. `cdkm` validates the YAML before writing `cdkm.yaml`.

After the file is created, edit `tags`, `groups`, and `stacks` to match your project layout, then run your first deployment:

### Multiple projects

If you work with the same AWS accounts across many CDK projects, run `cdkm init --global` once to write the shared accounts and groups config to `~/.config/cdkm/config.yaml`:

```bash
cdkm init --global                  # interactive — pick accounts, bulk-tag, write global config
cdkm init --global --non-interactive  # include all profiles, no prompts
cdkm init --global --stdout         # preview accounts-only YAML without writing
cdkm init --global --force          # overwrite an existing global config
```

The global config path is resolved in this order:

1. `$CDKM_GLOBAL_CONFIG` (if set)
2. `$XDG_CONFIG_HOME/cdkm/config.yaml` (if `$XDG_CONFIG_HOME` is set)
3. `~/.config/cdkm/config.yaml` (default)

The global file contains only `accounts:` and `groups:` — no `stacks:`. Each project's `cdkm.yaml` then only needs a `stacks:` section (or can be omitted entirely to deploy all stacks). Local values override global ones: accounts/groups are merged by key (local wins on conflict), and `stacks` from the local file takes precedence wholesale.

```bash
cdkm deploy --group prod
```

## Quick Start

Create `cdkm.yaml` in your CDK project root manually if preferred:

```yaml
defaults:
  concurrency: 4
  requireApproval: broadening

accounts:
  prod-us-east-1:
    profile: prod
    region: us-east-1
    tags: [prod, prod-us]
    context:
      account: "123456789012"
      environment: prod

  prod-us-west-2:
    profile: prod
    region: us-west-2
    tags: [prod, prod-us]
    context:
      account: "123456789012"
      environment: prod

  staging-us-east-1:
    profile: staging
    region: us-east-1
    tags: [staging]
    context:
      account: "111111111111"
      environment: staging

groups:
  prod:
    tags: [prod]
    accounts: []  # inherits all accounts matching prod tag

  staging:
    accounts: [staging-us-east-1]

stacks:
  shared:
    - VpcStack
    - IamStack
  perAccount:
    staging-us-east-1:
      - LocalTestStack
```

Deploy all production accounts in parallel:

```bash
cdkm deploy --group prod
```

Review changes before deploying:

```bash
cdkm diff --group prod
cdkm deploy --group prod
```

Run a custom command across all staging accounts:

```bash
cdkm run --group staging -- aws sts get-caller-identity
```

## Commands

All commands require exactly one of `--all`, `--group`, `--account`, or `--tag` to select targets.

### deploy

Synthesize and deploy CDK stacks to target accounts.

```bash
cdkm deploy --group prod [--stacks Stack1,Stack2] [--dry-run] [--concurrency N] [--require-approval LEVEL]
```

**Flags:**
- `--stacks`: Override stacks to deploy (comma-separated)
- `--dry-run`: Print CDK commands without executing
- `--concurrency`: Max parallel deployments (0 = use config default)
- `--require-approval`: CDK approval level (`never`, `any-change`, `broadening`)
- `--fail-fast`: Abort remaining jobs on first failure

Each account gets its own isolated output directory (`cdk.out/<account>`), logs in `.cdkm/logs/`, and a final state summary in `.cdkm/state.json`.

### destroy

Destroy CDK stacks in target accounts (with typed confirmation prompt).

```bash
cdkm destroy --account prod-us-east-1 [--stacks Stack1,Stack2] [--dry-run]
```

**Flags:** Same as `deploy`, plus typed confirmation for safety.

### diff

Show CloudFormation diff for target accounts without deploying.

```bash
cdkm diff --tag prod
```

Use this before `deploy` to review changes across all targets.

### synth

Synthesize CDK stacks (generate CloudFormation templates) without deploying.

```bash
cdkm synth --all
```

### run

Execute an arbitrary command against each target, with template variable substitution.

```bash
cdkm run --group staging -- aws sts get-caller-identity
cdkm run --group staging -- sh -c "aws iam list-roles --profile {{profile}}"
cdkm run --all -- echo "Account: {{context.account}} Region: {{region}}"
```

**Template variables:**
- `{{profile}}`: AWS profile name
- `{{region}}`: AWS region
- `{{account}}`: Account ID (from context)
- `{{target}}`: Target name (config key)
- `{{outdir}}`: Output directory (`.cdkm/out/<target>`)
- `{{context.KEY}}`: Custom context values

Environment variables `AWS_PROFILE` and `AWS_REGION` are automatically set for each target.

### list

List resolved targets and their effective stacks.

```bash
cdkm list --group prod
```

Output: target name, profile/region, and applicable stacks.

### status

Show the result summary from the last command run.

```bash
cdkm status
```

Reads `.cdkm/state.json` and prints a summary without re-running.

## Global Flags

Available on all commands:

- `--config`: Path to config file (default: `cdkm.yaml`)
- `--all`: Target all accounts
- `--group`: Target a named group
- `--account`: Target a single account by name
- `--tag`: Target all accounts matching a tag
- `--concurrency`: Max parallel jobs (0 = use config default)
- `--dry-run`: Print commands without executing
- `--fail-fast`: Abort remaining jobs on first failure
- `--require-approval`: Override CDK approval level

## Safety

**Destroy Confirmation:** The `destroy` command requires typed confirmation of the operation label (e.g., `prod` or `staging-us-east-1`) to prevent accidental deletions.

**Dry Run:** Use `--dry-run` to preview CDK commands and output without executing.

**Logs & State:** Every run stores per-target logs in `.cdkm/logs/` and a final state summary in `.cdkm/state.json` for auditing.

**Isolation:** Each account's CloudFormation output is isolated in `cdk.out/<account>`, ensuring parallel deployments don't interfere.

## Limitations

- **No automatic diff-before-deploy gate:** Inline diff preview before deploying is not automatic in v1. Run `cdkm diff <selector>` manually before `cdkm deploy` to review changes.
- **`--profile-override` not yet implemented:** The design referenced a per-run profile override flag; it is not available in the current release.

## License

Apache-2.0

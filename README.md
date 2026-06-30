# cdkm

Fan AWS CDK operations across many accounts in parallel — from your laptop.

`cdkm deploy --group prod` synthesizes and deploys to every account in the `prod` group concurrently, with each account's output isolated in `.cdkm/out/<account>`. Live progress table, safety gates on destroy, and a single CLI to rule them all.

## Install

**Prerequisites:** Go 1.24+ and `aws-cdk` (Node.js CDK CLI).

```bash
go install github.com/nguyenquangkhai/cdk-manager/cmd/cdkm@latest
```

Once built, download prebuilt binaries from [Releases](https://github.com/nguyenquangkhai/cdk-manager/releases).

## Quick Start

Create `cdkm.yaml` in your CDK project root:

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

Each account gets its own isolated output directory (`.cdkm/out/<account>`), logs in `.cdkm/logs/`, and a final state summary in `.cdkm/state.json`.

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

**Isolation:** Each account's CloudFormation output is isolated in `.cdkm/out/<account>`, ensuring parallel deployments don't interfere.

## License

Apache-2.0

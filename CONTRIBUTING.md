# Contributing

## Build & test

    go build ./...
    go test ./...
    go vet ./...
    golangci-lint run

## Git hooks (optional)

We use [lefthook](https://github.com/evilmartians/lefthook) for fast local
checks before commit/push. They are optional — CI runs the same checks — but
they catch failures earlier.

    # install lefthook (pick one)
    brew install lefthook
    go install github.com/evilmartians/lefthook@latest

    # enable the hooks in your clone
    lefthook install

pre-commit runs `gofmt` + `go vet`; pre-push runs `go test ./...`.

## Workflow

- Branch from `main`.
- Conventional Commits (`feat:`, `fix:`, `docs:`, ...).
- One logical change per PR; include tests.
- Add a bullet under `## [Unreleased]` in `CHANGELOG.md` for user-facing changes.

## Releasing

Releases are cut from the hand-written `CHANGELOG.md`. Add your notes under
`## [Unreleased]`, then run:

    scripts/release.sh X.Y.Z            # e.g. scripts/release.sh 0.5.0
    scripts/release.sh X.Y.Z --dry-run  # preview the CHANGELOG change, no push

The script (on a clean `main`) rolls `## [Unreleased]` into `## [X.Y.Z] - <date>`,
runs the tests, commits, tags `vX.Y.Z`, and pushes. The tag push triggers the
GoReleaser workflow, which builds the cross-platform binaries and publishes the
GitHub release. Use `--dry-run` first to review the changelog diff.

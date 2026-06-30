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

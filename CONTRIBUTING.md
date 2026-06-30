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

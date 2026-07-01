#!/usr/bin/env bash
# Cut a release: roll CHANGELOG [Unreleased] into a versioned section, commit,
# tag, and push — the tag push triggers the GoReleaser workflow.
#
# Usage:
#   scripts/release.sh X.Y.Z [--dry-run]
#
# Preconditions (enforced): on `main`, clean working tree, tag does not exist,
# CHANGELOG has content under `## [Unreleased]`, and `go test ./...` passes.
set -euo pipefail

die() { echo "error: $*" >&2; exit 1; }

VERSION="${1:-}"
DRY_RUN="${2:-}"
[ -n "$VERSION" ] || die "usage: scripts/release.sh X.Y.Z [--dry-run]"
[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "version must be X.Y.Z (no leading v), got: $VERSION"
[ -z "$DRY_RUN" ] || [ "$DRY_RUN" = "--dry-run" ] || die "second arg must be --dry-run or omitted"

cd "$(git rev-parse --show-toplevel)"
TAG="v$VERSION"
DATE="$(date +%F)"
CHANGELOG="CHANGELOG.md"

# --- preconditions ---
# Release-time gates (skipped for --dry-run, which only previews the changelog).
if [ "$DRY_RUN" != "--dry-run" ]; then
  [ "$(git rev-parse --abbrev-ref HEAD)" = "main" ] || die "must be on main"
  [ -z "$(git status --porcelain)" ] || die "working tree not clean; commit or stash first"
fi
git rev-parse -q --verify "refs/tags/$TAG" >/dev/null && die "tag $TAG already exists"
grep -q "^## \[$VERSION\]" "$CHANGELOG" && die "CHANGELOG already has a [$VERSION] section"

# Require non-empty content under [Unreleased].
content="$(awk '/^## \[Unreleased\]/{f=1;next} f&&/^## \[/{exit} f&&NF{print}' "$CHANGELOG")"
[ -n "$content" ] || die "nothing under ## [Unreleased] in $CHANGELOG — add entries before releasing"

# --- roll the changelog: insert a versioned header just under [Unreleased] ---
tmp="$(mktemp)"
awk -v ver="$VERSION" -v date="$DATE" '
  !done && /^## \[Unreleased\]/ {
    print
    print ""
    print "## [" ver "] - " date
    done=1
    next
  }
  { print }
' "$CHANGELOG" > "$tmp"

if [ "$DRY_RUN" = "--dry-run" ]; then
  echo "=== DRY RUN: proposed CHANGELOG change ==="
  diff -u "$CHANGELOG" "$tmp" || true
  echo "=== would run: go test ./... ; commit ; tag $TAG ; push main + $TAG ==="
  rm -f "$tmp"
  exit 0
fi

# --- verify the build before releasing ---
echo "› go test ./..."
go test ./... >/dev/null || die "tests failed; not releasing"

mv "$tmp" "$CHANGELOG"

git add "$CHANGELOG"
git commit -q -m "docs: changelog for $TAG"
git tag -a "$TAG" -m "$TAG"
git push origin main
git push origin "$TAG"

echo
echo "✓ released $TAG — GoReleaser is building the binaries."
echo "  https://github.com/nguyenquangkhai/cdk-manager/releases/tag/$TAG"

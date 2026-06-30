# cdkm — lefthook + version-update check Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Add opt-in local git hooks (lefthook) and a version-update check (conservative auto-warn + explicit `cdkm version --check`) to the `cdkm` CLI.

**Architecture:** lefthook is config-only (no Go). The version check lives in a new, fully-testable `internal/version` package (cache + semver compare + injected HTTP fetcher), wired into the cobra root as a non-blocking best-effort refresh (PersistentPreRun) + cache-only warning (PersistentPostRun), plus a synchronous `version` subcommand.

**Tech Stack:** Go 1.24, cobra, lipgloss (already deps), `golang.org/x/mod/semver` (new), stdlib net/http.

## Global Constraints

- Module path: `github.com/nguyenquangkhai/cdk-manager` (verbatim in imports).
- Go floor: **1.24** (matches existing go.mod).
- Cache file: `.cdkm/version-check.json`; TTL **24h**.
- GitHub source: `https://api.github.com/repos/nguyenquangkhai/cdk-manager/releases/latest` (JSON field `tag_name`).
- Opt-out env var: `CDKM_NO_UPDATE_CHECK` (any non-empty value disables auto-check).
- Auto-check is skipped when: current version == `"dev"`, opt-out set, or stdout is not a TTY.
- On-demand sync fetch timeout: **1.5s**.
- Warning color: yellow via lipgloss.
- No real network in tests — inject the fetcher.
- Commit after each task; Conventional Commits. Commit body ends with:
  `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`

---

### Task 1: lefthook config + CONTRIBUTING docs

**Files:**
- Create: `lefthook.yml`
- Modify: `CONTRIBUTING.md`

**Interfaces:** none (config + docs). No Go, no tests.

- [ ] **Step 1: Write `lefthook.yml`**

```yaml
# Opt-in local git hooks. Install once: `lefthook install`.
# CI remains the authoritative gate; these hooks are a fast local convenience.
pre-commit:
  parallel: true
  commands:
    gofmt:
      glob: "*.go"
      run: |
        unformatted=$(gofmt -l {staged_files})
        if [ -n "$unformatted" ]; then
          echo "Unformatted files (run: gofmt -w .):"
          echo "$unformatted"
          exit 1
        fi
    vet:
      run: go vet ./...
pre-push:
  commands:
    test:
      run: go test ./...
```

- [ ] **Step 2: Update CONTRIBUTING.md**

Add a "Git hooks (optional)" subsection under the build/test section:

```markdown
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
```

- [ ] **Step 3: Verify nothing broke**

Run: `go build ./... && go test ./...`
Expected: PASS (unchanged — this task adds no Go).

- [ ] **Step 4: Commit**

```bash
git add lefthook.yml CONTRIBUTING.md
git commit -m "chore: add opt-in lefthook hooks and document contributor setup"
```

---

### Task 2: `internal/version` core (cache + semver + fetch)

**Files:**
- Create: `internal/version/version.go`
- Test: `internal/version/version_test.go`

**Interfaces:**
- Produces:
  ```go
  package version

  // Fetcher returns the latest release tag (e.g. "v0.2.0"). Injected for tests.
  type Fetcher func(ctx context.Context) (string, error)

  // GitHubFetcher fetches the latest release tag from the GitHub API.
  func GitHubFetcher(ctx context.Context) (string, error)

  // cache record persisted to disk.
  type cacheRecord struct {
      CheckedAt time.Time `json:"checked_at"`
      Latest    string    `json:"latest"`
  }

  // Outdated reports whether latest is a strictly newer semver than current.
  // Both may carry a leading "v"; non-semver inputs return false.
  func Outdated(current, latest string) bool

  // readCache returns the record and true if the cache file exists and parses.
  func readCache(path string) (cacheRecord, bool)
  // writeCache persists rec to path (creating parent dirs).
  func writeCache(path string, rec cacheRecord) error
  // fresh reports whether rec was checked within ttl of now.
  func fresh(rec cacheRecord, ttl time.Duration, now time.Time) bool

  // Refresh fetches the latest tag if the cache is stale and updates it.
  // Best-effort: returns nil on fetch error (cache simply not updated).
  func Refresh(ctx context.Context, cachePath string, fetch Fetcher, ttl time.Duration) error
  ```

- [ ] **Step 1: Write the failing test**

`internal/version/version_test.go`:

```go
package version

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestOutdated(t *testing.T) {
	cases := []struct {
		cur, latest string
		want        bool
	}{
		{"v0.1.0", "v0.2.0", true},
		{"0.1.0", "0.1.0", false},
		{"v0.2.0", "v0.1.0", false},
		{"v1.0.0", "v1.0.1", true},
		{"dev", "v0.2.0", false},   // non-semver current
		{"v0.1.0", "garbage", false}, // non-semver latest
	}
	for _, c := range cases {
		if got := Outdated(c.cur, c.latest); got != c.want {
			t.Errorf("Outdated(%q,%q)=%v want %v", c.cur, c.latest, got, c.want)
		}
	}
}

func TestCacheRoundTripAndFreshness(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "version-check.json")
	now := time.Now()
	rec := cacheRecord{CheckedAt: now, Latest: "v0.2.0"}
	if err := writeCache(path, rec); err != nil {
		t.Fatal(err)
	}
	got, ok := readCache(path)
	if !ok || got.Latest != "v0.2.0" {
		t.Fatalf("readCache = %+v ok=%v", got, ok)
	}
	if !fresh(got, 24*time.Hour, now.Add(time.Hour)) {
		t.Error("record 1h old should be fresh within 24h ttl")
	}
	if fresh(got, 24*time.Hour, now.Add(25*time.Hour)) {
		t.Error("record 25h old should be stale within 24h ttl")
	}
}

func TestRefreshFetchesWhenStaleAndSkipsWhenFresh(t *testing.T) {
	path := filepath.Join(t.TempDir(), "version-check.json")
	calls := 0
	fetch := func(ctx context.Context) (string, error) {
		calls++
		return "v9.9.9", nil
	}
	// No cache yet -> stale -> fetch.
	if err := Refresh(context.Background(), path, fetch, 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 fetch, got %d", calls)
	}
	rec, ok := readCache(path)
	if !ok || rec.Latest != "v9.9.9" {
		t.Fatalf("cache after refresh = %+v ok=%v", rec, ok)
	}
	// Fresh cache -> no fetch.
	if err := Refresh(context.Background(), path, fetch, 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("expected no extra fetch on fresh cache, got %d", calls)
	}
}

func TestRefreshSilentOnFetchError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "version-check.json")
	fetch := func(ctx context.Context) (string, error) {
		return "", context.DeadlineExceeded
	}
	if err := Refresh(context.Background(), path, fetch, 24*time.Hour); err != nil {
		t.Errorf("Refresh should be silent on fetch error, got %v", err)
	}
	if _, ok := readCache(path); ok {
		t.Error("cache should not be written on fetch error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/version/...`
Expected: FAIL — undefined.

- [ ] **Step 3: Write minimal implementation**

`internal/version/version.go`:

```go
package version

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/mod/semver"
)

type Fetcher func(ctx context.Context) (string, error)

type cacheRecord struct {
	CheckedAt time.Time `json:"checked_at"`
	Latest    string    `json:"latest"`
}

const releasesURL = "https://api.github.com/repos/nguyenquangkhai/cdk-manager/releases/latest"

// GitHubFetcher returns the latest release tag_name from the GitHub API.
func GitHubFetcher(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releasesURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github api status %d", resp.StatusCode)
	}
	var body struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return "", err
	}
	if body.TagName == "" {
		return "", fmt.Errorf("empty tag_name")
	}
	return body.TagName, nil
}

// Outdated reports whether latest is strictly newer than current (semver).
// Non-semver inputs (e.g. "dev") yield false.
func Outdated(current, latest string) bool {
	c, l := norm(current), norm(latest)
	if !semver.IsValid(c) || !semver.IsValid(l) {
		return false
	}
	return semver.Compare(c, l) < 0
}

func norm(v string) string {
	if v == "" {
		return v
	}
	if v[0] != 'v' {
		return "v" + v
	}
	return v
}

func readCache(path string) (cacheRecord, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return cacheRecord{}, false
	}
	var rec cacheRecord
	if err := json.Unmarshal(b, &rec); err != nil {
		return cacheRecord{}, false
	}
	return rec, true
}

func writeCache(path string, rec cacheRecord) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func fresh(rec cacheRecord, ttl time.Duration, now time.Time) bool {
	return now.Sub(rec.CheckedAt) < ttl
}

// Refresh updates the cache if it is stale. Best-effort: a fetch error is
// swallowed (returns nil) and the cache is left unchanged.
func Refresh(ctx context.Context, cachePath string, fetch Fetcher, ttl time.Duration) error {
	if rec, ok := readCache(cachePath); ok && fresh(rec, ttl, time.Now()) {
		return nil
	}
	latest, err := fetch(ctx)
	if err != nil {
		return nil
	}
	return writeCache(cachePath, cacheRecord{CheckedAt: time.Now(), Latest: latest})
}
```

- [ ] **Step 4: Add dep + run tests**

Run: `go get golang.org/x/mod/semver && go test ./internal/version/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/version go.mod go.sum
git commit -m "feat: version-check core (cache, semver compare, github fetcher)"
```

---

### Task 3: Wire auto-warn + `version` subcommand + docs

**Files:**
- Modify: `cmd/cdkm/root.go`
- Modify: `cmd/cdkm/commands.go`
- Create: `internal/version/warn.go`
- Test: `internal/version/warn_test.go`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: `internal/version` (Outdated, Refresh, GitHubFetcher, readCache).
- Produces in `internal/version/warn.go`:
  ```go
  // WarnIfOutdated reads the cache only (no network) and writes a yellow
  // warning to w if latest > current. Caller handles all gating
  // (dev/opt-out/TTY) before calling. cachePath is the version-check file.
  func WarnIfOutdated(w io.Writer, cachePath, current string)

  // CheckNow synchronously fetches latest (via fetch), prints current/latest
  // and up-to-date|outdated status to w, updates the cache, and returns the
  // latest tag. Used by `cdkm version --check`.
  func CheckNow(ctx context.Context, w io.Writer, cachePath, current string, fetch Fetcher) (string, error)
  ```

- [ ] **Step 1: Write the failing test**

`internal/version/warn_test.go`:

```go
package version

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWarnIfOutdated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "version-check.json")
	_ = writeCache(path, cacheRecord{CheckedAt: time.Now(), Latest: "v0.2.0"})

	var buf bytes.Buffer
	WarnIfOutdated(&buf, path, "v0.1.0")
	if !strings.Contains(buf.String(), "v0.2.0") {
		t.Errorf("expected warning mentioning v0.2.0, got %q", buf.String())
	}

	buf.Reset()
	WarnIfOutdated(&buf, path, "v0.2.0") // up to date
	if buf.Len() != 0 {
		t.Errorf("expected no warning when current, got %q", buf.String())
	}

	buf.Reset()
	WarnIfOutdated(&buf, filepath.Join(t.TempDir(), "missing.json"), "v0.1.0")
	if buf.Len() != 0 {
		t.Errorf("expected no warning when cache missing, got %q", buf.String())
	}
}

func TestCheckNowOutputAndCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "version-check.json")
	fetch := func(ctx context.Context) (string, error) { return "v0.5.0", nil }

	var buf bytes.Buffer
	latest, err := CheckNow(context.Background(), &buf, path, "v0.1.0", fetch)
	if err != nil {
		t.Fatal(err)
	}
	if latest != "v0.5.0" {
		t.Errorf("latest = %q", latest)
	}
	out := buf.String()
	if !strings.Contains(out, "v0.1.0") || !strings.Contains(out, "v0.5.0") {
		t.Errorf("output missing versions: %q", out)
	}
	if rec, ok := readCache(path); !ok || rec.Latest != "v0.5.0" {
		t.Errorf("cache not updated: %+v ok=%v", rec, ok)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/version/...`
Expected: FAIL — WarnIfOutdated/CheckNow undefined.

- [ ] **Step 3: Implement warn.go**

`internal/version/warn.go`:

```go
package version

import (
	"context"
	"fmt"
	"io"

	"github.com/charmbracelet/lipgloss"
)

var yellow = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))

// WarnIfOutdated reads the cache only (no network) and warns if latest is
// newer than current. All gating is the caller's responsibility.
func WarnIfOutdated(w io.Writer, cachePath, current string) {
	rec, ok := readCache(cachePath)
	if !ok {
		return
	}
	if Outdated(current, rec.Latest) {
		fmt.Fprintln(w, yellow.Render(fmt.Sprintf(
			"A new cdkm release is available: %s (you have %s). https://github.com/nguyenquangkhai/cdk-manager/releases",
			rec.Latest, current)))
	}
}

// CheckNow synchronously fetches the latest tag, reports status, updates the
// cache, and returns the latest tag.
func CheckNow(ctx context.Context, w io.Writer, cachePath, current string, fetch Fetcher) (string, error) {
	latest, err := fetch(ctx)
	if err != nil {
		return "", err
	}
	_ = writeCache(cachePath, cacheRecord{CheckedAt: timeNow(), Latest: latest})
	if Outdated(current, latest) {
		fmt.Fprintln(w, yellow.Render(fmt.Sprintf("cdkm %s is out of date — latest is %s", current, latest)))
	} else {
		fmt.Fprintf(w, "cdkm %s is up to date (latest %s)\n", current, latest)
	}
	return latest, nil
}
```

Add a tiny `timeNow` indirection in version.go for testability, or just use `time.Now()` directly — use `time.Now()` directly here to avoid a new symbol (replace `timeNow()` with `time.Now()` and add the `time` import to warn.go).

- [ ] **Step 4: Run version tests**

Run: `go test ./internal/version/...`
Expected: PASS.

- [ ] **Step 5: Wire into cobra root**

In `cmd/cdkm/root.go`, on the root command add:
- `PersistentPreRun`: best-effort async refresh when auto-check is allowed:

```go
const versionCachePath = ".cdkm/version-check.json"

func autoCheckAllowed() bool {
	if version == "dev" {
		return false
	}
	if os.Getenv("CDKM_NO_UPDATE_CHECK") != "" {
		return false
	}
	return isTerminal() // existing helper from Task 10
}

// in the root command:
PersistentPreRun: func(cmd *cobra.Command, args []string) {
	if autoCheckAllowed() {
		go ver.Refresh(context.Background(), versionCachePath, ver.GitHubFetcher, 24*time.Hour)
	}
},
PersistentPostRun: func(cmd *cobra.Command, args []string) {
	if autoCheckAllowed() {
		ver.WarnIfOutdated(os.Stderr, versionCachePath, version)
	}
},
```

(Import the package as `ver "github.com/nguyenquangkhai/cdk-manager/internal/version"`. Warning goes to **stderr** so it never pollutes piped stdout. Note the `version` subcommand below uses `DisableAutoGenTag`-style isolation is unnecessary — the post-run warning on `version --check` is harmless but to avoid double output, the `version` command may set its own `cmd.Run` and the persistent hooks still apply; that's acceptable since `--check` prints to stdout and the post-run warning to stderr.)

- [ ] **Step 6: Add `version` subcommand**

In `cmd/cdkm/commands.go` add a `newVersionCmd()` registered on root:

```go
func newVersionCmd() *cobra.Command {
	var check bool
	c := &cobra.Command{
		Use:   "version",
		Short: "Print cdkm version (and optionally check for updates)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !check {
				fmt.Printf("cdkm %s\n", version)
				return nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
			defer cancel()
			_, err := ver.CheckNow(ctx, os.Stdout, versionCachePath, version, ver.GitHubFetcher)
			if err != nil {
				fmt.Fprintf(os.Stderr, "update check failed: %v\n", err)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&check, "check", false, "query GitHub for the latest release")
	return c
}
```

Register it where the other subcommands are added to root. `version` is a string var already defined in `main.go`/root; reuse it (do not redeclare). If `version` currently lives only in `main.go`, move it to `root.go` as a package-level `var version = "dev"` and keep the `-ldflags -X main.version` target working (it must remain `main.version`).

- [ ] **Step 7: Verify full build + tests + smoke**

Run:
```
go build ./... && go vet ./... && go test ./...
go run ./cmd/cdkm version
```
Expected: tests PASS; `cdkm version` prints `cdkm dev`. (With version=="dev", auto-check is disabled, so no network.)

- [ ] **Step 8: Update CHANGELOG**

Under `## [Unreleased]` add:
```markdown
### Added
- Opt-in lefthook git hooks (gofmt/vet pre-commit, tests pre-push).
- Update check: `cdkm version --check` plus a conservative, cached,
  opt-out-able (`CDKM_NO_UPDATE_CHECK`) auto-warning when a newer release exists.
```

- [ ] **Step 9: Commit**

```bash
git add cmd/cdkm internal/version CHANGELOG.md
git commit -m "feat: version subcommand and conservative update-check warning"
```

---

## Self-Review Notes

- **Coverage:** lefthook (T1), version core cache+semver+fetch (T2), auto-warn + on-demand `--check` + wiring + docs (T3). All gating (dev/opt-out/TTY) in `autoCheckAllowed`. Tests inject the fetcher; no real network.
- **Non-blocking guarantee:** auto path = async `Refresh` (PreRun) + cache-only `WarnIfOutdated` (PostRun). Synchronous network only in `version --check` with a 1.5s timeout.
- **Type consistency:** `Fetcher`, `Refresh`, `Outdated`, `WarnIfOutdated`, `CheckNow`, `GitHubFetcher`, `readCache`/`writeCache`/`fresh` used identically across T2/T3 and the cobra wiring. `version` stays a single package-level var targeted by `-X main.version`.
- **Stderr for warnings:** keeps stdout clean for piping.

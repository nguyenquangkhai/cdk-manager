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

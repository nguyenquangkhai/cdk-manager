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

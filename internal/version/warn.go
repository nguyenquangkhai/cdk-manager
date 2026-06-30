package version

import (
	"context"
	"fmt"
	"io"
	"time"

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
	_ = writeCache(cachePath, cacheRecord{CheckedAt: time.Now(), Latest: latest})
	if Outdated(current, latest) {
		fmt.Fprintln(w, yellow.Render(fmt.Sprintf("cdkm %s is out of date — latest is %s", current, latest)))
	} else {
		fmt.Fprintf(w, "cdkm %s is up to date (latest %s)\n", current, latest)
	}
	return latest, nil
}

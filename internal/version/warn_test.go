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

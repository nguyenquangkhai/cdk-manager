package report

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/nguyenquangkhai/cdk-manager/internal/adapter"
	"github.com/nguyenquangkhai/cdk-manager/internal/engine"
)

func TestSummarizeExitCode(t *testing.T) {
	var buf bytes.Buffer
	code := Summarize(&buf, []engine.Result{
		{Target: "a", State: adapter.StateDone, Elapsed: time.Second},
		{Target: "b", State: adapter.StateFailed, ExitCode: 1, Elapsed: 2 * time.Second},
	})
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	out := buf.String()
	if !strings.Contains(out, "a") || !strings.Contains(out, "b") {
		t.Errorf("summary missing targets: %s", out)
	}
	if !strings.Contains(out, "failed") {
		t.Errorf("summary missing failed marker: %s", out)
	}
}

func TestSummarizeAllPass(t *testing.T) {
	var buf bytes.Buffer
	code := Summarize(&buf, []engine.Result{{Target: "a", State: adapter.StateDone}})
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

package report

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
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

func TestSaveStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "state.json")

	// Test data
	testResults := []engine.Result{
		{Target: "a", State: adapter.StateDone, ExitCode: 0},
		{Target: "b", State: adapter.StateFailed, ExitCode: 1},
	}

	// Save state
	err := SaveState(path, testResults)
	if err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	// Read file back
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	// Unmarshal into Result slice
	var results []engine.Result
	err = json.Unmarshal(data, &results)
	if err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	// Assert length and values round-trip
	if len(results) != 2 {
		t.Errorf("len(results) = %d, want 2", len(results))
	}

	if results[0].Target != "a" {
		t.Errorf("results[0].Target = %q, want %q", results[0].Target, "a")
	}
	if results[0].State != adapter.StateDone {
		t.Errorf("results[0].State = %q, want %q", results[0].State, adapter.StateDone)
	}
	if results[0].ExitCode != 0 {
		t.Errorf("results[0].ExitCode = %d, want 0", results[0].ExitCode)
	}

	if results[1].Target != "b" {
		t.Errorf("results[1].Target = %q, want %q", results[1].Target, "b")
	}
	if results[1].State != adapter.StateFailed {
		t.Errorf("results[1].State = %q, want %q", results[1].State, adapter.StateFailed)
	}
	if results[1].ExitCode != 1 {
		t.Errorf("results[1].ExitCode = %d, want 1", results[1].ExitCode)
	}
}

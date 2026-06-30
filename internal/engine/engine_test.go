package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nguyenquangkhai/cdk-manager/internal/adapter"
	"github.com/nguyenquangkhai/cdk-manager/internal/target"
)

func fakeJob(name, mode string) Job {
	abs, _ := filepath.Abs("testdata/fakecdk.sh")
	return Job{
		Target: target.Target{Name: name},
		Command: adapter.Command{
			Name: "bash",
			Args: []string{abs, "AppStack"},
			Env:  map[string]string{"FAKE_MODE": mode},
		},
	}
}

func parse(line string) (adapter.State, bool) {
	l := strings.ToLower(line)
	if strings.Contains(l, "synthesiz") {
		return adapter.StateSynth, true
	}
	if strings.Contains(l, "deploy") {
		return adapter.StateDeploy, true
	}
	return "", false
}

func TestRunSuccess(t *testing.T) {
	dir := t.TempDir()
	res := Run(context.Background(),
		[]Job{fakeJob("a", "ok"), fakeJob("b", "ok")},
		Options{Concurrency: 2, LogDir: dir, Parse: parse})

	if len(res) != 2 {
		t.Fatalf("got %d results", len(res))
	}
	for _, r := range res {
		if r.State != adapter.StateDone {
			t.Errorf("%s state = %s, want done", r.Target, r.State)
		}
		b, _ := os.ReadFile(filepath.Join(dir, r.Target+".log"))
		if !strings.Contains(string(b), "synthesizing") {
			t.Errorf("%s log missing output: %s", r.Target, b)
		}
	}
}

func TestRunFailureReported(t *testing.T) {
	dir := t.TempDir()
	res := Run(context.Background(),
		[]Job{fakeJob("a", "fail")},
		Options{Concurrency: 1, LogDir: dir, Parse: parse})
	if res[0].State != adapter.StateFailed || res[0].ExitCode != 1 {
		t.Fatalf("got %+v, want failed exit 1", res[0])
	}
}

func TestDryRunNoExec(t *testing.T) {
	dir := t.TempDir()
	res := Run(context.Background(),
		[]Job{fakeJob("a", "fail")}, // would fail if executed
		Options{Concurrency: 1, LogDir: dir, DryRun: true, Parse: parse})
	if res[0].State != adapter.StateDone {
		t.Fatalf("dry-run should report done, got %s", res[0].State)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "a.log"))
	if !strings.Contains(string(b), "DRY-RUN") {
		t.Errorf("dry-run log missing plan: %s", b)
	}
}

func TestFailFastCancelsPending(t *testing.T) {
	dir := t.TempDir()
	jobs := []Job{fakeJob("a", "fail"), fakeJob("b", "ok"), fakeJob("c", "ok")}
	res := Run(context.Background(), jobs,
		Options{Concurrency: 1, FailFast: true, LogDir: dir, Parse: parse})
	failed := 0
	for _, r := range res {
		if r.State == adapter.StateFailed {
			failed++
		}
	}
	if failed == 0 {
		t.Fatal("expected at least one failure recorded")
	}
}

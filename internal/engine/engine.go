package engine

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nguyenquangkhai/cdk-manager/internal/adapter"
	"github.com/nguyenquangkhai/cdk-manager/internal/target"
)

type Result struct {
	Target   string
	State    adapter.State
	ExitCode int
	Err      error
	LogPath  string
	Elapsed  time.Duration
}

type Update struct {
	Target string
	State  adapter.State
	Line   string
}

type Job struct {
	Target  target.Target
	Command adapter.Command
}

type Options struct {
	Concurrency int
	FailFast    bool
	LogDir      string
	DryRun      bool
	OnUpdate    func(Update)
	Parse       func(string) (adapter.State, bool)
}

func Run(ctx context.Context, jobs []Job, opts Options) []Result {
	if opts.Concurrency < 1 {
		opts.Concurrency = 1
	}
	if opts.Parse == nil {
		opts.Parse = func(string) (adapter.State, bool) { return "", false }
	}
	_ = os.MkdirAll(opts.LogDir, 0o755)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sem := make(chan struct{}, opts.Concurrency)
	results := make([]Result, len(jobs))
	var wg sync.WaitGroup

	for i, job := range jobs {
		wg.Add(1)
		go func(i int, job Job) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[i] = Result{Target: job.Target.Name, State: adapter.StateFailed, Err: ctx.Err()}
				return
			}
			defer func() { <-sem }()

			r := runOne(ctx, job, opts)
			results[i] = r
			if r.State == adapter.StateFailed && opts.FailFast {
				cancel()
			}
		}(i, job)
	}
	wg.Wait()
	return results
}

func runOne(ctx context.Context, job Job, opts Options) Result {
	start := time.Now()
	logPath := filepath.Join(opts.LogDir, job.Target.Name+".log")
	lf, err := os.Create(logPath)
	if err != nil {
		return Result{Target: job.Target.Name, State: adapter.StateFailed, Err: err, LogPath: logPath}
	}
	defer lf.Close()

	emit := func(state adapter.State, line string) {
		if opts.OnUpdate != nil {
			opts.OnUpdate(Update{Target: job.Target.Name, State: state, Line: line})
		}
	}

	if opts.DryRun {
		plan := fmt.Sprintf("DRY-RUN %s %s\n", job.Command.Name, strings.Join(job.Command.Args, " "))
		_, _ = lf.WriteString(plan)
		emit(adapter.StateDone, "dry-run")
		return Result{Target: job.Target.Name, State: adapter.StateDone, LogPath: logPath, Elapsed: time.Since(start)}
	}

	cmd := exec.CommandContext(ctx, job.Command.Name, job.Command.Args...)
	cmd.Dir = job.Command.Dir
	cmd.Env = os.Environ()
	for k, v := range job.Command.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	emit(adapter.StateRunning, "")
	if err := cmd.Start(); err != nil {
		_ = pr.Close()
		_ = pw.Close()
		return Result{Target: job.Target.Name, State: adapter.StateFailed, Err: err, LogPath: logPath, Elapsed: time.Since(start)}
	}

	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		sc := bufio.NewScanner(pr)
		for sc.Scan() {
			line := sc.Text()
			_, _ = lf.WriteString(line + "\n")
			if st, ok := opts.Parse(line); ok {
				emit(st, line)
			}
		}
	}()

	runErr := cmd.Wait()
	_ = pw.Close()
	<-scanDone

	res := Result{Target: job.Target.Name, LogPath: logPath, Elapsed: time.Since(start)}
	if runErr != nil {
		res.State = adapter.StateFailed
		res.Err = runErr
		if ee, ok := runErr.(*exec.ExitError); ok {
			res.ExitCode = ee.ExitCode()
		} else {
			res.ExitCode = -1
		}
		emit(adapter.StateFailed, runErr.Error())
		return res
	}
	res.State = adapter.StateDone
	emit(adapter.StateDone, "done")
	return res
}

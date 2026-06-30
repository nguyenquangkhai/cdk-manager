package ui

import (
	"fmt"
	"io"
	"sync"

	"github.com/nguyenquangkhai/cdk-manager/internal/engine"
)

// Reporter receives progress updates during a run. Two implementations:
// a bubbletea TUI (TTY) and a plain prefixed-line logger (non-TTY).
type Reporter interface {
	Update(u engine.Update)
	Done(results []engine.Result)
}

// PlainReporter is exported for testing.
type PlainReporter struct {
	mu sync.Mutex
	w  io.Writer
}

func (p *PlainReporter) Update(u engine.Update) {
	p.mu.Lock()
	defer p.mu.Unlock()
	line := u.Line
	if line == "" {
		line = string(u.State)
	}
	fmt.Fprintf(p.w, "[%s] %s\n", u.Target, line)
}

func (p *PlainReporter) Done(results []engine.Result) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, r := range results {
		fmt.Fprintf(p.w, "[%s] %s\n", r.Target, r.State)
	}
}

// New returns a reporter. v1: always PlainReporter; the bubbletea TUI is wired
// in cmd when isTTY is true (see Task 10). Kept simple + testable here.
func New(w io.Writer, isTTY bool) Reporter {
	return &PlainReporter{w: w}
}

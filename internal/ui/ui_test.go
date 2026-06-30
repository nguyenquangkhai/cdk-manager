package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nguyenquangkhai/cdk-manager/internal/adapter"
	"github.com/nguyenquangkhai/cdk-manager/internal/engine"
)

func TestPlainReporter(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false) // non-TTY -> plain
	r.Update(engine.Update{Target: "dev-eu", State: adapter.StateSynth, Line: "synthesizing"})
	r.Update(engine.Update{Target: "dev-eu", State: adapter.StateDeploy, Line: "deploying"})
	r.Done([]engine.Result{{Target: "dev-eu", State: adapter.StateDone}})

	out := buf.String()
	if !strings.Contains(out, "[dev-eu]") {
		t.Errorf("missing target prefix: %s", out)
	}
	if !strings.Contains(out, "synthesizing") {
		t.Errorf("missing update line: %s", out)
	}
}

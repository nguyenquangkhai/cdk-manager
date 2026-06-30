package report

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/nguyenquangkhai/cdk-manager/internal/adapter"
	"github.com/nguyenquangkhai/cdk-manager/internal/engine"
)

func Summarize(w io.Writer, results []engine.Result) int {
	code := 0
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "TARGET\tSTATE\tELAPSED\tEXIT")
	for _, r := range results {
		if r.State == adapter.StateFailed {
			code = 1
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\n", r.Target, r.State, r.Elapsed.Round(1e6), r.ExitCode) // Round to microsecond precision (1e6 ns)
	}
	tw.Flush()
	return code
}

func SaveState(path string, results []engine.Result) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

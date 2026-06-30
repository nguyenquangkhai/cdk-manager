package safety

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/nguyenquangkhai/cdk-manager/internal/adapter"
	"github.com/nguyenquangkhai/cdk-manager/internal/target"
)

func ConfirmDestroy(r io.Reader, w io.Writer, op adapter.Operation, selectorLabel string, targets []target.Target) error {
	if op != adapter.OpDestroy {
		return nil
	}
	fmt.Fprintf(w, "About to DESTROY %d target(s):\n", len(targets))
	for _, t := range targets {
		fmt.Fprintf(w, "  - %s (%s / %s)\n", t.Name, t.Profile, t.Region)
	}
	fmt.Fprintf(w, "Type %q to confirm: ", selectorLabel)

	sc := bufio.NewScanner(r)
	sc.Scan()
	got := strings.TrimSpace(sc.Text())
	if got != selectorLabel {
		return fmt.Errorf("confirmation %q did not match %q; aborting", got, selectorLabel)
	}
	return nil
}

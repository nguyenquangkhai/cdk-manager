package safety

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nguyenquangkhai/cdk-manager/internal/adapter"
	"github.com/nguyenquangkhai/cdk-manager/internal/target"
)

func tgts() []target.Target { return []target.Target{{Name: "prod-us"}} }

func TestConfirmDestroyMatch(t *testing.T) {
	var out bytes.Buffer
	err := ConfirmDestroy(strings.NewReader("prod\n"), &out, adapter.OpDestroy, "prod", tgts())
	if err != nil {
		t.Fatalf("expected confirm, got %v", err)
	}
}

func TestConfirmDestroyMismatch(t *testing.T) {
	var out bytes.Buffer
	err := ConfirmDestroy(strings.NewReader("nope\n"), &out, adapter.OpDestroy, "prod", tgts())
	if err == nil {
		t.Fatal("expected error on mismatch")
	}
}

func TestConfirmSkippedForDeploy(t *testing.T) {
	var out bytes.Buffer
	if err := ConfirmDestroy(strings.NewReader(""), &out, adapter.OpDeploy, "prod", tgts()); err != nil {
		t.Fatalf("deploy should not require confirm: %v", err)
	}
}

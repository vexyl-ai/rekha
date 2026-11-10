package latency

import (
	"os"
	"testing"
)

// TestReport prints the native latency table when REKHA_LATENCY=1; the
// wasm counterpart lives in cmd/rekha-wasm.
func TestReport(t *testing.T) {
	if os.Getenv("REKHA_LATENCY") == "" {
		t.Skip("set REKHA_LATENCY=1 to print the native latency table")
	}
	paths, err := Images("../../testdata/corpus", os.Getenv("REKHA_LOGO"))
	if err != nil {
		t.Fatal(err)
	}
	if err := Report(os.Stdout, paths, Cases()); err != nil {
		t.Fatal(err)
	}
}

func TestMeasureRunsEveryCase(t *testing.T) {
	paths, err := Images("../../testdata/corpus/synthetic")
	if err != nil || len(paths) == 0 {
		t.Fatalf("images: %v %d", err, len(paths))
	}
	if len(Cases()) != 3 {
		t.Fatalf("cases %d", len(Cases()))
	}
}

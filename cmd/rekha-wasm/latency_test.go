//go:build js && wasm

package main

import (
	"os"
	"testing"

	"github.com/vexyl-ai/rekha/internal/latency"
)

// TestWasmLatency prints the wasm latency table when REKHA_LATENCY=1
// (run with make wasm-test ARGS="-run TestWasmLatency -v"). Node's fs
// backs os here, so the corpus is read directly.
func TestWasmLatency(t *testing.T) {
	if os.Getenv("REKHA_LATENCY") == "" {
		t.Skip("set REKHA_LATENCY=1 to print the wasm latency table")
	}
	paths, err := latency.Images("../../testdata/corpus", os.Getenv("REKHA_LOGO"))
	if err != nil {
		t.Fatal(err)
	}
	if err := latency.Report(os.Stdout, paths, latency.Cases()); err != nil {
		t.Fatal(err)
	}
}

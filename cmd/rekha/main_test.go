package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const starPNG = "../../testdata/corpus/synthetic/star-5.png"

func TestRunTracesPNGToSVG(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "star.png")
	data, err := os.ReadFile(starPNG)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(in, data, 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := run([]string{in}, strings.NewReader(""), &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}
	svg, err := os.ReadFile(filepath.Join(dir, "star.svg"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(svg, []byte("<svg")) || !bytes.Contains(svg, []byte("<path")) {
		t.Fatalf("unexpected svg: %.80s", svg)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout should be empty when writing files, got %q", out.String())
	}
}

func TestRunStdinStdout(t *testing.T) {
	data, err := os.ReadFile(starPNG)
	if err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := run([]string{"-"}, bytes.NewReader(data), &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}
	if !strings.HasPrefix(out.String(), "<svg") {
		t.Fatalf("stdout = %.80q", out.String())
	}
}

func TestRunBadInputExit2(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"/nonexistent/missing.png"}, strings.NewReader(""), &out, &errb)
	if code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "missing.png") {
		t.Fatalf("stderr %q should name the input", errb.String())
	}
}

func TestRunContinuesAfterFailure(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "star.png")
	data, _ := os.ReadFile(starPNG)
	if err := os.WriteFile(in, data, 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	code := run([]string{filepath.Join(dir, "missing.png"), in}, strings.NewReader(""), &out, &errb)
	if code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "star.svg")); err != nil {
		t.Fatalf("second input not traced after first failed: %v", err)
	}
}

func TestRunThresholdAuto(t *testing.T) {
	data, _ := os.ReadFile(starPNG)
	var out, errb bytes.Buffer
	if code := run([]string{"--threshold", "auto", "-"}, bytes.NewReader(data), &out, &errb); code != 0 {
		t.Fatalf("--threshold auto: exit %d, stderr %q", code, errb.String())
	}
	out.Reset()
	errb.Reset()
	if code := run([]string{"--threshold", "1.5", "-"}, bytes.NewReader(data), &out, &errb); code != 1 {
		t.Fatalf("--threshold 1.5: exit %d, want 1 (stderr %q)", code, errb.String())
	}
	if code := run([]string{"--threshold", "dark", "-"}, bytes.NewReader(data), &out, &errb); code != 1 {
		t.Fatalf("--threshold dark: exit %d, want 1", code)
	}
}

func TestRunFlagsReachOptions(t *testing.T) {
	data, _ := os.ReadFile(starPNG)
	var out, errb bytes.Buffer
	args := []string{"--mode", "polygon", "--turn-policy", "black", "--simplify", "2", "--corner-threshold", "45", "--fit-tolerance", "0.5", "-"}
	if code := run(args, bytes.NewReader(data), &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}
	if strings.Contains(out.String(), "C") {
		t.Fatalf("polygon mode must not emit cubics: %.120q", out.String())
	}
	if code := run([]string{"--turn-policy", "spiral", "-"}, bytes.NewReader(data), &out, &errb); code != 1 {
		t.Fatalf("bad turn policy: exit %d, want 1", code)
	}
}

package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
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

const (
	stackPNG = "../../testdata/corpus/synthetic/three-layer-stack.png"
	abutPNG  = "../../testdata/corpus/synthetic/two-colour-abut.png"
	logoPNG  = "../../testdata/corpus/synthetic/logo-holes.png"
)

func runStdin(t *testing.T, png string, args ...string) (int, string, string) {
	t.Helper()
	data, err := os.ReadFile(png)
	if err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	code := run(append(args, "-"), bytes.NewReader(data), &out, &errb)
	return code, out.String(), errb.String()
}

func fills(svg string) []string {
	var f []string
	for _, m := range regexp.MustCompile(`fill="(#[0-9a-f]{6})"`).FindAllStringSubmatch(svg, -1) {
		f = append(f, m[1])
	}
	return f
}

func TestRunPosterPresetDefaults(t *testing.T) {
	code, out, errb := runStdin(t, stackPNG, "--preset", "poster")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errb)
	}
	f := fills(out)
	if len(f) != 3 || f[2] != "#ffffff" {
		t.Fatalf("fills %v, want blue, red, #ffffff", f)
	}
}

// Without --filter-speckle the photo preset's 16-px despeckle removes a
// 9-px speck that the bw default of 4 would keep.
func TestRunPresetSeedsDefaults(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			c := color.NRGBA{255, 255, 255, 255}
			if (x >= 8 && x < 40 && y >= 8 && y < 40) || (x >= 50 && x < 53 && y >= 50 && y < 53) {
				c = color.NRGBA{0, 0, 0, 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	in := filepath.Join(t.TempDir(), "speck.png")
	f, err := os.Create(in)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()
	for _, tc := range []struct {
		args     []string
		subpaths int
	}{
		{[]string{"--preset", "photo", "--mode", "pixel"}, 1},
		{[]string{"--preset", "photo", "--mode", "pixel", "--filter-speckle", "4"}, 2},
	} {
		code, out, errb := runStdin(t, in, tc.args...)
		if code != 0 {
			t.Fatalf("%v: exit %d, stderr %q", tc.args, code, errb)
		}
		if n := strings.Count(out, "M"); n != tc.subpaths {
			t.Errorf("%v: %d subpaths, want %d", tc.args, n, tc.subpaths)
		}
	}
}

func TestRunPaletteFlag(t *testing.T) {
	code, out, errb := runStdin(t, abutPNG, "--preset", "poster", "--palette", "#ff0000,#0000ff")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errb)
	}
	if f := fills(out); len(f) != 2 || f[0] != "#ff0000" || f[1] != "#0000ff" {
		t.Fatalf("fills %v, want exactly the palette", f)
	}
	if code, _, errb := runStdin(t, abutPNG, "--preset", "poster", "--palette", "zz"); code != 1 || !strings.Contains(errb, "palette") {
		t.Fatalf("--palette zz: exit %d, stderr %q, want 1", code, errb)
	}
}

func TestRunHierarchicalCutout(t *testing.T) {
	code, out, errb := runStdin(t, logoPNG, "--preset", "poster", "--hierarchical", "cutout")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errb)
	}
	found := false
	for _, d := range regexp.MustCompile(` d="([^"]*)"`).FindAllStringSubmatch(out, -1) {
		if strings.Count(d[1], "M") == 2 {
			found = true
		}
	}
	if !found {
		t.Fatalf("no path with two subpaths (outer and hole) in %.200q", out)
	}
	if code, out, _ := runStdin(t, logoPNG, "--preset", "poster", "--hierarchical", "sideways"); code != 1 {
		t.Fatalf("bad hierarchical: exit %d, want 1 (%.80q)", code, out)
	}
}

const circlePNG = "../../testdata/corpus/synthetic/circle-aa.png"

func TestRunRefineFlag(t *testing.T) {
	code, out, errb := runStdin(t, circlePNG, "--preset", "bw", "--refine", "--refine-passes", "2")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errb)
	}
	ds := regexp.MustCompile(` d="([^"]*)"`).FindAllStringSubmatch(out, -1)
	if len(ds) != 1 {
		t.Fatalf("want one path, got %d in %.200q", len(ds), out)
	}
	if n := strings.Count(ds[0][1], "C"); n != 4 {
		t.Fatalf("refined circle has %d cubics, want 4: %q", n, ds[0][1])
	}
	if code, _, errb := runStdin(t, circlePNG, "--refine", "--refine-passes", "0"); code != 1 || !strings.Contains(errb, "Refine") {
		t.Fatalf("bad passes: exit %d, stderr %q", code, errb)
	}
}

func TestRunCutoutStrokeFlag(t *testing.T) {
	code, out, errb := runStdin(t, logoPNG, "--preset", "poster", "--hierarchical", "cutout")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errb)
	}
	if !strings.Contains(out, `stroke-width="0.5"`) {
		t.Fatalf("default poster cutout should carry stroke-width 0.5: %.300q", out)
	}
	code, out, errb = runStdin(t, logoPNG, "--preset", "poster", "--hierarchical", "cutout", "--cutout-stroke", "0")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errb)
	}
	if strings.Contains(out, "stroke") {
		t.Fatalf("--cutout-stroke 0 should emit no stroke attributes: %.300q", out)
	}
}

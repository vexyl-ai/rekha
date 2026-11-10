//go:build !js

package bench

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func blackImage(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{0, 0, 0, 255})
		}
	}
	return img
}

func writePNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func stubRaster(w, h int) func(context.Context, string, int, int) (image.Image, error) {
	return func(context.Context, string, int, int) (image.Image, error) {
		return blackImage(w, h), nil
	}
}

func newRunner(t *testing.T, raster func(context.Context, string, int, int) (image.Image, error)) *Runner {
	t.Helper()
	return &Runner{Rasterize: raster, Timeout: 10 * time.Second, WorkDir: t.TempDir()}
}

func cpTool() Tool {
	abs, _ := filepath.Abs("testdata/fixture.svg")
	return Tool{Name: "fake", Bin: "cp", Args: []string{abs, "{out}"}, Input: "png"}
}

func TestRunSkipsMissingTool(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, filepath.Join(dir, "a.png"), blackImage(8, 8))
	r := newRunner(t, stubRaster(8, 8))
	rs, err := r.Run(context.Background(), dir, []Tool{{Name: "nope", Bin: "definitely-not-a-binary-xyz", Input: "png"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 || rs[0].Skipped != "not installed" {
		t.Fatalf("got %+v", rs)
	}
}

func TestRunIgnoresNonPNG(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, filepath.Join(dir, "a.png"), blackImage(256, 256))
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "c.jpg"), []byte("x"), 0o644)
	r := newRunner(t, stubRaster(256, 256))
	rs, err := r.Run(context.Background(), dir, []Tool{cpTool()})
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 || !strings.HasSuffix(rs[0].Image, "a.png") {
		t.Fatalf("got %+v", rs)
	}
}

func TestRunEmptyDir(t *testing.T) {
	r := newRunner(t, stubRaster(1, 1))
	rs, err := r.Run(context.Background(), t.TempDir(), []Tool{cpTool()})
	if err != nil || len(rs) != 0 {
		t.Fatalf("got %d results, err %v", len(rs), err)
	}
}

func TestRunFakeToolEndToEnd(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, filepath.Join(dir, "a.png"), blackImage(256, 256))
	r := newRunner(t, stubRaster(256, 256))
	rs, err := r.Run(context.Background(), dir, []Tool{cpTool()})
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 {
		t.Fatalf("got %d results", len(rs))
	}
	got := rs[0]
	if got.Skipped != "" || got.Err != "" {
		t.Fatalf("unexpected skip/err: %+v", got)
	}
	if !math.IsInf(got.PSNR, 1) {
		t.Fatalf("PSNR = %v, want +Inf", got.PSNR)
	}
	if got.Stats.Paths != 1 || got.Stats.Nodes != 5 {
		t.Fatalf("stats = %+v", got.Stats)
	}
}

func TestRunDimensionMismatchIsRowError(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, filepath.Join(dir, "a.png"), blackImage(256, 256))
	r := newRunner(t, stubRaster(100, 100))
	rs, err := r.Run(context.Background(), dir, []Tool{cpTool()})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rs[0].Err, "dimensions differ") || !math.IsNaN(rs[0].PSNR) {
		t.Fatalf("got %+v", rs[0])
	}
}

func TestRunToolTimeout(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, filepath.Join(dir, "a.png"), blackImage(8, 8))
	r := newRunner(t, stubRaster(8, 8))
	r.Timeout = 100 * time.Millisecond
	rs, err := r.Run(context.Background(), dir, []Tool{{Name: "slow", Bin: "sleep", Args: []string{"5"}, Input: "png"}})
	if err != nil {
		t.Fatal(err)
	}
	if rs[0].Err == "" || rs[0].Millis >= 2000 {
		t.Fatalf("got %+v", rs[0])
	}
}

func TestWriteMarkdownFormats(t *testing.T) {
	rs := []Result{
		{Image: "a.png", Tool: "x", PSNR: math.Inf(1)},
		{Image: "b.png", Tool: "x", PSNR: math.NaN(), Err: "boom happened"},
	}
	var buf bytes.Buffer
	if err := WriteMarkdown(&buf, rs); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "| inf |") || !strings.Contains(out, "boom happened") {
		t.Fatalf("markdown:\n%s", out)
	}
}

func TestWriteMarkdownEscapesCells(t *testing.T) {
	rs := []Result{{Image: "a|b.png", Tool: "x", PSNR: math.NaN(), Err: "line one | pipe\nline two\r\nline three"}}
	var buf bytes.Buffer
	if err := WriteMarkdown(&buf, rs); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("want header, separator and one row, got %d lines:\n%s", len(lines), buf.String())
	}
	row := lines[2]
	if !strings.Contains(row, `a\|b.png`) || !strings.Contains(row, `line one \| pipe line two line three`) {
		t.Fatalf("row not escaped/flattened: %s", row)
	}
}

func TestRunErrorsDoNotLeakWorkDir(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, filepath.Join(dir, "a.png"), blackImage(8, 8))
	r := newRunner(t, stubRaster(8, 8))
	// cat fails on the not-yet-existing out.svg and names it on stderr.
	rs, err := r.Run(context.Background(), dir, []Tool{{Name: "cat", Bin: "cat", Args: []string{"{out}"}, Input: "png"}})
	if err != nil {
		t.Fatal(err)
	}
	if rs[0].Err == "" {
		t.Fatal("expected a row error")
	}
	if strings.Contains(rs[0].Err, r.WorkDir) {
		t.Fatalf("error leaks work dir: %s", rs[0].Err)
	}
}

func TestWriteCSVHeader(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteCSV(&buf, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(buf.String(), "image,tool,psnr,paths,nodes,bytes,ms,skipped,err\n") {
		t.Fatalf("csv:\n%s", buf.String())
	}
}

func TestCheckRasterizerMessage(t *testing.T) {
	t.Setenv("PATH", "")
	err := CheckRasterizer()
	if err == nil || !strings.Contains(err.Error(), "rsvg-convert") || !strings.Contains(err.Error(), "brew install librsvg") {
		t.Fatalf("got %v", err)
	}
}

func TestRekhaToolArgs(t *testing.T) {
	tool := RekhaTool("/x/rekha")
	if tool.Name != "rekha" || tool.Bin != "/x/rekha" || tool.Input != "png" {
		t.Fatalf("got %+v", tool)
	}
	want := []string{"--preset", "bw", "-o", "{out}", "{in}"}
	if strings.Join(tool.Args, " ") != strings.Join(want, " ") {
		t.Fatalf("Args = %v, want %v", tool.Args, want)
	}
}

func TestByNameKnowsRekha(t *testing.T) {
	tools, unknown := ByName([]string{"rekha", "potrace"})
	if len(unknown) != 0 || len(tools) != 2 || tools[0].Name != "rekha" {
		t.Fatalf("tools %v unknown %v", tools, unknown)
	}
}

func TestRekhaToolsPresets(t *testing.T) {
	tools := RekhaTools("/x/rekha")
	want := map[string]string{
		"rekha": "--preset bw", "rekha-poster": "--preset poster", "rekha-photo": "--preset photo",
		"rekha-refine": "--preset bw --refine", "rekha-poster-refine": "--preset poster --refine",
		"rekha-photo-gradients": "--preset photo --gradients",
	}
	if len(tools) != len(want) {
		t.Fatalf("got %d tools", len(tools))
	}
	for _, tool := range tools {
		head, ok := want[tool.Name]
		if !ok || tool.Bin != "/x/rekha" || tool.Input != "png" {
			t.Fatalf("unexpected tool %+v", tool)
		}
		if got := strings.Join(tool.Args, " "); got != head+" -o {out} {in}" {
			t.Fatalf("%s args %q", tool.Name, got)
		}
	}
}

func TestByNameKnowsRekhaPoster(t *testing.T) {
	tools, unknown := ByName([]string{"rekha-poster", "vtracer", "rekha-photo"})
	if len(unknown) != 0 || len(tools) != 3 || tools[0].Name != "rekha-poster" || tools[2].Name != "rekha-photo" {
		t.Fatalf("tools %v unknown %v", tools, unknown)
	}
}

// refTool is a reference tool whose outputs live in dir (no binary).
func refTool(dir string) Tool { return Tool{Name: "ref", Dir: dir} }

func TestReferenceToolSVG(t *testing.T) {
	corpus, ref := t.TempDir(), t.TempDir()
	writePNG(t, filepath.Join(corpus, "a.png"), blackImage(256, 256))
	svg, err := os.ReadFile("testdata/fixture.svg")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(ref, "a.svg"), svg, 0o644)
	r := newRunner(t, stubRaster(256, 256))
	rs, err := r.Run(context.Background(), corpus, []Tool{refTool(ref)})
	if err != nil {
		t.Fatal(err)
	}
	got := rs[0]
	if got.Skipped != "" || got.Err != "" || !math.IsInf(got.PSNR, 1) {
		t.Fatalf("got %+v", got)
	}
	if got.Stats.Paths != 1 || got.Stats.Nodes != 5 || got.Millis != 0 {
		t.Fatalf("stats %+v millis %d", got.Stats, got.Millis)
	}
}

func TestReferenceToolPNG(t *testing.T) {
	corpus, ref := t.TempDir(), t.TempDir()
	writePNG(t, filepath.Join(corpus, "a.png"), blackImage(16, 16))
	writePNG(t, filepath.Join(ref, "a.png"), blackImage(16, 16))
	r := newRunner(t, func(context.Context, string, int, int) (image.Image, error) {
		t.Fatal("rasteriser called for a PNG reference")
		return nil, nil
	})
	rs, err := r.Run(context.Background(), corpus, []Tool{refTool(ref)})
	if err != nil {
		t.Fatal(err)
	}
	got := rs[0]
	if got.Skipped != "" || got.Err != "" || !math.IsInf(got.PSNR, 1) {
		t.Fatalf("got %+v", got)
	}
	if got.Stats.Paths != 0 || got.Stats.Nodes != 0 || got.Stats.Bytes != 0 || got.Millis != 0 {
		t.Fatalf("stats %+v millis %d", got.Stats, got.Millis)
	}
}

func TestReferenceToolMissing(t *testing.T) {
	corpus := t.TempDir()
	writePNG(t, filepath.Join(corpus, "a.png"), blackImage(8, 8))
	r := newRunner(t, stubRaster(8, 8))
	rs, err := r.Run(context.Background(), corpus, []Tool{refTool(t.TempDir())})
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 || rs[0].Skipped != "no reference" || rs[0].Err != "" || !math.IsNaN(rs[0].PSNR) {
		t.Fatalf("got %+v", rs)
	}
}

func TestReferenceToolSizeMismatch(t *testing.T) {
	corpus, ref := t.TempDir(), t.TempDir()
	writePNG(t, filepath.Join(corpus, "a.png"), blackImage(16, 16))
	writePNG(t, filepath.Join(ref, "a.png"), blackImage(8, 8))
	r := newRunner(t, stubRaster(16, 16))
	rs, err := r.Run(context.Background(), corpus, []Tool{refTool(ref)})
	if err != nil {
		t.Fatal(err)
	}
	if rs[0].Err == "" || !math.IsNaN(rs[0].PSNR) {
		t.Fatalf("got %+v", rs[0])
	}
}

func TestReferenceToolSVGViewBoxMismatch(t *testing.T) {
	corpus, ref := t.TempDir(), t.TempDir()
	writePNG(t, filepath.Join(corpus, "a.png"), blackImage(64, 64))
	os.WriteFile(filepath.Join(ref, "a.svg"), []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 100"><path d="M0 0h200v100H0z"/></svg>`), 0o644)
	r := newRunner(t, stubRaster(64, 64))
	rs, err := r.Run(context.Background(), corpus, []Tool{refTool(ref)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rs[0].Err, "200x100") || !math.IsNaN(rs[0].PSNR) {
		t.Fatalf("got %+v", rs[0])
	}
}

func TestReferenceToolSVGUniformScaleAccepted(t *testing.T) {
	corpus, ref := t.TempDir(), t.TempDir()
	writePNG(t, filepath.Join(corpus, "a.png"), blackImage(64, 64))
	os.WriteFile(filepath.Join(ref, "a.svg"), []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="128pt" height="128pt"><path d="M0 0h128v128H0z"/></svg>`), 0o644)
	r := newRunner(t, stubRaster(64, 64))
	rs, err := r.Run(context.Background(), corpus, []Tool{refTool(ref)})
	if err != nil {
		t.Fatal(err)
	}
	if rs[0].Err != "" || !math.IsInf(rs[0].PSNR, 1) {
		t.Fatalf("got %+v", rs[0])
	}
}

func TestReferenceToolRasterisesInScratch(t *testing.T) {
	corpus, ref := t.TempDir(), t.TempDir()
	writePNG(t, filepath.Join(corpus, "a.png"), blackImage(64, 64))
	svg, _ := os.ReadFile("testdata/fixture.svg")
	os.WriteFile(filepath.Join(ref, "a.svg"), svg, 0o644)
	var seen string
	r := newRunner(t, func(_ context.Context, p string, w, h int) (image.Image, error) {
		seen = p
		return blackImage(w, h), nil
	})
	rs, err := r.Run(context.Background(), corpus, []Tool{refTool(ref)})
	if err != nil {
		t.Fatal(err)
	}
	if rs[0].Err != "" {
		t.Fatalf("got %+v", rs[0])
	}
	if strings.HasPrefix(seen, ref) || !strings.HasPrefix(seen, r.WorkDir) {
		t.Fatalf("rasterised %q; want a copy under %q, not the reference directory", seen, r.WorkDir)
	}
}

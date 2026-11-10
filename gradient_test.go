package rekha

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/vexyl-ai/rekha/internal/bench"
	"github.com/vexyl-ai/rekha/internal/corpus"
	"github.com/vexyl-ai/rekha/internal/metrics"
)

func photoGradients() Options {
	o := PresetOptions(PresetPhoto)
	o.Gradients = true
	return o
}

// traceSVG renders the synthetic, traces it under o and returns the SVG.
func traceSVG(t *testing.T, name string, o Options) string {
	t.Helper()
	return traceImageSVG(t, synth(t, name), o)
}

func traceImageSVG(t *testing.T, img image.Image, o Options) string {
	t.Helper()
	r, err := Trace(img, o)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := r.WriteSVG(&buf, o); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestGradientHCollapses(t *testing.T) {
	svg := traceSVG(t, "gradient-h", photoGradients())
	if n := strings.Count(svg, "<path"); n > 3 {
		t.Errorf("%d paths", n)
	}
	if !strings.Contains(svg, "<linearGradient") {
		t.Error("no gradient")
	}
}

func TestGradientShapesFewerPaths(t *testing.T) {
	flat := traceSVG(t, "gradient-shapes", PresetOptions(PresetPhoto))
	grad := traceSVG(t, "gradient-shapes", photoGradients())
	if a, b := strings.Count(flat, "<path"), strings.Count(grad, "<path"); b*3 > a {
		t.Errorf("flat %d paths, gradients %d", a, b)
	}
	// The rectangle's mean red moves by one count when the ramp's red
	// band shares its label in the flat trace.
	for _, s := range []string{flat, grad} {
		if !strings.Contains(s, "fill=\"#dc1e1e\"") && !strings.Contains(s, "fill=\"#dc1f1f\"") {
			t.Error("red rectangle lost")
		}
	}
}

func TestGradientsFlatArtUnchanged(t *testing.T) {
	for _, s := range corpus.Small() {
		if s.Name == "gradient-h" || s.Name == "gradient-shapes" {
			continue
		}
		off := PresetOptions(PresetPoster)
		on := off
		on.Gradients = true
		if a, b := traceSVG(t, s.Name, off), traceSVG(t, s.Name, on); a != b {
			t.Errorf("%s changed with gradients on", s.Name)
		}
	}
}

func TestGradientsBigPosterUnchanged(t *testing.T) {
	// 2048² flat art under the photo preset: small pockets between
	// shapes must not become ramps.
	off := PresetOptions(PresetPhoto)
	on := off
	on.Gradients = true
	img := synth(t, "big-poster")
	if a, b := traceImageSVG(t, img, off), traceImageSVG(t, img, on); a != b {
		t.Error("big-poster changed with gradients on")
	}
}

func TestGradientsDeterministic(t *testing.T) {
	prev := runtime.GOMAXPROCS(1)
	a := traceSVG(t, "gradient-shapes", photoGradients())
	runtime.GOMAXPROCS(8)
	b := traceSVG(t, "gradient-shapes", photoGradients())
	runtime.GOMAXPROCS(prev)
	if a != b {
		t.Error("output depends on GOMAXPROCS")
	}
}

func TestRampPageNotOmitted(t *testing.T) {
	// A white-to-light-grey ramp over the whole page: one region at
	// depth 0, near white on average, but a ramp, so it is emitted.
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			v := uint8(255 - x*40/63)
			img.SetNRGBA(x, y, color.NRGBA{v, v, v, 255})
		}
	}
	res, err := Trace(img, photoGradients())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Paths) != 1 || !strings.HasPrefix(res.Paths[0].Fill, "url(#") {
		t.Errorf("paths %+v", res.Paths)
	}
}

func TestSecondRemapAfterMerge(t *testing.T) {
	s, err := Segment(synth(t, "gradient-h"), photoGradients())
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Ramps) != len(s.Palette) {
		t.Fatalf("%d ramps for %d colours", len(s.Ramps), len(s.Palette))
	}
	for p := range s.Labels {
		if int(s.Second[p]) >= len(s.Palette) {
			t.Fatalf("second %d past palette %d", s.Second[p], len(s.Palette))
		}
		if s.Ramps[s.Labels[p]] != nil && (s.Blend[p] != 255 || s.Second[p] != s.Labels[p]) {
			t.Fatalf("pixel %d inside a ramp: second %d blend %d", p, s.Second[p], s.Blend[p])
		}
	}
}

func TestGradientOptionsValidate(t *testing.T) {
	o := DefaultOptions()
	if o.GradientTolerance != 0.03 || o.Gradients {
		t.Errorf("defaults %+v", o)
	}
	o.GradientTolerance = 1.5
	if o.Validate() == nil {
		t.Error("1.5 accepted")
	}
}

// psnrOf traces img under o, rasterises the SVG with rsvg-convert at the
// image's size and returns the PSNR against img.
func psnrOf(t *testing.T, img image.Image, o Options) (psnr float64, paths, bytes int) {
	t.Helper()
	s := traceImageSVG(t, img, o)
	dir := t.TempDir()
	path := filepath.Join(dir, "out.svg")
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	got, err := bench.RSVGConvert(context.Background(), path, b.Dx(), b.Dy())
	if err != nil {
		t.Fatal(err)
	}
	psnr, err = metrics.PSNR(got, img)
	if err != nil {
		t.Fatal(err)
	}
	return psnr, strings.Count(s, "<path"), len(s)
}

func TestGradientsPSNR(t *testing.T) {
	if err := bench.CheckRasterizer(); err != nil {
		t.Skip(err)
	}
	for _, name := range []string{"gradient-h", "gradient-shapes"} {
		img := synth(t, name)
		flat, fp, _ := psnrOf(t, img, PresetOptions(PresetPhoto))
		grad, gp, _ := psnrOf(t, img, photoGradients())
		t.Logf("%s: flat %.2f dB %d paths, gradients %.2f dB %d paths", name, flat, fp, grad, gp)
		if grad < flat-0.05 {
			t.Errorf("%s: gradients %.2f dB below flat %.2f dB", name, grad, flat)
		}
	}
}

// TestBirdReport prints the photo preset with and without gradients on
// the image REKHA_BIRD names; it asserts nothing.
func TestBirdReport(t *testing.T) {
	path := os.Getenv("REKHA_BIRD")
	if path == "" {
		t.Skip("REKHA_BIRD not set")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	raster := bench.CheckRasterizer() == nil
	for _, c := range []struct {
		name string
		o    Options
	}{{"photo", PresetOptions(PresetPhoto)}, {"photo+gradients", photoGradients()}} {
		start := time.Now()
		svg := traceImageSVG(t, img, c.o)
		ms := time.Since(start).Milliseconds()
		line := fmt.Sprintf("%s: %d paths, %d bytes, %d ms", c.name, strings.Count(svg, "<path"), len(svg), ms)
		if raster {
			p, _, _ := psnrOf(t, img, c.o)
			line += fmt.Sprintf(", %.2f dB", p)
		}
		t.Log(line)
	}
}

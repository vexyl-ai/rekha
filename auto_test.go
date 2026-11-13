package rekha

import (
	"bytes"
	"context"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/vexyl-ai/rekha/internal/bench"
	"github.com/vexyl-ai/rekha/internal/metrics"
	"github.com/vexyl-ai/rekha/internal/par"
	"github.com/vexyl-ai/rekha/internal/render"
)

func TestAutoCandidatesCoverTheRace(t *testing.T) {
	cs := autoCandidates(Options{})
	if len(cs) != 4 {
		t.Fatalf("%d candidates, want 4", len(cs))
	}
	want := []struct {
		preset    Preset
		tol       float64
		gradients bool
		speckle   int
		auto      bool
	}{
		{PresetPoster, 0.06, false, 4, false},
		{PresetPoster, 0.04, true, 4, false},
		{PresetPhoto, 0.04, true, 2, false},
		{PresetBW, 0, false, 4, true},
	}
	for i, c := range cs {
		w := want[i]
		if c.Preset != w.preset || c.ColorTolerance != w.tol || c.Gradients != w.gradients || c.FilterSpeckle != w.speckle || c.AutoThreshold != w.auto {
			t.Errorf("candidate %d: %+v", i, c)
		}
		if !c.Refine.Enabled || c.Mode != ModeSpline || c.Auto {
			t.Errorf("candidate %d: refine %v mode %q auto %v", i, c.Refine.Enabled, c.Mode, c.Auto)
		}
		if err := c.Validate(); err != nil {
			t.Errorf("candidate %d: %v", i, err)
		}
	}
	pal := []color.NRGBA{{0, 0, 0, 255}, {255, 255, 255, 255}}
	cs = autoCandidates(Options{Palette: pal})
	if len(cs) != 5 || len(cs[4].Palette) != 2 || cs[4].Preset != PresetPoster || !cs[4].Refine.Enabled {
		t.Fatalf("palette candidate: %d candidates, last %+v", len(cs), cs[len(cs)-1])
	}
	for _, c := range cs[:4] {
		if c.Palette != nil {
			t.Error("palette leaked into a quantising candidate")
		}
	}
}

func TestAutoCandidatesCarryOutputOptions(t *testing.T) {
	seen := 0
	o := Options{PathPrecision: 4, NoMark: true, Stroke: 1.5, Format: FormatPDF,
		ExternalMasks: []Mask{{Coverage: []uint8{255}}},
		Progress:      func(string, int, int) { seen++ }}
	for i, c := range autoCandidates(o) {
		if c.PathPrecision != 4 || !c.NoMark || c.Stroke != 1.5 || c.Format != FormatPDF || len(c.ExternalMasks) != 1 || c.Progress == nil {
			t.Errorf("candidate %d lost output options: %+v", i, c)
		}
	}
}

func TestAutoPicksTheBestScore(t *testing.T) {
	img := synth(t, "gradient-shapes")
	best := math.Inf(-1)
	for _, c := range autoCandidates(Options{}) {
		r, err := Trace(img, c)
		if err != nil {
			t.Fatal(err)
		}
		best = max(best, score(img, r, c))
	}
	r, chosen, err := Auto(img, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := score(img, r, chosen); got != best {
		t.Errorf("auto scored %.2f dB, best candidate %.2f", got, best)
	}
	if chosen.Auto || !chosen.Refine.Enabled {
		t.Errorf("chosen options: %+v", chosen)
	}
	t.Logf("auto: %s tolerance %.2f gradients %v, %.2f dB", chosen.Preset, chosen.ColorTolerance, chosen.Gradients, best)
}

func TestAutoWorkersFollowTheImageSize(t *testing.T) {
	cases := []struct{ n, pixels, want int }{
		{4, 1 << 20, 4}, {4, 4 << 20, 4}, {5, 4 << 20, 4}, {4, 8 << 20, 2}, {4, 16 << 20, 1}, {4, 64 << 20, 1}, {4, 0, 4},
	}
	for _, c := range cases {
		if got := autoWorkers(c.n, c.pixels); got != min(c.want, par.Workers(c.n)) {
			t.Errorf("autoWorkers(%d, %d) = %d, want %d", c.n, c.pixels, got, min(c.want, par.Workers(c.n)))
		}
	}
}

func TestAutoIsDeterministic(t *testing.T) {
	img := synth(t, "gradient-shapes")
	var svgs [2]string
	for i := range svgs {
		r, o, err := Auto(img, Options{NoMark: true})
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if err := r.WriteSVG(&buf, o); err != nil {
			t.Fatal(err)
		}
		svgs[i] = buf.String()
	}
	if svgs[0] != svgs[1] {
		t.Error("two runs of Auto differ")
	}
}

func TestTraceHonoursAuto(t *testing.T) {
	img := synth(t, "star-5")
	o := Options{Auto: true, NoMark: true}
	viaTrace := traceImageSVG(t, img, o)
	r, chosen, err := Auto(img, o)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := r.WriteSVG(&buf, chosen); err != nil {
		t.Fatal(err)
	}
	if viaTrace != buf.String() {
		t.Error("Trace with Auto set differs from Auto")
	}
}

func TestAutoReportsProgress(t *testing.T) {
	img := synth(t, "star-5")
	var done, total int
	stages := map[string]bool{}
	_, _, err := Auto(img, Options{Progress: func(stage string, d, n int) {
		stages[stage] = true
		if stage == "auto" {
			done, total = d, n
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !stages["auto"] || done != 4 || total != 4 {
		t.Errorf("auto progress: seen %v, done %d of %d", stages["auto"], done, total)
	}
	if !stages["quantise"] || !stages["curves"] {
		t.Errorf("candidate stages not passed through: %v", stages)
	}
}

// TestPaintAgreesWithRSVG scores one gradient trace with the internal
// renderer and with rsvg-convert: the two renderings must agree with
// each other and give the same PSNR against the source to within half
// a decibel, so what --auto chooses is what the browser shows.
func TestPaintAgreesWithRSVG(t *testing.T) {
	if err := bench.CheckRasterizer(); err != nil {
		t.Skip(err)
	}
	img := synth(t, "gradient-shapes")
	o := photoGradients()
	o.Refine.Enabled = true
	r, err := Trace(img, o)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "out.svg")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.WriteSVG(f, o); err != nil {
		t.Fatal(err)
	}
	f.Close()
	ext, err := bench.RSVGConvert(context.Background(), path, r.Width, r.Height)
	if err != nil {
		t.Fatal(err)
	}
	mine := render.Paint(r.document(o))
	agree, _ := metrics.PSNR(ext, mine)
	ours, _ := metrics.PSNR(img, mine)
	theirs, _ := metrics.PSNR(img, ext)
	t.Logf("renderers agree at %.2f dB; against source %.2f (render) vs %.2f (rsvg)", agree, ours, theirs)
	if agree < 35 {
		t.Errorf("renderers agree only at %.2f dB", agree)
	}
	if math.Abs(ours-theirs) > 0.5 {
		t.Errorf("scores differ: %.2f vs %.2f dB", ours, theirs)
	}
}

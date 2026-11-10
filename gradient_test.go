package rekha

import (
	"bytes"
	"image"
	"image/color"
	"runtime"
	"strings"
	"testing"

	"github.com/vexyl-ai/rekha/internal/corpus"
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

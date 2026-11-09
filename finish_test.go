package rekha

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/vexyl-ai/rekha/internal/colour"
	"github.com/vexyl-ai/rekha/internal/corpus"
	"github.com/vexyl-ai/rekha/internal/rasttest"
)

func hexOf(c color.NRGBA) string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }

// paintLabels paints r's paths in order and reports the pixels whose
// painted index differs from s.Labels. Unpainted pixels take the palette
// index nearest white, which is what a transparent background renders as.
func paintLabels(t *testing.T, s *Segmentation, r *Result) int {
	t.Helper()
	pal := map[string]uint8{}
	white, best := uint8(255), 2.0
	for i, c := range s.Palette {
		pal[hexOf(c)] = uint8(i)
		if d := colour.Dist(labOf(c), labOf(color.NRGBA{255, 255, 255, 255})); d < best {
			white, best = uint8(i), d
		}
	}
	got := rasttest.Paint(s.Width, s.Height, r.Paths, pal)
	mis := 0
	for p, l := range got {
		if l == 255 {
			l = white
		}
		if l != s.Labels[p] {
			mis++
		}
	}
	return mis
}

func TestFinishBWEqualsPhase1(t *testing.T) {
	for _, syn := range corpus.All() {
		o := DefaultOptions()
		s, err := Segment(corpus.Render(syn), o)
		if err != nil {
			t.Fatal(err)
		}
		r, err := s.Finish(o)
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if err := r.WriteSVG(&buf, o); err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile(filepath.Join("testdata", "golden", syn.Name+"-spline.svg"))
		if err != nil {
			t.Fatal(err)
		}
		if !svgClose(want, buf.Bytes()) {
			t.Errorf("%s: Finish differs from the Phase 1 golden", syn.Name)
		}
	}
}

func testPixelModeReproducesLabels(t *testing.T, h Hierarchical) {
	t.Helper()
	for _, syn := range corpus.All() {
		o := PresetOptions(PresetPoster)
		o.Mode = ModePixel
		o.Hierarchical = h
		s, err := Segment(corpus.Render(syn), o)
		if err != nil {
			t.Fatal(err)
		}
		r, err := s.Finish(o)
		if err != nil {
			t.Fatal(err)
		}
		if mis := paintLabels(t, s, r); mis != 0 {
			t.Errorf("%s (%s): %d pixels differ from Labels", syn.Name, h, mis)
		}
	}
}

func TestStackedPixelModeReproducesLabels(t *testing.T) {
	testPixelModeReproducesLabels(t, HierarchicalStacked)
}

func TestCutoutPixelModeReproducesLabels(t *testing.T) {
	testPixelModeReproducesLabels(t, HierarchicalCutout)
}

func TestStackedOuterOnly(t *testing.T) {
	o := PresetOptions(PresetPoster)
	r, err := Trace(synth(t, "three-layer-stack"), o)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Paths) != 3 {
		t.Fatalf("got %d paths, want blue, red, white", len(r.Paths))
	}
	want := []color.NRGBA{{30, 60, 220, 255}, {220, 30, 30, 255}, {255, 255, 255, 255}}
	for i, p := range r.Paths {
		if len(p.Subpaths) != 1 {
			t.Errorf("path %d has %d subpaths, want the outer only", i, len(p.Subpaths))
		}
		var c color.NRGBA
		if _, err := fmt.Sscanf(p.Fill, "#%02x%02x%02x", &c.R, &c.G, &c.B); err != nil {
			t.Fatalf("path %d fill %q", i, p.Fill)
		}
		c.A = 255
		if d := colour.Dist(labOf(c), labOf(want[i])); d > 0.02 {
			t.Errorf("path %d fill %s, want near %s", i, p.Fill, hexOf(want[i]))
		}
	}
}

func TestStackedSameColourBorderShapes(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	red := color.NRGBA{220, 30, 30, 255}
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			c := color.NRGBA{255, 255, 255, 255}
			if y >= 16 && y < 48 && (x < 16 || x >= 48) {
				c = red
			}
			img.SetNRGBA(x, y, c)
		}
	}
	o := PresetOptions(PresetPoster)
	o.Mode = ModePixel
	r, err := Trace(img, o)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Paths) != 1 || len(r.Paths[0].Subpaths) != 2 || r.Paths[0].Fill != hexOf(red) {
		t.Fatalf("got %d paths; first has %d subpaths, fill %q; want one red path with two subpaths", len(r.Paths), len(r.Paths[0].Subpaths), r.Paths[0].Fill)
	}
}

func TestFinishAllWhiteIsEmpty(t *testing.T) {
	o := PresetOptions(PresetPoster)
	r, err := Trace(image.NewNRGBA(image.Rect(0, 0, 16, 16)), o)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Paths) != 0 {
		t.Fatalf("got %d paths, want none", len(r.Paths))
	}
	var buf bytes.Buffer
	if err := r.WriteSVG(&buf, o); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(buf.String(), "<svg") || !strings.Contains(buf.String(), "</svg>") {
		t.Fatalf("not an SVG document: %q", buf.String())
	}
}

func TestFinishDeterministicAcrossProcs(t *testing.T) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(0))
	for _, tc := range []struct {
		name string
		o    Options
	}{{"three-layer-stack", PresetOptions(PresetPoster)}, {"noisy-disc", PresetOptions(PresetPhoto)}} {
		img := synth(t, tc.name)
		var out [2][]byte
		for i, procs := range []int{1, 8} {
			runtime.GOMAXPROCS(procs)
			r, err := Trace(img, tc.o)
			if err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			if err := r.WriteSVG(&buf, tc.o); err != nil {
				t.Fatal(err)
			}
			out[i] = buf.Bytes()
		}
		if !bytes.Equal(out[0], out[1]) {
			t.Errorf("%s: output differs between GOMAXPROCS 1 and 8", tc.name)
		}
	}
}

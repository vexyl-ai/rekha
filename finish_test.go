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
	"time"

	"github.com/vexyl-ai/rekha/internal/colour"
	"github.com/vexyl-ai/rekha/internal/corpus"
	"github.com/vexyl-ai/rekha/internal/fit"
	"github.com/vexyl-ai/rekha/internal/geom"
	"github.com/vexyl-ai/rekha/internal/rasttest"
	"github.com/vexyl-ai/rekha/internal/svg"
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
	for _, syn := range corpus.Small() {
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
	for _, syn := range corpus.Small() {
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

// refined returns o with refinement on.
func refined(o Options) Options {
	o.Refine.Enabled = true
	return o
}

// TestFinishDeterministicAcrossProcs: the output never depends on the
// worker count; big-poster (skipped under -short) exercises the refiner's
// parallel piece classes over hundreds of loops.
func TestFinishDeterministicAcrossProcs(t *testing.T) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(0))
	for _, tc := range []struct {
		name string
		o    Options
	}{{"three-layer-stack", PresetOptions(PresetPoster)}, {"noisy-disc", PresetOptions(PresetPhoto)}, {"big-poster", refined(PresetOptions(PresetPoster))}} {
		if tc.name == "big-poster" && testing.Short() {
			continue
		}
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

// Two abutting colours at the same depth must not leave a seam: the
// earlier sibling extends under the later one, so its path alone covers
// the whole shared border and a margin beyond it. Siblings that do not
// touch stay separate, or the later one's anti-aliased edge would blend
// into the wrong colour.
func TestStackedTouchingSiblingsExtendUnderLater(t *testing.T) {
	red, blue, white := color.NRGBA{220, 30, 30, 255}, color.NRGBA{30, 60, 220, 255}, color.NRGBA{255, 255, 255, 255}
	paint := func(img *image.NRGBA, inside func(x, y int) color.NRGBA) {
		b := img.Bounds()
		for y := 0; y < b.Dy(); y++ {
			for x := 0; x < b.Dx(); x++ {
				img.SetNRGBA(x, y, inside(x, y))
			}
		}
	}
	o := PresetOptions(PresetPoster)
	o.Mode = ModePixel

	abut := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	paint(abut, func(x, _ int) color.NRGBA {
		if x < 32 {
			return red
		}
		return blue
	})
	r, err := Trace(abut, o)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Paths) != 2 || r.Paths[0].Fill != hexOf(red) {
		t.Fatalf("got %d paths, first %q; want red then blue", len(r.Paths), r.Paths[0].Fill)
	}
	under := rasttest.Fill(64, 64, r.Paths[:1])
	if n := under.Count(); n <= 32*64 {
		t.Errorf("red path alone covers %d pixels, want a margin under the blue", n)
	}
	for y := 0; y < 64; y++ {
		if !under.Get(32, y) {
			t.Fatalf("red does not run under the blue edge at row %d", y)
		}
	}

	apart := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	paint(apart, func(x, y int) color.NRGBA {
		switch {
		case x >= 4 && x < 28 && y >= 20 && y < 44:
			return red
		case x >= 36 && x < 60 && y >= 20 && y < 44:
			return blue
		}
		return white
	})
	r, err = Trace(apart, o)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Paths) != 2 {
		t.Fatalf("got %d paths, want red and blue", len(r.Paths))
	}
	if n := rasttest.Fill(64, 64, r.Paths[:1]).Count(); n != 24*24 {
		t.Errorf("red path covers %d pixels, want just its own %d", n, 24*24)
	}
}

// The minority colour connects across a checkerboard corner, so a
// one-pixel diagonal line traces as one shape, whichever palette index
// it got.
func TestStackedDiagonalLineIsOneShape(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 255, 255, 255, 255
	}
	for i := 1; i < 64; i++ { // off the corner, so white is palette entry 0
		img.SetNRGBA(i, i, color.NRGBA{0, 0, 0, 255})
	}
	o := PresetOptions(PresetPoster)
	o.Mode = ModePixel
	o.FilterSpeckle = 0
	r, err := Trace(img, o)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Paths) != 1 || len(r.Paths[0].Subpaths) != 1 {
		n := 0
		for _, p := range r.Paths {
			n += len(p.Subpaths)
		}
		t.Fatalf("got %d paths, %d subpaths; want one black shape", len(r.Paths), n)
	}
}

// A fully transparent input flattens to white; neither hierarchy may turn
// it into a white rectangle.
func TestCutoutTransparentIsEmpty(t *testing.T) {
	o := PresetOptions(PresetPoster)
	o.Hierarchical = HierarchicalCutout
	r, err := Trace(image.NewNRGBA(image.Rect(0, 0, 16, 16)), o)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Paths) != 0 {
		t.Fatalf("got %d paths (first fill %q), want none", len(r.Paths), r.Paths[0].Fill)
	}
}

// Only the page is dropped: white that is a minority of the depth-0 area
// is content and is emitted even where it reaches a canvas edge.
func TestWhiteContentAtEdgeKept(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	navy := color.NRGBA{20, 30, 60, 255}
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			c := navy
			if y < 16 && x >= 16 && x < 48 {
				c = color.NRGBA{255, 255, 255, 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	for _, h := range []Hierarchical{HierarchicalStacked, HierarchicalCutout} {
		o := PresetOptions(PresetPoster)
		o.Hierarchical = h
		o.Mode = ModePixel
		r, err := Trace(img, o)
		if err != nil {
			t.Fatal(err)
		}
		white := false
		for _, p := range r.Paths {
			white = white || p.Fill == "#ffffff"
		}
		if !white {
			t.Errorf("%s: white block at the top edge was dropped (%d paths)", h, len(r.Paths))
		}
	}
}

// Colour paths carry a stroke in their own colour so abutting shapes
// overlap by half its width, in cutout and stacked output alike; the bw
// preset never does.
func TestCutoutPathsCarryStroke(t *testing.T) {
	img := synth(t, "aa-abut")
	o := PresetOptions(PresetPoster)
	o.Hierarchical = HierarchicalCutout
	o.Stroke = 0.5 // opt-in
	r, err := Trace(img, o)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Paths) == 0 {
		t.Fatal("no paths")
	}
	for i, p := range r.Paths {
		if p.Stroke != p.Fill || p.StrokeWidth != 0.5 {
			t.Fatalf("cutout path %d: stroke %q width %v, want fill %q and 0.5", i, p.Stroke, p.StrokeWidth, p.Fill)
		}
	}
	o.Hierarchical = HierarchicalStacked
	if r, err = Trace(img, o); err != nil {
		t.Fatal(err)
	}
	for i, p := range r.Paths {
		if p.Stroke != p.Fill || p.StrokeWidth != 0.5 {
			t.Fatalf("stacked path %d: stroke %q width %v, want fill %q and 0.5", i, p.Stroke, p.StrokeWidth, p.Fill)
		}
	}
	if r, err = Trace(synth(t, "circle-aa"), PresetOptions(PresetBW)); err != nil {
		t.Fatal(err)
	}
	for i, p := range r.Paths {
		if p.Stroke != "" {
			t.Fatalf("bw path %d has stroke %q", i, p.Stroke)
		}
	}
}

// Pixel mode stays exact: no stroke, so the pixel oracle holds.
func TestCutoutStrokeNotInPixelMode(t *testing.T) {
	o := PresetOptions(PresetPoster)
	o.Hierarchical = HierarchicalCutout
	o.Mode = ModePixel
	r, err := Trace(synth(t, "aa-abut"), o)
	if err != nil {
		t.Fatal(err)
	}
	for i, p := range r.Paths {
		if p.Stroke != "" {
			t.Fatalf("pixel-mode path %d has stroke %q", i, p.Stroke)
		}
	}
}

// cutoutSpline traces name under poster, cutout spline, refined when
// refine is set.
func cutoutSpline(t *testing.T, name string, refine bool) (*Segmentation, *Result) {
	t.Helper()
	o := PresetOptions(PresetPoster)
	o.Hierarchical = HierarchicalCutout
	o.Mode = ModeSpline
	o.Refine.Enabled = refine
	s, err := Segment(synth(t, name), o)
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Finish(o)
	if err != nil {
		t.Fatal(err)
	}
	return s, r
}

// checkSharedSegments: every segment of one path whose ends are vertices
// of the other appears in the other path with the same numbers,
// reversed; at least one such segment exists.
func checkSharedSegments(t *testing.T, r *Result) {
	t.Helper()
	if len(r.Paths) != 2 {
		t.Fatalf("%d paths, want 2", len(r.Paths))
	}
	segsOf := func(p svg.Path) []fit.Segment {
		var out []fit.Segment
		for _, sp := range p.Subpaths {
			out = append(out, sp.Segs...)
		}
		return out
	}
	a, b := segsOf(r.Paths[0]), segsOf(r.Paths[1])
	ends := map[geom.Vec2]bool{}
	for _, s := range b {
		ends[s.P[0]], ends[s.P[3]] = true, true
	}
	shared := 0
	for _, s := range a {
		if !ends[s.P[0]] || !ends[s.P[3]] {
			continue
		}
		found := false
		for _, u := range b {
			if u.Line == s.Line && u.P[0] == s.P[3] && u.P[1] == s.P[2] && u.P[2] == s.P[1] && u.P[3] == s.P[0] {
				found = true
			}
		}
		if !found {
			t.Errorf("segment %v→%v of path 0 has no reversed twin in path 1", s.P[0], s.P[3])
		}
		shared++
	}
	if shared == 0 {
		t.Fatal("no shared segment between the two paths")
	}
}

// TestCutoutSharedPathData: on aa-abut the red and blue shapes meet along
// one boundary and carry the same numbers for it.
func TestCutoutSharedPathData(t *testing.T) {
	_, r := cutoutSpline(t, "aa-abut", false)
	checkSharedSegments(t, r)
}

// TestSharedRefineIdenticalBothSides: the shared boundary is refined
// once, so both refined paths still carry the same numbers for it.
func TestSharedRefineIdenticalBothSides(t *testing.T) {
	_, r := cutoutSpline(t, "aa-abut", true)
	checkSharedSegments(t, r)
}

// borderDisc is a 64×64 white page with an anti-aliased blue half-disc
// cut by the left image border.
func borderDisc() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			in := 0
			for sy := 0; sy < 4; sy++ {
				for sx := 0; sx < 4; sx++ {
					dx, dy := float64(x)+(float64(sx)+0.5)/4, float64(y)+(float64(sy)+0.5)/4-32
					if dx*dx+dy*dy <= 20*20 {
						in++
					}
				}
			}
			a := uint8(in * 255 / 16)
			img.SetNRGBA(x, y, color.NRGBA{255 - a, 255 - a, 255, 255})
		}
	}
	return img
}

// TestSharedRefineBorderShape: a shape on the image border whose only
// neighbour is the omitted white page owns all its arcs; its refined
// path is closed and differs from the plain fit.
func TestSharedRefineBorderShape(t *testing.T) {
	o := PresetOptions(PresetPoster)
	o.Hierarchical = HierarchicalCutout
	o.Mode = ModeSpline
	s, err := Segment(borderDisc(), o)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := s.Finish(o)
	if err != nil {
		t.Fatal(err)
	}
	o.Refine.Enabled = true
	refined, err := s.Finish(o)
	if err != nil {
		t.Fatal(err)
	}
	if len(refined.Paths) != 1 || len(refined.Paths[0].Subpaths) != 1 {
		t.Fatalf("got %d paths, want 1 with 1 subpath", len(refined.Paths))
	}
	segs := refined.Paths[0].Subpaths[0].Segs
	for i := range segs {
		if d := segs[i].P[0].Sub(segs[(i+len(segs)-1)%len(segs)].P[3]).Len(); d > 1e-9 {
			t.Fatalf("segment %d disconnected by %g", i, d)
		}
	}
	var a, b bytes.Buffer
	plain.WriteSVG(&a, o)
	refined.WriteSVG(&b, o)
	if bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Fatal("refined output equals the plain fit")
	}
}

// TestCutoutWithin3dBOfStacked: refined cutout without stroke stays
// within 3 dB of refined stacked on the seam synthetics.
func TestCutoutWithin3dBOfStacked(t *testing.T) {
	for _, name := range []string{"aa-abut", "three-layer-stack", "logo-holes", "poster-discs"} {
		img := synth(t, name)
		psnr := func(h Hierarchical) float64 {
			o := PresetOptions(PresetPoster)
			o.Hierarchical = h
			o.Mode = ModeSpline
			o.Refine.Enabled = true
			s, err := Segment(img, o)
			if err != nil {
				t.Fatal(err)
			}
			r, err := s.Finish(o)
			if err != nil {
				t.Fatal(err)
			}
			return superPSNR(t, img, s, r)
		}
		cut, stack := psnr(HierarchicalCutout), psnr(HierarchicalStacked)
		t.Logf("%s: cutout %.2f dB, stacked %.2f dB", name, cut, stack)
		if cut < stack-3 {
			t.Errorf("%s: cutout %.2f dB is more than 3 dB below stacked %.2f dB", name, cut, stack)
		}
	}
}

// TestCutoutPixelSetsUnchanged: the shapes assembled from the planar map,
// painted as polygons in output order, reproduce the labels exactly.
func TestCutoutPixelSetsUnchanged(t *testing.T) {
	for _, name := range []string{"logo-holes", "poster-discs"} {
		o := PresetOptions(PresetPoster)
		o.Hierarchical = HierarchicalCutout
		o.Mode = ModeSpline
		s, err := Segment(synth(t, name), o)
		if err != nil {
			t.Fatal(err)
		}
		layers, pm := s.buildLayersMap(o)
		if pm == nil {
			t.Fatalf("%s: no planar map", name)
		}
		r := &Result{Width: s.Width, Height: s.Height}
		for _, l := range layers {
			p := svg.Path{Fill: hexOf(s.Palette[l.colour])}
			for _, c := range l.contours {
				p.Subpaths = append(p.Subpaths, svg.Subpath{Segs: lines(toVec(c.Pts))})
			}
			r.Paths = append(r.Paths, p)
		}
		if mis := paintLabels(t, s, r); mis != 0 {
			t.Errorf("%s: %d pixels differ from Labels", name, mis)
		}
	}
}

// TestCutoutCheckerboardFast: a 2-px checkerboard has a junction at every
// other vertex; the map and the fit of its thousands of arcs stay quick.
func TestCutoutCheckerboardFast(t *testing.T) {
	start := time.Now()
	_, r := cutoutSpline(t, "checkerboard-2px", false)
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("took %v", d)
	}
	if len(r.Paths) == 0 {
		t.Fatal("no paths")
	}
}

// TestJunctionOnFlankLines: aa-abut's diagonal x+y = 256 meets the image
// border at (256, 0) and (0, 256), a pixel from the crack vertices the
// labels put it at; both shapes start their shared edge within the
// segmentation's blend estimate of it.
func TestJunctionOnFlankLines(t *testing.T) {
	_, r := cutoutSpline(t, "aa-abut", false)
	for _, w := range []geom.Vec2{{X: 256, Y: 0}, {X: 0, Y: 256}} {
		for i, p := range r.Paths {
			hit := false
			for _, sp := range p.Subpaths {
				for _, s := range sp.Segs {
					if s.P[0].Sub(w).Len() < 0.25 {
						hit = true
					}
				}
			}
			if !hit {
				t.Errorf("path %d has no anchor within 0.25 px of %v", i, w)
			}
		}
	}
}

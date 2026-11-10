package rekha

import (
	"bytes"
	"flag"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/vexyl-ai/rekha/internal/bitmap"
	"github.com/vexyl-ai/rekha/internal/contour"
	"github.com/vexyl-ai/rekha/internal/corpus"
	"github.com/vexyl-ai/rekha/internal/fit"
	"github.com/vexyl-ai/rekha/internal/geom"
	"github.com/vexyl-ai/rekha/internal/rasttest"
	"github.com/vexyl-ai/rekha/internal/svg"
)

var update = flag.Bool("update", false, "rewrite golden files")

func synth(t *testing.T, name string) *image.NRGBA {
	t.Helper()
	for _, s := range corpus.All() {
		if s.Name == name {
			return corpus.Render(s)
		}
	}
	t.Fatalf("no synthetic %q", name)
	return nil
}

func TestTraceEmptyImageNoPaths(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	r, err := Trace(img, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Paths) != 0 {
		t.Fatalf("got %d paths", len(r.Paths))
	}
	var buf bytes.Buffer
	if err := r.WriteSVG(&buf, DefaultOptions()); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("<svg")) || bytes.Contains(buf.Bytes(), []byte("<path")) {
		t.Fatalf("svg:\n%s", buf.String())
	}
}

func TestTracePixelModeRoundTrip(t *testing.T) {
	o := DefaultOptions()
	o.Mode = ModePixel
	o.FilterSpeckle = 0
	for _, s := range corpus.Small() {
		img := corpus.Render(s)
		r, err := Trace(img, o)
		if err != nil {
			t.Fatal(err)
		}
		gray, w, h := bitmap.Gray(img)
		want := bitmap.FromGray(gray, w, h, 128)
		got := rasttest.Fill(w, h, r.Paths)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				if got.Get(x, y) != want.Get(x, y) {
					t.Fatalf("%s: pixel (%d,%d) got %v want %v", s.Name, x, y, got.Get(x, y), want.Get(x, y))
				}
			}
		}
	}
}

func TestTraceGolden(t *testing.T) {
	for _, s := range corpus.Small() {
		for _, mode := range []Mode{ModePixel, ModePolygon, ModeSpline} {
			o := DefaultOptions()
			o.Mode = mode
			r, err := Trace(corpus.Render(s), o)
			if err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			if err := r.WriteSVG(&buf, o); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join("testdata", "golden", s.Name+"-"+string(mode)+".svg")
			if *update {
				if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
					t.Fatal(err)
				}
				continue
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%s: %v (run with -update)", path, err)
			}
			if !svgClose(want, buf.Bytes()) {
				t.Errorf("%s differs from golden", path)
			}
		}
	}
}

var svgNumber = regexp.MustCompile(`-?[0-9]+(\.[0-9]+)?`)

// svgClose compares two SVG documents: identical apart from numbers, which
// may differ by one unit in the last printed decimal. Floating-point
// rounding differs between architectures (fused multiply-add on arm64), so
// goldens are not byte-exact across them.
func svgClose(a, b []byte) bool {
	na, nb := svgNumber.FindAllIndex(a, -1), svgNumber.FindAllIndex(b, -1)
	if len(na) != len(nb) {
		return false
	}
	pa, pb := 0, 0
	for i := range na {
		if !bytes.Equal(a[pa:na[i][0]], b[pb:nb[i][0]]) {
			return false
		}
		x, err1 := strconv.ParseFloat(string(a[na[i][0]:na[i][1]]), 64)
		y, err2 := strconv.ParseFloat(string(b[nb[i][0]:nb[i][1]]), 64)
		if err1 != nil || err2 != nil || math.Abs(x-y) > 0.011 {
			return false
		}
		pa, pb = na[i][1], nb[i][1]
	}
	return bytes.Equal(a[pa:], b[pb:])
}

func TestTraceDeterministic(t *testing.T) {
	img := synth(t, "star-5")
	var first []byte
	for i := 0; i < 3; i++ {
		r, err := Trace(img, DefaultOptions())
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		r.WriteSVG(&buf, DefaultOptions())
		if first == nil {
			first = buf.Bytes()
		} else if !bytes.Equal(first, buf.Bytes()) {
			t.Fatal("output differs between runs")
		}
	}
}

func TestTraceSplineClosesEveryPath(t *testing.T) {
	for _, s := range corpus.Small() {
		r, err := Trace(corpus.Render(s), DefaultOptions())
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range r.Paths {
			for _, sp := range p.Subpaths {
				if len(sp.Segs) == 0 {
					t.Fatalf("%s: empty subpath", s.Name)
				}
				first := sp.Segs[0].P[0]
				last := sp.Segs[len(sp.Segs)-1].P[3]
				if d := last.Sub(first).Len(); d > 1e-9 {
					t.Fatalf("%s: subpath not closed, gap %g", s.Name, d)
				}
				for i := 1; i < len(sp.Segs); i++ {
					if d := sp.Segs[i].P[0].Sub(sp.Segs[i-1].P[3]).Len(); d > 1e-9 {
						t.Fatalf("%s: segment %d disconnected", s.Name, i)
					}
				}
			}
		}
	}
}

func FuzzTrace(f *testing.F) {
	f.Add(uint8(1), uint8(1), []byte{0xff})
	f.Add(uint8(2), uint8(2), []byte{0b1001})
	f.Add(uint8(3), uint8(3), []byte{0b11111111, 0b1})
	f.Fuzz(func(t *testing.T, w, h uint8, data []byte) {
		W, H := int(w)%64+1, int(h)%64+1
		img := image.NewNRGBA(image.Rect(0, 0, W, H))
		for y := 0; y < H; y++ {
			for x := 0; x < W; x++ {
				i := y*W + x
				bit := len(data) > 0 && data[(i/8)%len(data)]&(1<<(i%8)) != 0
				c := color.NRGBA{255, 255, 255, 255}
				if bit {
					c = color.NRGBA{0, 0, 0, 255}
				}
				img.SetNRGBA(x, y, c)
			}
		}
		for _, mode := range []Mode{ModePixel, ModePolygon, ModeSpline} {
			for _, pol := range []TurnPolicy{TurnMinority, TurnMajority, TurnBlack, TurnWhite} {
				o := DefaultOptions()
				o.Mode, o.TurnPolicy, o.FilterSpeckle = mode, pol, 0
				checkTraceSane(t, img, o)
			}
		}
	})
}

// checkTraceSane traces img with o and checks the output against the
// traced shapes: every path closed and finite, no more segments than the
// crack polygon has corners (plus slack), and the filled result disagreeing
// with the pixel-exact fill on no more than three quarters of a boundary's
// worth of pixels. It returns the result for further checks.
func checkTraceSane(t *testing.T, img image.Image, o Options) *Result {
	t.Helper()
	r, err := Trace(img, o)
	if err != nil {
		t.Fatal(err)
	}
	gray, w, h := bitmap.Gray(img)
	th := uint8(math.Round(o.Threshold * 255))
	if o.AutoThreshold {
		th = bitmap.Otsu(gray)
	}
	bm := bitmap.FromGray(gray, w, h, th)
	shapes := contour.BuildShapes(contour.Trace(bm, o.TurnPolicy), o.FilterSpeckle)
	if len(shapes) != len(r.Paths) {
		t.Fatalf("%d paths for %d shapes", len(r.Paths), len(shapes))
	}
	perimeter := 0
	for k, p := range r.Paths {
		if len(p.Subpaths) == 0 {
			t.Fatal("path without subpaths")
		}
		loops := append([]contour.Contour{shapes[k].Outer}, shapes[k].Holes...)
		if len(loops) != len(p.Subpaths) {
			t.Fatalf("path %d: %d subpaths for %d loops", k, len(p.Subpaths), len(loops))
		}
		for j, sp := range p.Subpaths {
			perimeter += len(loops[j].Pts)
			if len(sp.Segs) == 0 {
				t.Fatalf("path %d subpath %d: no segments", k, j)
			}
			if d := sp.Segs[len(sp.Segs)-1].P[3].Sub(sp.Segs[0].P[0]).Len(); d > 1e-9 {
				t.Fatalf("path %d subpath %d: not closed, gap %g", k, j, d)
			}
			for i, sg := range sp.Segs {
				for _, pt := range sg.P {
					if math.IsNaN(pt.X) || math.IsNaN(pt.Y) || math.IsInf(pt.X, 0) || math.IsInf(pt.Y, 0) {
						t.Fatal("non-finite coordinate")
					}
				}
				if i > 0 && sg.P[0].Sub(sp.Segs[i-1].P[3]).Len() > 1e-9 {
					t.Fatalf("path %d subpath %d: segment %d disconnected", k, j, i)
				}
			}
			if o.Mode != ModePixel {
				if budget := len(geom.MergeCollinearIndices(loops[j].Pts)) + 4; len(sp.Segs) > budget {
					t.Fatalf("path %d subpath %d: %d segments for a %d-corner loop", k, j, len(sp.Segs), budget-4)
				}
			}
		}
	}
	if len(r.Paths) > 0 && o.Mode != ModePixel {
		// The reference is the despeckled shape set, which pixel mode
		// reproduces exactly.
		po := o
		po.Mode = ModePixel
		pr, err := Trace(img, po)
		if err != nil {
			t.Fatal(err)
		}
		want := rasttest.Fill(w, h, pr.Paths)
		got := rasttest.Fill(w, h, r.Paths)
		mis := 0
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				if got.Get(x, y) != want.Get(x, y) {
					mis++
				}
			}
		}
		// Three quarters of the boundary length: spline mode legitimately
		// moves edges by up to the fit tolerance, which on one-pixel
		// features is most of a shape; the dedicated tests pin fidelity on
		// clean shapes, this catches output that is nothing like the input.
		if limit := perimeter*3/4 + 8; mis > limit {
			t.Fatalf("%d mismatched pixels for a boundary of %d (limit %d)", mis, perimeter, limit)
		}
	}
	var buf bytes.Buffer
	if err := r.WriteSVG(&buf, o); err != nil {
		t.Fatal(err)
	}
	return r
}

// Shapes outside the corpus that each exposed a defect once: an obtuse
// polygon (vertices from straight edges only), regions touching at a
// pixel corner (a loop that revisits a vertex), a one-pixel diagonal (a
// sliver), and a checkerboard (every turn policy's worst case).
func TestTraceSaneShapes(t *testing.T) {
	var oct []geom.Vec2
	for i := 0; i < 8; i++ {
		a := math.Pi/8 + float64(i)*math.Pi/4
		oct = append(oct, geom.Vec2{X: 64 + 50*math.Cos(a), Y: 64 + 50*math.Sin(a)})
	}
	touching := bitmap.New(64, 64)
	diagonal := bitmap.New(64, 64)
	checker := bitmap.New(16, 16)
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			if x >= 8 && x < 32 && y >= 8 && y < 32 {
				touching.Set(x, y, true)
				touching.Set(x+24, y+24, true)
			}
			if x == y && x >= 4 && x < 60 {
				diagonal.Set(x, y, true)
			}
			if x < 16 && y < 16 && (x+y)%2 == 0 {
				checker.Set(x, y, true)
			}
		}
	}
	cases := map[string]image.Image{
		"octagon":  polygonImage(128, 128, oct),
		"touching": bitmapImage(touching),
		"diagonal": bitmapImage(diagonal),
		"checker":  bitmapImage(checker),
	}
	for _, s := range corpus.Small() {
		cases[s.Name] = corpus.Render(s)
	}
	for name, img := range cases {
		for _, mode := range []Mode{ModePolygon, ModeSpline} {
			for _, pol := range []TurnPolicy{TurnMinority, TurnMajority, TurnBlack, TurnWhite} {
				o := DefaultOptions()
				o.Mode, o.TurnPolicy = mode, pol
				t.Run(name+"/"+string(mode)+"/"+string(pol), func(t *testing.T) { checkTraceSane(t, img, o) })
			}
		}
	}
}

// A star's tips are two crack corners 2 px apart after thresholding; the
// tracer must merge them and place one sharp vertex where the flanks meet,
// so the spline is exactly the 10-vertex polygon.
func TestTraceSplineStarIsTenLines(t *testing.T) {
	r, err := Trace(synth(t, "star-5"), DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Paths) != 1 || len(r.Paths[0].Subpaths) != 1 {
		t.Fatalf("paths %d", len(r.Paths))
	}
	segs := r.Paths[0].Subpaths[0].Segs
	if len(segs) != 10 {
		t.Fatalf("got %d segments, want 10", len(segs))
	}
	for i, s := range segs {
		if !s.Line {
			t.Errorf("segment %d is a cubic", i)
		}
	}
}

// Spline output filled at pixel centres must agree with the thresholded
// bitmap to within a fraction of a pixel along the boundary.
func TestTraceSplineMatchesBitmap(t *testing.T) {
	limits := map[string]int{"circle-aa": 200, "circle-hard": 200, "rounded-rect": 220, "star-5": 120, "noisy-disc": 220}
	for name, limit := range limits {
		img := synth(t, name)
		gray, w, h := bitmap.Gray(img)
		want := bitmap.FromGray(gray, w, h, 128)
		r, err := Trace(img, DefaultOptions())
		if err != nil {
			t.Fatal(err)
		}
		got := rasttest.Fill(w, h, r.Paths)
		mis := 0
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				if got.Get(x, y) != want.Get(x, y) {
					mis++
				}
			}
		}
		if mis > limit {
			t.Errorf("%s: %d mismatched pixels, limit %d", name, mis, limit)
		}
	}
}

// A rounded rectangle has no corners sharper than the threshold, but its
// four straight edges must still come out as lines joined by arcs, not as
// cubics whose joints sit mid-edge.
func TestTraceSplineRoundedRectEdgesAreLines(t *testing.T) {
	r, err := Trace(synth(t, "rounded-rect"), DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	segs := r.Paths[0].Subpaths[0].Segs
	lines := 0
	for _, s := range segs {
		if s.Line {
			lines++
		}
	}
	if lines != 4 {
		t.Fatalf("got %d lines in %d segments, want 4", lines, len(segs))
	}
	if len(segs) > 12 {
		t.Fatalf("got %d segments, want at most 12", len(segs))
	}
}

// Windows measured in pixels must scale with the shape: a 2000 px star
// traces to the same ten lines as a 256 px one.
func TestTraceSplineLargeStarIsTenLines(t *testing.T) {
	const w, h = 2000, 2000
	var segs []fit.Segment
	cx, cy, ro, ri := 1000.0, 1000.0, 900.0, 360.0
	var prev geom.Vec2
	for i := 0; i <= 10; i++ {
		a := -math.Pi/2 + float64(i)*math.Pi/5
		r := ro
		if i%2 == 1 {
			r = ri
		}
		p := geom.Vec2{X: cx + r*math.Cos(a), Y: cy + r*math.Sin(a)}
		if i > 0 {
			segs = append(segs, fit.Segment{Line: true, P: fit.Cubic{prev, prev, p, p}})
		}
		prev = p
	}
	bm := rasttest.Fill(w, h, []svg.Path{{Subpaths: []svg.Subpath{{Segs: segs}}}})
	img := image.NewGray(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if !bm.Get(x, y) {
				img.SetGray(x, y, color.Gray{Y: 255})
			}
		}
	}
	r, err := Trace(img, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	got := r.Paths[0].Subpaths[0].Segs
	if len(got) > 12 {
		t.Fatalf("got %d segments, want about 10", len(got))
	}
}

// bitmapImage returns a grey image that thresholds back to bm.
func bitmapImage(bm *bitmap.Bitmap) *image.Gray {
	img := image.NewGray(image.Rect(0, 0, bm.W, bm.H))
	for y := 0; y < bm.H; y++ {
		for x := 0; x < bm.W; x++ {
			if !bm.Get(x, y) {
				img.SetGray(x, y, color.Gray{Y: 255})
			}
		}
	}
	return img
}

// polygonImage rasterises a closed polygon into a w×h grey image.
func polygonImage(w, h int, poly []geom.Vec2) *image.Gray {
	var segs []fit.Segment
	for i := range poly {
		a, b := poly[i], poly[(i+1)%len(poly)]
		segs = append(segs, fit.Segment{Line: true, P: fit.Cubic{a, a, b, b}})
	}
	return bitmapImage(rasttest.Fill(w, h, []svg.Path{{Subpaths: []svg.Subpath{{Segs: segs}}}}))
}

// mismatches counts pixels where the filled result disagrees with the
// thresholded input.
func mismatches(t *testing.T, img image.Image, r *Result) int {
	t.Helper()
	gray, w, h := bitmap.Gray(img)
	want := bitmap.FromGray(gray, w, h, 128)
	got := rasttest.Fill(w, h, r.Paths)
	mis := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if got.Get(x, y) != want.Get(x, y) {
				mis++
			}
		}
	}
	return mis
}

func countSegs(r *Result) (segs, lines int) {
	for _, p := range r.Paths {
		for _, sp := range p.Subpaths {
			for _, s := range sp.Segs {
				segs++
				if s.Line {
					lines++
				}
			}
		}
	}
	return
}

// An octagon turns 45° at each vertex, under the corner threshold, so its
// vertices come from straight-edge detection. Adjacent edges share an end
// and must yield one vertex there, not two: eight lines, not a tangle.
func TestTraceSplineOctagonIsEightLines(t *testing.T) {
	var poly []geom.Vec2
	for i := 0; i < 8; i++ {
		a := math.Pi/8 + float64(i)*math.Pi/4
		poly = append(poly, geom.Vec2{X: 128 + 100*math.Cos(a), Y: 128 + 100*math.Sin(a)})
	}
	img := polygonImage(256, 256, poly)
	r, err := Trace(img, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	segs, lines := countSegs(r)
	if segs != 8 || lines != 8 {
		t.Errorf("got %d segments (%d lines), want 8 lines", segs, lines)
	}
	if mis := mismatches(t, img, r); mis > 60 {
		t.Errorf("%d mismatched pixels", mis)
	}
}

// With the corner threshold above 90° a rectangle has no corners, only
// four straight edges; it must still trace as four lines.
func TestTraceSplineRectHighCornerThresholdIsFourLines(t *testing.T) {
	img := polygonImage(160, 120, []geom.Vec2{{X: 20, Y: 20}, {X: 140, Y: 20}, {X: 140, Y: 100}, {X: 20, Y: 100}})
	o := DefaultOptions()
	o.CornerThreshold = 100
	r, err := Trace(img, o)
	if err != nil {
		t.Fatal(err)
	}
	segs, lines := countSegs(r)
	if segs != 4 || lines != 4 {
		t.Errorf("got %d segments (%d lines), want 4 lines", segs, lines)
	}
	if mis := mismatches(t, img, r); mis != 0 {
		t.Errorf("%d mismatched pixels", mis)
	}
}

// Two squares touching at one pixel corner trace, under the black policy,
// as a single loop that visits the shared corner twice. Index mapping by
// coordinate would confuse the two visits; the result must be the
// eight-sided outline with no spurious segments.
func TestTraceSplineTouchingSquaresUnderBlack(t *testing.T) {
	bm := bitmap.New(80, 80)
	for y := 10; y < 40; y++ {
		for x := 10; x < 40; x++ {
			bm.Set(x, y, true)
			bm.Set(x+30, y+30, true)
		}
	}
	img := bitmapImage(bm)
	o := DefaultOptions()
	o.TurnPolicy = TurnBlack
	r, err := Trace(img, o)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Paths) != 1 || len(r.Paths[0].Subpaths) != 1 {
		t.Fatalf("got %d paths, want one loop", len(r.Paths))
	}
	segs, lines := countSegs(r)
	if segs != 8 || lines != 8 {
		t.Errorf("got %d segments (%d lines), want 8 lines", segs, lines)
	}
	if mis := mismatches(t, img, r); mis != 0 {
		t.Errorf("%d mismatched pixels", mis)
	}
}

// A one-pixel-wide diagonal must survive every mode: simplification may
// not collapse the sliver to a zero-area chord.
func TestTraceThinDiagonalKeepsArea(t *testing.T) {
	bm := bitmap.New(128, 128)
	for i := 0; i < 100; i++ {
		bm.Set(10+i, 10+i, true)
	}
	img := bitmapImage(bm)
	for _, mode := range []Mode{ModePolygon, ModeSpline} {
		o := DefaultOptions()
		o.Mode = mode
		r, err := Trace(img, o)
		if err != nil {
			t.Fatal(err)
		}
		segs, _ := countSegs(r)
		if segs < 3 || segs > 6 {
			t.Errorf("%s: got %d segments, want 3..6", mode, segs)
		}
		if mis := mismatches(t, img, r); mis > 20 {
			t.Errorf("%s: %d of 100 pixels lost", mode, mis)
		}
	}
}

// Diagonal bands three pixels wide (fuzz-found) must not vanish: a fit
// that stays within tolerance of both long sides by running down the
// middle is one-sided, and the fitter must refuse it.
func TestTraceSplineThinDiagonalBandsKeepArea(t *testing.T) {
	const w, h = 53, 4
	bm := bitmap.New(w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if (x+y)%4 != 3 {
				bm.Set(x, y, true)
			}
		}
	}
	img := bitmapImage(bm)
	o := DefaultOptions()
	o.FilterSpeckle = 0
	r, err := Trace(img, o)
	if err != nil {
		t.Fatal(err)
	}
	if mis := mismatches(t, img, r); mis > 20 {
		t.Fatalf("%d mismatched pixels of %d", mis, bm.Count())
	}
}

// smallGlyphsFloor is the 4× rasttest PSNR of the bw plain spline trace
// of small-glyphs when Phase 3b's small-feature fit landed (32.26 dB by
// the benchmark's rsvg render, against Potrace's 32.22; the two renderers
// differ, so the floor is this renderer's own value, 31.87, less a margin).
const smallGlyphsFloor = 31.7

// TestSmallGlyphsFloor guards the small-feature fit: the bw plain spline
// trace of small-glyphs must not fall below the PSNR it reached in Phase
// 3b. internal/bench's TestSmallGlyphsBeatsPotrace is the Potrace
// comparison itself.
func TestSmallGlyphsFloor(t *testing.T) {
	img := synth(t, "small-glyphs")
	o := DefaultOptions()
	s, err := Segment(img, o)
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Finish(o)
	if err != nil {
		t.Fatal(err)
	}
	got := superPSNR(t, img, s, r)
	t.Logf("small-glyphs bw spline: %.2f dB", got)
	if got < smallGlyphsFloor {
		t.Errorf("small-glyphs %.2f dB, floor %.2f", got, smallGlyphsFloor)
	}
}

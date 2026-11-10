package refine

import (
	"fmt"
	"math"
	"math/rand/v2"
	"runtime"
	"testing"

	"github.com/vexyl-ai/rekha/internal/fit"
	"github.com/vexyl-ai/rekha/internal/geom"
	"github.com/vexyl-ai/rekha/internal/rasttest"
	"github.com/vexyl-ai/rekha/internal/svg"
)

func TestMinimizeQuadratic(t *testing.T) {
	f := func(x []float64) float64 {
		s := 0.0
		for i, v := range x {
			d := v - float64(i)
			s += d * d
		}
		return s
	}
	x := minimize(make([]float64, 10), f, nil, 30)
	// The forward-difference gradient shifts the minimum by fdStep/2.
	for i, v := range x {
		if math.Abs(v-float64(i)) > fdStep {
			t.Fatalf("x[%d] = %v", i, v)
		}
	}
}

// smoothBezigon is a seeded random star-like smooth closed curve with n
// segments inside a 64×64 image.
func smoothBezigon(seed uint64, n int) Bezigon {
	rng := rand.New(rand.NewPCG(seed, 7))
	anchors := make([]geom.Vec2, n)
	for i := range anchors {
		a := 2 * math.Pi * float64(i) / float64(n)
		r := 18 + 6*rng.Float64()
		anchors[i] = v(32+r*math.Cos(a), 32+r*math.Sin(a))
	}
	b := Bezigon{Pts: make([]geom.Vec2, 3*n)}
	for i := range anchors {
		prev, next := anchors[(i+n-1)%n], anchors[(i+1)%n]
		tangent := next.Sub(prev).Scale(1.0 / 6)
		b.Pts[3*i] = anchors[i]
		b.Pts[3*i+1] = anchors[i].Add(tangent)
		b.Pts[((3*i-1)%(3*n)+3*n)%(3*n)] = anchors[i].Sub(tangent)
	}
	return b
}

func perturb(b Bezigon, seed uint64, amp float64) Bezigon {
	rng := rand.New(rand.NewPCG(seed, 11))
	out := Bezigon{Pts: make([]geom.Vec2, len(b.Pts))}
	for i, p := range b.Pts {
		out.Pts[i] = p.Add(v((2*rng.Float64()-1)*amp, (2*rng.Float64()-1)*amp))
	}
	return out
}

// curveDist is the largest distance from any flattened point of a to the
// flattened outline of b and vice versa: how far the curves are apart,
// regardless of where the handles sit (handle positions are not pinned by
// coverage, the curve is).
func curveDist(a, b Bezigon) float64 {
	pa, pb := fine(a), fine(b)
	return math.Max(oneWay(pa, pb), oneWay(pb, pa))
}

// fine flattens at 64 points per segment so chord error (0.024 px for a
// radius-20 circle at Flatten) does not pollute sub-pixel comparisons.
func fine(b Bezigon) []geom.Vec2 {
	var out []geom.Vec2
	for j := 0; j < b.N(); j++ {
		c := b.Segment(j)
		for i := 0; i < 64; i++ {
			out = append(out, c.At(float64(i)/64))
		}
	}
	return out
}

// meanDist is the mean distance from a's flattened points to b's outline:
// what the squared-error metrics of the benchmark respond to.
func meanDist(a, b Bezigon) float64 {
	pa, pb := fine(a), fine(b)
	sum := 0.0
	for _, p := range pa {
		best := math.Inf(1)
		for i := range pb {
			best = math.Min(best, pointSegDist(p, pb[i], pb[(i+1)%len(pb)]))
		}
		sum += best
	}
	return sum / float64(len(pa))
}

func oneWay(pa, pb []geom.Vec2) float64 {
	worst := 0.0
	for _, p := range pa {
		best := math.Inf(1)
		for i := range pb {
			best = math.Min(best, pointSegDist(p, pb[i], pb[(i+1)%len(pb)]))
		}
		worst = math.Max(worst, best)
	}
	return worst
}

func pointSegDist(p, a, b geom.Vec2) float64 {
	d := b.Sub(a)
	l2 := d.LenSq()
	t := 0.0
	if l2 > 0 {
		t = math.Min(math.Max(p.Sub(a).Dot(d)/l2, 0), 1)
	}
	return p.Sub(a.Add(d.Scale(t))).Len()
}

func maxDist(a, b Bezigon) float64 {
	d := 0.0
	for i := range a.Pts {
		d = math.Max(d, a.Pts[i].Sub(b.Pts[i]).Len())
	}
	return d
}

var img64 = Window{X0: 0, Y0: 0, W: 64, H: 64}

func TestRefineRecoversPerturbedBezigon(t *testing.T) {
	truth := smoothBezigon(1, 5)
	m := maskOf(truth, img64)
	start := perturb(truth, 2, 1.5)
	if d := maxDist(start, truth); d < 0.5 {
		t.Fatalf("perturbation too small: %v", d)
	}
	got := Refine(start, m, img64, DefaultOptions)
	// The mean is what the benchmark's squared error sees; the max bounds
	// local excursions, invisible at 8 bits below a tenth of a pixel.
	t.Logf("curve recovered within %.4f px max, %.4f mean", curveDist(got, truth), meanDist(got, truth))
	if d := meanDist(got, truth); d > 0.02 {
		t.Fatalf("curve recovered within %v px mean, want 0.02", d)
	}
	if d := curveDist(got, truth); d > 0.1 {
		t.Fatalf("curve recovered within %v px, want 0.1", d)
	}
}

// supersampled renders b with the test rasteriser at 4× and box-filters
// down, an independent rasteriser with its own boundary noise.
func supersampled(b Bezigon, win Window) coverageMask {
	scaled := Bezigon{Pts: make([]geom.Vec2, len(b.Pts))}
	for i, p := range b.Pts {
		scaled.Pts[i] = p.Scale(4)
	}
	segs := scaled.Segments(0)
	for i := range segs {
		segs[i].Line = false
	}
	bm := rasttest.Fill(win.W*4, win.H*4, []svg.Path{{Subpaths: []svg.Subpath{{Segs: segs}}}})
	cov := make([]float32, win.W*win.H)
	for y := 0; y < win.H*4; y++ {
		for x := 0; x < win.W*4; x++ {
			if bm.Get(x, y) {
				cov[(y/4)*win.W+x/4] += 1.0 / 16
			}
		}
	}
	return coverageMask{win, cov}
}

func TestRefineRecoversAgainstSupersampledMask(t *testing.T) {
	truth := smoothBezigon(3, 5)
	m := supersampled(truth, img64)
	got := Refine(perturb(truth, 4, 1.5), m, img64, DefaultOptions)
	t.Logf("curve recovered within %.4f px max, %.4f mean", curveDist(got, truth), meanDist(got, truth))
	if d := meanDist(got, truth); d > 0.03 {
		t.Fatalf("curve recovered within %v px mean against an independent rasteriser, want 0.03", d)
	}
	if d := curveDist(got, truth); d > 0.15 {
		t.Fatalf("curve recovered within %v px against an independent rasteriser, want 0.15", d)
	}
}

// A small loop where two handles cross (the paper's Fig. 3a failure) is
// pulled out by the self-intersection and length priors with the data.
func TestRefineRemovesLoop(t *testing.T) {
	truth := square(16, 16, 48, 48)
	m := maskOf(truth, img64)
	looped := square(16, 16, 48, 48)
	looped.Pts[1], looped.Pts[2] = v(62, 26), v(2, 26) // crossed handles on the top side, inside the image
	if SelfIntersectionPrior(looped) == 0 {
		t.Fatal("fixture has no loop")
	}
	// The handles start 14 px off; each sweep moves a point at most
	// moveLimit, so a gross error like this takes the full sweep budget.
	o := DefaultOptions
	o.Sweeps = 10
	got := Refine(looped, m, img64, o)
	if e := SelfIntersectionPrior(got); e != 0 {
		t.Fatalf("still self-intersecting: %v", e)
	}
	// Un-knotting is the point; the handles end up overshooting the chord
	// and the shape settles within half a pixel, not the sub-tenth a
	// sane start reaches.
	if d := curveDist(got, truth); d > 0.5 {
		t.Fatalf("curve %v px from the square", d)
	}
}

func TestRefineKeepsSquareCorners(t *testing.T) {
	truth := square(10, 10, 40, 40)
	m := maskOf(truth, img64)
	got := Refine(perturb(truth, 5, 1.0), m, img64, DefaultOptions)
	// 0.1 px: the result lands near 0.05 and float rounding differs across
	// architectures.
	if a := AnglePrior(got); math.Abs(a-2*math.Pi) > 0.1 {
		t.Fatalf("corners rounded: angle prior %v, want 2π", a)
	}
	for j := 0; j < 4; j++ {
		if d := got.Pts[3*j].Sub(truth.Pts[3*j]).Len(); d > 0.1 {
			t.Fatalf("corner %d off by %v", j, d)
		}
	}
}

func TestRefineLengthensShortHandle(t *testing.T) {
	truth := circle(32, 32, 20)
	m := maskOf(truth, img64)
	start := circle(32, 32, 20)
	start.Pts[1] = start.Pts[0].Add(v(0, 1e-3))
	got := Refine(start, m, img64, DefaultOptions)
	if l := got.Pts[1].Sub(got.Pts[0]).Len(); l < 1 {
		t.Fatalf("handle still %v px", l)
	}
}

func TestRefineDegenerate(t *testing.T) {
	m := maskOf(circle(32, 32, 10), img64)
	cases := map[string]Bezigon{
		"one":        {Pts: []geom.Vec2{v(20, 20), v(40, 20), v(40, 40)}},
		"two":        {Pts: []geom.Vec2{v(20, 20), v(40, 10), v(50, 30), v(30, 40), v(10, 40), v(10, 25)}},
		"coincident": {Pts: []geom.Vec2{v(5, 5), v(5, 5), v(5, 5), v(5, 5), v(5, 5), v(5, 5), v(5, 5), v(5, 5), v(5, 5)}},
		"zero-area":  FromSegments([]fit.Segment{ln(v(1, 1), v(30, 1)), ln(v(30, 1), v(1, 1)), ln(v(1, 1), v(1, 1))}),
		"empty":      {},
	}
	for name, b := range cases {
		got := Refine(b, m, img64, DefaultOptions)
		if got.N() != b.N() {
			t.Fatalf("%s: N changed %d → %d", name, b.N(), got.N())
		}
		for _, p := range got.Pts {
			if math.IsNaN(p.X) || math.IsNaN(p.Y) || math.IsInf(p.X, 0) || math.IsInf(p.Y, 0) {
				t.Fatalf("%s: non-finite point %v", name, p)
			}
		}
	}
}

func TestRefineClampsToImage(t *testing.T) {
	b := square(-5, -5, 70, 70)
	got := Refine(b, maskOf(square(0, 0, 64, 64), img64), img64, DefaultOptions)
	for _, p := range got.Pts {
		if p.X < -1 || p.Y < -1 || p.X > 65 || p.Y > 65 {
			t.Fatalf("point outside the padded image: %v", p)
		}
	}
}

func TestRefineSkipsConvergedPieces(t *testing.T) {
	truth := smoothBezigon(6, 6)
	got := Refine(truth, maskOf(truth, img64), img64, DefaultOptions)
	if d := maxDist(got, truth); d != 0 {
		t.Fatalf("a perfect fit moved by %v", d)
	}
}

func TestRefineNeverRaisesEnergy(t *testing.T) {
	truth := smoothBezigon(8, 7)
	m := supersampled(truth, img64)
	start := perturb(truth, 9, 2.0)
	got := Refine(start, m, img64, DefaultOptions)
	l0 := math.Max(start.Length(), 1)
	if e0, e1 := energyWith(start, m, img64, DefaultWeights, l0), energyWith(got, m, img64, DefaultWeights, l0); e1 > e0 {
		t.Fatalf("energy rose %v → %v", e0, e1)
	}
}

// circleN is a circle of n equal cubic arcs with the standard handle
// length 4/3·tan(θ/4)·r.
func circleN(cx, cy, r float64, n int) Bezigon {
	th := 2 * math.Pi / float64(n)
	h := 4.0 / 3 * math.Tan(th/4) * r
	pts := make([]geom.Vec2, 0, 3*n)
	for i := 0; i < n; i++ {
		a0, a1 := th*float64(i), th*float64(i+1)
		p0 := v(cx+r*math.Cos(a0), cy+r*math.Sin(a0))
		p3 := v(cx+r*math.Cos(a1), cy+r*math.Sin(a1))
		pts = append(pts, p0, p0.Add(v(-math.Sin(a0), math.Cos(a0)).Scale(h)), p3.Sub(v(-math.Sin(a1), math.Cos(a1)).Scale(h)))
	}
	return Bezigon{Pts: pts}
}

// BenchmarkRefineLong refines a 200-segment outline: the cost per piece
// must not grow with the outline's length.
func BenchmarkRefineLong(b *testing.B) {
	img := Window{X0: 0, Y0: 0, W: 512, H: 512}
	m := maskOf(circle(256, 256, 200), img)
	for _, n := range []int{25, 200} {
		src := circleN(256, 256, 200, n)
		for i := range src.Pts {
			src.Pts[i] = src.Pts[i].Add(v(0.7, -0.4))
		}
		b.Run(fmt.Sprintf("n=%d", src.N()), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				Refine(src, m, img, Options{Sweeps: 1, MaxIter: 30, Weights: DefaultWeights})
			}
		})
	}
}

// TestRefineFixedAnchorStays: a fixed anchor keeps its exact position
// while the rest of the bezigon still moves towards the mask.
func TestRefineFixedAnchorStays(t *testing.T) {
	ideal := smoothBezigon(1, 6)
	m := maskOf(ideal, img64)
	b := perturb(ideal, 2, 1.5)
	o := DefaultOptions
	o.Fixed = make([]bool, b.N())
	o.Fixed[0] = true
	got := Refine(b, m, img64, o)
	if got.Pts[0] != b.Pts[0] {
		t.Fatalf("fixed anchor moved %v → %v", b.Pts[0], got.Pts[0])
	}
	moved := 0
	for i := range b.Pts {
		if got.Pts[i].Sub(b.Pts[i]).Len() > 1e-6 {
			moved++
		}
	}
	if moved == 0 {
		t.Fatal("nothing moved")
	}
}

// TestRefineFrozenSegmentUnchanged: a frozen segment's four points are
// untouched; the piece it shares with the next segment still moves that
// segment's handles.
func TestRefineFrozenSegmentUnchanged(t *testing.T) {
	ideal := smoothBezigon(3, 6)
	m := maskOf(ideal, img64)
	b := perturb(ideal, 4, 1.5)
	o := DefaultOptions
	o.Frozen = make([]bool, b.N())
	o.Frozen[2] = true
	got := Refine(b, m, img64, o)
	for i := 6; i <= 9; i++ {
		if got.Pts[i] != b.Pts[i] {
			t.Fatalf("frozen point %d moved %v → %v", i, b.Pts[i], got.Pts[i])
		}
	}
	if got.Pts[10] == b.Pts[10] && got.Pts[11] == b.Pts[11] {
		t.Fatal("handles of the segment after the frozen one did not move")
	}
}

// TestMergeSplitRespectsFixed: a merge that would remove a fixed joint
// is not taken, and a frozen segment is neither split nor moved however
// badly it fits.
func TestMergeSplitRespectsFixed(t *testing.T) {
	ideal := circle(32, 32, 20)
	m := maskOf(ideal, img64)
	eight := ideal
	for j := 7; j >= 0; j -= 2 {
		eight = splitSegment(eight, j/2)
	}
	o := DefaultOptions
	o.Fixed, o.Frozen = make([]bool, 8), make([]bool, 8)
	o.Fixed[3] = true
	got, oo := MergeSplitTracked(eight, m, img64, o)
	if got.N() >= 8 {
		t.Fatalf("N = %d, nothing merged", got.N())
	}
	found := false
	for j := 0; j < got.N(); j++ {
		if got.Pts[3*j] == eight.Pts[9] {
			found = true
			if !oo.fixed(j) || len(oo.Fixed) != got.N() {
				t.Fatalf("fixed flag not carried to joint %d: %v", j, oo.Fixed)
			}
		}
	}
	if !found {
		t.Fatalf("fixed joint %v merged away (N %d → %d)", eight.Pts[9], eight.N(), got.N())
	}
	// A frozen segment with handles pulled 6 px off the circle.
	bad := Bezigon{Pts: append([]geom.Vec2(nil), eight.Pts...)}
	bad.Pts[4] = bad.Pts[4].Add(v(6, 6))
	bad.Pts[5] = bad.Pts[5].Add(v(6, -6))
	o = DefaultOptions
	o.Fixed, o.Frozen = make([]bool, 8), make([]bool, 8)
	o.Frozen[1] = true
	got = MergeSplit(bad, m, img64, o)
	want := bad.Segment(1)
	found = false
	for j := 0; j < got.N(); j++ {
		if got.Segment(j) == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("frozen segment %v changed or split", want)
	}
}

// TestRefineParallelClassesDeterministic: a 200-segment loop refined on
// one worker and on eight gives identical points.
func TestRefineParallelClassesDeterministic(t *testing.T) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(0))
	ideal := circleN(32, 32, 24, 200)
	m := maskOf(ideal, img64)
	b := perturb(ideal, 5, 0.6)
	var out [2]Bezigon
	for i, procs := range []int{1, 8} {
		runtime.GOMAXPROCS(procs)
		out[i] = Refine(b, m, img64, DefaultOptions)
	}
	for i := range out[0].Pts {
		if out[0].Pts[i] != out[1].Pts[i] {
			t.Fatalf("point %d differs: %v vs %v", i, out[0].Pts[i], out[1].Pts[i])
		}
	}
}

// TestRefineNoNewCrossings: a thin two-pixel bar refined against an empty
// mask wants to collapse; its sides may not cross.
func TestRefineNoNewCrossings(t *testing.T) {
	bar := FromSegments([]fit.Segment{
		{Line: true, P: fit.Cubic{v(30, 10), v(30, 10), v(32, 10), v(32, 10)}},
		{Line: true, P: fit.Cubic{v(32, 10), v(32, 10), v(32, 54), v(32, 54)}},
		{Line: true, P: fit.Cubic{v(32, 54), v(32, 54), v(30, 54), v(30, 54)}},
		{Line: true, P: fit.Cubic{v(30, 54), v(30, 54), v(30, 10), v(30, 10)}},
	})
	empty := coverageMask{win: img64, cov: make([]float32, 64*64)}
	if CrossingCount(bar) != 0 {
		t.Fatal("fixture crosses itself")
	}
	got := Refine(bar, empty, img64, DefaultOptions)
	if c := CrossingCount(got); c != 0 {
		t.Fatalf("%d crossings after refinement", c)
	}
}

// TestRefineTinyLoops: bezigons of one to four segments refine without
// panicking and come back finite.
func TestRefineTinyLoops(t *testing.T) {
	for n := 1; n <= 4; n++ {
		b := circleN(32, 32, 16, n)
		m := maskOf(circle(32, 32, 16), img64)
		got := Refine(perturb(b, uint64(n), 0.5), m, img64, DefaultOptions)
		if got.N() != n {
			t.Fatalf("N = %d → %d", n, got.N())
		}
		for _, p := range got.Pts {
			if !finite(p.X) || !finite(p.Y) {
				t.Fatalf("N = %d: non-finite point", n)
			}
		}
	}
}

// TestSweepEarlyExit: a loop already at its minimum stops after one
// sweep.
func TestSweepEarlyExit(t *testing.T) {
	ideal := circle(32, 32, 20)
	m := maskOf(ideal, img64)
	sweeps := 0
	sweepHook = func() { sweeps++ }
	defer func() { sweepHook = nil }()
	Refine(ideal, m, img64, DefaultOptions)
	if sweeps != 1 {
		t.Fatalf("%d sweeps, want 1", sweeps)
	}
}

// circleFrom is circleN with its first anchor at angle a0.
func circleFrom(cx, cy, r float64, n int, a0 float64) Bezigon {
	th := 2 * math.Pi / float64(n)
	h := 4.0 / 3 * math.Tan(th/4) * r
	pts := make([]geom.Vec2, 0, 3*n)
	for i := 0; i < n; i++ {
		b0, b1 := a0+th*float64(i), a0+th*float64(i+1)
		p0 := v(cx+r*math.Cos(b0), cy+r*math.Sin(b0))
		p3 := v(cx+r*math.Cos(b1), cy+r*math.Sin(b1))
		pts = append(pts, p0, p0.Add(v(-math.Sin(b0), math.Cos(b0)).Scale(h)), p3.Sub(v(-math.Sin(b1), math.Cos(b1)).Scale(h)))
	}
	return Bezigon{Pts: pts}
}

// A long arc hugging the image border has handles well outside the
// image (a 120° arc's handles reach 0.77 r along the tangents). They
// are not on the curve, so they must stay where the fit put them: a
// bezigon that already matches its mask is not moved by refinement.
func TestRefineKeepsHandlesBeyondImage(t *testing.T) {
	truth := circleFrom(32, 32, 31, 3, math.Pi/6) // anchors at 30°, 150°, 270°: one arc centred on the bottom edge
	outside := 0
	for i, p := range truth.Pts {
		if i%3 != 0 && (p.Y > 65 || p.X < -1 || p.X > 65) {
			outside++
		}
	}
	if outside == 0 {
		t.Fatal("fixture has no handle beyond the padded image")
	}
	m := supersampled(truth, img64)
	got := Refine(truth, m, img64, DefaultOptions)
	l0 := math.Max(truth.Length(), 1)
	e0, e1 := energyWith(truth, m, img64, DefaultWeights, l0), energyWith(got, m, img64, DefaultWeights, l0)
	t.Logf("curve moved %.4f px max; energy %.4f → %.4f", curveDist(got, truth), e0, e1)
	if d := curveDist(got, truth); d > 0.2 {
		t.Fatalf("a fitting arc with handles beyond the image moved %v px, want ≤ 0.2", d)
	}
	if e1 > e0 {
		t.Fatalf("energy rose %v → %v", e0, e1)
	}
}

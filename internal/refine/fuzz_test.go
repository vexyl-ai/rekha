package refine

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/vexyl-ai/rekha/internal/fit"
	"github.com/vexyl-ai/rekha/internal/geom"
)

func FuzzRefine(f *testing.F) {
	f.Add(uint64(1), 4, uint64(2))
	f.Add(uint64(3), 1, uint64(4))
	f.Add(uint64(5), 12, uint64(6))
	f.Add(uint64(7), 2, uint64(7))
	f.Fuzz(func(t *testing.T, seed uint64, n int, maskSeed uint64) {
		n = ((n%12)+12)%12 + 1
		rng := rand.New(rand.NewPCG(seed, 1))
		b := Bezigon{Pts: make([]geom.Vec2, 3*n)}
		for i := range b.Pts {
			if rng.IntN(5) == 0 && i > 0 {
				b.Pts[i] = b.Pts[i-1] // coincident points
			} else {
				b.Pts[i] = v(rng.Float64()*32, rng.Float64()*32)
			}
		}
		img := Window{X0: 0, Y0: 0, W: 32, H: 32}
		m := maskOf(smoothBezigon(maskSeed, 5), img)
		for i := range m.cov { // 64×64 mask shrunk: only the 32² window matters
			m.cov[i] = float32(math.Min(float64(m.cov[i]), 1))
		}
		got := Refine(b, m, img, DefaultOptions)
		if got.N() != b.N() {
			t.Fatalf("N changed")
		}
		for _, p := range got.Pts {
			if math.IsNaN(p.X) || math.IsNaN(p.Y) || math.IsInf(p.X, 0) || math.IsInf(p.Y, 0) {
				t.Fatalf("non-finite point %v", p)
			}
		}
		l0 := math.Max(b.Length(), 1)
		if e0, e1 := energyWith(b, m, img, DefaultWeights, l0), energyWith(got, m, img, DefaultWeights, l0); !math.IsNaN(e0) && !math.IsInf(e0, 0) && e1 > e0 {
			t.Fatalf("energy rose %v → %v", e0, e1)
		}
	})
}

// FuzzSnapPieces feeds SnapPieces and CommonRadius loops of 2–15 random
// segments (lines, bows, exact arcs, coincident anchors) with random fixed
// anchors and frozen segments: the result is finite, fixed anchors do not
// move, frozen segments keep their control points, and the option slices
// never outrun the loop.
func FuzzSnapPieces(f *testing.F) {
	f.Add(uint64(1), 4, uint64(2))
	f.Add(uint64(3), 2, uint64(4))
	f.Add(uint64(5), 15, uint64(6))
	f.Add(uint64(7), 8, uint64(7))
	f.Fuzz(func(t *testing.T, seed uint64, n int, maskSeed uint64) {
		n = ((n%14)+14)%14 + 2
		rng := rand.New(rand.NewPCG(seed, 3))
		centre, r := v(16, 16), 6+rng.Float64()*8
		angles := make([]float64, n)
		a := rng.Float64() * 2 * math.Pi
		for i := range angles {
			angles[i] = a
			a += (0.2 + rng.Float64()) * 2 * math.Pi / float64(n)
		}
		at := func(a float64) geom.Vec2 { return v(centre.X+r*math.Cos(a), centre.Y+r*math.Sin(a)) }
		segs := make([]fit.Segment, n)
		for i := range segs {
			a0, a1 := angles[i], angles[(i+1)%n]
			if i == n-1 {
				a1 = angles[0] + 2*math.Pi
			}
			p, q := at(a0), at(a1)
			switch rng.IntN(4) {
			case 0:
				segs[i] = ln(p, q)
			case 1:
				segs[i] = cubic(p, p.Add(v(rng.Float64()*8-4, rng.Float64()*8-4)), q.Add(v(rng.Float64()*8-4, rng.Float64()*8-4)), q)
			case 2:
				segs[i] = cubic(p, p, p, p) // anchor and both handles coincide; the next anchor is q
			default:
				if a1-a0 < math.Pi {
					segs[i] = arcSeg(centre, r, a0, a1)
				} else {
					segs[i] = ln(p, q)
				}
			}
		}
		b := FromSegments(segs)
		o := Options{Fixed: make([]bool, b.N()), Frozen: make([]bool, b.N())}
		for i := range o.Fixed {
			o.Fixed[i] = rng.IntN(4) == 0
			o.Frozen[i] = rng.IntN(5) == 0
		}
		var fixedPts []geom.Vec2
		var frozenSegs []fit.Cubic
		for i := 0; i < b.N(); i++ {
			if o.Fixed[i] {
				fixedPts = append(fixedPts, b.Pts[3*i])
			}
			if o.Frozen[i] {
				frozenSegs = append(frozenSegs, b.Segment(i))
			}
		}
		img := Window{X0: 0, Y0: 0, W: 32, H: 32}
		m := maskOf(smoothBezigon(maskSeed, 5), img)
		for i := range m.cov {
			m.cov[i] = float32(math.Min(float64(m.cov[i]), 1))
		}
		got, co := SnapPieces(b, m, img, 0.5, o)
		got, co = CommonRadius(got, m, img, 0.5, co)
		if got.N() < 1 {
			t.Fatalf("loop vanished")
		}
		for _, p := range got.Pts {
			if math.IsNaN(p.X) || math.IsNaN(p.Y) || math.IsInf(p.X, 0) || math.IsInf(p.Y, 0) {
				t.Fatalf("non-finite point %v", p)
			}
		}
		if len(co.Fixed) > got.N() || len(co.Frozen) > got.N() {
			t.Fatalf("options outrun the loop: %d fixed, %d frozen, N %d", len(co.Fixed), len(co.Frozen), got.N())
		}
		nFixed, nFrozen := 0, 0
		for i := 0; i < got.N(); i++ {
			if co.fixed(i) {
				nFixed++
				p, ok := got.Pts[3*i], false
				for _, q := range fixedPts {
					ok = ok || near(p, q, 1e-9)
				}
				if !ok {
					t.Fatalf("fixed anchor %d moved to %v", i, p)
				}
			}
			if co.frozen(i) {
				nFrozen++
				c, ok := got.Segment(i), false
				for _, s := range frozenSegs {
					ok = ok || (near(c[0], s[0], 1e-9) && near(c[1], s[1], 1e-9) && near(c[2], s[2], 1e-9) && near(c[3], s[3], 1e-9))
				}
				if !ok {
					t.Fatalf("frozen segment %d changed: %v", i, c)
				}
			}
		}
		if nFixed != len(fixedPts) || nFrozen != len(frozenSegs) {
			t.Fatalf("fixed %d→%d, frozen %d→%d", len(fixedPts), nFixed, len(frozenSegs), nFrozen)
		}
	})
}

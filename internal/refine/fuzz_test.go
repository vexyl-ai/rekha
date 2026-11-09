package refine

import (
	"math"
	"math/rand/v2"
	"testing"

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

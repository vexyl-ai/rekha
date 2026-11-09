package fit

import (
	"testing"

	"github.com/vexyl-ai/rekha/internal/geom"
)

// FitOne on points sampled from a cubic, with that cubic's end tangents,
// returns the cubic to within a few hundredths of a pixel (the
// alternating fit converges linearly; refinement finishes the job).
func TestFitOneMatchesGenerateBezier(t *testing.T) {
	want := Cubic{{X: 10, Y: 10}, {X: 20, Y: 40}, {X: 50, Y: 45}, {X: 60, Y: 10}}
	var pts []geom.Vec2
	for i := 0; i <= 40; i++ {
		pts = append(pts, want.At(float64(i)/40))
	}
	t0 := want[1].Sub(want[0]).Norm()
	t1 := want[2].Sub(want[3]).Norm()
	got := FitOne(pts, t0, t1)
	for i := range want {
		if d := got[i].Sub(want[i]).Len(); d > 0.02 {
			t.Fatalf("point %d off by %v: got %v want %v", i, d, got, want)
		}
	}
}

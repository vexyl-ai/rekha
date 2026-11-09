package refine

import (
	"math"
	"testing"

	"github.com/vexyl-ai/rekha/internal/fit"
	"github.com/vexyl-ai/rekha/internal/geom"
)

func v(x, y float64) geom.Vec2 { return geom.Vec2{X: x, Y: y} }

func ln(a, b geom.Vec2) fit.Segment { return fit.Segment{Line: true, P: fit.Cubic{a, a, b, b}} }

func cubic(a, b, c, d geom.Vec2) fit.Segment { return fit.Segment{P: fit.Cubic{a, b, c, d}} }

// square returns the axis-aligned square (x0,y0)-(x1,y1) as four line cubics.
func square(x0, y0, x1, y1 float64) Bezigon {
	return FromSegments([]fit.Segment{
		ln(v(x0, y0), v(x1, y0)), ln(v(x1, y0), v(x1, y1)), ln(v(x1, y1), v(x0, y1)), ln(v(x0, y1), v(x0, y0)),
	})
}

// circle returns the four-cubic approximation of a circle.
func circle(cx, cy, r float64) Bezigon {
	k := 0.5523 * r
	return Bezigon{Pts: []geom.Vec2{
		v(cx+r, cy), v(cx+r, cy+k), v(cx+k, cy+r),
		v(cx, cy+r), v(cx-k, cy+r), v(cx-r, cy+k),
		v(cx-r, cy), v(cx-r, cy-k), v(cx-k, cy-r),
		v(cx, cy-r), v(cx+k, cy-r), v(cx+r, cy-k),
	}}
}

func near(a, b geom.Vec2, tol float64) bool { return a.Sub(b).Len() <= tol }

func TestFromSegmentsRoundTrip(t *testing.T) {
	segs := []fit.Segment{
		ln(v(0, 0), v(10, 0)),
		cubic(v(10, 0), v(12, 3), v(12, 7), v(10, 10)),
		ln(v(10, 10), v(0, 10)),
		cubic(v(0, 10), v(-2, 7), v(-2, 3), v(0, 0)),
	}
	b := FromSegments(segs)
	if b.N() != 4 || len(b.Pts) != 12 {
		t.Fatalf("N %d, %d points", b.N(), len(b.Pts))
	}
	if !near(b.Pts[1], v(10.0/3, 0), 1e-9) || !near(b.Pts[2], v(20.0/3, 0), 1e-9) {
		t.Fatalf("line handles %v %v", b.Pts[1], b.Pts[2])
	}
	out := b.Segments(0.1)
	if len(out) != 4 || !out[0].Line || out[1].Line || !out[2].Line || out[3].Line {
		t.Fatalf("segments %+v", out)
	}
	for k := range out[1].P {
		if !near(out[1].P[k], segs[1].P[k], 1e-12) {
			t.Fatalf("cubic changed: %v vs %v", out[1].P, segs[1].P)
		}
	}
	if !near(out[0].P[0], v(0, 0), 0) || !near(out[0].P[3], v(10, 0), 0) {
		t.Fatalf("line endpoints %v", out[0].P)
	}
}

func TestSegmentsCollapsesStraightCubic(t *testing.T) {
	almost := Bezigon{Pts: []geom.Vec2{v(0, 0), v(3, 0.05), v(7, -0.05), v(10, 0), v(10, 3), v(10, 7), v(10, 10), v(7, 10), v(3, 10)}}
	segs := almost.Segments(0.1)
	if !segs[0].Line {
		t.Fatalf("0.05 px off the chord should be a line")
	}
	bent := Bezigon{Pts: []geom.Vec2{v(0, 0), v(3, 0.5), v(7, 0.5), v(10, 0), v(10, 3), v(10, 7), v(10, 10), v(7, 10), v(3, 10)}}
	if bent.Segments(0.1)[0].Line {
		t.Fatalf("0.5 px off the chord must stay a cubic")
	}
}

func TestPolylineAndLength(t *testing.T) {
	sq := square(0, 0, 10, 10)
	if got := sq.Length(); math.Abs(got-40) > 1e-9 {
		t.Fatalf("square length %v", got)
	}
	if n := len(sq.Polyline()); n != 4*Flatten {
		t.Fatalf("polyline has %d points, want %d", n, 4*Flatten)
	}
	c := circle(32, 32, 20)
	if got := c.Length(); math.Abs(got-2*math.Pi*20) > 0.2 {
		t.Fatalf("circle length %v", got)
	}
}

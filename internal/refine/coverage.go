package refine

import (
	"math"

	"github.com/vexyl-ai/rekha/internal/geom"
)

// Window is a pixel rectangle of the image: cells X0..X0+W-1, Y0..Y0+H-1.
type Window struct{ X0, Y0, W, H int }

// Coverage rasterises b into out (len W*H, row-major) as the exact area of
// the flattened polygon inside each pixel, clamped to [0, 1]. The result is
// continuous in every control point.
func Coverage(b Bezigon, win Window, out []float32) {
	for i := range out[:win.W*win.H] {
		out[i] = 0
	}
	AccumulateEdges(b.Polyline(), true, win, out)
	Resolve(out, win, out)
}

// AccumulateEdges adds the signed area and cover of a polyline's edges to
// acc (len W*H). After Resolve, pixel (x, y) holds the signed area of the
// polygon to the left of x+1 within that row, which for a simple closed
// polygon is its coverage. Edges may be accumulated in any order and in
// several calls, so the edges that do not move during an optimisation can
// be accumulated once.
func AccumulateEdges(pts []geom.Vec2, closed bool, win Window, acc []float32) {
	n := len(pts)
	last := n - 1
	if closed {
		last = n
	}
	for i := 1; i <= last; i++ {
		accumulateEdge(pts[i-1], pts[i%n], win, acc)
	}
}

func accumulateEdge(p0, p1 geom.Vec2, win Window, acc []float32) {
	if p0.Y == p1.Y {
		return
	}
	dir := 1.0
	if p0.Y > p1.Y {
		p0, p1, dir = p1, p0, -1
	}
	// Window-relative coordinates.
	p0 = p0.Sub(geom.Vec2{X: float64(win.X0), Y: float64(win.Y0)})
	p1 = p1.Sub(geom.Vec2{X: float64(win.X0), Y: float64(win.Y0)})
	dxdy := (p1.X - p0.X) / (p1.Y - p0.Y)
	yStart, yEnd := max(int(math.Floor(p0.Y)), 0), min(int(math.Ceil(p1.Y)), win.H)
	for y := yStart; y < yEnd; y++ {
		ya, yb := math.Max(float64(y), p0.Y), math.Min(float64(y+1), p1.Y)
		if yb <= ya {
			continue
		}
		xa, xb := p0.X+(ya-p0.Y)*dxdy, p0.X+(yb-p0.Y)*dxdy
		row := acc[y*win.W : (y+1)*win.W]
		accumulateRow(row, xa, ya, xb, yb, dir, win.W)
	}
}

// accumulateRow splits the sub-edge (xa,ya)-(xb,yb), which lies within one
// row, at integer x boundaries and adds each piece to its cell: the area to
// the right of the piece within the cell goes to the cell, the rest to the
// next cell, so a prefix sum along the row yields the area left of each
// cell's right edge.
func accumulateRow(row []float32, xa, ya, xb, yb, dir float64, w int) {
	dy := math.Abs(yb - ya) // the piece's height; its sign is dir
	if xa > xb {
		xa, xb = xb, xa
	}
	span := xb - xa
	x0 := xa
	for {
		cell := int(math.Floor(x0))
		x1 := math.Min(xb, float64(cell+1))
		// Height of this sub-piece: proportional to its share of the x span.
		d := dy * dir
		if span > 0 {
			d *= (x1 - x0) / span
		}
		if d != 0 {
			xmf := 0.5*(x0+x1) - float64(cell)
			add(row, cell, w, d*(1-xmf))
			add(row, cell+1, w, d*xmf)
		}
		if x1 >= xb {
			return
		}
		x0 = x1
	}
}

// add deposits v into row[i], folding cells left of the window into cell 0
// and dropping cells right of it.
func add(row []float32, i, w int, v float64) {
	if i >= w {
		return
	}
	row[max(i, 0)] += float32(v)
}

// Resolve turns accumulated edge contributions into coverage: a prefix sum
// along each row, absolute value, clamped to 1. acc and out may alias.
func Resolve(acc []float32, win Window, out []float32) {
	for y := 0; y < win.H; y++ {
		var s float32
		for x := 0; x < win.W; x++ {
			s += acc[y*win.W+x]
			out[y*win.W+x] = min(float32(math.Abs(float64(s))), 1)
		}
	}
}

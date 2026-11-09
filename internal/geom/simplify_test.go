package geom

import (
	"math"
	"sort"
	"testing"
)

// crackRect returns the crack loop of a w×h rectangle at origin, clockwise,
// one point per unit step.
func crackRect(w, h int) []Point {
	var p []Point
	for x := 0; x < w; x++ {
		p = append(p, Point{x, 0})
	}
	for y := 0; y < h; y++ {
		p = append(p, Point{w, y})
	}
	for x := w; x > 0; x-- {
		p = append(p, Point{x, h})
	}
	for y := h; y > 0; y-- {
		p = append(p, Point{0, y})
	}
	return p
}

func TestMergeCollinearSquare(t *testing.T) {
	got := MergeCollinear(crackRect(10, 10))
	if len(got) != 4 {
		t.Fatalf("got %d points %v", len(got), got)
	}
}

func TestStaircaseBecomesDiagonal(t *testing.T) {
	// Triangle-ish loop: staircase from (0,0) to (8,8), then straight to (0,8), then up.
	var p []Point
	for i := 0; i < 8; i++ {
		p = append(p, Point{i, i}, Point{i + 1, i})
	}
	p = append(p, Point{8, 8}, Point{0, 8})
	got := RemoveStaircases(MergeCollinear(p))
	if len(got) != 3 {
		t.Fatalf("got %d points %v, want 3", len(got), got)
	}
}

func TestStaircaseKeepsOnePxRect(t *testing.T) {
	got := RemoveStaircases(MergeCollinear(crackRect(1, 5)))
	if len(got) != 4 {
		t.Fatalf("got %d points %v, want 4", len(got), got)
	}
}

func TestStaircaseKeepsIsolatedNotch(t *testing.T) {
	// 10x10 square with a 1-px notch cut into the top edge at x=5.
	sq := []Point{{0, 0}, {5, 0}, {5, 1}, {6, 1}, {6, 0}, {10, 0}, {10, 10}, {0, 10}}
	got := RemoveStaircases(sq)
	if len(got) != 8 {
		t.Fatalf("got %d points %v, want 8", len(got), got)
	}
}

func TestStaircaseOneByThreeBecomesDiagonal(t *testing.T) {
	// Slope-3 digital line from (0,0) to (6,18), then straight back around.
	var p []Point
	for i := 0; i < 6; i++ {
		p = append(p, Point{i, 3 * i}, Point{i + 1, 3 * i})
	}
	p = append(p, Point{6, 18}, Point{0, 18})
	got := RemoveStaircases(MergeCollinear(p))
	if len(got) != 3 {
		t.Fatalf("got %d points %v, want 3", len(got), got)
	}
}

func TestStaircaseRunsMayDifferByOne(t *testing.T) {
	// Runs of 3,4,3,4 with unit steps: still one digital straight segment.
	p := []Point{{0, 0}, {1, 0}, {1, 3}, {2, 3}, {2, 7}, {3, 7}, {3, 10}, {4, 10}, {4, 14}, {5, 14}, {5, 15}, {0, 15}}
	got := RemoveStaircases(p)
	if len(got) != 3 {
		t.Fatalf("got %d points %v, want 3", len(got), got)
	}
}

func TestStaircaseKeepsBigZigzag(t *testing.T) {
	// 3-px steps with 3-px runs is a real zigzag, not a digital line.
	p := []Point{{0, 0}, {3, 0}, {3, 3}, {6, 3}, {6, 6}, {9, 6}, {9, 9}, {12, 9}, {12, 12}, {0, 12}}
	got := RemoveStaircases(p)
	if len(got) != len(p) {
		t.Fatalf("got %d points, want %d (unchanged)", len(got), len(p))
	}
}

func TestStaircaseAdjacentChainsKeepJunction(t *testing.T) {
	// Slope-3 digital line (0,0)→(4,12), then slope-1 line (4,12)→(10,18),
	// then straight sides back. Two chains meet at (4,12); it must survive.
	var p []Point
	for i := 0; i < 4; i++ {
		p = append(p, Point{i, 3 * i}, Point{i + 1, 3 * i})
	}
	for i := 0; i < 6; i++ {
		p = append(p, Point{4 + i, 12 + i}, Point{5 + i, 12 + i})
	}
	p = append(p, Point{10, 18}, Point{0, 18})
	got := RemoveStaircases(MergeCollinear(p))
	if len(got) < 4 || len(got) > 5 {
		t.Fatalf("got %d points %v, want 4 or 5 (junction kept)", len(got), got)
	}
	found := false
	for _, q := range got {
		dx, dy := q.X-4, q.Y-12
		if dx*dx+dy*dy <= 2 { // the chain boundary may sit one stair vertex past the junction
			found = true
		}
	}
	if !found {
		t.Fatalf("no vertex near junction (4,12) kept: %v", got)
	}
}

// Two staircases meeting at a sharp apex (a star tip) must keep the apex:
// the left flank climbs with 3-long runs, the top is a 2 px flat, the
// right flank descends.
func TestStaircaseKeepsApex(t *testing.T) {
	// Starts at the apex, as a traced contour does (top-left-most point).
	poly := []Point{
		{127, 29}, {129, 29},
		{129, 32}, {130, 32}, {130, 35}, {131, 35}, {131, 38}, {132, 38},
		{132, 60}, {124, 60},
		{124, 38}, {125, 38}, {125, 35}, {126, 35}, {126, 32}, {127, 32},
	}
	got := RemoveStaircases(poly)
	top := got[0]
	for _, p := range got {
		if p.Y < top.Y {
			top = p
		}
	}
	if top.Y != 29 {
		t.Fatalf("apex lost: top is %v in %v", top, got)
	}
}

// discCrack returns the crack boundary of the pixel set whose centres lie
// inside a disc of radius r centred on the origin, as a closed polygon of
// unit axis-aligned steps (clockwise on screen).
func discCrack(r float64) []Point {
	ymin, ymax := int(math.Floor(-r)), int(math.Ceil(r))
	type ext struct{ l, rr int }
	rows := map[int]ext{}
	for y := ymin; y <= ymax; y++ {
		cy := float64(y) + 0.5
		half := r*r - cy*cy
		if half < 0 {
			continue
		}
		w := math.Sqrt(half)
		l := int(math.Ceil(-w - 0.5))
		rr := int(math.Floor(w - 0.5))
		if l <= rr {
			rows[y] = ext{l, rr + 1} // crack x-range [l, rr+1]
		}
	}
	var ys []int
	for y := range rows {
		ys = append(ys, y)
	}
	sort.Ints(ys)
	var poly []Point
	for _, y := range ys { // right side, going down
		e := rows[y]
		poly = append(poly, Point{e.rr, y}, Point{e.rr, y + 1})
	}
	for i := len(ys) - 1; i >= 0; i-- { // left side, going up
		e := rows[ys[i]]
		poly = append(poly, Point{e.l, ys[i] + 1}, Point{e.l, ys[i]})
	}
	return MergeCollinear(poly)
}

// Straightening must never replace a chain by a chord the chain's vertices
// are more than a pixel from: a 45° stretch of a circle mixes 1- and
// 2-long runs and looks like one staircase, but it bends.
func TestStaircaseStaysWithinOnePixel(t *testing.T) {
	poly := discCrack(96)
	got := RemoveStaircases(poly)
	if len(got) < 24 {
		t.Fatalf("only %d vertices left for a radius-96 circle", len(got))
	}
	// Every input vertex must be within 1 px of the output polygon.
	worst := 0.0
	for _, p := range poly {
		best := math.Inf(1)
		for i := range got {
			a, b := got[i].Vec(), got[(i+1)%len(got)].Vec()
			best = math.Min(best, PerpDist(p.Vec(), a, b))
		}
		worst = math.Max(worst, best)
	}
	if worst > 1.0 {
		t.Fatalf("input vertex %.2f px from the straightened polygon", worst)
	}
}

// A one-pixel-wide diagonal, traced as a loop through its pixel corners,
// is two anti-parallel staircases meeting at one-vertex tips. Each chain
// must keep its own end vertex: extending both into the shared tip would
// leave a two-point polygon.
func TestStaircaseKeepsSliverEnds(t *testing.T) {
	var p []Point
	for i := 0; i < 20; i++ {
		p = append(p, Point{i, i}, Point{i + 1, i})
	}
	p = append(p, Point{20, 20})
	for i := 20; i > 0; i-- {
		p = append(p, Point{i - 1, i}, Point{i - 1, i - 1})
	}
	p = p[:len(p)-1] // (0,0) is already the first point
	got := RemoveStaircases(MergeCollinear(p))
	if len(got) != 6 {
		t.Fatalf("got %d points %v, want 6", len(got), got)
	}
}

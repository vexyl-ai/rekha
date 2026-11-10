package planar

import (
	"testing"
	"time"

	"github.com/vexyl-ai/rekha/internal/bitmap"
	"github.com/vexyl-ai/rekha/internal/contour"
	"github.com/vexyl-ai/rekha/internal/geom"
)

// minoritySaddle is Segmentation.saddle: at a 2×2 checkerboard of c
// against d the colour with fewer pixels stays connected (ties to the
// lower index).
func minoritySaddle(labels []uint8, w, h int) func(c uint8) func(x, y int) bool {
	counts := make([]int, 256)
	for _, l := range labels {
		counts[l]++
	}
	return func(c uint8) func(x, y int) bool {
		return func(x, y int) bool {
			nw, ne := labels[(y-1)*w+x-1], labels[(y-1)*w+x]
			sw, se := labels[y*w+x-1], labels[y*w+x]
			var d1, d2 uint8
			if nw == c {
				d1, d2 = ne, sw
			} else {
				d1, d2 = nw, se
			}
			return d1 != d2 || counts[c] < counts[d1] || (counts[c] == counts[d1] && c < d1)
		}
	}
}

func coloursOf(labels []uint8) []uint8 {
	seen := map[uint8]bool{}
	var out []uint8
	for _, l := range labels {
		if !seen[l] {
			seen[l] = true
			out = append(out, l)
		}
	}
	// ascending
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func grid(w, h int, f func(x, y int) uint8) []uint8 {
	labels := make([]uint8, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			labels[y*w+x] = f(x, y)
		}
	}
	return labels
}

// fixtures: name → labels, w, h.
func fixtures() map[string]struct {
	labels []uint8
	w, h   int
} {
	type fx = struct {
		labels []uint8
		w, h   int
	}
	return map[string]fx{
		"tee": {grid(8, 8, func(x, y int) uint8 {
			switch {
			case x < 4:
				return 0
			case y < 4:
				return 1
			}
			return 2
		}), 8, 8},
		"saddle": {grid(4, 4, func(x, y int) uint8 {
			if (x == 2 && y == 1) || (x == 1 && y == 2) {
				return 1
			}
			return 0
		}), 4, 4},
		"island": {grid(8, 8, func(x, y int) uint8 {
			if x >= 2 && x < 6 && y >= 2 && y < 6 {
				return 1
			}
			return 0
		}), 8, 8},
		"checker": {grid(64, 64, func(x, y int) uint8 { return uint8((x/2 + y/2) % 2) }), 64, 64},
		"nested": {grid(12, 12, func(x, y int) uint8 {
			switch {
			case x >= 5 && x < 7 && y >= 5 && y < 7:
				return 2
			case x >= 2 && x < 10 && y >= 2 && y < 10:
				return 1
			}
			return 0
		}), 12, 12},
	}
}

func build(labels []uint8, w, h int) Map {
	return Build(labels, w, h, coloursOf(labels), minoritySaddle(labels, w, h))
}

// traceLoops is what Segmentation.layers traces per colour: the loops of
// each colour's label mask under the saddle rule, colour then scan order.
func traceLoops(labels []uint8, w, h int) [][]geom.Point {
	saddle := minoritySaddle(labels, w, h)
	var out [][]geom.Point
	for _, c := range coloursOf(labels) {
		bm := bitmap.New(w, h)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				if labels[y*w+x] == c {
					bm.Set(x, y, true)
				}
			}
		}
		for _, l := range contour.TraceFunc(bm, saddle(c)) {
			out = append(out, l.Pts)
		}
	}
	return out
}

func edgeSet(pts []geom.Point, closed bool) map[[2]geom.Point]bool {
	s := map[[2]geom.Point]bool{}
	n := len(pts)
	last := n - 1
	if closed {
		last = n
	}
	for i := 0; i < last; i++ {
		a, b := pts[i], pts[(i+1)%n]
		if b.Y < a.Y || (b.Y == a.Y && b.X < a.X) {
			a, b = b, a
		}
		s[[2]geom.Point{a, b}] = true
	}
	return s
}

func TestPlanarEveryArcHasOnePartner(t *testing.T) {
	for name, fx := range fixtures() {
		m := build(fx.labels, fx.w, fx.h)
		refs := make([]int, len(m.Arcs))
		for _, l := range m.Loops {
			for _, r := range l.Refs {
				refs[r.Arc]++
			}
		}
		for i, a := range m.Arcs {
			want := 2
			if a.Left == None {
				want = 1
			}
			if refs[i] != want {
				t.Errorf("%s: arc %d (left %d) referenced %d times, want %d", name, i, a.Left, refs[i], want)
			}
			if a.Right == None {
				t.Errorf("%s: arc %d has no right colour", name, i)
			}
		}
		// Every edge of every traced loop is in exactly one arc.
		edges := map[[2]geom.Point]int{}
		for _, a := range m.Arcs {
			for e := range edgeSet(a.Pts, a.Closed) {
				edges[e]++
			}
		}
		for _, l := range traceLoops(fx.labels, fx.w, fx.h) {
			for e := range edgeSet(l, true) {
				if edges[e] != 1 {
					t.Errorf("%s: edge %v in %d arcs", name, e, edges[e])
				}
			}
		}
	}
}

func TestPlanarAssembleMatchesTrace(t *testing.T) {
	for name, fx := range fixtures() {
		m := build(fx.labels, fx.w, fx.h)
		loops := traceLoops(fx.labels, fx.w, fx.h)
		if len(m.Loops) != len(loops) {
			t.Fatalf("%s: %d loops, traced %d", name, len(m.Loops), len(loops))
		}
		for i, l := range m.Loops {
			got := m.Assemble(l)
			if len(got) != len(loops[i]) {
				t.Fatalf("%s loop %d: %d points, traced %d\n%v\n%v", name, i, len(got), len(loops[i]), got, loops[i])
			}
			for k := range got {
				if got[k] != loops[i][k] {
					t.Fatalf("%s loop %d: point %d %v, traced %v", name, i, k, got[k], loops[i][k])
				}
			}
			if l.Area2 != geom.Area2(loops[i]) {
				t.Errorf("%s loop %d: area %d, traced %d", name, i, l.Area2, geom.Area2(loops[i]))
			}
		}
	}
}

func TestPlanarJunctions(t *testing.T) {
	fx := fixtures()["tee"]
	m := build(fx.labels, fx.w, fx.h)
	ends := 0
	for _, a := range m.Arcs {
		if a.Closed {
			t.Fatalf("tee has a closed arc: %+v", a)
		}
		for _, p := range []geom.Point{a.Pts[0], a.Pts[len(a.Pts)-1]} {
			if p == (geom.Point{X: 4, Y: 4}) {
				ends++
			}
		}
	}
	if ends != 3 {
		t.Errorf("tee vertex ends %d arcs, want 3", ends)
	}
	fx = fixtures()["saddle"]
	m = build(fx.labels, fx.w, fx.h)
	ends = 0
	for _, a := range m.Arcs {
		for _, p := range []geom.Point{a.Pts[0], a.Pts[len(a.Pts)-1]} {
			if p == (geom.Point{X: 2, Y: 2}) {
				ends++
			}
		}
	}
	if ends != 4 {
		t.Errorf("saddle vertex ends %d arcs, want 4", ends)
	}
}

func TestPlanarIsland(t *testing.T) {
	fx := fixtures()["island"]
	m := build(fx.labels, fx.w, fx.h)
	closed := 0
	for i, a := range m.Arcs {
		if !a.Closed || a.Left == None {
			continue // the border loop is closed too, with one side
		}
		closed++
		fwd, rev := 0, 0
		for _, l := range m.Loops {
			for _, r := range l.Refs {
				if r.Arc == i {
					if r.Reverse {
						rev++
					} else {
						fwd++
					}
				}
			}
		}
		if fwd != 1 || rev != 1 {
			t.Errorf("island arc referenced forward %d reverse %d", fwd, rev)
		}
	}
	if closed != 1 {
		t.Errorf("%d shared closed arcs, want 1", closed)
	}
}

func TestPlanarCheckerboard(t *testing.T) {
	fx := fixtures()["checker"]
	start := time.Now()
	m := build(fx.labels, fx.w, fx.h)
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Errorf("build took %v", d)
	}
	// Every interior vertex at even coordinates is a saddle, so interior
	// arcs are the two edges of one cell side.
	for _, a := range m.Arcs {
		if a.Left != None && len(a.Pts) != 3 {
			t.Fatalf("interior arc with %d points", len(a.Pts))
		}
	}
}

func TestPlanarShapes(t *testing.T) {
	fx := fixtures()["nested"]
	m := build(fx.labels, fx.w, fx.h)
	shapes := m.Shapes()
	// colour 0: outer (border) with one hole; colour 1: outer with one hole; colour 2: outer.
	if len(shapes) != 3 {
		t.Fatalf("%d shapes: %v", len(shapes), shapes)
	}
	for i, want := range []int{2, 2, 1} {
		if len(shapes[i]) != want {
			t.Errorf("shape %d has %d loops, want %d", i, len(shapes[i]), want)
		}
		if m.Loops[shapes[i][0]].Area2 <= 0 {
			t.Errorf("shape %d first loop is not an outer", i)
		}
	}
}

func FuzzPlanar(f *testing.F) {
	f.Add(uint8(4), uint8(4), []byte{0, 1, 1, 0, 1, 0, 0, 1, 2, 2, 0, 0, 1, 1, 1, 0})
	f.Fuzz(func(t *testing.T, w, h uint8, raw []byte) {
		W, H := int(w%16)+1, int(h%16)+1
		labels := make([]uint8, W*H)
		for i := range labels {
			if i < len(raw) {
				labels[i] = raw[i] % 4
			}
		}
		m := build(labels, W, H)
		loops := traceLoops(labels, W, H)
		if len(m.Loops) != len(loops) {
			t.Fatalf("%d loops vs %d", len(m.Loops), len(loops))
		}
		for i, l := range m.Loops {
			got := m.Assemble(l)
			if len(got) != len(loops[i]) {
				t.Fatalf("loop %d: %d points vs %d", i, len(got), len(loops[i]))
			}
			for k := range got {
				if got[k] != loops[i][k] {
					t.Fatalf("loop %d point %d", i, k)
				}
			}
		}
		refs := make([]int, len(m.Arcs))
		for _, l := range m.Loops {
			for _, r := range l.Refs {
				refs[r.Arc]++
			}
		}
		for i, a := range m.Arcs {
			want := 2
			if a.Left == None {
				want = 1
			}
			if refs[i] != want {
				t.Fatalf("arc %d referenced %d times", i, refs[i])
			}
		}
	})
}

// TestPlanarShapesLinear: Shapes on a 1024² 2-px checkerboard (131 072
// loops per colour) must not search every contour of a colour per shape.
func TestPlanarShapesLinear(t *testing.T) {
	const n = 1024
	labels := grid(n, n, func(x, y int) uint8 { return uint8((x/2 + y/2) % 2) })
	m := build(labels, n, n)
	start := time.Now()
	shapes := m.Shapes()
	// The linear search took 9 s natively; 5 s leaves room for amd64
	// under emulation (1.7 s) and loaded runners.
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("Shapes took %v", d)
	}
	// Under the minority saddle rule one colour joins across every
	// saddle (one outer, the other colour's cells as holes); every loop
	// appears in exactly one shape.
	seen := make([]bool, len(m.Loops))
	for _, s := range shapes {
		for _, i := range s {
			if i < 0 || seen[i] {
				t.Fatalf("loop %d missing or repeated", i)
			}
			seen[i] = true
		}
	}
	for i, ok := range seen {
		if !ok {
			t.Fatalf("loop %d in no shape", i)
		}
	}
	if len(shapes) < n*n/8 {
		t.Fatalf("%d shapes, want at least %d", len(shapes), n*n/8)
	}
}

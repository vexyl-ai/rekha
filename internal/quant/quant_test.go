package quant

import (
	"testing"

	"github.com/vexyl-ai/rekha/internal/bitmap"
	"github.com/vexyl-ai/rekha/internal/colour"
	"github.com/vexyl-ai/rekha/internal/corpus"
)

func rgbOf(colours ...[3]uint8) []uint8 {
	var out []uint8
	for _, c := range colours {
		out = append(out, c[0], c[1], c[2])
	}
	return out
}

func TestCellsCountsAndOrder(t *testing.T) {
	red, blue := [3]uint8{255, 0, 0}, [3]uint8{0, 0, 255}
	cells := Cells(rgbOf(red, blue, red, red), 6)
	if len(cells) != 2 {
		t.Fatalf("got %d cells", len(cells))
	}
	// blue (r=0) sorts before red by index.
	if cells[0].Weight != 1 || cells[1].Weight != 3 || cells[0].Index >= cells[1].Index {
		t.Fatalf("got %+v", cells)
	}
	// Cell centre of 6-bit cell containing 255 is 254 (252 + 2).
	if d := colour.Dist(cells[1].Lab, colour.FromSRGB8(254, 2, 2)); d > 1e-9 {
		t.Fatalf("red cell lab off by %v", d)
	}
}

func cluster(c [3]uint8, weight int) Cell {
	return Cell{Lab: colour.FromSRGB8(c[0], c[1], c[2]), Weight: weight}
}

func TestWuRecoversThreeClusters(t *testing.T) {
	cells := []Cell{cluster([3]uint8{255, 0, 0}, 100), cluster([3]uint8{0, 255, 0}, 50), cluster([3]uint8{0, 0, 255}, 25)}
	pal := Wu(cells, 8, 0.01)
	if len(pal) != 3 {
		t.Fatalf("got %d entries %v", len(pal), pal)
	}
	for _, c := range cells {
		best := 10.0
		for _, p := range pal {
			best = min(best, colour.Dist(p, c.Lab))
		}
		if best > 0.02 {
			t.Errorf("cluster %v not represented (nearest %.3f)", c.Lab, best)
		}
	}
}

func TestWuToleranceStopsAtTwo(t *testing.T) {
	red, blue := colour.FromSRGB8(255, 0, 0), colour.FromSRGB8(0, 0, 255)
	mid := colour.OKLab{L: (red.L + blue.L) / 2, A: (red.A + blue.A) / 2, B: (red.B + blue.B) / 2}
	cells := []Cell{{Lab: red, Weight: 1000}, {Lab: blue, Weight: 1000}, {Lab: mid, Weight: 40}}
	if pal := Wu(cells, 16, 0.08); len(pal) != 2 {
		t.Fatalf("got %d entries %v, want 2", len(pal), pal)
	}
}

func TestWuCapsAt256(t *testing.T) {
	var cells []Cell
	for i := 0; i < 4096; i++ {
		v := uint8(i / 16)
		lab := colour.FromSRGB8(v, v, v)
		lab.A += float64(i%16) * 0.025 // spread over more than 256 histogram bins
		cells = append(cells, Cell{Index: i, Lab: lab, Weight: 1})
	}
	if pal := Wu(cells, 0, 0); len(pal) != 256 {
		t.Fatalf("got %d entries, want 256", len(pal))
	}
}

func TestKMeansMovesSeed(t *testing.T) {
	cells := []Cell{{Lab: colour.OKLab{L: 0.49}, Weight: 1}, {Lab: colour.OKLab{L: 0.51}, Weight: 1}}
	pal := KMeans(cells, []colour.OKLab{{L: 0.6}}, 10)
	if len(pal) != 1 || pal[0].L < 0.5-1e-9 || pal[0].L > 0.5+1e-9 {
		t.Fatalf("got %v", pal)
	}
}

func TestLookupNearestSecondBlend(t *testing.T) {
	pal := []colour.OKLab{colour.FromSRGB8(0, 0, 0), colour.FromSRGB8(255, 255, 255)}
	l := NewLookup(pal, 6)
	i64, i32, i0 := CellIndex(64, 64, 64, 6), CellIndex(32, 32, 32, 6), CellIndex(0, 0, 0, 6)
	if l.Nearest[i64] != 0 || l.Second[i64] != 1 {
		t.Fatalf("grey 64: nearest %d second %d", l.Nearest[i64], l.Second[i64])
	}
	if b := l.Blend[i64]; b <= 128 || b >= 255 {
		t.Fatalf("grey 64 blend %d, want strictly between 128 and 255", b)
	}
	if l.Blend[i32] <= l.Blend[i64] {
		t.Fatalf("blend not monotonic: grey32 %d, grey64 %d", l.Blend[i32], l.Blend[i64])
	}
	// The table sees cell centres: the black cell's centre (2,2,2) is far
	// from black in OKLab (the cube root stretches dark tones), so only
	// monotonicity is asserted there; the white cell (254,254,254) sits on
	// the palette entry.
	if l.Blend[i0] <= l.Blend[i32] {
		t.Fatalf("blend not monotonic: grey0 %d, grey32 %d", l.Blend[i0], l.Blend[i32])
	}
	if b := l.Blend[CellIndex(255, 255, 255, 6)]; b < 250 {
		t.Fatalf("white blend %d", b)
	}
	one := NewLookup(pal[:1], 6)
	if one.Second[i64] != 0 || one.Blend[i64] != 255 {
		t.Fatalf("single entry: second %d blend %d", one.Second[i64], one.Blend[i64])
	}
	labels, second, blend := make([]uint8, 2), make([]uint8, 2), make([]uint8, 2)
	l.Map(rgbOf([3]uint8{0, 0, 0}, [3]uint8{255, 255, 255}), labels, second, blend)
	if labels[0] != 0 || labels[1] != 1 || second[0] != 1 || second[1] != 0 || blend[1] < 250 {
		t.Fatalf("map: %v %v %v", labels, second, blend)
	}
}

func TestWuDeterministic(t *testing.T) {
	var img = corpus.Render(corpus.All()[len(corpus.All())-1]) // noisy-disc
	rgb, _, _ := bitmap.Flatten(img)
	cells := Cells(rgb, 6)
	a := KMeans(cells, Wu(cells, 0, 0.04), 10)
	b := KMeans(cells, Wu(cells, 0, 0.04), 10)
	if len(a) != len(b) {
		t.Fatalf("%d vs %d entries", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("entry %d differs: %v vs %v", i, a[i], b[i])
		}
	}
}

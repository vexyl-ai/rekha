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
	// The extreme cells are centred on the channel extremes.
	if d := colour.Dist(cells[1].Lab, colour.FromSRGB8(255, 0, 0)); d > 1e-9 {
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

// Cells within the tolerance of a cluster join it; a cell further away
// gets its own entry however few pixels it holds.
func TestWuToleranceStopsAtTwo(t *testing.T) {
	red, blue := colour.FromSRGB8(255, 0, 0), colour.FromSRGB8(0, 0, 255)
	nearRed := colour.OKLab{L: red.L + 0.03, A: red.A, B: red.B}
	cells := []Cell{{Lab: red, Weight: 1000}, {Lab: blue, Weight: 1000}, {Lab: nearRed, Weight: 40}}
	if pal := Wu(cells, 16, 0.08); len(pal) != 2 {
		t.Fatalf("got %d entries %v, want 2", len(pal), pal)
	}
	mid := colour.OKLab{L: (red.L + blue.L) / 2, A: (red.A + blue.A) / 2, B: (red.B + blue.B) / 2}
	cells = []Cell{{Lab: red, Weight: 1000}, {Lab: blue, Weight: 1000}, {Lab: mid, Weight: 40}}
	if pal := Wu(cells, 16, 0.08); len(pal) != 3 {
		t.Fatalf("got %d entries %v, want 3", len(pal), pal)
	}
}

// A small subject must not be averaged into a large background: a 20×20
// black square on a 256×256 white canvas is 0.6 % of the pixels, and a
// weight-normalised stop would call the single box "within tolerance".
func TestWuKeepsSmallFeature(t *testing.T) {
	cells := []Cell{cluster([3]uint8{255, 255, 255}, 256*256-400), cluster([3]uint8{0, 0, 0}, 400)}
	if pal := Wu(cells, 0, 0.08); len(pal) != 2 {
		t.Fatalf("got %d entries %v, want black and white", len(pal), pal)
	}
	// Likewise a small red/blue object on a lightly noisy background.
	cells = nil
	for i := 0; i < 64; i++ {
		v := uint8(230 + i%16)
		cells = append(cells, cluster([3]uint8{v, uint8(235 + i/16*4), uint8(240 + i%8)}, 1000))
	}
	cells = append(cells, cluster([3]uint8{220, 30, 30}, 500), cluster([3]uint8{30, 60, 220}, 500))
	pal := Wu(cells, 0, 0.08)
	for _, want := range [][3]uint8{{220, 30, 30}, {30, 60, 220}} {
		best := 10.0
		for _, p := range pal {
			best = min(best, colour.Dist(p, colour.FromSRGB8(want[0], want[1], want[2])))
		}
		if best > 0.02 {
			t.Errorf("%v not in palette (nearest %.3f): %v", want, best, pal)
		}
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

// The extreme cells sit on pure black and pure white so that a flat
// black-and-white picture quantises to #000000 and #ffffff exactly.
func TestCellCentreEndpointsExact(t *testing.T) {
	for bits := 1; bits <= 8; bits++ {
		if r, g, b := CellCentre(0, bits); r != 0 || g != 0 || b != 0 {
			t.Errorf("bits %d: cell 0 centre (%d,%d,%d), want black", bits, r, g, b)
		}
		top := 1<<(3*bits) - 1
		if r, g, b := CellCentre(top, bits); r != 255 || g != 255 || b != 255 {
			t.Errorf("bits %d: top cell centre (%d,%d,%d), want white", bits, r, g, b)
		}
		// Centres stay inside their own cells.
		for idx := 0; idx < 1<<(3*bits); idx += 1 + idx/7 {
			r, g, b := CellCentre(idx, bits)
			if CellIndex(r, g, b, bits) != idx {
				t.Fatalf("bits %d: centre of cell %d maps to cell %d", bits, idx, CellIndex(r, g, b, bits))
			}
		}
	}
}

// Between its two nearest entries a cell is labelled by the sRGB midpoint,
// where anti-aliasing mixes, not by the OKLab midpoint (grey 100 between
// black and white), which would thin every dark shape on a light ground.
func TestLookupSplitsAtSRGBMidpoint(t *testing.T) {
	pal := []colour.OKLab{colour.FromSRGB8(0, 0, 0), colour.FromSRGB8(255, 255, 255)}
	l := NewLookup(pal, 7)
	if got := l.Nearest[CellIndex(120, 120, 120, 7)]; got != 0 {
		t.Fatalf("grey 120 labelled %d, want black", got)
	}
	if got := l.Nearest[CellIndex(136, 136, 136, 7)]; got != 1 {
		t.Fatalf("grey 136 labelled %d, want white", got)
	}
	// Blend is the share of the label's colour in the mix, 128..255: grey
	// 136 is just over half white.
	if b := l.Blend[CellIndex(136, 136, 136, 7)]; b < 130 || b > 145 {
		t.Fatalf("grey 136 blend %d, want about 136", b)
	}
	if b := l.Blend[CellIndex(250, 250, 250, 7)]; b < 245 {
		t.Fatalf("grey 250 blend %d", b)
	}
}

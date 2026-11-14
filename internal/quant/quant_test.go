package quant

import (
	"math"
	"math/rand"
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
// gets its own entry however few pixels it holds, unless it is a mixture
// of two existing entries (see TestWuIgnoresAABand).
func TestWuToleranceStopsAtTwo(t *testing.T) {
	red, blue := colour.FromSRGB8(255, 0, 0), colour.FromSRGB8(0, 0, 255)
	nearRed := colour.OKLab{L: red.L + 0.03, A: red.A, B: red.B}
	cells := []Cell{{Lab: red, Weight: 1000}, {Lab: blue, Weight: 1000}, {Lab: nearRed, Weight: 40}}
	if pal := Wu(cells, 16, 0.08); len(pal) != 2 {
		t.Fatalf("got %d entries %v, want 2", len(pal), pal)
	}
	green := colour.FromSRGB8(0, 255, 0)
	cells = []Cell{{Lab: red, Weight: 1000}, {Lab: blue, Weight: 1000}, {Lab: green, Weight: 40}}
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
	n64, s64, b64 := l.At(i64)
	_, _, b32 := l.At(i32)
	_, _, b0 := l.At(i0)
	if n64 != 0 || s64 != 1 {
		t.Fatalf("grey 64: nearest %d second %d", n64, s64)
	}
	if b64 <= 128 || b64 >= 255 {
		t.Fatalf("grey 64 blend %d, want strictly between 128 and 255", b64)
	}
	if b32 <= b64 {
		t.Fatalf("blend not monotonic: grey32 %d, grey64 %d", b32, b64)
	}
	// The table sees cell centres: the black cell's centre (2,2,2) is far
	// from black in OKLab (the cube root stretches dark tones), so only
	// monotonicity is asserted there; the white cell (254,254,254) sits on
	// the palette entry.
	if b0 <= b32 {
		t.Fatalf("blend not monotonic: grey0 %d, grey32 %d", b0, b32)
	}
	if _, _, b := l.At(CellIndex(255, 255, 255, 6)); b < 250 {
		t.Fatalf("white blend %d", b)
	}
	one := NewLookup(pal[:1], 6)
	if _, s, b := one.At(i64); s != 0 || b != 255 {
		t.Fatalf("single entry: second %d blend %d", s, b)
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
	if got, _, _ := l.At(CellIndex(120, 120, 120, 7)); got != 0 {
		t.Fatalf("grey 120 labelled %d, want black", got)
	}
	if got, _, _ := l.At(CellIndex(136, 136, 136, 7)); got != 1 {
		t.Fatalf("grey 136 labelled %d, want white", got)
	}
	// Blend is the share of the label's colour in the mix, 128..255: grey
	// 136 is just over half white.
	if _, _, b := l.At(CellIndex(136, 136, 136, 7)); b < 130 || b > 145 {
		t.Fatalf("grey 136 blend %d, want about 136", b)
	}
	if _, _, b := l.At(CellIndex(250, 250, 250, 7)); b < 245 {
		t.Fatalf("grey 250 blend %d", b)
	}
}

// mixRGB is the corpus anti-aliasing mix: sRGB channels blended by coverage.
func mixRGB(a, b [3]uint8, cov float64) [3]uint8 {
	var out [3]uint8
	for i := range out {
		out[i] = uint8(float64(a[i])*(1-cov) + float64(b[i])*cov + 0.5)
	}
	return out
}

// An anti-aliased edge between two flat colours puts a few heavy cells on
// the segment between them in colour space; those are mixtures, not a
// third colour, and must not earn a palette entry.
func TestWuIgnoresAABand(t *testing.T) {
	red, blue := [3]uint8{220, 30, 30}, [3]uint8{30, 60, 220}
	cells := []Cell{cluster(red, 32000), cluster(blue, 32000)}
	for i := 1; i < 16; i++ {
		cells = append(cells, cluster(mixRGB(red, blue, float64(i)/16), 100))
	}
	if pal := Wu(cells, 0, 0.08); len(pal) != 2 {
		t.Fatalf("got %d entries %v, want red and blue", len(pal), pal)
	}
}

// Noise around a flat colour has a thin tail beyond the tolerance; the tail
// is not a subject, so a noisy disc on a noisy ground stays two colours.
func TestWuIgnoresNoiseTail(t *testing.T) {
	img := corpus.Render(corpus.All()[len(corpus.All())-1]) // noisy-disc
	rgb, _, _ := bitmap.Flatten(img)
	pal := Wu(Cells(rgb, 6), 0, 0.08)
	if len(pal) > 3 {
		t.Fatalf("got %d entries %v, want black, white and at most an edge band", len(pal), pal)
	}
}

// TestLookupSecondIsMixPartner: the runner-up is the entry the cell is
// mixed with, not the second nearest. A pixel that is navy with a little
// red (an anti-aliased red edge on navy) lies on the navy–red segment;
// blue is nearer to navy than red is, but explains none of the pixel.
func TestLookupSecondIsMixPartner(t *testing.T) {
	navy, red, blue := [3]uint8{20, 30, 60}, [3]uint8{219, 30, 30}, [3]uint8{30, 60, 220}
	pal := []colour.OKLab{colour.FromSRGB8(navy[0], navy[1], navy[2]), colour.FromSRGB8(red[0], red[1], red[2]), colour.FromSRGB8(blue[0], blue[1], blue[2])}
	l := NewLookup(pal, 6)
	c := mixRGB(navy, red, 0.065)
	n, s2, b := l.At(CellIndex(c[0], c[1], c[2], 6))
	if n != 0 || s2 != 1 {
		t.Fatalf("navy with 6.5 %% red: nearest %d second %d, want 0 and 1", n, s2)
	}
	if b < 230 || b > 245 {
		t.Fatalf("blend %d, want about 238", b)
	}
}

// Lookup cells are computed on first use; At must agree with Map and be
// stable across calls.
func TestLookupAtMatchesMap(t *testing.T) {
	pal := []colour.OKLab{colour.FromSRGB8(0, 0, 0), colour.FromSRGB8(255, 255, 255), colour.FromSRGB8(200, 30, 30)}
	l := NewLookup(pal, 6)
	rgb := rgbOf([3]uint8{0, 0, 0}, [3]uint8{255, 255, 255}, [3]uint8{128, 128, 128}, [3]uint8{210, 40, 40}, [3]uint8{128, 128, 128})
	labels, second, blend := make([]uint8, 5), make([]uint8, 5), make([]uint8, 5)
	l.Map(rgb, labels, second, blend)
	for p := 0; p < 5; p++ {
		n1, n2, b := l.At(CellIndex(rgb[3*p], rgb[3*p+1], rgb[3*p+2], 6))
		if n1 != labels[p] || n2 != second[p] || b != blend[p] {
			t.Fatalf("pixel %d: At (%d %d %d) vs Map (%d %d %d)", p, n1, n2, b, labels[p], second[p], blend[p])
		}
	}
	fresh := NewLookup(pal, 6)
	for p := 0; p < 5; p++ {
		idx := CellIndex(rgb[3*p], rgb[3*p+1], rgb[3*p+2], 6)
		a1, a2, ab := fresh.At(idx)
		b1, b2, bb := l.At(idx)
		if a1 != b1 || a2 != b2 || ab != bb {
			t.Fatalf("cell %d differs between a fresh and a used lookup", idx)
		}
	}
}

// A light grey that happens to lie on the black–white mixture line is
// still a colour of its own when it holds far more mass than
// anti-aliasing spreads along that line: a soft shadow crescent on a
// white page, 4 % of the image in one cell, beside the usual thin AA band.
func TestWuKeepsHeavyMixtureCell(t *testing.T) {
	white, black, grey := [3]uint8{255, 255, 255}, [3]uint8{0, 0, 0}, [3]uint8{223, 223, 223}
	cells := []Cell{cluster(white, 28000), cluster(black, 40000), cluster(grey, 3800)}
	for i := 1; i < 16; i++ {
		cells = append(cells, cluster(mixRGB(black, white, float64(i)/16), 30))
	}
	pal := Wu(cells, 0, 0.08)
	want := colour.FromSRGB8(grey[0], grey[1], grey[2])
	best := math.Inf(1)
	for _, c := range pal {
		best = math.Min(best, colour.Dist(c, want))
	}
	if best > 0.02 {
		t.Fatalf("grey crescent lost: nearest palette entry is %.3f away in %v", best, pal)
	}
	if len(pal) > 4 {
		t.Fatalf("got %d entries %v, want black, white, grey and at most an edge band", len(pal), pal)
	}
}

// A flat colour at the end of a ramp from the box's core is a step of
// the ramp, not a subject: cutting it out would run a region boundary
// through the ramp. The box stays whole for the gradient fit.
func TestWuKeepsRampPlateauInItsBox(t *testing.T) {
	cells := []Cell{cluster([3]uint8{255, 255, 255}, 400000), cluster([3]uint8{0, 158, 223}, 250000), cluster([3]uint8{0, 174, 219}, 100000)}
	for i, g := range []uint8{162, 166, 170} {
		cells = append(cells, cluster([3]uint8{0, g, 221}, []int{3400, 5000, 8000}[i]))
	}
	for k := 1; k <= 6; k++ {
		cells = append(cells, cluster([3]uint8{0, uint8(158 - 4*k), uint8(223 - 4*k)}, 10000))
	}
	if pal := Wu(cells, 0, 0.04); len(pal) != 2 {
		t.Fatalf("got %d entries %v, want white and one cyan box", len(pal), pal)
	}
}

// field is a w×h sRGB buffer of one colour with a bounded ±amp texture,
// the same offset on every channel, so every pixel stays within reach
// of the colour's cell.
func field(w, h int, c [3]uint8, amp int) []uint8 {
	rgb := make([]uint8, w*h*3)
	for p := 0; p < w*h; p++ {
		d := (p*7919)%(2*amp+1) - amp
		for k := 0; k < 3; k++ {
			rgb[3*p+k] = uint8(min(max(int(c[k])+d, 0), 255))
		}
	}
	return rgb
}

// disc paints a disc of radius r at (cx, cy) ramping from a to b over
// eight levels left to right.
func disc(rgb []uint8, w, cx, cy, r int, a, b [3]uint8) {
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			if (x-cx)*(x-cx)+(y-cy)*(y-cy) > r*r {
				continue
			}
			t := float64((x-cx+r)*8/(2*r+1)) / 7
			for k := 0; k < 3; k++ {
				rgb[3*(y*w+x)+k] = uint8(float64(a[k]) + t*(float64(b[k])-float64(a[k])) + 0.5)
			}
		}
	}
}

func palDist(pal []colour.OKLab, c [3]uint8) float64 {
	best := 10.0
	for _, p := range pal {
		best = min(best, colour.Dist(p, colour.FromSRGB8(c[0], c[1], c[2])))
	}
	return best
}

// Two shaded objects of 0.7 % each in a near-grey field dilute each other
// in the field's box: neither has one cell heavy enough on its own, but
// each is a colour group that is.
func TestWuGroupsTwoShadedSubjects(t *testing.T) {
	w, h := 512, 512
	rgb := field(w, h, [3]uint8{0xcf, 0xc6, 0xbf}, 2)
	disc(rgb, w, 128, 256, 24, [3]uint8{0xf7, 0x75, 0x2a}, [3]uint8{0xc8, 0x5a, 0x1c})
	disc(rgb, w, 384, 256, 24, [3]uint8{0xff, 0xff, 0xff}, [3]uint8{0xe8, 0xe8, 0xe8})
	pal := Wu(Cells(rgb, 6), 0, 0.08)
	if len(pal) < 3 {
		t.Fatalf("got %d entries, want the field, orange and white: %v", len(pal), pal)
	}
	for _, want := range [][3]uint8{{0xe0, 0x68, 0x23}, {0xf4, 0xf4, 0xf4}} {
		if d := palDist(pal, want); d > 0.06 {
			t.Errorf("%v not in palette (nearest %.3f): %v", want, d, pal)
		}
	}
}

// A patch under 1/512 of the image is not a group: a gradient's tail of a
// few hundred pixels on a large image must not force a split.
func TestWuGroupFloorKeepsRamp(t *testing.T) {
	w, h := 1024, 1024
	ramp := func() []uint8 {
		rgb := make([]uint8, w*h*3)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				v := uint8(x * 255 / (w - 1))
				i := 3 * (y*w + x)
				rgb[i], rgb[i+1], rgb[i+2] = v, v, v
			}
		}
		return rgb
	}
	plain := len(Wu(Cells(ramp(), 6), 0, 0.08))
	// The patch is shaded over eight levels so no single cell holds a
	// quarter of the far pile: only a colour group could force a split.
	rgb := ramp()
	for p := 0; p < 150; p++ {
		i := 3 * (512*w + 512 + p)
		t := float64(p*8/150) / 7
		rgb[i] = uint8(0xf7 - t*(0xf7-0xc8))
		rgb[i+1] = uint8(0x75 - t*(0x75-0x5a))
		rgb[i+2] = uint8(0x2a - t*(0x2a-0x1c))
	}
	if got := len(Wu(Cells(rgb, 6), 0, 0.08)); got != plain {
		t.Fatalf("150-pixel patch changed the palette from %d to %d entries", plain, got)
	}
}

// grain is a w×h sRGB buffer of one colour under Gaussian luminance
// noise of the given sigma, seeded, so its far tail reaches past the
// tolerance in both directions the way film grain and sensor noise do.
func grain(w, h int, c [3]uint8, sigma float64) []uint8 {
	r := rand.New(rand.NewSource(1))
	rgb := make([]uint8, w*h*3)
	for p := 0; p < w*h; p++ {
		d := int(math.Round(r.NormFloat64() * sigma))
		for k := 0; k < 3; k++ {
			rgb[3*p+k] = uint8(min(max(int(c[k])+d, 0), 255))
		}
	}
	return rgb
}

// Noise on a flat field is not a subject. Its far tail groups (a cap of
// the noise at one end holds a quarter of the far mass within the
// tolerance of one cell) and clears the floor on any field of size, so
// the far rule needs the group halo too: the cells next to a noise group
// are the field's own tail, and they are heavy.
func TestWuGroupIgnoresNoise(t *testing.T) {
	for _, tc := range []struct {
		c     [3]uint8
		sigma float64
		tol   float64
	}{
		{[3]uint8{0x30, 0x60, 0xa0}, 20, 0.08},
		{[3]uint8{0xcf, 0xc6, 0xbf}, 16, 0.06},
	} {
		rgb := grain(512, 512, tc.c, tc.sigma)
		if pal := Wu(Cells(rgb, 6), 0, tc.tol); len(pal) != 1 {
			t.Errorf("%v sigma %v tol %v: got %d entries for a noisy flat field: %v", tc.c, tc.sigma, tc.tol, len(pal), pal)
		}
	}
}

// A plain ramp has no hidden subject: the tail of a ramp box is a run of
// cells in a chain, and the chain continues on both sides of that run.
// The group halo must see the chain's neighbours, or every ramp box with
// a tail would promote a band of itself (the gradient-h golden grew from
// 5 to 7 entries when the halo was taken around the seed alone).
func TestWuGroupHaloSeesRampNeighbours(t *testing.T) {
	w, h := 256, 256
	rgb := make([]uint8, w*h*3)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := uint8(x * 255 / (w - 1))
			i := 3 * (y*w + x)
			rgb[i], rgb[i+1], rgb[i+2] = v, v, v
		}
	}
	// Five is what the RMS test alone gives this ramp at the poster
	// tolerance; the group rules may not add to it.
	if got := len(Wu(Cells(rgb, 6), 0, 0.08)); got != 5 {
		t.Fatalf("ramp palette has %d entries, want 5", got)
	}
}

// A hidden object shaded past the tolerance has a heavy halo of its own:
// the shades beyond the group's reach sit next to it. The far rule cannot
// use the halo to tell it from noise, so it looks across the centroid
// instead: noise has a mirror cap, an object has nothing opposite.
func TestWuFarGroupSplitsShadedObjectWithHalo(t *testing.T) {
	rgb := field(512, 512, [3]uint8{0x80, 0x7a, 0x74}, 2)
	disc(rgb, 512, 256, 256, 32, [3]uint8{0xff, 0xff, 0xff}, [3]uint8{0xc0, 0xc0, 0xc0})
	pal := Wu(Cells(rgb, 6), 0, 0.08)
	if len(pal) < 2 {
		t.Fatalf("got %d entries, want the field and the disc apart: %v", len(pal), pal)
	}
	if d := palDist(pal, [3]uint8{0xf0, 0xf0, 0xf0}); d > 0.08 {
		t.Fatalf("nearest entry to the disc is %.3f away: %v", d, pal)
	}
}

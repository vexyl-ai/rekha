package region

import (
	"testing"

	"github.com/vexyl-ai/rekha/internal/colour"
)

// squareScene is a w×w grey image (label 0) with a side×side square of
// colour c in the middle inside a one-pixel black outline (label 1), as
// a drawn object sits in its line; the square's pixels carry label 0,
// the grey the quantiser found nearest.
func squareScene(w, side int, c [3]uint8) (labels, rgb []uint8, inSquare func(p int) bool) {
	labels = make([]uint8, w*w)
	rgb = make([]uint8, 3*w*w)
	lo, hi := (w-side)/2, (w-side)/2+side
	inSquare = func(p int) bool {
		x, y := p%w, p/w
		return x >= lo && x < hi && y >= lo && y < hi
	}
	ring := func(p int) bool {
		x, y := p%w, p/w
		return x >= lo-1 && x < hi+1 && y >= lo-1 && y < hi+1 && !inSquare(p)
	}
	for p := range labels {
		col := [3]uint8{200, 200, 200}
		switch {
		case inSquare(p):
			col = c
		case ring(p):
			col = [3]uint8{0, 0, 0}
			labels[p] = 1
		}
		rgb[3*p], rgb[3*p+1], rgb[3*p+2] = col[0], col[1], col[2]
	}
	return labels, rgb, inSquare
}

var (
	greyL  = colour.FromSRGB8(200, 200, 200)
	blackL = colour.FromSRGB8(0, 0, 0)
	blue8  = [3]uint8{60, 90, 200}
)

func checkSquare(t *testing.T, labels []uint8, inSquare func(int) bool, want uint8) {
	t.Helper()
	for p, l := range labels {
		exp := uint8(0)
		if inSquare(p) {
			exp = want
		}
		if l != exp && l != 1 {
			t.Fatalf("pixel %d labelled %d, want %d", p, l, exp)
		}
	}
}

// A region of a colour the palette lacks gets its own entry at its mean.
func TestRescueAddsMissingColour(t *testing.T) {
	labels, rgb, in := squareScene(20, 10, blue8)
	pal := Rescue(labels, rgb, 20, 20, []colour.OKLab{greyL, blackL}, 4, 0.08)
	if len(pal) != 3 {
		t.Fatalf("palette %v, want grey, black and blue", pal)
	}
	if d := colour.Dist(pal[2], colour.FromSRGB8(blue8[0], blue8[1], blue8[2])); d > 0.01 {
		t.Fatalf("new entry %s is %.3f from the square's colour", pal[2].Hex(), d)
	}
	checkSquare(t, labels, in, 2)
}

// Below sixteen pixels a blob is left to the speck and sliver rules.
func TestRescueIgnoresSmallBlob(t *testing.T) {
	labels, rgb, in := squareScene(20, 3, blue8)
	pal := Rescue(labels, rgb, 20, 20, []colour.OKLab{greyL, blackL}, 4, 0.08)
	if len(pal) != 2 {
		t.Fatalf("palette %v, want grey and black alone", pal)
	}
	checkSquare(t, labels, in, 0)
}

// When an existing entry is within tolerance of the region's mean, the
// region moves to it rather than adding a near-duplicate.
func TestRescueRelabelsToExistingColour(t *testing.T) {
	labels, rgb, in := squareScene(20, 10, blue8)
	near := colour.FromSRGB8(66, 94, 196)
	pal := Rescue(labels, rgb, 20, 20, []colour.OKLab{greyL, blackL, near}, 4, 0.08)
	if len(pal) != 3 || pal[2] != near {
		t.Fatalf("palette %v, want grey, black and the existing blue", pal)
	}
	checkSquare(t, labels, in, 2)
}

// Two regions of one missing colour share one new entry.
func TestRescueSharesEntryBetweenRegions(t *testing.T) {
	w := 30
	labels := make([]uint8, w*w)
	rgb := make([]uint8, 3*w*w)
	box := func(p, x0, y0, n int) bool { x, y := p%w, p/w; return x >= x0 && x < x0+n && y >= y0 && y < y0+n }
	sq := func(p, x0, y0 int) bool { return box(p, x0, y0, 8) }
	ring := func(p, x0, y0 int) bool { return box(p, x0-1, y0-1, 10) && !sq(p, x0, y0) }
	for p := range labels {
		col := [3]uint8{200, 200, 200}
		switch {
		case sq(p, 2, 2) || sq(p, 18, 18):
			col = blue8
		case ring(p, 2, 2) || ring(p, 18, 18):
			col = [3]uint8{0, 0, 0}
			labels[p] = 1
		}
		rgb[3*p], rgb[3*p+1], rgb[3*p+2] = col[0], col[1], col[2]
	}
	pal := Rescue(labels, rgb, w, w, []colour.OKLab{greyL, blackL}, 4, 0.08)
	if len(pal) != 3 {
		t.Fatalf("palette %v, want grey, black and one blue", pal)
	}
	for p, l := range labels {
		if (l == 2) != (sq(p, 2, 2) || sq(p, 18, 18)) {
			t.Fatalf("pixel %d labelled %d", p, l)
		}
	}
}

// Without the pixels there is nothing to measure against.
func TestRescueNeedsPixels(t *testing.T) {
	labels, _, _ := squareScene(20, 10, blue8)
	pal := Rescue(labels, nil, 20, 20, []colour.OKLab{greyL, blackL}, 4, 0.08)
	if len(pal) != 2 {
		t.Fatalf("palette %v", pal)
	}
}

// A region whose pixels are unlike each other (anti-aliased glyph strokes:
// black and light grey, mean a dark grey no pixel has) keeps its label:
// the mean is not a colour of the image.
func TestRescueSkipsMixedRegion(t *testing.T) {
	labels, rgb, in := squareScene(20, 10, blue8)
	for p := range labels {
		if !in(p) {
			continue
		}
		v := uint8(0)
		if (p%20+p/20)%2 == 1 {
			v = 220
		}
		rgb[3*p], rgb[3*p+1], rgb[3*p+2] = v, v, v
	}
	pal := Rescue(labels, rgb, 20, 20, []colour.OKLab{greyL, blackL}, 4, 0.08)
	if len(pal) != 2 {
		t.Fatalf("palette %v, want grey and black alone", pal)
	}
	checkSquare(t, labels, in, 0)
}

// A region whose pixels run through a range of colours (a rim light from
// cream to white), none of which is its label, gets the mean: no entry
// fits a third of its pixels, and the mean is the colour nearest all
// of them.
func TestRescueTakesMeanOfGradedRegion(t *testing.T) {
	labels, rgb, in := squareScene(20, 10, blue8)
	for p := range labels {
		if !in(p) {
			continue
		}
		// Ten columns from (60,90,200) to (150,180,250): neighbours a few
		// levels apart, the ends far from each other and from grey.
		x := p%20 - 5
		rgb[3*p], rgb[3*p+1], rgb[3*p+2] = uint8(60+10*x), uint8(90+10*x), uint8(200+5*x)
	}
	pal := Rescue(labels, rgb, 20, 20, []colour.OKLab{greyL, blackL}, 4, 0.04)
	if len(pal) != 3 {
		t.Fatalf("palette %v, want grey, black and the square's mean", pal)
	}
	checkSquare(t, labels, in, 2)
}

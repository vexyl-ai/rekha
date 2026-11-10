package region

import (
	"testing"

	"github.com/vexyl-ai/rekha/internal/colour"
)

var (
	red    = colour.FromSRGB8(255, 0, 0)
	blue   = colour.FromSRGB8(0, 0, 255)
	yellow = colour.FromSRGB8(255, 255, 0)
	purple = colour.OKLab{L: (red.L + blue.L) / 2, A: (red.A + blue.A) / 2, B: (red.B + blue.B) / 2}
)

// grid builds a w×h label map from a function of x, y.
func grid(w, h int, f func(x, y int) uint8) []uint8 {
	out := make([]uint8, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			out[y*w+x] = f(x, y)
		}
	}
	return out
}

func allEqual(labels []uint8, want uint8) bool {
	for _, l := range labels {
		if l != want {
			return false
		}
	}
	return true
}

func TestCleanupAbsorbsSpeck(t *testing.T) {
	labels := grid(8, 8, func(x, y int) uint8 {
		if y == 3 && (x == 3 || x == 4) {
			return 1
		}
		return 0
	})
	pal := Cleanup(labels, nil, 8, 8, []colour.OKLab{red, blue}, 4, 0.08)
	if len(pal) != 1 || pal[0] != red || !allEqual(labels, 0) {
		t.Fatalf("palette %v labels %v", pal, labels)
	}
}

func TestCleanupAbsorbsAABand(t *testing.T) {
	labels := grid(9, 4, func(x, y int) uint8 {
		switch {
		case x < 4:
			return 0
		case x == 4:
			return 2
		default:
			return 1
		}
	})
	pal := Cleanup(labels, nil, 9, 4, []colour.OKLab{red, blue, purple}, 4, 0.08)
	if len(pal) != 2 || pal[0] != red || pal[1] != blue {
		t.Fatalf("palette %v", pal)
	}
	for y := 0; y < 4; y++ {
		for x := 0; x < 9; x++ {
			want := uint8(0) // tie between red and blue borders goes to the lower label
			if x > 4 {
				want = 1
			}
			if labels[y*9+x] != want {
				t.Fatalf("pixel (%d,%d) = %d, want %d", x, y, labels[y*9+x], want)
			}
		}
	}
}

func TestCleanupKeepsThinLine(t *testing.T) {
	labels := grid(9, 4, func(x, y int) uint8 {
		switch {
		case x < 4:
			return 0
		case x == 4:
			return 2
		default:
			return 1
		}
	})
	pal := Cleanup(labels, nil, 9, 4, []colour.OKLab{red, blue, yellow}, 4, 0.08)
	if len(pal) != 3 || pal[labels[4]] != yellow {
		t.Fatalf("palette %v, pixel (4,0) = %d", pal, labels[4])
	}
}

func TestCleanupSingleNeighbourSliverKept(t *testing.T) {
	labels := grid(8, 8, func(x, y int) uint8 {
		if x == 0 {
			return 1
		}
		return 0
	})
	pal := Cleanup(labels, nil, 8, 8, []colour.OKLab{red, purple}, 4, 0.08)
	if len(pal) != 2 || pal[labels[0]] != purple {
		t.Fatalf("palette %v, pixel (0,0) = %d", pal, labels[0])
	}
}

func TestCleanupCompactsInScanOrder(t *testing.T) {
	labels := grid(8, 4, func(x, y int) uint8 {
		if x < 4 {
			return 3
		}
		return 1
	})
	pal := Cleanup(labels, nil, 8, 4, []colour.OKLab{yellow, blue, purple, red}, 0, 0.08)
	if len(pal) != 2 || pal[0] != red || pal[1] != blue {
		t.Fatalf("palette %v", pal)
	}
	if labels[0] != 0 || labels[7] != 1 {
		t.Fatalf("labels %v", labels[:8])
	}
}

// mixBand is a 9×4 map: cyan on the left, navy on the right, and one
// column labelled with the third palette entry whose pixels are col.
func mixBand(col [3]uint8) (labels, rgb []uint8) {
	cyan, navy := [3]uint8{38, 184, 219}, [3]uint8{2, 58, 130}
	labels = grid(9, 4, func(x, y int) uint8 {
		switch {
		case x < 4:
			return 0
		case x == 4:
			return 2
		default:
			return 1
		}
	})
	rgb = make([]uint8, 3*9*4)
	for p, l := range labels {
		c := [][3]uint8{cyan, navy, col}[l]
		rgb[3*p], rgb[3*p+1], rgb[3*p+2] = c[0], c[1], c[2]
	}
	return labels, rgb
}

var (
	cyanL = colour.FromSRGB8(38, 184, 219)
	navyL = colour.FromSRGB8(2, 58, 130)
	tealL = colour.FromSRGB8(52, 145, 132) // a real third colour, 0.09 off the cyan–navy segment
)

// Anti-aliased pixels between cyan and navy quantise to teal when teal is
// the nearest palette entry; the band's pixels, not teal, say what it is.
func TestCleanupAbsorbsMixBandLabelledAnotherColour(t *testing.T) {
	labels, rgb := mixBand([3]uint8{20, 118, 174}) // the sRGB midpoint of cyan and navy
	pal := Cleanup(labels, rgb, 9, 4, []colour.OKLab{cyanL, navyL, tealL}, 4, 0.08)
	if len(pal) != 2 {
		t.Fatalf("palette %v: the mix band must join a neighbour", pal)
	}
}

func TestCleanupKeepsThinLineOfItsOwnColour(t *testing.T) {
	labels, rgb := mixBand([3]uint8{52, 145, 132})
	pal := Cleanup(labels, rgb, 9, 4, []colour.OKLab{cyanL, navyL, tealL}, 4, 0.08)
	if len(pal) != 3 {
		t.Fatalf("palette %v: a teal line between cyan and navy is content", pal)
	}
}

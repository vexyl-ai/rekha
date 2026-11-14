package region

import (
	"math"
	"testing"

	"github.com/vexyl-ai/rekha/internal/colour"
)

const reclaimTol = 0.04

var (
	skinLab   = colour.OKLab{L: 0.86, A: 0.045, B: 0.075}
	shadowLab = shiftL(skinLab, -0.10)
)

// shiftL is c with its lightness moved by dL.
func shiftL(c colour.OKLab, dL float64) colour.OKLab {
	return colour.OKLab{L: c.L + dL, A: c.A, B: c.B}
}

// field is a w×h image of one colour, labelled 0 throughout.
func field(w, h int, c colour.OKLab) (labels, rgb []uint8) {
	labels = make([]uint8, w*h)
	rgb = make([]uint8, 3*w*h)
	r, g, b := c.ToSRGB8()
	for p := 0; p < w*h; p++ {
		rgb[3*p], rgb[3*p+1], rgb[3*p+2] = r, g, b
	}
	return labels, rgb
}

// paint fills the rectangle x0..x1, y0..y1 (exclusive) with c.
func paint(rgb []uint8, w, x0, y0, x1, y1 int, c colour.OKLab) {
	r, g, b := c.ToSRGB8()
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			p := y*w + x
			rgb[3*p], rgb[3*p+1], rgb[3*p+2] = r, g, b
		}
	}
}

// labelIn is the one label every pixel of the rectangle carries, or -1.
func labelIn(labels []uint8, w, x0, y0, x1, y1 int) int {
	l := -1
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			v := int(labels[y*w+x])
			if l < 0 {
				l = v
			} else if l != v {
				return -1
			}
		}
	}
	return l
}

func TestReclaimBlobInsideComponent(t *testing.T) {
	labels, rgb := field(64, 64, skinLab)
	paint(rgb, 64, 20, 20, 40, 32, shadowLab)
	pal := Reclaim(labels, rgb, 64, 64, []colour.OKLab{skinLab}, reclaimTol)
	if len(pal) != 2 {
		t.Fatalf("palette %d entries, want 2", len(pal))
	}
	if got := labelIn(labels, 64, 20, 20, 40, 32); got != 1 {
		t.Fatalf("patch label %d, want 1", got)
	}
	if got := labelIn(labels, 64, 0, 0, 64, 20); got != 0 {
		t.Fatalf("field above the patch label %d, want 0", got)
	}
	if d := colour.Dist(pal[1], shadowLab); d > 0.005 {
		t.Fatalf("new entry %.4f from the patch colour", d)
	}
}

func TestReclaimTakesExistingEntry(t *testing.T) {
	labels, rgb := field(64, 64, skinLab)
	paint(rgb, 64, 20, 20, 40, 32, shadowLab)
	near := shiftL(shadowLab, 0.02)
	pal := Reclaim(labels, rgb, 64, 64, []colour.OKLab{skinLab, near}, reclaimTol)
	if len(pal) != 2 {
		t.Fatalf("palette %d entries, want 2", len(pal))
	}
	if got := labelIn(labels, 64, 20, 20, 40, 32); got != 1 {
		t.Fatalf("patch label %d, want the existing entry 1", got)
	}
}

func TestReclaimIgnoresStrand(t *testing.T) {
	labels, rgb := field(256, 64, skinLab)
	paint(rgb, 256, 20, 30, 220, 32, shadowLab)
	pal := Reclaim(labels, rgb, 256, 64, []colour.OKLab{skinLab}, reclaimTol)
	if len(pal) != 1 {
		t.Fatalf("palette %d entries, want 1", len(pal))
	}
	if got := labelIn(labels, 256, 0, 0, 256, 64); got != 0 {
		t.Fatalf("labels changed under a two-pixel strand")
	}
}

func TestReclaimIgnoresRamp(t *testing.T) {
	labels, rgb := field(64, 64, skinLab)
	for x := 20; x < 40; x++ {
		c := shiftL(skinLab, -0.24*float64(x-20)/19)
		paint(rgb, 64, x, 20, x+1, 32, c)
	}
	pal := Reclaim(labels, rgb, 64, 64, []colour.OKLab{skinLab}, reclaimTol)
	if len(pal) != 1 {
		t.Fatalf("palette %d entries, want 1: a ramp is not a blob of one colour", len(pal))
	}
}

func TestReclaimIgnoresSmall(t *testing.T) {
	labels, rgb := field(64, 64, skinLab)
	paint(rgb, 64, 20, 20, 28, 26, shadowLab)
	pal := Reclaim(labels, rgb, 64, 64, []colour.OKLab{skinLab}, reclaimTol)
	if len(pal) != 1 {
		t.Fatalf("palette %d entries, want 1 for a 48-pixel patch", len(pal))
	}
}

func TestReclaimBlobAtBorder(t *testing.T) {
	labels, rgb := field(64, 64, skinLab)
	paint(rgb, 64, 0, 0, 64, 20, shadowLab)
	pal := Reclaim(labels, rgb, 64, 64, []colour.OKLab{skinLab}, reclaimTol)
	if len(pal) != 2 {
		t.Fatalf("palette %d entries, want 2", len(pal))
	}
	// The band comes first in scan order, so compaction gives it label 0.
	band, rest := labelIn(labels, 64, 0, 0, 64, 20), labelIn(labels, 64, 0, 20, 64, 64)
	if band < 0 || rest < 0 || band == rest {
		t.Fatalf("band label %d, field label %d, want two labels", band, rest)
	}
	if d := colour.Dist(pal[band], shadowLab); d > 0.005 {
		t.Fatalf("band entry %.4f from its colour", d)
	}
}

func TestReclaimSharesAddedEntry(t *testing.T) {
	labels, rgb := field(64, 64, skinLab)
	paint(rgb, 64, 4, 20, 24, 32, shadowLab)
	paint(rgb, 64, 40, 20, 60, 32, shadowLab)
	pal := Reclaim(labels, rgb, 64, 64, []colour.OKLab{skinLab}, reclaimTol)
	if len(pal) != 2 {
		t.Fatalf("palette %d entries, want 2: the second patch shares the first's", len(pal))
	}
	if a, b := labelIn(labels, 64, 4, 20, 24, 32), labelIn(labels, 64, 40, 20, 60, 32); a != 1 || b != 1 {
		t.Fatalf("patch labels %d and %d, want 1 and 1", a, b)
	}
}

func TestReclaimCapsEntries(t *testing.T) {
	const w = 256
	labels, rgb := field(w, w, skinLab)
	k := 0
	for y := 0; y+12 <= w && k < 40; y += 16 {
		for x := 0; x+12 <= w && k < 40; x += 16 {
			hue := 2 * math.Pi * float64(k) / 40
			// Neighbouring hues 0.05 apart, so no two patches share an entry.
			c := colour.OKLab{L: 0.45 + 0.1*float64(k%2), A: 0.32 * math.Cos(hue), B: 0.32 * math.Sin(hue)}
			paint(rgb, w, x, y, x+12, y+12, c)
			k++
		}
	}
	pal := Reclaim(labels, rgb, w, w, []colour.OKLab{skinLab}, reclaimTol)
	if len(pal) > 1+reclaimMax {
		t.Fatalf("palette %d entries, want at most %d", len(pal), 1+reclaimMax)
	}
	if len(pal) < 1+reclaimMax {
		t.Fatalf("palette %d entries, want the cap %d reached with 40 far patches", len(pal), 1+reclaimMax)
	}
}

package region

import (
	"math"
	"testing"

	"github.com/vexyl-ai/rekha/internal/colour"
)

// bands builds a 20-wide, 4-tall map of vertical bands, one label per
// colour in cols, each band 20/len(cols) pixels wide and filled with
// its colour.
func bands(cols ...[3]uint8) (labels, rgb []uint8, w, h int) {
	widths := make([]int, len(cols))
	for i := range widths {
		widths[i] = 20 / len(cols)
	}
	return bandsOf(widths, cols...)
}

// bandsOf is bands with the width of each band given, summing to 20.
func bandsOf(widths []int, cols ...[3]uint8) (labels, rgb []uint8, w, h int) {
	w, h = 20, 4
	labels = make([]uint8, w*h)
	rgb = make([]uint8, 3*w*h)
	for p := range labels {
		x, l := p%w, 0
		for l+1 < len(widths) && x >= widths[l] {
			x -= widths[l]
			l++
		}
		labels[p] = uint8(l)
		c := cols[l]
		rgb[3*p], rgb[3*p+1], rgb[3*p+2] = c[0], c[1], c[2]
	}
	return labels, rgb, w, h
}

func labs(cols ...[3]uint8) []colour.OKLab {
	out := make([]colour.OKLab, len(cols))
	for i, c := range cols {
		out[i] = colour.FromSRGB8(c[0], c[1], c[2])
	}
	return out
}

// Fixture colours, with their OKLab lightness: the browns are one hue
// (0.86–0.92 rad) in steps of 0.08 and 0.09 L; olive is 0.8 rad away
// from brown at the pale brown's lightness; grey is neutral at the mid
// brown's lightness.
var (
	brownDark = [3]uint8{64, 37, 22}   // L 0.295
	brownMid  = [3]uint8{90, 56, 36}   // L 0.376
	brownPale = [3]uint8{120, 80, 55}  // L 0.469
	brownTan  = [3]uint8{150, 100, 70} // L 0.550
	olive     = [3]uint8{113, 106, 72} // L 0.522
	grey      = [3]uint8{70, 70, 70}   // L 0.394
)

// Two shades of one hue side by side are one flat colour: the colour of
// the larger, which the smaller is the shading of (a logo's brand red
// does not drift when its highlight crescent joins it).
func TestFlattenMergesLightnessStep(t *testing.T) {
	labels, rgb, w, h := bandsOf([]int{8, 12}, brownDark, brownMid)
	pal := Flatten(labels, rgb, w, h, labs(brownDark, brownMid), 0.12)
	if len(pal) != 1 {
		t.Fatalf("palette %v, want one brown", pal)
	}
	want := colour.FromSRGB8(brownMid[0], brownMid[1], brownMid[2])
	if d := colour.Dist(pal[0], want); d > 1e-6 {
		t.Fatalf("merged colour %s, want the larger band's %s (%.4f off)", pal[0].Hex(), want.Hex(), d)
	}
	if !allEqual(labels, 0) {
		t.Fatal("labels not all merged")
	}
}

// A hue step at the same lightness is content, not shading.
func TestFlattenKeepsHueStep(t *testing.T) {
	labels, rgb, w, h := bands(brownTan, olive)
	a, b := labs(brownTan, olive)[0], labs(brownTan, olive)[1]
	if math.Abs(a.L-b.L) > 0.12 {
		t.Fatalf("fixture: lightness differs by %.3f", a.L-b.L)
	}
	pal := Flatten(labels, rgb, w, h, labs(brownTan, olive), 0.12)
	if len(pal) != 2 {
		t.Fatalf("palette %v, want both", pal)
	}
}

// A neutral beside a chromatic colour of the same lightness stays: the
// grey outline is not the hair's shade.
func TestFlattenKeepsNeutralBesideChromatic(t *testing.T) {
	labels, rgb, w, h := bands(grey, brownMid)
	pal := Flatten(labels, rgb, w, h, labs(grey, brownMid), 0.12)
	if len(pal) != 2 {
		t.Fatalf("palette %v, want both", pal)
	}
}

// A chain of steps merges while the next step is within the light
// tolerance of the merged group's colour (its largest member's), and
// stops there: mid joins dark, and pale, 0.17 from dark, stays apart
// although it is only 0.09 from mid.
func TestFlattenChainStopsAtGroupColour(t *testing.T) {
	labels, rgb, w, h := bandsOf([]int{10, 5, 5}, brownDark, brownMid, brownPale)
	l := labs(brownDark, brownMid, brownPale)
	if d := l[1].L - l[0].L; d > 0.12 {
		t.Fatalf("fixture: first step %.3f", d)
	}
	if d := l[2].L - l[1].L; d > 0.12 {
		t.Fatalf("fixture: second step %.3f", d)
	}
	if d := l[2].L - l[0].L; d <= 0.12 {
		t.Fatalf("fixture: pale is %.3f from dark, want more than 0.12", d)
	}
	pal := Flatten(labels, rgb, w, h, l, 0.12)
	if len(pal) != 2 || colour.Dist(pal[0], l[0]) > 1e-9 {
		t.Fatalf("palette %v, want dark (with mid in it) and pale", pal)
	}
}

// Labels that never touch are not merged, whatever their colours.
func TestFlattenNeedsAdjacency(t *testing.T) {
	labels, rgb, w, h := bands(brownPale, olive, brownTan)
	pal := Flatten(labels, rgb, w, h, labs(brownPale, olive, brownTan), 0.12)
	if len(pal) != 3 {
		t.Fatalf("palette %v, want all three", pal)
	}
}

// TestFlattenLightnessThreshold: the threshold is the caller's. The
// dark and mid browns are 0.08 apart in lightness: at 0.05 they stay
// two colours, at 0.12 they merge.
func TestFlattenLightnessThreshold(t *testing.T) {
	labels, rgb, w, h := bandsOf([]int{8, 12}, brownDark, brownMid)
	if pal := Flatten(labels, rgb, w, h, labs(brownDark, brownMid), 0.05); len(pal) != 2 {
		t.Fatalf("at 0.05: %d colours, want 2", len(pal))
	}
	if pal := Flatten(labels, rgb, w, h, labs(brownDark, brownMid), 0.12); len(pal) != 1 {
		t.Fatalf("at 0.12: %d colours, want 1", len(pal))
	}
}

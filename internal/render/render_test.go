package render

import (
	"image/color"
	"math"
	"testing"

	"github.com/vexyl-ai/rekha/internal/fit"
	"github.com/vexyl-ai/rekha/internal/geom"
	"github.com/vexyl-ai/rekha/internal/svg"
)

func v(x, y float64) geom.Vec2 { return geom.Vec2{X: x, Y: y} }

func line(a, b geom.Vec2) fit.Segment { return fit.Segment{Line: true, P: fit.Cubic{a, a, b, b}} }

// rect is a closed subpath round (x0,y0)-(x1,y1), clockwise in image
// coordinates when x1 > x0 and y1 > y0; swap x0 and x1 to reverse it.
func rect(x0, y0, x1, y1 float64) svg.Subpath {
	return svg.Subpath{Segs: []fit.Segment{
		line(v(x0, y0), v(x1, y0)), line(v(x1, y0), v(x1, y1)), line(v(x1, y1), v(x0, y1)), line(v(x0, y1), v(x0, y0)),
	}}
}

func at(t *testing.T, d svg.Document, x, y int) color.NRGBA {
	t.Helper()
	img := Paint(d)
	return color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
}

func near(a, b color.NRGBA, tol int) bool {
	d := func(x, y uint8) int { return int(math.Abs(float64(x) - float64(y))) }
	return d(a.R, b.R) <= tol && d(a.G, b.G) <= tol && d(a.B, b.B) <= tol && d(a.A, b.A) <= tol
}

func TestPaintFlatRectCoversItsPixelsOnly(t *testing.T) {
	d := svg.Document{Width: 10, Height: 10, Paths: []svg.Path{{Fill: "#ff0000", Subpaths: []svg.Subpath{rect(2, 2, 6, 6)}}}}
	if got := at(t, d, 3, 3); got != (color.NRGBA{255, 0, 0, 255}) {
		t.Errorf("inside: %v", got)
	}
	if got := at(t, d, 5, 5); got != (color.NRGBA{255, 0, 0, 255}) {
		t.Errorf("inside far corner: %v", got)
	}
	for _, p := range [][2]int{{1, 1}, {6, 6}, {8, 8}, {3, 7}} {
		if got := at(t, d, p[0], p[1]); got.A != 0 {
			t.Errorf("outside %v: %v", p, got)
		}
	}
}

func TestPaintHalfCoveredPixelIsHalfOpaque(t *testing.T) {
	d := svg.Document{Width: 4, Height: 4, Paths: []svg.Path{{Fill: "#000000", Subpaths: []svg.Subpath{rect(0, 0, 2.5, 4)}}}}
	got := at(t, d, 2, 1)
	if !near(got, color.NRGBA{0, 0, 0, 128}, 2) {
		t.Errorf("half pixel: %v", got)
	}
}

func TestPaintReversedSubpathIsAHole(t *testing.T) {
	outer := rect(0, 0, 10, 10)
	inner := rect(7, 3, 3, 7) // opposite orientation
	d := svg.Document{Width: 10, Height: 10, Paths: []svg.Path{{Fill: "#00ff00", Subpaths: []svg.Subpath{outer, inner}}}}
	if got := at(t, d, 5, 5); got.A != 0 {
		t.Errorf("hole: %v", got)
	}
	if got := at(t, d, 1, 1); got != (color.NRGBA{0, 255, 0, 255}) {
		t.Errorf("ring: %v", got)
	}
}

func TestPaintLaterPathCoversEarlier(t *testing.T) {
	d := svg.Document{Width: 4, Height: 4, Paths: []svg.Path{
		{Fill: "#ff0000", Subpaths: []svg.Subpath{rect(0, 0, 4, 4)}},
		{Fill: "#0000ff", Subpaths: []svg.Subpath{rect(1, 1, 3, 3)}},
	}}
	if got := at(t, d, 2, 2); got != (color.NRGBA{0, 0, 255, 255}) {
		t.Errorf("top: %v", got)
	}
	if got := at(t, d, 0, 0); got != (color.NRGBA{255, 0, 0, 255}) {
		t.Errorf("under: %v", got)
	}
}

func TestPaintLinearGradientRunsBetweenItsPoints(t *testing.T) {
	d := svg.Document{Width: 10, Height: 2,
		Gradients: []svg.LinearGradient{{ID: "g0", X1: 0, Y1: 0, X2: 10, Y2: 0, Stops: []svg.Stop{{Offset: 0, Colour: "#000000", Opacity: 1}, {Offset: 1, Colour: "#ffffff", Opacity: 1}}}},
		Paths:     []svg.Path{{Fill: "url(#g0)", Subpaths: []svg.Subpath{rect(0, 0, 10, 2)}}},
	}
	// Pixel centres: x = 0.5 → t 0.05, x = 9.5 → t 0.95.
	if got := at(t, d, 0, 0); !near(got, color.NRGBA{13, 13, 13, 255}, 2) {
		t.Errorf("start: %v", got)
	}
	if got := at(t, d, 9, 1); !near(got, color.NRGBA{242, 242, 242, 255}, 2) {
		t.Errorf("end: %v", got)
	}
	if got := at(t, d, 5, 0); !near(got, color.NRGBA{140, 140, 140, 255}, 2) {
		t.Errorf("middle: %v", got)
	}
}

func TestPaintGradientPadsBeyondItsPoints(t *testing.T) {
	d := svg.Document{Width: 10, Height: 1,
		Gradients: []svg.LinearGradient{{ID: "g0", X1: 4, Y1: 0, X2: 6, Y2: 0, Stops: []svg.Stop{{Offset: 0, Colour: "#ff0000", Opacity: 1}, {Offset: 1, Colour: "#0000ff", Opacity: 1}}}},
		Paths:     []svg.Path{{Fill: "url(#g0)", Subpaths: []svg.Subpath{rect(0, 0, 10, 1)}}},
	}
	if got := at(t, d, 0, 0); got != (color.NRGBA{255, 0, 0, 255}) {
		t.Errorf("before: %v", got)
	}
	if got := at(t, d, 9, 0); got != (color.NRGBA{0, 0, 255, 255}) {
		t.Errorf("after: %v", got)
	}
}

func TestPaintRadialGradientRunsFromCentre(t *testing.T) {
	d := svg.Document{Width: 11, Height: 11,
		Radials: []svg.RadialGradient{{ID: "r0", CX: 5.5, CY: 5.5, R: 5, Stops: []svg.Stop{{Offset: 0, Colour: "#ffffff", Opacity: 1}, {Offset: 1, Colour: "#000000", Opacity: 1}}}},
		Paths:   []svg.Path{{Fill: "url(#r0)", Subpaths: []svg.Subpath{rect(0, 0, 11, 11)}}},
	}
	if got := at(t, d, 5, 5); got != (color.NRGBA{255, 255, 255, 255}) {
		t.Errorf("centre: %v", got)
	}
	if got := at(t, d, 0, 0); got != (color.NRGBA{0, 0, 0, 255}) {
		t.Errorf("corner beyond R: %v", got)
	}
	// Pixel (5,3)'s centre (5.5, 3.5) is 2 out: t 0.4.
	if got := at(t, d, 5, 3); !near(got, color.NRGBA{153, 153, 153, 255}, 2) {
		t.Errorf("half way: %v", got)
	}
}

func TestPaintTranslucentStopCompositesOver(t *testing.T) {
	d := svg.Document{Width: 2, Height: 2,
		Gradients: []svg.LinearGradient{{ID: "g0", X1: 0, Y1: 0, X2: 2, Y2: 0, Stops: []svg.Stop{{Offset: 0, Colour: "#0000ff", Opacity: 0.5}, {Offset: 1, Colour: "#0000ff", Opacity: 0.5}}}},
		Paths: []svg.Path{
			{Fill: "#ff0000", Subpaths: []svg.Subpath{rect(0, 0, 2, 2)}},
			{Fill: "url(#g0)", Subpaths: []svg.Subpath{rect(0, 0, 2, 2)}},
		},
	}
	if got := at(t, d, 1, 1); !near(got, color.NRGBA{128, 0, 128, 255}, 2) {
		t.Errorf("blend: %v", got)
	}
}

func TestPaintCubicCircleHasItsArea(t *testing.T) {
	// Four cubic arcs, radius 4 about (5,5): k = 0.5523 r.
	const r, k = 4.0, 0.5523 * 4.0
	c := v(5, 5)
	arc := func(p0, p1, p2, p3 geom.Vec2) fit.Segment { return fit.Segment{P: fit.Cubic{p0, p1, p2, p3}} }
	segs := []fit.Segment{
		arc(v(c.X+r, c.Y), v(c.X+r, c.Y+k), v(c.X+k, c.Y+r), v(c.X, c.Y+r)),
		arc(v(c.X, c.Y+r), v(c.X-k, c.Y+r), v(c.X-r, c.Y+k), v(c.X-r, c.Y)),
		arc(v(c.X-r, c.Y), v(c.X-r, c.Y-k), v(c.X-k, c.Y-r), v(c.X, c.Y-r)),
		arc(v(c.X, c.Y-r), v(c.X+k, c.Y-r), v(c.X+r, c.Y-k), v(c.X+r, c.Y)),
	}
	d := svg.Document{Width: 10, Height: 10, Paths: []svg.Path{{Fill: "#000000", Subpaths: []svg.Subpath{{Segs: segs}}}}}
	img := Paint(d)
	var area float64
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			area += float64(a) / 0xffff
		}
	}
	if want := math.Pi * r * r; math.Abs(area-want) > 0.02*want {
		t.Errorf("area %.2f, want %.2f", area, want)
	}
}

func TestPaintIgnoresStrokeAndUnknownFill(t *testing.T) {
	d := svg.Document{Width: 4, Height: 4, Paths: []svg.Path{
		{Fill: "url(#nope)", Subpaths: []svg.Subpath{rect(0, 0, 4, 4)}},
		{Fill: "#00ff00", Stroke: "#00ff00", StrokeWidth: 1, Subpaths: []svg.Subpath{rect(1, 1, 3, 3)}},
	}}
	if got := at(t, d, 0, 0); got.A != 0 {
		t.Errorf("unknown fill painted: %v", got)
	}
	if got := at(t, d, 2, 2); got != (color.NRGBA{0, 255, 0, 255}) {
		t.Errorf("fill: %v", got)
	}
}

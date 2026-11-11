package svg

import (
	"bytes"
	"strings"
	"testing"

	"github.com/vexyl-ai/rekha/internal/fit"
	"github.com/vexyl-ai/rekha/internal/geom"
)

func ln(a, b geom.Vec2) fit.Segment { return fit.Segment{Line: true, P: fit.Cubic{a, a, b, b}} }

func TestFormatNum(t *testing.T) {
	cases := []struct {
		v    float64
		prec int
		want string
	}{
		{1.5, 2, "1.5"}, {2, 2, "2"}, {-0.001, 2, "0"}, {-0.004, 2, "0"},
		{-0.006, 2, "-0.01"}, {3, 0, "3"}, {-0.4, 0, "0"}, {12.345, 2, "12.35"},
	}
	for _, c := range cases {
		if got := FormatNum(c.v, c.prec); got != c.want {
			t.Errorf("FormatNum(%v,%d) = %q, want %q", c.v, c.prec, got, c.want)
		}
	}
}

func TestPathDataSquareUsesHV(t *testing.T) {
	p := Path{Fill: "#000000", Subpaths: []Subpath{{Segs: []fit.Segment{
		ln(geom.Vec2{X: 0, Y: 0}, geom.Vec2{X: 1, Y: 0}),
		ln(geom.Vec2{X: 1, Y: 0}, geom.Vec2{X: 1, Y: 1}),
		ln(geom.Vec2{X: 1, Y: 1}, geom.Vec2{X: 0, Y: 1}),
		ln(geom.Vec2{X: 0, Y: 1}, geom.Vec2{X: 0, Y: 0}),
	}}}}
	if got := PathData(p, 2); got != "M0 0H1V1H0Z" {
		t.Fatalf("got %q", got)
	}
}

func TestPathDataNegativeNoSpace(t *testing.T) {
	p := Path{Subpaths: []Subpath{{Segs: []fit.Segment{
		ln(geom.Vec2{X: 0, Y: 0}, geom.Vec2{X: -1, Y: -2}),
		ln(geom.Vec2{X: -1, Y: -2}, geom.Vec2{X: 0, Y: 0}),
	}}}}
	if got := PathData(p, 2); got != "M0 0L-1-2Z" {
		t.Fatalf("got %q", got)
	}
}

func TestPathDataCubic(t *testing.T) {
	c := fit.Segment{P: fit.Cubic{{X: 0, Y: 0}, {X: 1, Y: 2}, {X: 3, Y: 2}, {X: 4, Y: 0}}}
	p := Path{Subpaths: []Subpath{{Segs: []fit.Segment{c, ln(geom.Vec2{X: 4, Y: 0}, geom.Vec2{X: 0, Y: 0})}}}}
	if got := PathData(p, 1); got != "M0 0C1 2 3 2 4 0Z" {
		t.Fatalf("got %q", got)
	}
}

func TestWriteDocument(t *testing.T) {
	p := Path{Fill: "#000000", Subpaths: []Subpath{{Segs: []fit.Segment{
		ln(geom.Vec2{X: 0, Y: 0}, geom.Vec2{X: 2, Y: 0}),
		ln(geom.Vec2{X: 2, Y: 0}, geom.Vec2{X: 2, Y: 2}),
		ln(geom.Vec2{X: 2, Y: 2}, geom.Vec2{X: 0, Y: 2}),
		ln(geom.Vec2{X: 0, Y: 2}, geom.Vec2{X: 0, Y: 0}),
	}}}}
	var buf bytes.Buffer
	if err := Write(&buf, 2, 2, []Path{p}, 2); err != nil {
		t.Fatal(err)
	}
	want := "<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"2\" height=\"2\" viewBox=\"0 0 2 2\">\n" +
		"<path fill=\"#000000\" d=\"M0 0H2V2H0Z\"/>\n" +
		"</svg>\n"
	if buf.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", buf.String(), want)
	}
}

// A path with a stroke carries the stroke attributes; one without carries
// none, so existing output is unchanged.
func TestWriteStrokeAttributes(t *testing.T) {
	sq := []Subpath{{Segs: []fit.Segment{
		ln(geom.Vec2{X: 0, Y: 0}, geom.Vec2{X: 2, Y: 0}),
		ln(geom.Vec2{X: 2, Y: 0}, geom.Vec2{X: 2, Y: 2}),
		ln(geom.Vec2{X: 2, Y: 2}, geom.Vec2{X: 0, Y: 2}),
		ln(geom.Vec2{X: 0, Y: 2}, geom.Vec2{X: 0, Y: 0}),
	}}}
	var buf bytes.Buffer
	if err := Write(&buf, 2, 2, []Path{{Fill: "#ff0000", Stroke: "#ff0000", StrokeWidth: 0.5, Subpaths: sq}, {Fill: "#000000", Subpaths: sq}}, 2); err != nil {
		t.Fatal(err)
	}
	want := "<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"2\" height=\"2\" viewBox=\"0 0 2 2\">\n" +
		"<path fill=\"#ff0000\" stroke=\"#ff0000\" stroke-width=\"0.5\" stroke-linejoin=\"round\" d=\"M0 0H2V2H0Z\"/>\n" +
		"<path fill=\"#000000\" d=\"M0 0H2V2H0Z\"/>\n" +
		"</svg>\n"
	if buf.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", buf.String(), want)
	}
}

func square4() []fit.Segment {
	return []fit.Segment{
		ln(geom.Vec2{X: 0, Y: 0}, geom.Vec2{X: 4, Y: 0}),
		ln(geom.Vec2{X: 4, Y: 0}, geom.Vec2{X: 4, Y: 4}),
		ln(geom.Vec2{X: 4, Y: 4}, geom.Vec2{X: 0, Y: 4}),
		ln(geom.Vec2{X: 0, Y: 4}, geom.Vec2{X: 0, Y: 0}),
	}
}

func TestWriteDocumentDefs(t *testing.T) {
	d := Document{Width: 4, Height: 4,
		Gradients: []LinearGradient{{ID: "g0", X1: 0.5, Y1: 1, X2: 3.5, Y2: 1, Stops: []Stop{{Offset: 0, Colour: "#000000", Opacity: 1}, {Offset: 1, Colour: "#ffffff", Opacity: 1}}}},
		Paths:     []Path{{Fill: "url(#g0)", Subpaths: []Subpath{{Segs: square4()}}}}}
	var b bytes.Buffer
	if err := WriteDocument(&b, d, 2); err != nil {
		t.Fatal(err)
	}
	want := "<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"4\" height=\"4\" viewBox=\"0 0 4 4\">\n<defs><linearGradient id=\"g0\" gradientUnits=\"userSpaceOnUse\" x1=\"0.5\" y1=\"1\" x2=\"3.5\" y2=\"1\"><stop offset=\"0\" stop-color=\"#000000\"/><stop offset=\"1\" stop-color=\"#ffffff\"/></linearGradient></defs>\n<path fill=\"url(#g0)\" d=\"M0 0H4V4H0Z\"/>\n</svg>\n"
	if b.String() != want {
		t.Errorf("got\n%s\nwant\n%s", b.String(), want)
	}
}

func TestWriteDocumentNoDefsMatchesWrite(t *testing.T) {
	paths := []Path{{Fill: "#000000", Subpaths: []Subpath{{Segs: square4()}}}}
	var a, b bytes.Buffer
	_ = Write(&a, 4, 4, paths, 2)
	_ = WriteDocument(&b, Document{Width: 4, Height: 4, Paths: paths}, 2)
	if a.String() != b.String() {
		t.Errorf("WriteDocument without gradients differs from Write")
	}
}

func TestWriteDocumentRadialDefs(t *testing.T) {
	d := Document{Width: 4, Height: 4,
		Radials: []RadialGradient{{ID: "g0", CX: 2, CY: 2.25, R: 1.5, Stops: []Stop{{Offset: 0.25, Colour: "#000000", Opacity: 1}, {Offset: 1, Colour: "#ffffff", Opacity: 1}}}},
		Paths:   []Path{{Fill: "url(#g0)", Subpaths: []Subpath{{Segs: square4()}}}}}
	var b bytes.Buffer
	if err := WriteDocument(&b, d, 2); err != nil {
		t.Fatal(err)
	}
	want := "<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"4\" height=\"4\" viewBox=\"0 0 4 4\">\n<defs><radialGradient id=\"g0\" gradientUnits=\"userSpaceOnUse\" cx=\"2\" cy=\"2.25\" r=\"1.5\"><stop offset=\"0.25\" stop-color=\"#000000\"/><stop offset=\"1\" stop-color=\"#ffffff\"/></radialGradient></defs>\n<path fill=\"url(#g0)\" d=\"M0 0H4V4H0Z\"/>\n</svg>\n"
	if b.String() != want {
		t.Errorf("got\n%s\nwant\n%s", b.String(), want)
	}
}

// A stop with an opacity under 1 carries it as stop-opacity; an opaque
// stop (Opacity 1) carries nothing.
func TestWriteDocumentStopOpacity(t *testing.T) {
	d := Document{Width: 4, Height: 4,
		Radials: []RadialGradient{{ID: "g0", CX: 2, CY: 2, R: 2, Stops: []Stop{{Offset: 0, Colour: "#ff0000", Opacity: 0.5}, {Offset: 1, Colour: "#ff0000", Opacity: 0}}}},
		Paths:   []Path{{Fill: "url(#g0)", Subpaths: []Subpath{{Segs: square4()}}}}}
	var b bytes.Buffer
	if err := WriteDocument(&b, d, 2); err != nil {
		t.Fatal(err)
	}
	want := "<defs><radialGradient id=\"g0\" gradientUnits=\"userSpaceOnUse\" cx=\"2\" cy=\"2\" r=\"2\"><stop offset=\"0\" stop-color=\"#ff0000\" stop-opacity=\"0.5\"/><stop offset=\"1\" stop-color=\"#ff0000\" stop-opacity=\"0\"/></radialGradient></defs>\n"
	if !strings.Contains(b.String(), want) {
		t.Errorf("got\n%s\nwant a defs line\n%s", b.String(), want)
	}
}

package svg

import (
	"bytes"
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

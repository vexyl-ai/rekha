package export

import (
	"bytes"
	"compress/zlib"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/vexyl-ai/rekha/internal/fit"
	"github.com/vexyl-ai/rekha/internal/geom"
	"github.com/vexyl-ai/rekha/internal/svg"
)

func pt(x, y float64) geom.Vec2 { return geom.Vec2{X: x, Y: y} }

func line(a, b geom.Vec2) fit.Segment { return fit.Segment{Line: true, P: fit.Cubic{a, a, b, b}} }

// square is a 10×10 document: a red square from (1,1) to (9,9) with a
// square hole from (4,4) to (6,6) wound the other way, and one cubic.
func square() svg.Document {
	outer := svg.Subpath{Segs: []fit.Segment{line(pt(1, 1), pt(9, 1)), line(pt(9, 1), pt(9, 9)), {P: fit.Cubic{pt(9, 9), pt(6, 10), pt(4, 10), pt(1, 9)}}, line(pt(1, 9), pt(1, 1))}}
	hole := svg.Subpath{Segs: []fit.Segment{line(pt(4, 4), pt(4, 6)), line(pt(4, 6), pt(6, 6)), line(pt(6, 6), pt(6, 4)), line(pt(6, 4), pt(4, 4))}}
	return svg.Document{Width: 10, Height: 10, Paths: []svg.Path{{Fill: "#ff0000", Subpaths: []svg.Subpath{outer, hole}}}, Generator: "rekha dev"}
}

func gradientDoc() svg.Document {
	d := square()
	d.Gradients = []svg.LinearGradient{{ID: "g0", X1: 0, Y1: 0, X2: 10, Y2: 0, Stops: []svg.Stop{{Offset: 0, Colour: "#000000", Opacity: 1}, {Offset: 0.25, Colour: "#ff0000", Opacity: 1}, {Offset: 1, Colour: "#ffffff", Opacity: 1}}}}
	d.Radials = []svg.RadialGradient{{ID: "g1", CX: 5, CY: 5, R: 4, Stops: []svg.Stop{{Offset: 0, Colour: "#00ff00", Opacity: 0.5}, {Offset: 1, Colour: "#00ff00", Opacity: 0}}}}
	d.Paths[0].Fill = "url(#g0)"
	d.Paths = append(d.Paths, svg.Path{Fill: "url(#g1)", Subpaths: d.Paths[0].Subpaths})
	return d
}

func pdfOf(t *testing.T, d svg.Document) string {
	t.Helper()
	var b bytes.Buffer
	if err := WritePDF(&b, d, 2); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func epsOf(t *testing.T, d svg.Document) string {
	t.Helper()
	var b bytes.Buffer
	if err := WriteEPS(&b, d, 2); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// content returns the inflated content stream of the page.
func content(t *testing.T, pdf string) string {
	t.Helper()
	m := regexp.MustCompile(`(?s)/Filter /FlateDecode >>\nstream\n(.*?)\nendstream`).FindStringSubmatch(pdf)
	if m == nil {
		t.Fatal("no flate stream")
	}
	r, err := zlib.NewReader(strings.NewReader(m[1]))
	if err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func want(t *testing.T, got string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if !strings.Contains(got, p) {
			t.Errorf("missing %q in:\n%s", p, got)
		}
	}
}

func TestPDFPageAndMark(t *testing.T) {
	pdf := pdfOf(t, square())
	if !strings.HasPrefix(pdf, "%PDF-1.4\n") {
		t.Fatalf("header: %q", pdf[:12])
	}
	want(t, pdf, "/MediaBox [0 0 7.5 7.5]", "/Producer (Rekha dev, "+svg.Repository+")", "/Count 1")
	c := content(t, pdf)
	want(t, c, "0.75 0 0 -0.75 0 7.5 cm", "1 1 m", "9 1 l", "9 9 l", "6 10 4 10 1 9 c", "h", "4 4 m", "1 0 0 rg", "f\n")
	if strings.Contains(c, "1 1 l\nh") {
		t.Error("closing line should be left to h")
	}
}

func TestPDFNoMarkWithoutGenerator(t *testing.T) {
	d := square()
	d.Generator = ""
	if strings.Contains(pdfOf(t, d), "/Producer") {
		t.Error("unmarked document has a Producer")
	}
}

func TestPDFXrefOffsets(t *testing.T) {
	pdf := pdfOf(t, gradientDoc())
	i := strings.LastIndex(pdf, "startxref\n")
	if i < 0 {
		t.Fatal("no startxref")
	}
	start, err := strconv.Atoi(strings.TrimSpace(strings.TrimSuffix(pdf[i+len("startxref\n"):], "%%EOF\n")))
	if err != nil || !strings.HasPrefix(pdf[start:], "xref\n") {
		t.Fatalf("startxref %d does not point at xref (%v)", start, err)
	}
	lines := strings.Split(pdf[start:], "\n")
	n, _ := strconv.Atoi(strings.Fields(lines[1])[1])
	for obj := 1; obj < n; obj++ {
		off, _ := strconv.Atoi(lines[2+obj][:10])
		head := strconv.Itoa(obj) + " 0 obj"
		if !strings.HasPrefix(pdf[off:], head) {
			t.Errorf("object %d: offset %d points at %q", obj, off, pdf[off:off+12])
		}
	}
	if !strings.Contains(pdf, "/Size "+strconv.Itoa(n)) {
		t.Errorf("trailer Size is not %d", n)
	}
}

func TestPDFLinearGradientPattern(t *testing.T) {
	pdf := pdfOf(t, gradientDoc())
	want(t, pdf,
		"/PatternType 2", "/ShadingType 2", "/Coords [0 0 10 0]", "/Extend [true true]",
		"/Matrix [0.75 0 0 -0.75 0 7.5]",
		"/FunctionType 3", "/Bounds [0.25]", "/Encode [0 1 0 1]",
		"/C0 [0 0 0] /C1 [1 0 0]", "/C0 [1 0 0] /C1 [1 1 1]",
		"/Pattern << /P0 ")
	want(t, content(t, pdf), "/Pattern cs /P0 scn\n", "f\n")
}

func TestPDFRadialTranslucentUsesSoftMask(t *testing.T) {
	pdf := pdfOf(t, gradientDoc())
	want(t, pdf,
		"/ShadingType 3", "/Coords [5 5 0 5 5 4]",
		"/SMask << /S /Luminosity /G ", "/CS /DeviceGray", "/ColorSpace /DeviceGray", "/C0 [0.5] /C1 [0]",
		"/ExtGState << /GS1 ")
	want(t, content(t, pdf), "/GS1 gs\n/Pattern cs /P1 scn\n")
}

func TestPDFStroke(t *testing.T) {
	d := square()
	d.Paths[0].Stroke = "#ff0000"
	d.Paths[0].StrokeWidth = 0.5
	want(t, content(t, pdfOf(t, d)), "1 0 0 RG 0.5 w 1 j\n", "B\n")
}

func TestEPSHeaderAndPaint(t *testing.T) {
	eps := epsOf(t, square())
	if !strings.HasPrefix(eps, "%!PS-Adobe-3.0 EPSF-3.0\n%%BoundingBox: 0 0 8 8\n%%HiResBoundingBox: 0 0 7.5 7.5\n") {
		t.Fatalf("header:\n%s", eps[:120])
	}
	want(t, eps, "%%Creator: Rekha dev, "+svg.Repository, "%%LanguageLevel: 3", "%%EndComments",
		"[0.75 0 0 -0.75 0 7.5] concat", "1 1 m", "6 10 4 10 1 9 c", "h", "1 0 0 rg f newpath\n", "showpage", "%%EOF\n")
}

func TestEPSGradientsAndTranslucentSkipped(t *testing.T) {
	eps := epsOf(t, gradientDoc())
	want(t, eps, "clip", "/ShadingType 2", "/Coords [0 0 10 0]", "shfill", "/FunctionType 3", "/Bounds [0.25]")
	if strings.Contains(eps, "/ShadingType 3") {
		t.Error("translucent radial overlay written to EPS, which has no transparency")
	}
	if n := strings.Count(eps, "shfill"); n != 1 {
		t.Errorf("%d shfill, want 1", n)
	}
}

func TestEPSStroke(t *testing.T) {
	d := square()
	d.Paths[0].Stroke = "#00ff00"
	d.Paths[0].StrokeWidth = 2
	want(t, epsOf(t, d), "2 setlinewidth 1 setlinejoin 0 1 0 rg stroke")
}

func TestStopsFunctionPadsToTheDomain(t *testing.T) {
	// Stops from 0.4 to 0.8: constant before and after, so the function
	// covers 0..1 like an SVG gradient does inside its vector.
	f := stopsFunction([]svg.Stop{{Offset: 0.4, Colour: "#000000", Opacity: 1}, {Offset: 0.8, Colour: "#ffffff", Opacity: 1}}, false, 3)
	want(t, f, "/Bounds [0.4 0.8]", "/C0 [0 0 0] /C1 [0 0 0]", "/C0 [0 0 0] /C1 [1 1 1]", "/C0 [1 1 1] /C1 [1 1 1]")
}

func TestStopsFunctionSingleStop(t *testing.T) {
	f := stopsFunction([]svg.Stop{{Offset: 0.5, Colour: "#808080", Opacity: 1}}, false, 3)
	want(t, f, "/FunctionType 2", "/C0 [0.502 0.502 0.502] /C1 [0.502 0.502 0.502]", "/Domain [0 1]")
	if strings.Contains(f, "/FunctionType 3") {
		t.Error("one stop needs no stitching function")
	}
}

func TestGradientStrokeUsesTheGradient(t *testing.T) {
	// Cutout output seals seams with a stroke in the fill's own paint,
	// a gradient included; it must not fall back to black.
	d := gradientDoc()
	d.Paths[0].Stroke = "url(#g0)"
	d.Paths[0].StrokeWidth = 0.5
	want(t, content(t, pdfOf(t, d)), "/Pattern CS /P0 SCN 0.5 w 1 j\n", "B\n")
	eps := epsOf(t, d)
	want(t, eps, "strokepath clip", "0.5 setlinewidth 1 setlinejoin")
	if n := strings.Count(eps, "shfill"); n != 2 {
		t.Errorf("%d shfill, want 2 (fill and stroke)", n)
	}
}

func TestPDFPageIsDisplaySize(t *testing.T) {
	d := square()
	d.Width, d.Height, d.Scale = 200, 100, 2
	pdf := pdfOf(t, d)
	want(t, pdf, "/MediaBox [0 0 75 37.5]")
	want(t, content(t, pdf), "0.375 0 0 -0.375 0 37.5 cm")
	eps := epsOf(t, d)
	want(t, eps, "%%BoundingBox: 0 0 75 38", "%%HiResBoundingBox: 0 0 75 37.5")
}

package svg

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vexyl-ai/rekha/internal/geom"
)

// TestParseRoundTripsGoldens reads every golden SVG back and writes it
// again: the bytes must match, so a document edited in the playground
// can come back through the PDF and EPS writers unchanged.
func TestParseRoundTripsGoldens(t *testing.T) {
	files, err := filepath.Glob("../../testdata/golden/*.svg")
	if err != nil || len(files) == 0 {
		t.Fatalf("goldens: %v (%d files)", err, len(files))
	}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		d, prec, err := Parse(bytes.NewReader(src))
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		var out bytes.Buffer
		if err := WriteDocument(&out, d, prec); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(out.Bytes(), src) {
			t.Errorf("%s: round trip differs (precision %d)", f, prec)
		}
	}
}

func TestParseGradientsStrokeAndOpacity(t *testing.T) {
	const src = `<svg xmlns="http://www.w3.org/2000/svg" width="20" height="10" viewBox="0 0 20 10" data-generator="rekha v1.2.3">
<!-- Traced by Rekha v1.2.3, https://github.com/vexyl-ai/rekha -->
<defs><linearGradient id="g0" gradientUnits="userSpaceOnUse" x1="0" y1="0" x2="20" y2="0"><stop offset="0" stop-color="#000000"/><stop offset="0.25" stop-color="#ff0000"/></linearGradient><radialGradient id="g1" gradientUnits="userSpaceOnUse" cx="10" cy="5" r="4.5"><stop offset="0.1234" stop-color="#00ff00" stop-opacity="0.5"/><stop offset="1" stop-color="#00ff00" stop-opacity="0"/></radialGradient></defs>
<path fill="url(#g0)" stroke="url(#g0)" stroke-width="0.5" stroke-linejoin="round" d="M1 1H9V9C6 10 4 10 1 9ZM4 4V6H6V4Z"/>
<path fill="url(#g1)" d="M1 1H9V9C6 10 4 10 1 9ZM4 4V6H6V4Z"/>
</svg>
`
	d, prec, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if d.Width != 20 || d.Height != 10 || d.Generator != "rekha v1.2.3" || prec != 1 {
		t.Errorf("header: %d×%d %q prec %d", d.Width, d.Height, d.Generator, prec)
	}
	if len(d.Gradients) != 1 || d.Gradients[0].X2 != 20 || d.Gradients[0].Stops[1].Offset != 0.25 || d.Gradients[0].Stops[1].Opacity != 1 {
		t.Errorf("linear: %+v", d.Gradients)
	}
	if len(d.Radials) != 1 || d.Radials[0].R != 4.5 || d.Radials[0].Stops[0].Opacity != 0.5 || d.Radials[0].Stops[1].Opacity != 0 {
		t.Errorf("radial: %+v", d.Radials)
	}
	if len(d.Paths) != 2 || d.Paths[0].Stroke != "url(#g0)" || d.Paths[0].StrokeWidth != 0.5 || d.Paths[1].Stroke != "" {
		t.Errorf("paths: %+v", d.Paths)
	}
	p := d.Paths[0]
	if len(p.Subpaths) != 2 || len(p.Subpaths[0].Segs) != 3 || p.Subpaths[0].Segs[2].Line || p.Subpaths[0].Segs[2].P[3] != (geom.Vec2{X: 1, Y: 9}) {
		t.Errorf("segments: %+v", p.Subpaths)
	}
	var out bytes.Buffer
	if err := WriteDocument(&out, d, prec); err != nil {
		t.Fatal(err)
	}
	if out.String() != src {
		t.Errorf("round trip:\n%s", out.String())
	}
}

func TestParseRejectsWhatRekhaNeverWrites(t *testing.T) {
	for _, src := range []string{
		`<svg xmlns="http://www.w3.org/2000/svg" width="2" height="2" viewBox="0 0 2 2"><path fill="#000" d="m0 0l1 1z"/></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg" width="2" height="2" viewBox="0 0 2 2"><path fill="#000" d="M0 0A1 1 0 0 1 1 1Z"/></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg" width="2" height="2" viewBox="0 0 2 2"><rect width="1" height="1"/></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 2 2"></svg>`,
		`not svg`,
	} {
		if _, _, err := Parse(strings.NewReader(src)); err == nil {
			t.Errorf("accepted %q", src)
		}
	}
}

func TestParseWithoutViewBox(t *testing.T) {
	d, _, err := Parse(strings.NewReader(`<svg xmlns="http://www.w3.org/2000/svg" width="20" height="10"><path fill="#000000" d="M0 0H20V10H0Z"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	if d.Width != 20 || d.Height != 10 || d.Scale != 1 {
		t.Fatalf("%+v", d)
	}
}

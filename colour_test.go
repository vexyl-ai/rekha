package rekha

import (
	"bytes"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/vexyl-ai/rekha/internal/contour"
	"github.com/vexyl-ai/rekha/internal/corpus"
	"github.com/vexyl-ai/rekha/internal/geom"
	"github.com/vexyl-ai/rekha/internal/rasttest"
)

func TestCorpusHasColourSynthetics(t *testing.T) {
	for _, name := range []string{"poster-discs", "aa-abut", "logo-holes"} {
		synth(t, name)
	}
	s, err := Segment(synth(t, "logo-holes"), PresetOptions(PresetPoster))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Palette) != 3 {
		t.Fatalf("logo-holes palette %v, want background and two letter colours", s.Palette)
	}
}

func TestTraceGoldenColour(t *testing.T) {
	type tc struct {
		name   string
		preset Preset
		mode   Mode
	}
	var cases []tc
	for _, s := range corpus.All() {
		for _, mode := range []Mode{ModePixel, ModePolygon, ModeSpline} {
			cases = append(cases, tc{s.Name, PresetPoster, mode})
		}
	}
	cases = append(cases, tc{"gradient-h", PresetPhoto, ModeSpline})
	for _, c := range cases {
		o := PresetOptions(c.preset)
		o.Mode = c.mode
		r, err := Trace(synth(t, c.name), o)
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if err := r.WriteSVG(&buf, o); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join("testdata", "golden", c.name+"-"+string(c.preset)+"-"+string(c.mode)+".svg")
		if *update {
			if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v (run with -update)", path, err)
		}
		if !svgClose(want, buf.Bytes()) {
			t.Errorf("%s differs from golden", path)
		}
	}
}

// logo-holes: the background is depth 0, the two letters depth 1 and the
// background showing through their holes depth 2, in the background colour.
// Goldens for the Phase 3 goal images with refinement on: bw for the
// bilevel synthetics, poster for the colour ones.
func TestTraceGoldenRefine(t *testing.T) {
	cases := []struct {
		name   string
		preset Preset
	}{
		{"circle-aa", PresetBW}, {"rounded-rect", PresetBW}, {"star-5", PresetBW},
		{"poster-discs", PresetPoster}, {"aa-abut", PresetPoster}, {"logo-holes", PresetPoster}, {"three-layer-stack", PresetPoster},
	}
	for _, c := range cases {
		o := PresetOptions(c.preset)
		o.Refine.Enabled = true
		r, err := Trace(synth(t, c.name), o)
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if err := r.WriteSVG(&buf, o); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join("testdata", "golden", c.name+"-refine-spline.svg")
		if *update {
			if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v (run with -update)", path, err)
		}
		if !svgClose(want, buf.Bytes()) {
			t.Errorf("%s differs from golden", path)
		}
	}
}

func TestLogoHolesDepths(t *testing.T) {
	o := PresetOptions(PresetPoster)
	s, err := Segment(synth(t, "logo-holes"), o)
	if err != nil {
		t.Fatal(err)
	}
	layers := s.layers(o)
	shapes := make([]contour.Shape, len(layers))
	for i, l := range layers {
		shapes[i] = l.shape
	}
	parents := contour.Nest(shapes, s.Width, s.Height)
	bg := s.Labels[0]
	byDepth := map[int][]int{}
	for i := range layers {
		byDepth[depthOf(parents, i)] = append(byDepth[depthOf(parents, i)], layers[i].colour)
	}
	if len(byDepth[0]) != 1 || byDepth[0][0] != int(bg) {
		t.Errorf("depth 0 colours %v, want just the background %d", byDepth[0], bg)
	}
	if len(byDepth[1]) != 2 || byDepth[1][0] == int(bg) || byDepth[1][1] == int(bg) || byDepth[1][0] == byDepth[1][1] {
		t.Errorf("depth 1 colours %v, want two different letter colours", byDepth[1])
	}
	if len(byDepth[2]) != 3 {
		t.Errorf("depth 2 has %d shapes, want 3 holes", len(byDepth[2]))
	}
	for _, c := range byDepth[2] {
		if c != int(bg) {
			t.Errorf("hole content colour %d, want background %d", c, bg)
		}
	}
	if len(byDepth) != 3 {
		t.Errorf("depths %v, want 0..2", byDepth)
	}
}

// checkColourSane traces img under o in spline (or o.Mode) and pixel mode
// and checks the output: palette within cap, labels in range, the painted
// pixel-mode output equal to Labels, and the fitted output closed, finite,
// within the corner budget of each loop and rasterising close to pixel mode.
func checkColourSane(t *testing.T, img image.Image, o Options) {
	t.Helper()
	s, err := Segment(img, o)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Palette) > 256 || len(s.Palette) == 0 {
		t.Fatalf("palette size %d", len(s.Palette))
	}
	for _, l := range s.Labels {
		if int(l) >= len(s.Palette) {
			t.Fatalf("label %d for a %d-colour palette", l, len(s.Palette))
		}
	}
	po := o
	po.Mode = ModePixel
	pr, err := s.Finish(po)
	if err != nil {
		t.Fatal(err)
	}
	if mis := paintLabels(t, s, pr); mis != 0 {
		t.Fatalf("pixel mode: %d painted pixels differ from Labels", mis)
	}
	r, err := s.Finish(o)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Paths) != len(pr.Paths) {
		t.Fatalf("%d paths in %s mode, %d in pixel mode", len(r.Paths), o.Mode, len(pr.Paths))
	}
	perimeter := 0
	for k, p := range r.Paths {
		if p.Fill != pr.Paths[k].Fill || len(p.Subpaths) != len(pr.Paths[k].Subpaths) {
			t.Fatalf("path %d: fill/subpaths differ from pixel mode", k)
		}
		for j, sp := range p.Subpaths {
			pix := pr.Paths[k].Subpaths[j].Segs
			perimeter += len(pix)
			if len(sp.Segs) == 0 {
				t.Fatalf("path %d subpath %d: no segments", k, j)
			}
			if d := sp.Segs[len(sp.Segs)-1].P[3].Sub(sp.Segs[0].P[0]).Len(); d > 1e-9 {
				t.Fatalf("path %d subpath %d: not closed, gap %g", k, j, d)
			}
			for i, sg := range sp.Segs {
				for _, pt := range sg.P {
					if math.IsNaN(pt.X) || math.IsNaN(pt.Y) || math.IsInf(pt.X, 0) || math.IsInf(pt.Y, 0) {
						t.Fatal("non-finite coordinate")
					}
				}
				if i > 0 && sg.P[0].Sub(sp.Segs[i-1].P[3]).Len() > 1e-9 {
					t.Fatalf("path %d subpath %d: segment %d disconnected", k, j, i)
				}
			}
			if o.Mode != ModePixel {
				loop := make([]geom.Point, len(pix))
				for i, sg := range pix {
					loop[i] = geom.Point{X: int(sg.P[0].X), Y: int(sg.P[0].Y)}
				}
				if budget := len(geom.MergeCollinearIndices(loop)) + 4; len(sp.Segs) > budget {
					t.Fatalf("path %d subpath %d: %d segments for a %d-corner loop", k, j, len(sp.Segs), budget-4)
				}
			}
		}
	}
	if o.Mode != ModePixel {
		pal := map[string]uint8{}
		for i, c := range s.Palette {
			pal[hexOf(c)] = uint8(i)
		}
		want := rasttest.Paint(s.Width, s.Height, pr.Paths, pal)
		got := rasttest.Paint(s.Width, s.Height, r.Paths, pal)
		mis := 0
		for i := range want {
			if want[i] != got[i] {
				mis++
			}
		}
		if limit := perimeter*3/4 + 8; mis > limit {
			t.Fatalf("%d mismatched pixels for a boundary of %d (limit %d)", mis, perimeter, limit)
		}
	}
	var buf bytes.Buffer
	if err := r.WriteSVG(&buf, o); err != nil {
		t.Fatal(err)
	}
}

func FuzzSegment(f *testing.F) {
	f.Add(uint8(1), uint8(1), []byte{0xff, 0, 0})
	f.Add(uint8(4), uint8(4), []byte{200, 30, 30, 30, 60, 220, 255, 255, 255})
	f.Add(uint8(9), uint8(5), []byte{0, 0, 0, 10, 10, 10, 250, 250, 250, 128, 0, 128})
	f.Fuzz(func(t *testing.T, w, h uint8, data []byte) {
		W, H := int(w)%32+1, int(h)%32+1
		img := image.NewNRGBA(image.Rect(0, 0, W, H))
		for y := 0; y < H; y++ {
			for x := 0; x < W; x++ {
				c := color.NRGBA{255, 255, 255, 255}
				if len(data) >= 3 {
					i := 3 * ((y*W + x) % (len(data) / 3))
					c = color.NRGBA{data[i], data[i+1], data[i+2], 255}
				}
				img.SetNRGBA(x, y, c)
			}
		}
		for _, preset := range []Preset{PresetPoster, PresetPhoto} {
			for _, hier := range []Hierarchical{HierarchicalStacked, HierarchicalCutout} {
				o := PresetOptions(preset)
				o.Hierarchical = hier
				o.FilterSpeckle = 0
				checkColourSane(t, img, o)
			}
		}
		// Refinement on one configuration: it costs most of an input's
		// time, and a stacked poster trace exercises bands and holes.
		{
			o := PresetOptions(PresetPoster)
			o.FilterSpeckle = 0
			o.Refine.Enabled = true
			checkColourSane(t, img, o)
		}
	})
}

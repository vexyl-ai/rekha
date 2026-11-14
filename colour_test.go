package rekha

import (
	"bytes"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/vexyl-ai/rekha/internal/contour"
	"github.com/vexyl-ai/rekha/internal/corpus"
	"github.com/vexyl-ai/rekha/internal/fit"
	"github.com/vexyl-ai/rekha/internal/geom"
	"github.com/vexyl-ai/rekha/internal/metrics"
	"github.com/vexyl-ai/rekha/internal/rasttest"
	"github.com/vexyl-ai/rekha/internal/svg"
)

func TestCorpusHasColourSynthetics(t *testing.T) {
	for _, name := range []string{"poster-discs", "aa-abut", "logo-holes", "small-glyphs", "rect-rot", "big-poster"} {
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
	for _, s := range corpus.Small() {
		for _, mode := range []Mode{ModePixel, ModePolygon, ModeSpline} {
			cases = append(cases, tc{s.Name, PresetPoster, mode})
		}
	}
	cases = append(cases, tc{"gradient-h", PresetPhoto, ModeSpline})
	// The gradient option has its own goldens on the two ramp fixtures.
	for _, name := range []string{"gradient-h", "gradient-shapes"} {
		cases = append(cases, tc{name, PresetPhoto, "gradients-spline"})
	}
	for _, c := range cases {
		o := PresetOptions(c.preset)
		o.Mode = c.mode
		if c.mode == "gradients-spline" {
			o.Mode, o.Gradients = ModeSpline, true
		}
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
func TestTraceGoldenRefine(t *testing.T) {
	cases := []struct {
		name   string
		preset Preset
	}{
		{"circle-aa", PresetBW}, {"rounded-rect", PresetBW}, {"star-5", PresetBW},
		{"poster-discs", PresetPoster}, {"aa-abut", PresetPoster}, {"logo-holes", PresetPoster}, {"three-layer-stack", PresetPoster},
		{"half-disc", PresetBW}, {"annular-sector", PresetBW}, {"fillet-arc", PresetBW}, {"triangle", PresetBW}, {"ring", PresetBW},
	}
	for _, c := range cases {
		o := PresetOptions(c.preset)
		o.Refine.Enabled = true
		img := synth(t, c.name)
		s, err := Segment(img, o)
		if err != nil {
			t.Fatal(err)
		}
		r, err := s.Finish(o)
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
		// The optimiser amplifies last-bit differences (the plain fit
		// itself differs by ulps between amd64 and arm64) into different
		// merge decisions, and the primitive pass searches a radius
		// against the rendered energy, so the two arches land 0.01 px
		// apart on a large circle and snap a differently merged loop into
		// different arcs. The refined goldens are therefore compared as
		// quality: same paths and fills, at most 20 % (+2) more nodes
		// than the golden, and either the two renderings at 4× within
		// 36 dB of each other (the same geometry) or the trace rendering
		// within 1 dB of the golden's PSNR against the source image (an
		// equivalent trace of different geometry: fillet-arc's two-radius
		// fillet cubics measure 33.5 dB between arches at equal PSNR).
		got := parseSVGPaths(t, buf.Bytes())
		exp := parseSVGPaths(t, want)
		if len(got) != len(exp) {
			t.Errorf("%s: %d paths, golden %d", path, len(got), len(exp))
			continue
		}
		ng, ne := 0, 0
		for i := range got {
			if got[i].Fill != exp[i].Fill || len(got[i].Subpaths) != len(exp[i].Subpaths) {
				t.Errorf("%s: path %d fill/subpaths differ from golden", path, i)
			}
			ng, ne = ng+nodeCount(got[i]), ne+nodeCount(exp[i])
		}
		if float64(ng) > 1.2*float64(ne)+2 {
			t.Errorf("%s: %d nodes, golden %d", path, ng, ne)
		}
		if d := renderPSNR(t, got, exp, r.Width, r.Height); d < 36 {
			goldenRes := &Result{Width: r.Width, Height: r.Height, Paths: exp}
			pg, pn := superPSNR(t, img, s, goldenRes), superPSNR(t, img, s, r)
			if pn < pg-1 {
				t.Errorf("%s: rendering %.1f dB from golden and %.2f dB against the source, golden %.2f", path, d, pn, pg)
			}
		}
	}
}

func parseHex(h string) (color.NRGBA, error) {
	v, err := strconv.ParseUint(h[1:], 16, 32)
	if err != nil {
		return color.NRGBA{}, err
	}
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}, nil
}

func nodeCount(p svg.Path) int {
	n := 0
	for _, sp := range p.Subpaths {
		n += len(sp.Segs)
	}
	return n
}

// parseSVGPaths reads the writer's own output back: absolute M, L, H, V,
// C and Z only, one fill per path.
func parseSVGPaths(t *testing.T, data []byte) []svg.Path {
	t.Helper()
	var paths []svg.Path
	for _, m := range regexp.MustCompile(`<path fill="(#[0-9a-f]{6})"[^>]* d="([^"]*)"`).FindAllSubmatch(data, -1) {
		p := svg.Path{Fill: string(m[1])}
		d := string(m[2])
		nums := regexp.MustCompile(`-?\d+(?:\.\d+)?`)
		var cur, start geom.Vec2
		var sp *svg.Subpath
		for i := 0; i < len(d); {
			cmd := d[i]
			i++
			j := i
			for j < len(d) && !(d[j] >= 'A' && d[j] <= 'Z') {
				j++
			}
			var v []float64
			for _, n := range nums.FindAllString(d[i:j], -1) {
				f, err := strconv.ParseFloat(n, 64)
				if err != nil {
					t.Fatal(err)
				}
				v = append(v, f)
			}
			i = j
			switch cmd {
			case 'M':
				p.Subpaths = append(p.Subpaths, svg.Subpath{})
				sp = &p.Subpaths[len(p.Subpaths)-1]
				cur = geom.Vec2{X: v[0], Y: v[1]}
				start = cur
			case 'L', 'H', 'V':
				next := cur
				switch cmd {
				case 'L':
					next = geom.Vec2{X: v[0], Y: v[1]}
				case 'H':
					next.X = v[0]
				case 'V':
					next.Y = v[0]
				}
				sp.Segs = append(sp.Segs, fit.Segment{Line: true, P: fit.Cubic{cur, cur, next, next}})
				cur = next
			case 'C':
				next := geom.Vec2{X: v[4], Y: v[5]}
				sp.Segs = append(sp.Segs, fit.Segment{P: fit.Cubic{cur, {X: v[0], Y: v[1]}, {X: v[2], Y: v[3]}, next}})
				cur = next
			case 'Z':
				if cur != start {
					sp.Segs = append(sp.Segs, fit.Segment{Line: true, P: fit.Cubic{cur, cur, start, start}})
				}
				cur = start
			default:
				t.Fatalf("unexpected path command %q", cmd)
			}
		}
		paths = append(paths, p)
	}
	return paths
}

// renderPSNR paints both path sets at 4× over white and returns the PSNR
// between the two renderings.
func renderPSNR(t *testing.T, a, b []svg.Path, w, h int) float64 {
	t.Helper()
	const k = 4
	pal := map[string]uint8{}
	var colours []color.NRGBA
	for _, ps := range [][]svg.Path{a, b} {
		for _, p := range ps {
			if _, ok := pal[p.Fill]; !ok {
				c, err := parseHex(p.Fill)
				if err != nil {
					t.Fatal(err)
				}
				pal[p.Fill] = uint8(len(colours))
				colours = append(colours, c)
			}
		}
	}
	render := func(ps []svg.Path) *image.NRGBA {
		scaled := make([]svg.Path, len(ps))
		for i, p := range ps {
			scaled[i] = svg.Path{Fill: p.Fill}
			for _, sp := range p.Subpaths {
				segs := make([]fit.Segment, len(sp.Segs))
				for j, sg := range sp.Segs {
					segs[j] = sg
					for q := range sg.P {
						segs[j].P[q] = geom.Vec2{X: sg.P[q].X * k, Y: sg.P[q].Y * k}
					}
				}
				scaled[i].Subpaths = append(scaled[i].Subpaths, svg.Subpath{Segs: segs})
			}
		}
		idx := rasttest.Paint(w*k, h*k, scaled, pal)
		out := image.NewNRGBA(image.Rect(0, 0, w*k, h*k))
		for i, v := range idx {
			c := color.NRGBA{255, 255, 255, 255}
			if v != 255 {
				c = colours[v]
			}
			out.SetNRGBA(i%(w*k), i/(w*k), c)
		}
		return out
	}
	d, err := metrics.PSNR(render(a), render(b))
	if err != nil {
		t.Fatal(err)
	}
	return d
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
				budget := len(geom.MergeCollinearIndices(loop)) + 4
				if usePlanar(o) {
					// Shared boundaries break at every junction, so a
					// straight run of edges with junctions along it is
					// one line per edge.
					budget = max(budget, len(pix)+4)
				}
				if len(sp.Segs) > budget {
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
	})
}

// FuzzSegmentRefine is FuzzSegment with refinement on, over images small
// enough (≤ 12×12) and one sweep so the fuzzer gets through inputs at a
// useful rate; a stacked poster trace exercises bands and holes.
func FuzzSegmentRefine(f *testing.F) {
	f.Add(uint8(1), uint8(1), []byte{0xff, 0, 0})
	f.Add(uint8(4), uint8(4), []byte{200, 30, 30, 30, 60, 220, 255, 255, 255})
	f.Add(uint8(9), uint8(5), []byte{0, 0, 0, 10, 10, 10, 250, 250, 250, 128, 0, 128})
	f.Fuzz(func(t *testing.T, w, h uint8, data []byte) {
		W, H := int(w)%12+1, int(h)%12+1
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
		o := PresetOptions(PresetPoster)
		o.FilterSpeckle = 0
		o.Refine.Enabled = true
		o.Refine.Sweeps = 1
		checkColourSane(t, img, o)
	})
}

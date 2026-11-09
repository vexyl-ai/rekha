package rekha

import (
	"bytes"
	"image"
	"image/color"
	"math"
	"runtime"
	"testing"

	"github.com/vexyl-ai/rekha/internal/fit"
	"github.com/vexyl-ai/rekha/internal/geom"
	"github.com/vexyl-ai/rekha/internal/metrics"
	"github.com/vexyl-ai/rekha/internal/rasttest"
	"github.com/vexyl-ai/rekha/internal/svg"
)

// superPSNR renders r at 4× with the test rasteriser, box-filters down,
// and returns the PSNR against img. Uncovered pixels are white.
func superPSNR(t *testing.T, img image.Image, s *Segmentation, r *Result) float64 {
	t.Helper()
	const k = 4
	pal := map[string]uint8{}
	for i, c := range s.Palette {
		pal[hexOf(c)] = uint8(i)
	}
	scaled := make([]svg.Path, len(r.Paths))
	for i, p := range r.Paths {
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
	idx := rasttest.Paint(s.Width*k, s.Height*k, scaled, pal)
	out := image.NewNRGBA(image.Rect(0, 0, s.Width, s.Height))
	for y := 0; y < s.Height; y++ {
		for x := 0; x < s.Width; x++ {
			var sum [3]int
			for dy := 0; dy < k; dy++ {
				for dx := 0; dx < k; dx++ {
					c := color.NRGBA{255, 255, 255, 255}
					if i := idx[(y*k+dy)*s.Width*k+x*k+dx]; i != 255 {
						c = s.Palette[i]
					}
					sum[0] += int(c.R)
					sum[1] += int(c.G)
					sum[2] += int(c.B)
				}
			}
			out.SetNRGBA(x, y, color.NRGBA{uint8((sum[0] + k*k/2) / (k * k)), uint8((sum[1] + k*k/2) / (k * k)), uint8((sum[2] + k*k/2) / (k * k)), 255})
		}
	}
	psnr, err := metrics.PSNR(img, out)
	if err != nil {
		t.Fatal(err)
	}
	return psnr
}

func tracePair(t *testing.T, name string, preset Preset) (img image.Image, s *Segmentation, plain, refined *Result, o Options) {
	t.Helper()
	img = synth(t, name)
	o = PresetOptions(preset)
	var err error
	s, err = Segment(img, o)
	if err != nil {
		t.Fatal(err)
	}
	if plain, err = s.Finish(o); err != nil {
		t.Fatal(err)
	}
	o.Refine.Enabled = true
	if refined, err = s.Finish(o); err != nil {
		t.Fatal(err)
	}
	return img, s, plain, refined, o
}

func TestRefineImprovesPSNR(t *testing.T) {
	cases := []struct {
		name   string
		preset Preset
		gain   float64 // least dB the refinement must add
	}{{"circle-aa", PresetBW, 1}, {"aa-abut", PresetPoster, 1}, {"thin-lines", PresetBW, -0.1}}
	for _, c := range cases {
		img, s, plain, refined, _ := tracePair(t, c.name, c.preset)
		before, after := superPSNR(t, img, s, plain), superPSNR(t, img, s, refined)
		t.Logf("%s %s: %.2f → %.2f dB", c.name, c.preset, before, after)
		if after < before+c.gain {
			t.Errorf("%s %s: PSNR %.2f → %.2f dB, want at least %+.1f", c.name, c.preset, before, after, c.gain)
		}
	}
}

func TestRefineLeavesPerfectFitAlone(t *testing.T) {
	_, _, plain, refined, o := tracePair(t, "two-colour-abut", PresetPoster)
	var a, b bytes.Buffer
	if err := plain.WriteSVG(&a, o); err != nil {
		t.Fatal(err)
	}
	if err := refined.WriteSVG(&b, o); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Fatalf("refinement changed a perfect fit:\n%s\n---\n%s", a.String(), b.String())
	}
}

func TestRefineCircleAAIsFourCubics(t *testing.T) {
	_, _, _, refined, _ := tracePair(t, "circle-aa", PresetBW)
	if len(refined.Paths) != 1 || len(refined.Paths[0].Subpaths) != 1 {
		t.Fatalf("%d paths", len(refined.Paths))
	}
	segs := refined.Paths[0].Subpaths[0].Segs
	if len(segs) != 4 {
		t.Fatalf("%d segments, want 4", len(segs))
	}
	var centre geom.Vec2
	for _, sg := range segs {
		centre = centre.Add(sg.P[0])
	}
	centre = centre.Scale(0.25)
	if d := centre.Sub(geom.Vec2{X: 128, Y: 128}).Len(); d > 0.1 {
		t.Fatalf("centre %v, want (128,128) within 0.1", centre)
	}
	for _, sg := range segs {
		r := sg.P[0].Sub(centre).Len()
		if ratio := sg.P[1].Sub(sg.P[0]).Len() / r; math.Abs(ratio-0.5523) > 0.001 {
			t.Fatalf("handle/radius %v, want 0.5523", ratio)
		}
	}
}

func TestRefineDeterministicAcrossProcs(t *testing.T) {
	img := synth(t, "poster-discs")
	o := PresetOptions(PresetPoster)
	o.Refine.Enabled = true
	var outs [2][]byte
	for i, procs := range []int{1, 8} {
		prev := runtime.GOMAXPROCS(procs)
		r, err := Trace(img, o)
		runtime.GOMAXPROCS(prev)
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if err := r.WriteSVG(&buf, o); err != nil {
			t.Fatal(err)
		}
		outs[i] = buf.Bytes()
	}
	if !bytes.Equal(outs[0], outs[1]) {
		t.Fatal("output differs between GOMAXPROCS 1 and 8")
	}
}

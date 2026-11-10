package rekha

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"

	"github.com/vexyl-ai/rekha/internal/fit"
	"github.com/vexyl-ai/rekha/internal/geom"
	"github.com/vexyl-ai/rekha/internal/metrics"
	"github.com/vexyl-ai/rekha/internal/rasttest"
	"github.com/vexyl-ai/rekha/internal/svg"
)

// BenchmarkRefineBigPoster times Segment and Finish of the 2048-px
// big-poster synthetic under poster, stacked, --refine, and logs the
// refined PSNR of the last run (4× supersampled, as superPSNR measures).
func BenchmarkRefineBigPoster(b *testing.B) {
	f, err := os.Open("testdata/corpus/synthetic/big-poster.png")
	if err != nil {
		b.Skip(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		b.Fatal(err)
	}
	o := PresetOptions(PresetPoster)
	o.Refine.Enabled = true
	var s *Segmentation
	var r *Result
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if s, err = Segment(img, o); err != nil {
			b.Fatal(err)
		}
		if r, err = s.Finish(o); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	b.Logf("big-poster refined: %.2f dB", benchPSNR(b, img, s, r))
}

// benchPSNR is superPSNR for benchmarks: the paths painted at 4× and box
// filtered against the input.
func benchPSNR(tb testing.TB, img image.Image, s *Segmentation, r *Result) float64 {
	tb.Helper()
	const k = 4
	pal := map[string]uint8{}
	for i, c := range s.Palette {
		pal[fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)] = uint8(i)
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
		tb.Fatal(err)
	}
	return psnr
}

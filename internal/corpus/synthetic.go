// Package corpus generates the deterministic synthetic test images used by
// the benchmark harness. Real-world images live under testdata/corpus and
// are not produced here.
package corpus

import (
	"image"
	"image/color"
	"math"
	"math/rand/v2"
)

// Size is the side length of every synthetic image.
const Size = 256

// Synthetic is one generated test image.
type Synthetic struct {
	Name string
	Draw func(*image.NRGBA)
}

var (
	black = color.NRGBA{0, 0, 0, 255}
	white = color.NRGBA{255, 255, 255, 255}
	red   = color.NRGBA{220, 30, 30, 255}
	blue  = color.NRGBA{30, 60, 220, 255}
)

// All returns the fixed list of synthetics, in a fixed order.
func All() []Synthetic {
	return []Synthetic{
		{"circle-aa", func(img *image.NRGBA) { fill(img, disc(128, 128, 96), black, 4) }},
		{"circle-hard", func(img *image.NRGBA) { fill(img, disc(128, 128, 96), black, 1) }},
		{"rounded-rect", func(img *image.NRGBA) { fill(img, roundedRect(48, 48, 208, 208, 24), black, 4) }},
		{"star-5", func(img *image.NRGBA) { fill(img, polygon(star(128, 128, 100, 40, 5)), black, 4) }},
		{"two-colour-abut", func(img *image.NRGBA) {
			fill(img, func(x, _ float64) bool { return x < 128 }, red, 1)
			fill(img, func(x, _ float64) bool { return x >= 128 }, blue, 1)
		}},
		{"three-layer-stack", func(img *image.NRGBA) {
			fill(img, rect(32, 32, 224, 224), blue, 1)
			fill(img, disc(128, 128, 72), red, 4)
			fill(img, rect(104, 104, 152, 152), white, 1)
		}},
		{"checkerboard-2px", func(img *image.NRGBA) {
			fill(img, func(x, y float64) bool { return (int(x)/2+int(y)/2)%2 == 0 }, black, 1)
		}},
		{"thin-lines", func(img *image.NRGBA) {
			for i := 0; i < Size; i++ {
				img.SetNRGBA(i, 64, black)
				img.SetNRGBA(64, i, black)
				img.SetNRGBA(i, i, black)
			}
		}},
		{"gradient-h", func(img *image.NRGBA) {
			for y := 0; y < Size; y++ {
				for x := 0; x < Size; x++ {
					v := uint8(x)
					img.SetNRGBA(x, y, color.NRGBA{v, v, v, 255})
				}
			}
		}},
		{"noisy-disc", func(img *image.NRGBA) {
			fill(img, disc(128, 128, 96), black, 4)
			rng := rand.New(rand.NewPCG(1, 1))
			for i := 0; i < len(img.Pix); i += 4 {
				for c := 0; c < 3; c++ {
					n := rng.IntN(49) - 24
					img.Pix[i+c] = clamp(int(img.Pix[i+c]) + n)
				}
			}
		}},
	}
}

// Render draws s onto a fresh Size×Size white canvas.
func Render(s Synthetic) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, Size, Size))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 255, 255, 255, 255
	}
	s.Draw(img)
	return img
}

// fill paints c wherever inside is true, supersampling each pixel with
// samples×samples points and blending by coverage. samples=1 gives hard edges.
func fill(img *image.NRGBA, inside func(x, y float64) bool, c color.NRGBA, samples int) {
	step := 1 / float64(samples)
	total := float64(samples * samples)
	for py := 0; py < Size; py++ {
		for px := 0; px < Size; px++ {
			hit := 0
			for sy := 0; sy < samples; sy++ {
				for sx := 0; sx < samples; sx++ {
					x := float64(px) + (float64(sx)+0.5)*step
					y := float64(py) + (float64(sy)+0.5)*step
					if inside(x, y) {
						hit++
					}
				}
			}
			if hit == 0 {
				continue
			}
			cov := float64(hit) / total
			o := img.PixOffset(px, py)
			img.Pix[o] = mix(img.Pix[o], c.R, cov)
			img.Pix[o+1] = mix(img.Pix[o+1], c.G, cov)
			img.Pix[o+2] = mix(img.Pix[o+2], c.B, cov)
		}
	}
}

func mix(bg, fg uint8, cov float64) uint8 {
	return clamp(int(math.Round(float64(bg)*(1-cov) + float64(fg)*cov)))
}

func clamp(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

func disc(cx, cy, r float64) func(x, y float64) bool {
	return func(x, y float64) bool {
		dx, dy := x-cx, y-cy
		return dx*dx+dy*dy <= r*r
	}
}

func rect(x0, y0, x1, y1 float64) func(x, y float64) bool {
	return func(x, y float64) bool { return x >= x0 && x < x1 && y >= y0 && y < y1 }
}

func roundedRect(x0, y0, x1, y1, r float64) func(x, y float64) bool {
	return func(x, y float64) bool {
		if x < x0 || x >= x1 || y < y0 || y >= y1 {
			return false
		}
		cx := math.Max(x0+r, math.Min(x, x1-r))
		cy := math.Max(y0+r, math.Min(y, y1-r))
		dx, dy := x-cx, y-cy
		return dx*dx+dy*dy <= r*r
	}
}

type pt struct{ x, y float64 }

func star(cx, cy, outer, inner float64, points int) []pt {
	var p []pt
	for i := 0; i < points*2; i++ {
		r := outer
		if i%2 == 1 {
			r = inner
		}
		a := -math.Pi/2 + float64(i)*math.Pi/float64(points)
		p = append(p, pt{cx + r*math.Cos(a), cy + r*math.Sin(a)})
	}
	return p
}

// polygon returns an even-odd point-in-polygon test.
func polygon(p []pt) func(x, y float64) bool {
	return func(x, y float64) bool {
		in := false
		for i, j := 0, len(p)-1; i < len(p); j, i = i, i+1 {
			if (p[i].y > y) != (p[j].y > y) {
				xi := (p[j].x-p[i].x)*(y-p[i].y)/(p[j].y-p[i].y) + p[i].x
				if x < xi {
					in = !in
				}
			}
		}
		return in
	}
}

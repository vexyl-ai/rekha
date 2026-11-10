// Package corpus generates the deterministic synthetic test images used by
// the benchmark harness. Real-world images live under testdata/corpus and
// are not produced here.
package corpus

import (
	"image"
	"image/color"
	"math"
	"math/rand/v2"

	"github.com/vexyl-ai/rekha/internal/geom"
)

// Size is the default side length of a synthetic image.
const Size = 256

// Synthetic is one generated test image. Size is its side length; 0
// means the default.
type Synthetic struct {
	Name string
	Draw func(*image.NRGBA)
	Size int
}

var (
	black  = color.NRGBA{0, 0, 0, 255}
	white  = color.NRGBA{255, 255, 255, 255}
	red    = color.NRGBA{220, 30, 30, 255}
	blue   = color.NRGBA{30, 60, 220, 255}
	green  = color.NRGBA{40, 160, 60, 255}
	yellow = color.NRGBA{240, 200, 30, 255}
	purple = color.NRGBA{130, 40, 160, 255}
	navy   = color.NRGBA{20, 30, 60, 255}
)

// All returns the fixed list of synthetics, in a fixed order.
func All() []Synthetic {
	return []Synthetic{
		{Name: "circle-aa", Draw: func(img *image.NRGBA) { fill(img, disc(128, 128, 96), black, 4) }},
		{Name: "circle-hard", Draw: func(img *image.NRGBA) { fill(img, disc(128, 128, 96), black, 1) }},
		{Name: "rounded-rect", Draw: func(img *image.NRGBA) { fill(img, roundedRect(48, 48, 208, 208, 24), black, 4) }},
		{Name: "star-5", Draw: func(img *image.NRGBA) { fill(img, polygon(star(128, 128, 100, 40, 5)), black, 4) }},
		{Name: "two-colour-abut", Draw: func(img *image.NRGBA) {
			fill(img, func(x, _ float64) bool { return x < 128 }, red, 1)
			fill(img, func(x, _ float64) bool { return x >= 128 }, blue, 1)
		}},
		{Name: "three-layer-stack", Draw: func(img *image.NRGBA) {
			fill(img, rect(32, 32, 224, 224), blue, 1)
			fill(img, disc(128, 128, 72), red, 4)
			fill(img, rect(104, 104, 152, 152), white, 1)
		}},
		{Name: "checkerboard-2px", Draw: func(img *image.NRGBA) {
			fill(img, func(x, y float64) bool { return (int(x)/2+int(y)/2)%2 == 0 }, black, 1)
		}},
		{Name: "thin-lines", Draw: func(img *image.NRGBA) {
			for i := 0; i < Size; i++ {
				img.SetNRGBA(i, 64, black)
				img.SetNRGBA(64, i, black)
				img.SetNRGBA(i, i, black)
			}
		}},
		{Name: "gradient-h", Draw: func(img *image.NRGBA) {
			for y := 0; y < Size; y++ {
				for x := 0; x < Size; x++ {
					v := uint8(x)
					img.SetNRGBA(x, y, color.NRGBA{v, v, v, 255})
				}
			}
		}},
		{Name: "poster-discs", Draw: func(img *image.NRGBA) {
			// Five overlapping anti-aliased discs in five flat colours.
			cols := []color.NRGBA{red, blue, green, yellow, purple}
			for i, c := range cols {
				a := -math.Pi/2 + 2*math.Pi*float64(i)/5
				fill(img, disc(128+56*math.Cos(a), 128+56*math.Sin(a), 60), c, 4)
			}
		}},
		{Name: "aa-abut", Draw: func(img *image.NRGBA) {
			// Red and blue meeting along an anti-aliased diagonal.
			fill(img, func(_, _ float64) bool { return true }, red, 1)
			fill(img, func(x, y float64) bool { return x+y >= Size }, blue, 4)
		}},
		{Name: "logo-holes", Draw: func(img *image.NRGBA) {
			// Two letter-like shapes with holes, in two colours, on a third
			// background colour: an O-like ring and a B-like block.
			fill(img, func(_, _ float64) bool { return true }, navy, 1)
			fill(img, roundedRect(40, 64, 120, 192, 24), red, 4)
			fill(img, roundedRect(64, 92, 96, 164, 12), navy, 4)
			fill(img, roundedRect(144, 64, 224, 192, 16), blue, 4)
			fill(img, rect(164, 84, 204, 120), navy, 4)
			fill(img, rect(164, 136, 204, 172), navy, 4)
		}},
		{Name: "small-glyphs", Draw: smallGlyphs},
		{Name: "rect-rot", Draw: func(img *image.NRGBA) {
			var p []pt
			for _, c := range RectRotCorners() {
				p = append(p, pt{c.X, c.Y})
			}
			fill(img, polygon(p), black, 4)
		}},
		{Name: "big-poster", Size: 2048, Draw: bigPoster},
		{Name: "noisy-disc", Draw: func(img *image.NRGBA) {
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

// Small-glyph rows: glyph heights and the baseline y of each row.
var smallGlyphRows = [3]struct{ h, y float64 }{{24, 40}, {32, 112}, {40, 196}}

// SmallGlyphRingCentres returns the centre of each row's ring: the middle
// of its 5-px white counter.
func SmallGlyphRingCentres() [3]geom.Vec2 {
	var out [3]geom.Vec2
	for i, r := range smallGlyphRows {
		out[i] = geom.Vec2{X: 40, Y: r.y}
	}
	return out
}

// smallGlyphs draws, in three rows at heights 24, 32 and 40 px, a ring
// (elliptical outer, ratio 0.8, 5-px disc counter), a stem a quarter as
// wide as it is tall with a diagonal flag from its top-left, and a rounded
// rectangle 1.5 h wide with corner radius h/5; all anti-aliased, black.
func smallGlyphs(img *image.NRGBA) {
	for _, r := range smallGlyphRows {
		h, cy := r.h, r.y
		// Ring at x = 40.
		ring := func(x, y float64) bool {
			dx, dy := (x-40)/(0.4*h), (y-cy)/(0.5*h)
			if dx*dx+dy*dy > 1 {
				return false
			}
			ex, ey := x-40, y-cy
			return ex*ex+ey*ey > 25
		}
		fill(img, ring, black, 4)
		// Stem at x = 90..90+h/4, flag running up-left from the stem top.
		w := h / 4
		x0, top := 90.0, cy-h/2
		stem := polygon([]pt{{x0, top}, {x0 + w, top}, {x0 + w, cy + h/2}, {x0, cy + h/2}, {x0, top + h/2}, {x0 - w, top + h/3}})
		fill(img, stem, black, 4)
		// Rounded rectangle at x = 140.
		fill(img, roundedRect(140, cy-h/2, 140+1.5*h, cy+h/2, h/5), black, 4)
	}
}

// RectRotCorners returns the corners of rect-rot: 120×80, rotated 20°
// about (128, 128), in order around the rectangle.
func RectRotCorners() [4]geom.Vec2 {
	a := 20 * math.Pi / 180
	c, s := math.Cos(a), math.Sin(a)
	var out [4]geom.Vec2
	for i, d := range [4][2]float64{{-60, -40}, {60, -40}, {60, 40}, {-60, 40}} {
		out[i] = geom.Vec2{X: 128 + d[0]*c - d[1]*s, Y: 128 + d[0]*s + d[1]*c}
	}
	return out
}

// bigPoster scatters about 300 discs, 100 rings and 100 rounded rectangles
// in six flat colours over a 2048-px canvas, deterministically.
func bigPoster(img *image.NRGBA) {
	rng := rand.New(rand.NewPCG(3, 3))
	cols := []color.NRGBA{red, blue, green, yellow, purple, navy}
	n := float64(img.Bounds().Dx())
	for i := 0; i < 500; i++ {
		c := cols[rng.IntN(len(cols))]
		x, y := 60+rng.Float64()*(n-120), 60+rng.Float64()*(n-120)
		box := func(inside func(px, py float64) bool, hw, hh float64) {
			fillBox(img, inside, c, 4, int(x-hw)-1, int(y-hh)-1, int(x+hw)+2, int(y+hh)+2)
		}
		switch {
		case i < 300:
			r := 20 + rng.Float64()*40
			box(disc(x, y, r), r, r)
		case i < 400:
			r := 20 + rng.Float64()*40
			outer, inner := disc(x, y, r), disc(x, y, r*0.5)
			box(func(px, py float64) bool { return outer(px, py) && !inner(px, py) }, r, r)
		default:
			w, h := 40+rng.Float64()*80, 40+rng.Float64()*80
			box(roundedRect(x-w/2, y-h/2, x+w/2, y+h/2, math.Min(w, h)/5), w/2, h/2)
		}
	}
}

// Small returns the synthetics at the default size: the golden and
// per-image tests run over these; larger ones are benchmark fixtures.
func Small() []Synthetic {
	var out []Synthetic
	for _, s := range All() {
		if s.Size == 0 || s.Size == Size {
			out = append(out, s)
		}
	}
	return out
}

// Render draws s onto a fresh white canvas of its size.
func Render(s Synthetic) *image.NRGBA {
	size := s.Size
	if size == 0 {
		size = Size
	}
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 255, 255, 255, 255
	}
	s.Draw(img)
	return img
}

// fill paints c wherever inside is true, supersampling each pixel with
// samples×samples points and blending by coverage. samples=1 gives hard edges.
func fill(img *image.NRGBA, inside func(x, y float64) bool, c color.NRGBA, samples int) {
	fillBox(img, inside, c, samples, 0, 0, img.Bounds().Dx(), img.Bounds().Dy())
}

// fillBox is fill restricted to the pixel box [x0,x1)×[y0,y1), clipped to
// the image, for shapes much smaller than the canvas.
func fillBox(img *image.NRGBA, inside func(x, y float64) bool, c color.NRGBA, samples, x0, y0, x1, y1 int) {
	step := 1 / float64(samples)
	total := float64(samples * samples)
	x0, y0 = max(x0, 0), max(y0, 0)
	x1, y1 = min(x1, img.Bounds().Dx()), min(y1, img.Bounds().Dy())
	for py := y0; py < y1; py++ {
		for px := x0; px < x1; px++ {
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

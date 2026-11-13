// Package render rasterises a traced document the way a browser would,
// near enough to score it against the source: antialiased, nonzero
// winding, flat and gradient fills in user space with pad spread, a
// translucent stop composited over what is already painted. Strokes,
// which only seal hairline seams, are not drawn. It is what --auto
// scores its candidates with, so it has no dependency a browser-less
// build cannot carry: the rasteriser is golang.org/x/image/vector.
package render

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"golang.org/x/image/vector"

	"github.com/vexyl-ai/rekha/internal/fit"
	"github.com/vexyl-ai/rekha/internal/geom"
	"github.com/vexyl-ai/rekha/internal/svg"
)

// Paint draws d on a transparent canvas of its size and returns it
// premultiplied, which is what the compositing needs and what a PSNR
// over white reads without loss.
func Paint(d svg.Document) *image.RGBA {
	w, h := d.Width, d.Height
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	if w <= 0 || h <= 0 {
		return dst
	}
	fills := map[string]image.Image{}
	for _, g := range d.Gradients {
		fills["url(#"+g.ID+")"] = &linear{g: g, bounds: dst.Rect, ramp: newRamp(g.Stops)}
	}
	for _, g := range d.Radials {
		fills["url(#"+g.ID+")"] = &radial{g: g, bounds: dst.Rect, ramp: newRamp(g.Stops)}
	}
	z := vector.NewRasterizer(w, h)
	for _, p := range d.Paths {
		src := fillOf(p.Fill, fills)
		if src == nil {
			continue
		}
		box := bounds(p).Intersect(dst.Rect)
		if box.Empty() {
			continue
		}
		z.Reset(box.Dx(), box.Dy())
		ox, oy := float32(box.Min.X), float32(box.Min.Y)
		for _, sp := range p.Subpaths {
			if len(sp.Segs) == 0 {
				continue
			}
			s0 := sp.Segs[0].P[0]
			z.MoveTo(float32(s0.X)-ox, float32(s0.Y)-oy)
			for _, s := range sp.Segs {
				if s.Line {
					z.LineTo(float32(s.P[3].X)-ox, float32(s.P[3].Y)-oy)
					continue
				}
				// The rasteriser's own CubeTo flattens by a tolerance
				// of pixels (two chords on a radius-4 arc), far
				// coarser than cairo, which rsvg-convert and so every
				// benchmark number rests on: subdivide as cairo does.
				flatten(s.P, func(q geom.Vec2) {
					z.LineTo(float32(q.X)-ox, float32(q.Y)-oy)
				})
			}
			z.ClosePath()
		}
		z.DrawOp = draw.Over
		z.Draw(dst, box, src, box.Min)
	}
	return dst
}

// flatTol is cairo's default tolerance: a cubic is split until its
// handles sit within this many pixels of the chord.
const flatTol = 0.1

// flatten emits the end points of the chords that approximate c, the
// start point excluded, splitting in halves (de Casteljau) until each
// piece's handles lie within flatTol of its chord, as cairo does; the
// chords of a convex curve sit inside it, and matching the benchmark
// renderer's sag keeps the score the same one rsvg-convert gives.
func flatten(c fit.Cubic, emit func(geom.Vec2)) {
	flattenTo(c, 0, emit)
}

func flattenTo(c fit.Cubic, depth int, emit func(geom.Vec2)) {
	if depth >= 16 || handleError(c) <= flatTol*flatTol {
		emit(c[3])
		return
	}
	ab := c[0].Add(c[1]).Scale(0.5)
	bc := c[1].Add(c[2]).Scale(0.5)
	cd := c[2].Add(c[3]).Scale(0.5)
	abc := ab.Add(bc).Scale(0.5)
	bcd := bc.Add(cd).Scale(0.5)
	mid := abc.Add(bcd).Scale(0.5)
	flattenTo(fit.Cubic{c[0], ab, abc, mid}, depth+1, emit)
	flattenTo(fit.Cubic{mid, bcd, cd, c[3]}, depth+1, emit)
}

// handleError is the larger squared distance of the two handles from
// the chord, cairo's spline error; when the chord has no length it is
// the handles' distance from the point.
func handleError(c fit.Cubic) float64 {
	d := c[3].Sub(c[0])
	l2 := d.X*d.X + d.Y*d.Y
	dist := func(p geom.Vec2) float64 {
		v := p.Sub(c[0])
		if l2 == 0 {
			return v.X*v.X + v.Y*v.Y
		}
		cross := v.X*d.Y - v.Y*d.X
		return cross * cross / l2
	}
	return max(dist(c[1]), dist(c[2]))
}

// fillOf is the paint for a fill attribute: a uniform colour for
// #rrggbb, a gradient image for url(#id), nil for anything else.
func fillOf(fill string, fills map[string]image.Image) image.Image {
	if g, ok := fills[fill]; ok {
		return g
	}
	if c, ok := parseHex(fill); ok {
		return image.NewUniform(c)
	}
	return nil
}

func parseHex(s string) (color.NRGBA, bool) {
	if len(s) != 7 || s[0] != '#' {
		return color.NRGBA{}, false
	}
	var v [3]uint8
	for i := range v {
		hi, ok1 := hexDigit(s[1+2*i])
		lo, ok2 := hexDigit(s[2+2*i])
		if !ok1 || !ok2 {
			return color.NRGBA{}, false
		}
		v[i] = hi<<4 | lo
	}
	return color.NRGBA{v[0], v[1], v[2], 255}, true
}

func hexDigit(c byte) (uint8, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// bounds is the pixel rectangle the path's control polygon spans, which
// contains the curve; the rasteriser works in it rather than over the
// whole canvas, so a document of thousands of small paths stays quick.
func bounds(p svg.Path) image.Rectangle {
	x0, y0 := math.Inf(1), math.Inf(1)
	x1, y1 := math.Inf(-1), math.Inf(-1)
	add := func(v geom.Vec2) {
		x0, y0 = min(x0, v.X), min(y0, v.Y)
		x1, y1 = max(x1, v.X), max(y1, v.Y)
	}
	for _, sp := range p.Subpaths {
		for _, s := range sp.Segs {
			add(s.P[0])
			add(s.P[3])
			if !s.Line {
				add(s.P[1])
				add(s.P[2])
			}
		}
	}
	if x0 > x1 {
		return image.Rectangle{}
	}
	return image.Rect(int(math.Floor(x0)), int(math.Floor(y0)), int(math.Ceil(x1)), int(math.Ceil(y1)))
}

// ramp is a gradient's stops sampled at 256 positions, straight alpha,
// stops interpolated separately in colour and opacity as SVG says.
type ramp [256]color.NRGBA

func newRamp(stops []svg.Stop) *ramp {
	var r ramp
	var cs []color.NRGBA
	for _, s := range stops {
		c, _ := parseHex(s.Colour)
		c.A = uint8(math.Round(255 * max(0, min(1, s.Opacity))))
		cs = append(cs, c)
	}
	for i := range r {
		t := float64(i) / 255
		switch {
		case len(cs) == 0:
			r[i] = color.NRGBA{}
		case t <= stops[0].Offset || len(cs) == 1:
			r[i] = cs[0]
		case t >= stops[len(stops)-1].Offset:
			r[i] = cs[len(cs)-1]
		default:
			j := 1
			for j < len(stops)-1 && stops[j].Offset < t {
				j++
			}
			a, b := stops[j-1], stops[j]
			f := 0.0
			if b.Offset > a.Offset {
				f = (t - a.Offset) / (b.Offset - a.Offset)
			}
			r[i] = lerp(cs[j-1], cs[j], f)
		}
	}
	return &r
}

func lerp(a, b color.NRGBA, f float64) color.NRGBA {
	m := func(x, y uint8) uint8 { return uint8(math.Round(float64(x) + (float64(y)-float64(x))*f)) }
	return color.NRGBA{m(a.R, b.R), m(a.G, b.G), m(a.B, b.B), m(a.A, b.A)}
}

func (r *ramp) at(t float64) color.NRGBA {
	return r[int(math.Round(255*max(0, min(1, t))))]
}

// linear is a <linearGradient> in user space as an image.
type linear struct {
	g      svg.LinearGradient
	bounds image.Rectangle
	ramp   *ramp
}

func (l *linear) ColorModel() color.Model { return color.NRGBAModel }
func (l *linear) Bounds() image.Rectangle { return l.bounds }
func (l *linear) At(x, y int) color.Color {
	dx, dy := l.g.X2-l.g.X1, l.g.Y2-l.g.Y1
	den := dx*dx + dy*dy
	if den == 0 {
		return l.ramp.at(0)
	}
	t := ((float64(x)+0.5-l.g.X1)*dx + (float64(y)+0.5-l.g.Y1)*dy) / den
	return l.ramp.at(t)
}

// radial is a <radialGradient> in user space as an image.
type radial struct {
	g      svg.RadialGradient
	bounds image.Rectangle
	ramp   *ramp
}

func (r *radial) ColorModel() color.Model { return color.NRGBAModel }
func (r *radial) Bounds() image.Rectangle { return r.bounds }
func (r *radial) At(x, y int) color.Color {
	if r.g.R <= 0 {
		return r.ramp.at(1)
	}
	return r.ramp.at(math.Hypot(float64(x)+0.5-r.g.CX, float64(y)+0.5-r.g.CY) / r.g.R)
}

// Package export writes a traced document as PDF or EPS, from the same
// paths and gradients the SVG writer uses, so the three outputs agree.
//
// A CSS pixel is 0.75 PostScript points (96 pixels and 72 points to the
// inch), the convention browsers and Inkscape use, so a 1000-pixel trace
// is a 750-point page. The page is flipped once with a matrix so paths
// keep the image's y-down coordinates and the SVG's numbers.
package export

import (
	"strconv"
	"strings"

	"github.com/vexyl-ai/rekha/internal/svg"
)

// Scale is points per pixel.
const Scale = 0.75

// num prints v with prec decimals the way the SVG writer does.
func num(v float64, prec int) string { return svg.FormatNum(v, prec) }

// nums joins numbers with spaces.
func nums(prec int, vs ...float64) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = num(v, prec)
	}
	return strings.Join(parts, " ")
}

// rgb parses "#rrggbb" (or "#rgb") into components in 0..1; anything
// else is black.
func rgb(hex string) [3]float64 {
	s := strings.TrimPrefix(hex, "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return [3]float64{}
	}
	var c [3]float64
	for i := range c {
		v, err := strconv.ParseUint(s[2*i:2*i+2], 16, 8)
		if err != nil {
			return [3]float64{}
		}
		c[i] = float64(v) / 255
	}
	return c
}

func rgbStr(hex string) string {
	c := rgb(hex)
	return nums(3, c[0], c[1], c[2])
}

// matrix is the pixel-to-page transform: scale, and flip y about the
// page height.
func matrix(d svg.Document) string {
	return nums(2, Scale, 0, 0, -Scale, 0, float64(d.Height)*Scale)
}

// pathOps renders p's subpaths with the operators given for moveto,
// lineto, curveto and closepath, one operation per line. A closing line
// back to the start is left to closepath; zero-length lines are dropped.
func pathOps(p svg.Path, prec int, m, l, c, h string) string {
	var b strings.Builder
	for _, sp := range p.Subpaths {
		if len(sp.Segs) == 0 {
			continue
		}
		start := sp.Segs[0].P[0]
		fs := nums(prec, start.X, start.Y)
		b.WriteString(fs + " " + m + "\n")
		for i, s := range sp.Segs {
			if s.Line {
				fa := nums(prec, s.P[0].X, s.P[0].Y)
				fc := nums(prec, s.P[3].X, s.P[3].Y)
				if fa == fc || (i == len(sp.Segs)-1 && fc == fs) {
					continue
				}
				b.WriteString(fc + " " + l + "\n")
				continue
			}
			b.WriteString(nums(prec, s.P[1].X, s.P[1].Y, s.P[2].X, s.P[2].Y, s.P[3].X, s.P[3].Y) + " " + c + "\n")
		}
		b.WriteString(h + "\n")
	}
	return b.String()
}

// gradient is one fill's gradient in either geometry.
type gradient struct {
	radial bool
	coords string // PDF/PostScript Coords
	stops  []svg.Stop
}

// gradients indexes a document's gradients by id, in the order the SVG
// writer emits them (linear first), which is the order of the pattern
// names P0, P1, …
func gradients(d svg.Document, prec int) (byID map[string]int, list []gradient) {
	byID = map[string]int{}
	for _, g := range d.Gradients {
		byID[g.ID] = len(list)
		list = append(list, gradient{coords: nums(prec, g.X1, g.Y1, g.X2, g.Y2), stops: g.Stops})
	}
	for _, g := range d.Radials {
		byID[g.ID] = len(list)
		list = append(list, gradient{radial: true, coords: nums(prec, g.CX, g.CY, 0, g.CX, g.CY, g.R), stops: g.Stops})
	}
	return byID, list
}

// translucent reports whether any stop is not fully opaque.
func (g gradient) translucent() bool {
	for _, s := range g.stops {
		if s.Opacity < 1 {
			return true
		}
	}
	return false
}

// fillGradient returns the index of the gradient a fill refers to
// ("url(#id)"), or -1 for a solid colour.
func fillGradient(fill string, byID map[string]int) int {
	if !strings.HasPrefix(fill, "url(#") || !strings.HasSuffix(fill, ")") {
		return -1
	}
	if i, ok := byID[fill[5:len(fill)-1]]; ok {
		return i
	}
	return -1
}

// shadingDict is the shading dictionary shared by PDF patterns and
// PostScript shfill: axial or radial, in DeviceRGB from the stop colours
// or, for a soft mask, in DeviceGray from the stop opacities.
func shadingDict(g gradient, grey bool, prec int) string {
	kind, space := "2", "/DeviceRGB"
	if g.radial {
		kind = "3"
	}
	if grey {
		space = "/DeviceGray"
	}
	return "<< /ShadingType " + kind + " /ColorSpace " + space + " /Coords [" + g.coords + "] /Function " + stopsFunction(g.stops, grey, prec) + " /Extend [true true] >>"
}

// stopsFunction builds the PDF function (also valid PostScript) that
// maps 0..1 along the gradient to colour, or to opacity as grey. SVG
// holds the first stop's colour before its offset and the last stop's
// after, so the function is padded to the whole domain; stops at the
// same offset make a hard edge there. One segment needs no stitching.
func stopsFunction(stops []svg.Stop, grey bool, prec int) string {
	value := func(s svg.Stop) string {
		if grey {
			return num(s.Opacity, prec)
		}
		return rgbStr(s.Colour)
	}
	type seg struct {
		t0, t1 float64
		c0, c1 string
	}
	var segs []seg
	if len(stops) == 0 {
		segs = []seg{{0, 1, "0 0 0", "0 0 0"}}
	} else {
		first, last := stops[0], stops[len(stops)-1]
		if first.Offset > 0 {
			segs = append(segs, seg{0, first.Offset, value(first), value(first)})
		}
		for i := 0; i+1 < len(stops); i++ {
			if stops[i+1].Offset > stops[i].Offset {
				segs = append(segs, seg{stops[i].Offset, stops[i+1].Offset, value(stops[i]), value(stops[i+1])})
			}
		}
		if last.Offset < 1 {
			segs = append(segs, seg{last.Offset, 1, value(last), value(last)})
		}
		if len(segs) == 0 { // every stop at one offset
			segs = []seg{{0, 1, value(last), value(last)}}
		}
	}
	exp := func(s seg) string {
		return "<< /FunctionType 2 /Domain [0 1] /C0 [" + s.c0 + "] /C1 [" + s.c1 + "] /N 1 >>"
	}
	constant := true
	for _, s := range segs {
		constant = constant && s.c0 == segs[0].c0 && s.c1 == segs[0].c0
	}
	if constant {
		return exp(seg{0, 1, segs[0].c0, segs[0].c0})
	}
	if len(segs) == 1 {
		return exp(segs[0])
	}
	var fns, bounds, encode []string
	for i, s := range segs {
		fns = append(fns, exp(s))
		if i > 0 {
			bounds = append(bounds, num(s.t0, prec))
		}
		encode = append(encode, "0 1")
	}
	return "<< /FunctionType 3 /Domain [0 1] /Functions [" + strings.Join(fns, " ") + "] /Bounds [" + strings.Join(bounds, " ") + "] /Encode [" + strings.Join(encode, " ") + "] >>"
}

// mark is the generator text for metadata: "Rekha <version>, <repo>".
func mark(d svg.Document) string {
	if d.Generator == "" {
		return ""
	}
	return strings.Replace(d.Generator, "rekha", "Rekha", 1) + ", " + svg.Repository
}

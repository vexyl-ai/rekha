// Package svg writes traced paths as a compact, deterministic SVG document.
package svg

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/vexyl-ai/rekha/internal/fit"
)

// Subpath is one closed loop; the path closes implicitly with Z.
type Subpath struct {
	Segs []fit.Segment
}

// Path is one filled shape: the first subpath is the outer boundary, the
// rest are holes (opposite orientation, nonzero fill). A non-empty Stroke
// is drawn StrokeWidth wide with round joins, which cutout output uses to
// cover the hairline seams between abutting shapes.
type Path struct {
	Fill        string
	Stroke      string
	StrokeWidth float64
	Subpaths    []Subpath
}

// FormatNum prints v with prec decimals, trimming trailing zeros and the
// trailing dot, and never printing "-0".
func FormatNum(v float64, prec int) string {
	s := strconv.FormatFloat(v, 'f', prec, 64)
	if strings.ContainsRune(s, '.') {
		s = strings.TrimRight(s, "0")
		s = strings.TrimSuffix(s, ".")
	}
	if s == "-0" {
		return "0"
	}
	return s
}

// PathData renders p's subpaths as SVG path data using absolute commands.
// Straight segments that change only one coordinate become H or V.
func PathData(p Path, prec int) string {
	var b strings.Builder
	for _, sp := range p.Subpaths {
		if len(sp.Segs) == 0 {
			continue
		}
		start := sp.Segs[0].P[0]
		b.WriteByte('M')
		writeNums(&b, prec, start.X, start.Y)
		fs := [2]string{FormatNum(start.X, prec), FormatNum(start.Y, prec)}
		for i, s := range sp.Segs {
			if s.Line {
				a, c := s.P[0], s.P[3]
				fa := [2]string{FormatNum(a.X, prec), FormatNum(a.Y, prec)}
				fc := [2]string{FormatNum(c.X, prec), FormatNum(c.Y, prec)}
				if i == len(sp.Segs)-1 && fc == fs {
					continue // Z closes the subpath; the explicit line is redundant
				}
				switch {
				case fa[1] == fc[1] && fa[0] != fc[0]:
					b.WriteByte('H')
					writeNums(&b, prec, c.X)
				case fa[0] == fc[0] && fa[1] != fc[1]:
					b.WriteByte('V')
					writeNums(&b, prec, c.Y)
				case fa == fc:
					// zero-length after rounding: skip
				default:
					b.WriteByte('L')
					writeNums(&b, prec, c.X, c.Y)
				}
				continue
			}
			b.WriteByte('C')
			writeNums(&b, prec, s.P[1].X, s.P[1].Y, s.P[2].X, s.P[2].Y, s.P[3].X, s.P[3].Y)
		}
		b.WriteByte('Z')
	}
	return b.String()
}

// writeNums appends numbers separated by a space unless the next number is
// negative, in which case its sign is the separator.
func writeNums(b *strings.Builder, prec int, nums ...float64) {
	for i, v := range nums {
		s := FormatNum(v, prec)
		if i > 0 && s[0] != '-' {
			b.WriteByte(' ')
		}
		b.WriteString(s)
	}
}

// Stop is one colour of a gradient at an offset in 0..1, at an opacity
// (1 is opaque and the usual case; a translucent stop is written with
// stop-opacity).
type Stop struct {
	Offset  float64
	Colour  string // "#rrggbb"
	Opacity float64
}

// LinearGradient is a <linearGradient> in user space (pixel coordinates)
// from (X1,Y1) to (X2,Y2); paths reference it as "url(#ID)".
type LinearGradient struct {
	ID             string
	X1, Y1, X2, Y2 float64
	Stops          []Stop
}

// RadialGradient is a <radialGradient> in user space (pixel coordinates)
// about (CX,CY) out to radius R; paths reference it as "url(#ID)".
type RadialGradient struct {
	ID        string
	CX, CY, R float64
	Stops     []Stop
}

// Document is a complete SVG: its size, the gradients its paths use and
// the paths in paint order.
type Document struct {
	Width, Height int
	Gradients     []LinearGradient
	Radials       []RadialGradient
	Paths         []Path
	// Generator, when set, names the tool and version that produced the
	// document: it goes on the root as data-generator and, with the
	// project URL, in a comment on the first line inside the root.
	Generator string
}

// Repository is the project URL the generator comment points at.
const Repository = "https://github.com/vexyl-ai/rekha"

// writeStop writes one <stop/> with its offset to prec decimals; the
// opacity, when under 1, to three.
func writeStop(b *strings.Builder, s Stop, prec int) {
	fmt.Fprintf(b, "<stop offset=\"%s\" stop-color=\"%s\"", FormatNum(s.Offset, prec), s.Colour)
	if s.Opacity < 1 {
		fmt.Fprintf(b, " stop-opacity=\"%s\"", FormatNum(s.Opacity, 3))
	}
	b.WriteString("/>")
}

// Write emits a complete SVG document with no gradients.
func Write(w io.Writer, width, height int, paths []Path, prec int) error {
	return WriteDocument(w, Document{Width: width, Height: height, Paths: paths}, prec)
}

// WriteDocument emits d: the gradients, when any, as one <defs> line
// before the paths, linear ones first. Offsets are printed with three
// decimals, four on a radial gradient (whose first stop may sit most of
// the way out).
func WriteDocument(w io.Writer, d Document, prec int) error {
	mark := ""
	if d.Generator != "" {
		mark = fmt.Sprintf(" data-generator=\"%s\"", d.Generator)
	}
	if _, err := fmt.Fprintf(w, "<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"%d\" height=\"%d\" viewBox=\"0 0 %d %d\"%s>\n", d.Width, d.Height, d.Width, d.Height, mark); err != nil {
		return err
	}
	if d.Generator != "" {
		if _, err := fmt.Fprintf(w, "<!-- Traced by %s, %s -->\n", strings.Replace(d.Generator, "rekha", "Rekha", 1), Repository); err != nil {
			return err
		}
	}
	if len(d.Gradients)+len(d.Radials) > 0 {
		var b strings.Builder
		b.WriteString("<defs>")
		for _, g := range d.Gradients {
			fmt.Fprintf(&b, "<linearGradient id=\"%s\" gradientUnits=\"userSpaceOnUse\" x1=\"%s\" y1=\"%s\" x2=\"%s\" y2=\"%s\">",
				g.ID, FormatNum(g.X1, prec), FormatNum(g.Y1, prec), FormatNum(g.X2, prec), FormatNum(g.Y2, prec))
			for _, s := range g.Stops {
				writeStop(&b, s, 3)
			}
			b.WriteString("</linearGradient>")
		}
		for _, g := range d.Radials {
			fmt.Fprintf(&b, "<radialGradient id=\"%s\" gradientUnits=\"userSpaceOnUse\" cx=\"%s\" cy=\"%s\" r=\"%s\">",
				g.ID, FormatNum(g.CX, prec), FormatNum(g.CY, prec), FormatNum(g.R, prec))
			for _, s := range g.Stops {
				writeStop(&b, s, 4)
			}
			b.WriteString("</radialGradient>")
		}
		b.WriteString("</defs>\n")
		if _, err := io.WriteString(w, b.String()); err != nil {
			return err
		}
	}
	for _, p := range d.Paths {
		d := PathData(p, prec)
		if d == "" {
			continue
		}
		stroke := ""
		if p.Stroke != "" {
			stroke = fmt.Sprintf(" stroke=\"%s\" stroke-width=\"%s\" stroke-linejoin=\"round\"", p.Stroke, FormatNum(p.StrokeWidth, prec))
		}
		if _, err := fmt.Fprintf(w, "<path fill=\"%s\"%s d=\"%s\"/>\n", p.Fill, stroke, d); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, "</svg>\n")
	return err
}

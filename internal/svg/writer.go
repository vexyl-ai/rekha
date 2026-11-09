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

// Write emits a complete SVG document.
func Write(w io.Writer, width, height int, paths []Path, prec int) error {
	if _, err := fmt.Fprintf(w, "<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"%d\" height=\"%d\" viewBox=\"0 0 %d %d\">\n", width, height, width, height); err != nil {
		return err
	}
	for _, p := range paths {
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

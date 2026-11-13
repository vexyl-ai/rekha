package export

import (
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/vexyl-ai/rekha/internal/svg"
)

// WriteEPS writes d as an EPSF-3.0 file at PostScript language level 3,
// which shfill needs for the gradients. PostScript has no transparency,
// so a translucent gradient (an overlay layer with stop opacity) is
// left out and the base fill under it stands alone; every opaque fill,
// gradient and stroke is written as in the PDF.
func WriteEPS(w io.Writer, d svg.Document, prec int) error {
	byID, grads := gradients(d, prec)
	pageW, pageH := float64(d.Width)*pageScale(d), float64(d.Height)*pageScale(d)
	var b strings.Builder
	b.WriteString("%!PS-Adobe-3.0 EPSF-3.0\n")
	fmt.Fprintf(&b, "%%%%BoundingBox: 0 0 %d %d\n", int(math.Ceil(pageW)), int(math.Ceil(pageH)))
	fmt.Fprintf(&b, "%%%%HiResBoundingBox: 0 0 %s %s\n", num(pageW, 2), num(pageH, 2))
	if m := mark(d); m != "" {
		b.WriteString("%%Creator: " + m + "\n")
	}
	b.WriteString("%%LanguageLevel: 3\n%%Pages: 1\n%%EndComments\n")
	b.WriteString("%%BeginProlog\n/m {moveto} bind def /l {lineto} bind def /c {curveto} bind def /h {closepath} bind def /f {fill} bind def /rg {setrgbcolor} bind def\n%%EndProlog\n")
	b.WriteString("%%Page: 1 1\ngsave\n[" + matrix(d) + "] concat\n")
	for _, p := range d.Paths {
		gi := fillGradient(p.Fill, byID)
		if gi >= 0 && grads[gi].translucent() {
			continue
		}
		b.WriteString(pathOps(p, prec, "m", "l", "c", "h"))
		if p.Stroke != "" {
			b.WriteString("gsave ")
		}
		if gi >= 0 {
			b.WriteString("gsave clip " + shadingDict(grads[gi], false, prec) + " shfill grestore")
		} else {
			b.WriteString(rgbStr(p.Fill) + " rg f")
		}
		if p.Stroke != "" {
			// grestore brings the path back; a gradient stroke is the
			// stroke's outline clipped around the shading.
			b.WriteString(" grestore " + num(p.StrokeWidth, prec) + " setlinewidth 1 setlinejoin ")
			if si := fillGradient(p.Stroke, byID); si >= 0 {
				b.WriteString("gsave strokepath clip " + shadingDict(grads[si], false, prec) + " shfill grestore")
			} else {
				b.WriteString(rgbStr(p.Stroke) + " rg stroke")
			}
		}
		// clip and grestore leave the current path behind; the next
		// shape must not be appended to it.
		b.WriteString(" newpath\n")
	}
	b.WriteString("grestore\nshowpage\n%%Trailer\n%%EOF\n")
	_, err := io.WriteString(w, b.String())
	return err
}

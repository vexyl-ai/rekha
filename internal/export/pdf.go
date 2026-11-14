package export

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"strings"

	"github.com/vexyl-ai/rekha/internal/svg"
)

// WritePDF writes d as a one-page PDF 1.4. Solid fills paint directly;
// a gradient paints through an axial or radial shading pattern; a
// translucent gradient (an overlay layer with stop opacity) paints
// through a luminosity soft mask carrying the same shading in grey, so
// the overlay blends as it does in the SVG. The content stream is
// flate-compressed; coordinates are printed with prec decimals.
func WritePDF(w io.Writer, d svg.Document, prec int) error {
	var objs []string // body of object i+1
	alloc := func() int { objs = append(objs, ""); return len(objs) }
	set := func(n int, body string) { objs[n-1] = body }

	catalog, pages, page, contents := alloc(), alloc(), alloc(), alloc()
	byID, grads := gradients(d, prec)
	pageW, pageH := num(float64(d.Width)*pageScale(d), 2), num(float64(d.Height)*pageScale(d), 2)
	bbox := "[0 0 " + pageW + " " + pageH + "]"

	// One pattern per gradient, plus a mask pattern, a form and an
	// ExtGState for each translucent one.
	var patterns, states []string
	for i, g := range grads {
		p := alloc()
		set(p, "<< /Type /Pattern /PatternType 2 /Shading "+shadingDict(g, false, prec)+" /Matrix ["+matrix(d)+"] >>")
		patterns = append(patterns, fmt.Sprintf("/P%d %d 0 R", i, p))
		if !g.translucent() {
			continue
		}
		mp := alloc()
		set(mp, "<< /Type /Pattern /PatternType 2 /Shading "+shadingDict(g, true, prec)+" /Matrix ["+matrix(d)+"] >>")
		form := alloc()
		body := "/Pattern cs /M scn\n0 0 " + pageW + " " + pageH + " re\nf\n"
		set(form, fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox %s /Group << /S /Transparency /CS /DeviceGray >> /Resources << /Pattern << /M %d 0 R >> >> /Length %d >>\nstream\n%sendstream", bbox, mp, len(body), body))
		gs := alloc()
		set(gs, fmt.Sprintf("<< /Type /ExtGState /SMask << /S /Luminosity /G %d 0 R /BC [0] >> >>", form))
		states = append(states, fmt.Sprintf("/GS%d %d 0 R", i, gs))
	}

	// Each path paints in its own q … Q: the soft-mask state first,
	// while the CTM is still the page's default space (a soft mask's
	// coordinate system is the CTM when its ExtGState is set, and its
	// form paints the page rectangle in points through the mask
	// pattern's own matrix), then the pixel-to-page matrix, then the
	// fill. With the matrix applied once for the whole stream the mask
	// went through it twice and the overlays painted far too strong.
	var c strings.Builder
	cm := matrix(d) + " cm\n"
	for _, p := range d.Paths {
		c.WriteString("q\n")
		if gi := fillGradient(p.Fill, byID); gi >= 0 {
			if grads[gi].translucent() {
				fmt.Fprintf(&c, "/GS%d gs\n", gi)
			}
			c.WriteString(cm)
			fmt.Fprintf(&c, "/Pattern cs /P%d scn\n", gi)
		} else {
			c.WriteString(cm)
			c.WriteString(rgbStr(p.Fill) + " rg\n")
		}
		paint := "f"
		if p.Stroke != "" {
			if si := fillGradient(p.Stroke, byID); si >= 0 {
				fmt.Fprintf(&c, "/Pattern CS /P%d SCN ", si)
			} else {
				c.WriteString(rgbStr(p.Stroke) + " RG ")
			}
			c.WriteString(num(p.StrokeWidth, prec) + " w 1 j\n")
			paint = "B"
		}
		c.WriteString(pathOps(p, prec, "m", "l", "c", "h"))
		c.WriteString(paint + "\nQ\n")
	}
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	if _, err := zw.Write([]byte(c.String())); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	set(contents, fmt.Sprintf("<< /Length %d /Filter /FlateDecode >>\nstream\n%s\nendstream", z.Len(), z.Bytes()))

	res := "<< "
	if len(patterns) > 0 {
		res += "/Pattern << " + strings.Join(patterns, " ") + " >> "
	}
	if len(states) > 0 {
		res += "/ExtGState << " + strings.Join(states, " ") + " >> "
	}
	res += ">>"
	set(page, fmt.Sprintf("<< /Type /Page /Parent %d 0 R /MediaBox %s /Group << /S /Transparency /CS /DeviceRGB >> /Resources %s /Contents %d 0 R >>", pages, bbox, res, contents))
	set(pages, fmt.Sprintf("<< /Type /Pages /Kids [%d 0 R] /Count 1 >>", page))
	set(catalog, fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pages))
	info := 0
	if m := mark(d); m != "" {
		info = alloc()
		set(info, "<< /Producer ("+pdfString(m)+") /Creator ("+pdfString(strings.Replace(d.Generator, "rekha", "Rekha", 1))+") >>")
	}

	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	offsets := make([]int, len(objs))
	for i, body := range objs {
		offsets[i] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, body)
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root %d 0 R", len(objs)+1, catalog)
	if info > 0 {
		fmt.Fprintf(&out, " /Info %d 0 R", info)
	}
	fmt.Fprintf(&out, " >>\nstartxref\n%d\n%%%%EOF\n", xref)
	_, err := w.Write(out.Bytes())
	return err
}

// pdfString escapes a literal string.
func pdfString(s string) string {
	return strings.NewReplacer(`\`, `\\`, "(", `\(`, ")", `\)`).Replace(s)
}

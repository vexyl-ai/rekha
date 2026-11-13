package rekha

import (
	"fmt"
	"io"
	"strings"

	"github.com/vexyl-ai/rekha/internal/export"
	"github.com/vexyl-ai/rekha/internal/svg"
)

// Format is an output file format. The SVG is the reference: the PDF
// and the EPS are written from the same paths and gradients at 0.75
// points per pixel (96 pixels and 72 points to the inch). PDF keeps
// every gradient, translucent overlays through soft masks; EPS has no
// transparency and leaves translucent overlay layers out.
type Format string

const (
	FormatSVG Format = "svg"
	FormatPDF Format = "pdf"
	FormatEPS Format = "eps"
)

// ParseFormat reads a format name, case-insensitively, with or without a
// leading dot; "" is SVG.
func ParseFormat(s string) (Format, error) {
	switch f := Format(strings.ToLower(strings.TrimPrefix(s, "."))); f {
	case "":
		return FormatSVG, nil
	case FormatSVG, FormatPDF, FormatEPS:
		return f, nil
	default:
		return "", &ValidationError{"Format", fmt.Sprintf("unknown value %q, want svg, pdf or eps", s)}
	}
}

// document assembles the writer's view of r, marked unless o.NoMark.
func (r *Result) document(o Options) svg.Document {
	d := svg.Document{Width: r.Width, Height: r.Height, Gradients: r.Gradients, Radials: r.Radials, Paths: r.Paths}
	if !o.NoMark {
		d.Generator = "rekha " + Version()
	}
	return d
}

// Write writes r in o.Format (SVG when unset) with o.PathPrecision
// decimals, marked with Rekha's name and version unless o.NoMark.
func (r *Result) Write(w io.Writer, o Options) error {
	f, err := ParseFormat(string(o.Format))
	if err != nil {
		return err
	}
	switch f {
	case FormatPDF:
		return r.WritePDF(w, o)
	case FormatEPS:
		return r.WriteEPS(w, o)
	default:
		return r.WriteSVG(w, o)
	}
}

// WritePDF writes r as a one-page PDF; see Format.
func (r *Result) WritePDF(w io.Writer, o Options) error {
	return export.WritePDF(w, r.document(o), o.PathPrecision)
}

// WriteEPS writes r as an EPS file; see Format.
func (r *Result) WriteEPS(w io.Writer, o Options) error {
	return export.WriteEPS(w, r.document(o), o.PathPrecision)
}

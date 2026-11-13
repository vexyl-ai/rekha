package svg

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/vexyl-ai/rekha/internal/fit"
	"github.com/vexyl-ai/rekha/internal/geom"
)

// Parse reads a document this package wrote (or one edited in place,
// with paths removed or recoloured) back into a Document, and reports
// the most decimals any coordinate carried, so WriteDocument at that
// precision gives the same bytes. It is a reader for Rekha's own
// dialect, not for SVG at large: absolute M, L, H, V, C and Z path
// commands, fills and strokes as written, linear and radial gradients
// in user space with stop opacity. Anything else is an error.
func Parse(r io.Reader) (Document, int, error) {
	var d Document
	p := &parser{}
	dec := xml.NewDecoder(r)
	var stack []string
	var cur *LinearGradient
	var curRadial *RadialGradient
	seenRoot := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return Document{}, 0, fmt.Errorf("svg: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := t.Name.Local
			stack = append(stack, name)
			switch {
			case len(stack) == 1:
				if name != "svg" {
					return Document{}, 0, fmt.Errorf("svg: root element is <%s>", name)
				}
				seenRoot = true
				w, h := attr(t, "width"), attr(t, "height")
				if w == "" || h == "" {
					return Document{}, 0, errors.New("svg: root without width and height")
				}
				if d.Width, err = strconv.Atoi(w); err != nil {
					return Document{}, 0, fmt.Errorf("svg: width %q", w)
				}
				if d.Height, err = strconv.Atoi(h); err != nil {
					return Document{}, 0, fmt.Errorf("svg: height %q", h)
				}
				d.Generator = attr(t, "data-generator")
			case name == "defs":
			case name == "linearGradient":
				g := LinearGradient{ID: attr(t, "id")}
				for _, f := range []struct {
					k string
					v *float64
				}{{"x1", &g.X1}, {"y1", &g.Y1}, {"x2", &g.X2}, {"y2", &g.Y2}} {
					if *f.v, err = p.num(attr(t, f.k)); err != nil {
						return Document{}, 0, err
					}
				}
				d.Gradients = append(d.Gradients, g)
				cur = &d.Gradients[len(d.Gradients)-1]
			case name == "radialGradient":
				g := RadialGradient{ID: attr(t, "id")}
				for _, f := range []struct {
					k string
					v *float64
				}{{"cx", &g.CX}, {"cy", &g.CY}, {"r", &g.R}} {
					if *f.v, err = p.num(attr(t, f.k)); err != nil {
						return Document{}, 0, err
					}
				}
				d.Radials = append(d.Radials, g)
				curRadial = &d.Radials[len(d.Radials)-1]
			case name == "stop":
				// Offsets and opacities are written at fixed precisions
				// of their own; they say nothing about the coordinates.
				s := Stop{Colour: attr(t, "stop-color"), Opacity: 1}
				if s.Offset, err = plain(attr(t, "offset")); err != nil {
					return Document{}, 0, err
				}
				if o := attr(t, "stop-opacity"); o != "" {
					if s.Opacity, err = plain(o); err != nil {
						return Document{}, 0, err
					}
				}
				switch {
				case len(stack) >= 2 && stack[len(stack)-2] == "linearGradient" && cur != nil:
					cur.Stops = append(cur.Stops, s)
				case len(stack) >= 2 && stack[len(stack)-2] == "radialGradient" && curRadial != nil:
					curRadial.Stops = append(curRadial.Stops, s)
				default:
					return Document{}, 0, errors.New("svg: <stop> outside a gradient")
				}
			case name == "path":
				path := Path{Fill: attr(t, "fill"), Stroke: attr(t, "stroke")}
				if sw := attr(t, "stroke-width"); sw != "" {
					if path.StrokeWidth, err = p.num(sw); err != nil {
						return Document{}, 0, err
					}
				}
				if path.Subpaths, err = p.pathData(attr(t, "d")); err != nil {
					return Document{}, 0, err
				}
				d.Paths = append(d.Paths, path)
			default:
				return Document{}, 0, fmt.Errorf("svg: <%s> is not something Rekha writes", name)
			}
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		}
	}
	if !seenRoot {
		return Document{}, 0, errors.New("svg: no <svg> element")
	}
	return d, p.prec, nil
}

func attr(e xml.StartElement, name string) string {
	for _, a := range e.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// parser counts the decimals seen so the caller can write at the same
// precision.
type parser struct{ prec int }

func plain(s string) (float64, error) {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("svg: number %q", s)
	}
	return v, nil
}

func (p *parser) num(s string) (float64, error) {
	if s == "" {
		return 0, errors.New("svg: number missing")
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("svg: number %q", s)
	}
	if i := strings.IndexByte(s, '.'); i >= 0 {
		if n := len(s) - i - 1; n > p.prec {
			p.prec = n
		}
	}
	return v, nil
}

// pathData reads absolute M, L, H, V, C and Z, as PathData writes them,
// into subpaths of line and cubic segments. Z closes the subpath
// implicitly; the closing line is not stored, matching the writer.
func (p *parser) pathData(d string) ([]Subpath, error) {
	var out []Subpath
	var sp *Subpath
	var start, pos geom.Vec2
	i := 0
	// next reads one number; numbers run together with a sign as the
	// only separator, as the writer prints them.
	next := func() (float64, error) {
		for i < len(d) && (d[i] == ' ' || d[i] == ',') {
			i++
		}
		j := i
		if j < len(d) && (d[j] == '-' || d[j] == '+') {
			j++
		}
		for j < len(d) && (d[j] >= '0' && d[j] <= '9' || d[j] == '.') {
			j++
		}
		if j < len(d) && (d[j] == 'e' || d[j] == 'E') {
			j++
			if j < len(d) && (d[j] == '-' || d[j] == '+') {
				j++
			}
			for j < len(d) && d[j] >= '0' && d[j] <= '9' {
				j++
			}
		}
		if j == i {
			return 0, fmt.Errorf("svg: path data: number expected at %d in %q", i, d)
		}
		v, err := p.num(d[i:j])
		i = j
		return v, err
	}
	point := func() (geom.Vec2, error) {
		x, err := next()
		if err != nil {
			return geom.Vec2{}, err
		}
		y, err := next()
		return geom.Vec2{X: x, Y: y}, err
	}
	for i < len(d) {
		c := d[i]
		i++
		if c == ' ' {
			continue
		}
		if c != 'M' && sp == nil {
			return nil, fmt.Errorf("svg: path data starts with %q, not M", c)
		}
		switch c {
		case 'M':
			out = append(out, Subpath{})
			sp = &out[len(out)-1]
			pt, err := point()
			if err != nil {
				return nil, err
			}
			start, pos = pt, pt
		case 'L', 'H', 'V':
			var to geom.Vec2
			var err error
			switch c {
			case 'L':
				to, err = point()
			case 'H':
				to = pos
				to.X, err = next()
			case 'V':
				to = pos
				to.Y, err = next()
			}
			if err != nil {
				return nil, err
			}
			sp.Segs = append(sp.Segs, fit.Segment{Line: true, P: fit.Cubic{pos, pos, to, to}})
			pos = to
		case 'C':
			c1, err := point()
			if err != nil {
				return nil, err
			}
			c2, err := point()
			if err != nil {
				return nil, err
			}
			to, err := point()
			if err != nil {
				return nil, err
			}
			sp.Segs = append(sp.Segs, fit.Segment{P: fit.Cubic{pos, c1, c2, to}})
			pos = to
		case 'Z':
			pos = start
			sp = nil
		default:
			return nil, fmt.Errorf("svg: path command %q is not something Rekha writes", c)
		}
	}
	return out, nil
}

package rekha

import (
	"github.com/vexyl-ai/rekha/internal/contour"
	"github.com/vexyl-ai/rekha/internal/geom"
)

// maskPad is how many pixels beyond the contour's bounding box the mask
// stores explicitly; everything further out is empty.
const maskPad = 2

// coverageMask is the refinement target for one contour: the coverage the
// contour's colour should have in each pixel of a box around it, 0
// outside the box and outside the image. Weight is 1 everywhere.
type coverageMask struct {
	x0, y0, w, h int
	cov          []float32
}

// At implements refine.Mask.
func (m *coverageMask) At(x, y int) (cov, weight float32) {
	x, y = x-m.x0, y-m.y0
	if x < 0 || y < 0 || x >= m.w || y >= m.h {
		return 0, 1
	}
	return m.cov[y*m.w+x], 1
}

// mask builds the coverage mask for a contour of colour c (its polygon
// poly, a hole when hole is set). Inside the polygon the coverage is 1,
// outside 0; on boundary pixels (inside with a 4-neighbour outside, or the
// reverse) it is the share of c the segmentation measured in that pixel
// (Blend when c is the pixel's label, its complement when c is the
// second colour, else the hard value), complemented for a hole, whose
// curve encloses the other side. Pixels for which band reports true (the
// stacked extension band) are hard 1, so the refined outline keeps
// running along the dilated band.
func (s *Segmentation) mask(c uint8, poly []geom.Point, hole bool, band func(p int) bool) *coverageMask {
	minX, minY, maxX, maxY := s.Width, s.Height, 0, 0
	for _, p := range poly {
		minX, minY = min(minX, p.X), min(minY, p.Y)
		maxX, maxY = max(maxX, p.X), max(maxY, p.Y)
	}
	x0, y0 := max(minX-maskPad, 0), max(minY-maskPad, 0)
	x1, y1 := min(maxX+maskPad, s.Width), min(maxY+maskPad, s.Height)
	m := &coverageMask{x0: x0, y0: y0, w: max(x1-x0, 0), h: max(y1-y0, 0)}
	if m.w == 0 || m.h == 0 {
		return m
	}
	inside := make([]bool, m.w*m.h)
	contour.FillOuter(poly, s.Width, s.Height, func(x, y int) {
		if x >= x0 && y >= y0 && x < x1 && y < y1 {
			inside[(y-y0)*m.w+(x-x0)] = true
		}
	})
	m.cov = make([]float32, m.w*m.h)
	at := func(x, y int) bool { // inside, with the box edge as outside
		return x >= 0 && y >= 0 && x < m.w && y < m.h && inside[y*m.w+x]
	}
	for y := 0; y < m.h; y++ {
		for x := 0; x < m.w; x++ {
			i := y*m.w + x
			in := inside[i]
			hard := float32(0)
			if in {
				hard = 1
			}
			p := (y+y0)*s.Width + x + x0
			if in && band != nil && band(p) {
				m.cov[i] = 1
				continue
			}
			boundary := at(x-1, y) != in || at(x+1, y) != in || at(x, y-1) != in || at(x, y+1) != in
			if !boundary {
				m.cov[i] = hard
				continue
			}
			share := hard
			switch {
			case s.Labels[p] == c:
				share = float32(s.Blend[p]) / 255
			case s.Second[p] == c:
				share = 1 - float32(s.Blend[p])/255
			}
			if hole {
				share = 1 - share
			}
			m.cov[i] = share
		}
	}
	return m
}

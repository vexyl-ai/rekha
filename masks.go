package rekha

import (
	"fmt"
	"image"
	"image/color"

	"github.com/vexyl-ai/rekha/internal/bitmap"
)

// checkMasks reports a mask whose coverage does not match the image.
func checkMasks(img image.Image, masks []Mask) error {
	b := img.Bounds()
	n := b.Dx() * b.Dy()
	for i, m := range masks {
		if len(m.Coverage) != n {
			return &ValidationError{fmt.Sprintf("ExternalMasks[%d]", i), fmt.Sprintf("coverage has %d values, the image has %d pixels", len(m.Coverage), n)}
		}
	}
	if len(masks) > 0 && len(masks) > maxPalette {
		return &ValidationError{"ExternalMasks", fmt.Sprintf("%d masks, at most %d", len(masks), maxPalette)}
	}
	return nil
}

// maxPalette is the number of labels a uint8 label map can address.
const maxPalette = 256

// applyMasks paints the masks over the segmentation, in order. Each mask
// is a palette entry of its own; a cut-out shares the page's white entry
// and counts as transparent, so Finish omits it as it omits a
// transparent page. Every pixel a mask covers by half or more takes its
// label, with the pixel's old label as the second colour and the
// coverage as the blend; a pixel covered less keeps its label and takes
// the mask as its second colour, so a soft matte edge reaches the
// refinement from both sides.
func (s *Segmentation) applyMasks(img image.Image, masks []Mask) {
	n := s.Width * s.Height
	if s.pixels == nil {
		s.pixels, s.clear = make([]int64, len(s.Palette)), make([]int64, len(s.Palette))
		for _, l := range s.Labels {
			s.pixels[l]++
		}
	}
	var rgb []uint8
	for _, m := range masks {
		var label int
		switch {
		case m.Cut:
			label = s.pageLabel()
		case m.Fill == (color.NRGBA{}):
			if rgb == nil {
				rgb, _, _ = bitmap.Flatten(img)
			}
			label = s.addColour(meanUnder(rgb, m.Coverage))
		default:
			label = s.addColour(m.Fill)
		}
		for p := 0; p < n; p++ {
			c := int(m.Coverage[p])
			switch {
			case c == 0:
			case c >= 128:
				old := s.Labels[p]
				s.pixels[old]--
				s.pixels[label]++
				if m.Cut {
					s.clear[label]++
				}
				if int(old) != label {
					s.Second[p] = old
				}
				s.Labels[p], s.Blend[p] = uint8(label), uint8(c)
			default:
				if int(s.Labels[p]) != label {
					s.Second[p], s.Blend[p] = uint8(label), uint8(255-c)
				}
			}
		}
	}
}

// addColour appends c to the palette (flat, no ramp) and returns its
// label. The caller has checked that the palette has room.
func (s *Segmentation) addColour(c color.NRGBA) int {
	s.Palette = append(s.Palette, c)
	if s.Ramps != nil {
		s.Ramps = append(s.Ramps, nil)
	}
	s.pixels, s.clear = append(s.pixels, 0), append(s.clear, 0)
	return len(s.Palette) - 1
}

// pageLabel is the palette's opaque white entry, added when there is
// none: the colour a cut-out region is counted under.
func (s *Segmentation) pageLabel() int {
	white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	for i, c := range s.Palette {
		if c == white && !s.hasRamp(i) {
			return i
		}
	}
	return s.addColour(white)
}

// meanUnder is the mean colour of the pixels covered by half or more.
func meanUnder(rgb []uint8, cov []uint8) color.NRGBA {
	var r, g, b, n int64
	for p, c := range cov {
		if c >= 128 {
			r += int64(rgb[3*p])
			g += int64(rgb[3*p+1])
			b += int64(rgb[3*p+2])
			n++
		}
	}
	if n == 0 {
		return color.NRGBA{A: 255}
	}
	round := func(v int64) uint8 { return uint8((2*v + n) / (2 * n)) }
	return color.NRGBA{R: round(r), G: round(g), B: round(b), A: 255}
}

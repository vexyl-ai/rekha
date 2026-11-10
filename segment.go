package rekha

import (
	"image"
	"image/color"
	"math"

	"github.com/vexyl-ai/rekha/internal/bitmap"
	"github.com/vexyl-ai/rekha/internal/colour"
	"github.com/vexyl-ai/rekha/internal/gradient"
	"github.com/vexyl-ai/rekha/internal/quant"
	"github.com/vexyl-ai/rekha/internal/region"
)

// kmeansIters bounds the Lloyd refinement of the Wu palette.
const kmeansIters = 10

// Segmentation is an image reduced to a palette and a label per pixel: the
// input to Finish, and the hand-off point for callers that quantise
// themselves.
type Segmentation struct {
	Width, Height int
	Palette       []color.NRGBA // final palette after cleanup; bw: {white, black}
	Labels        []uint8       // palette index per pixel
	Second        []uint8       // second-nearest palette index per pixel
	Blend         []uint8       // 128..255, share of Labels' colour in the pixel
	// Ramps, when Options.Gradients merged bands, has one entry per
	// palette index: the linear ramp that fills it, or nil for a flat
	// colour. Nil altogether when no option asked for ramps.
	Ramps []*gradient.Ramp
}

// hasRamp reports whether palette index c is filled by a gradient.
func (s *Segmentation) hasRamp(c int) bool {
	return s.Ramps != nil && s.Ramps[c] != nil
}

// Segment reduces img to a Segmentation under o. The bw preset thresholds
// grey exactly as Phase 1 and labels foreground 1 on a {white, black}
// palette; the colour presets quantise in OKLab (or snap to o.Palette),
// then despeckle, absorb anti-aliasing slivers and compact the palette.
func Segment(img image.Image, o Options) (*Segmentation, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}
	if o.Preset == PresetBW {
		o.report("label")
		return segmentBW(img, o), nil
	}
	return segmentColour(img, o), nil
}

func segmentBW(img image.Image, o Options) *Segmentation {
	gray, w, h := bitmap.Gray(img)
	th := uint8(math.Round(o.Threshold * 255))
	if o.AutoThreshold {
		th = bitmap.Otsu(gray)
	}
	bm := bitmap.FromGray(gray, w, h, th)
	s := newSegmentation(w, h)
	s.Palette = []color.NRGBA{{R: 255, G: 255, B: 255, A: 255}, {A: 255}}
	// Blend is the labelled colour's share of the pixel's grey, so the
	// refinement can read the anti-aliasing; the SVG never sees it.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := y*w + x
			g := int(gray[p])
			share := g
			if bm.Get(x, y) {
				s.Labels[p], s.Second[p] = 1, 0
				share = 255 - g
			} else {
				s.Labels[p], s.Second[p] = 0, 1
			}
			s.Blend[p] = uint8(max(share, 128))
		}
	}
	return s
}

func segmentColour(img image.Image, o Options) *Segmentation {
	rgb, w, h := bitmap.Flatten(img)
	s := newSegmentation(w, h)
	if w*h == 0 {
		return s
	}
	o.report("quantise")
	cells := quant.Cells(rgb, o.ColorPrecision)

	var pal []colour.OKLab
	if len(o.Palette) > 0 {
		pal = make([]colour.OKLab, len(o.Palette))
		for i, c := range o.Palette {
			pal[i] = colour.FromSRGB8(c.R, c.G, c.B)
		}
	} else {
		pal = quant.KMeans(cells, quant.Wu(cells, o.MaxColors, o.ColorTolerance), kmeansIters)
	}

	o.report("label")
	scratch := make([]uint8, w*h)
	quant.NewLookup(pal, o.ColorPrecision).Map(rgb, s.Labels, s.Second, s.Blend)
	final := region.Cleanup(s.Labels, rgb, w, h, pal, o.FilterSpeckle, o.ColorTolerance)

	// Second and Blend against the final palette. Where cleanup moved a
	// pixel off its nearest colour, that colour becomes the second.
	quant.NewLookup(final, o.ColorPrecision).Map(rgb, scratch, s.Second, s.Blend)
	for p, l := range s.Labels {
		if scratch[p] != l {
			s.Second[p] = scratch[p]
		}
	}

	if o.Gradients {
		o.report("ramps")
		s.mergeRamps(rgb, final, pal, o)
		return s
	}
	s.Palette = make([]color.NRGBA, len(final))
	if len(o.Palette) > 0 {
		for i, c := range final {
			s.Palette[i] = fixedEntry(c, o.Palette, pal)
		}
	} else {
		meanColours(rgb, s.Labels, s.Palette)
	}
	return s
}

// mergeRamps joins adjacent bands that form one linear ramp (see
// internal/gradient), relabels, and rebuilds Second, Blend and the
// palette for the new labels. Inside a ramp a pixel is wholly its own
// colour; elsewhere a second colour that was absorbed entirely becomes
// the pixel's own. A fixed palette keeps its entries for the labels that
// stay flat; merged regions take their mean.
func (s *Segmentation) mergeRamps(rgb []uint8, final, pal []colour.OKLab, o Options) {
	g := gradient.Merge(rgb, s.Labels, s.Width, s.Height, len(final), o.GradientTolerance)
	s.Labels = g.Labels
	for p, l := range s.Labels {
		if g.Ramps[l] != nil {
			s.Second[p], s.Blend[p] = l, 255
			continue
		}
		if ns := g.OldToNew[s.Second[p]]; ns >= 0 {
			s.Second[p] = uint8(ns)
		} else {
			s.Second[p], s.Blend[p] = l, 255
		}
	}
	s.Ramps = g.Ramps
	s.Palette = make([]color.NRGBA, g.N)
	meanColours(rgb, s.Labels, s.Palette)
	if len(o.Palette) > 0 {
		for i, c := range final {
			if ns := g.OldToNew[i]; ns >= 0 {
				s.Palette[ns] = fixedEntry(c, o.Palette, pal)
			}
		}
	}
}

// meanColours sets each palette entry to the mean sRGB of the pixels that
// carry its label: exact for flat art, and the colour that minimises the
// squared error of a region elsewhere.
func meanColours(rgb, labels []uint8, pal []color.NRGBA) {
	sums := make([][4]int64, len(pal))
	for p, l := range labels {
		s := &sums[l]
		s[0] += int64(rgb[3*p])
		s[1] += int64(rgb[3*p+1])
		s[2] += int64(rgb[3*p+2])
		s[3]++
	}
	for i, s := range sums {
		if s[3] == 0 {
			continue
		}
		div := func(v int64) uint8 { return uint8((v + s[3]/2) / s[3]) }
		pal[i] = color.NRGBA{R: div(s[0]), G: div(s[1]), B: div(s[2]), A: 255}
	}
}

// fixedEntry returns the input palette colour that final entry c came from.
func fixedEntry(c colour.OKLab, fixed []color.NRGBA, lab []colour.OKLab) color.NRGBA {
	for i, l := range lab {
		if l == c {
			return fixed[i]
		}
	}
	r, g, b := c.ToSRGB8()
	return color.NRGBA{R: r, G: g, B: b, A: 255}
}

func newSegmentation(w, h int) *Segmentation {
	return &Segmentation{
		Width: w, Height: h,
		Labels: make([]uint8, w*h),
		Second: make([]uint8, w*h),
		Blend:  make([]uint8, w*h),
	}
}

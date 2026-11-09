package rekha

import (
	"image"
	"image/color"
	"math"

	"github.com/vexyl-ai/rekha/internal/bitmap"
	"github.com/vexyl-ai/rekha/internal/colour"
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
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := y*w + x
			if bm.Get(x, y) {
				s.Labels[p], s.Second[p] = 1, 0
			} else {
				s.Labels[p], s.Second[p] = 0, 1
			}
			s.Blend[p] = 255
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

	scratch := make([]uint8, w*h)
	quant.NewLookup(pal, o.ColorPrecision).Map(rgb, s.Labels, s.Second, s.Blend)
	final := region.Cleanup(s.Labels, w, h, pal, o.FilterSpeckle, o.ColorTolerance)

	// Second and Blend against the final palette. Where cleanup moved a
	// pixel off its nearest colour, that colour becomes the second.
	quant.NewLookup(final, o.ColorPrecision).Map(rgb, scratch, s.Second, s.Blend)
	for p, l := range s.Labels {
		if scratch[p] != l {
			s.Second[p] = scratch[p]
		}
	}

	s.Palette = make([]color.NRGBA, len(final))
	for i, c := range final {
		s.Palette[i] = nrgbaOf(c, o.Palette, pal)
	}
	return s
}

// nrgbaOf converts a final palette entry to sRGB. An entry that came from
// a fixed palette keeps its exact input value.
func nrgbaOf(c colour.OKLab, fixed []color.NRGBA, lab []colour.OKLab) color.NRGBA {
	for i, l := range lab[:len(fixed)] {
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

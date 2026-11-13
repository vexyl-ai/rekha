package rekha

import (
	"image"

	"github.com/vexyl-ai/rekha/internal/metrics"
	"github.com/vexyl-ai/rekha/internal/render"
)

// Auto traces img under each of a few candidate option sets and keeps
// the one whose rendering is closest to the image: PSNR over white, the
// same measure the benchmarks use, from a renderer built in. It returns
// that result and the options that produced it, so the caller can
// write the SVG with them or run them again. The candidates, all spline
// mode with refinement, are the poster preset at tolerance 0.06; poster
// at 0.04 with gradients; photo with gradients, speckle filter 2; and
// bw with the automatic threshold. A palette in o adds a fifth, poster
// at 0.06 fixed to that palette. The candidates take o's output
// options (PathPrecision, Stroke, NoMark, Format, ExternalMasks,
// Progress) and nothing else of it. Auto costs the four or five traces
// and their renderings, run one after another; Progress sees stage
// "auto" counting candidates round the stages of each.
func Auto(img image.Image, o Options) (*Result, Options, error) {
	cands := autoCandidates(o)
	next := o.counter("auto", len(cands))
	var best *Result
	var bestOpts Options
	bestScore := -1.0
	for _, c := range cands {
		r, err := Trace(img, c)
		if err != nil {
			return nil, Options{}, err
		}
		if s := score(img, r, c); s > bestScore {
			best, bestOpts, bestScore = r, c, s
		}
		next()
	}
	return best, bestOpts, nil
}

// autoCandidates is the race Auto runs for o: see Auto.
func autoCandidates(o Options) []Options {
	with := func(p Preset, set func(c *Options)) Options {
		c := PresetOptions(p)
		c.Refine.Enabled = true
		c.PathPrecision = o.PathPrecision
		c.Stroke = o.Stroke
		c.CutoutStroke = o.CutoutStroke
		c.NoMark = o.NoMark
		c.Format = o.Format
		c.ExternalMasks = o.ExternalMasks
		c.Progress = o.Progress
		set(&c)
		return c
	}
	cands := []Options{
		with(PresetPoster, func(c *Options) { c.ColorTolerance = 0.06 }),
		with(PresetPoster, func(c *Options) { c.ColorTolerance = 0.04; c.Gradients = true }),
		with(PresetPhoto, func(c *Options) { c.Gradients = true; c.FilterSpeckle = 2 }),
		with(PresetBW, func(c *Options) { c.AutoThreshold = true }),
	}
	if len(o.Palette) > 0 {
		cands = append(cands, with(PresetPoster, func(c *Options) { c.ColorTolerance = 0.06; c.Palette = o.Palette }))
	}
	return cands
}

// score is the PSNR in dB of r rendered under o against img; +Inf when
// they match exactly.
func score(img image.Image, r *Result, o Options) float64 {
	psnr, err := metrics.PSNR(img, render.Paint(r.document(o)))
	if err != nil {
		return -1
	}
	return psnr
}

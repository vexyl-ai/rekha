package rekha

import (
	"image"
	"sync"

	"github.com/vexyl-ai/rekha/internal/metrics"
	"github.com/vexyl-ai/rekha/internal/par"
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
// and their renderings. They run at once as far as memory allows (a
// trace keeps about a hundred bytes a pixel: all of them on an image up
// to four megapixels, two on eight, one on sixteen) and never more than
// GOMAXPROCS, so the WebAssembly build runs them one after another;
// Progress sees stage "auto" counting candidates round the stages of
// each, interleaved when they run together. The choice does not depend
// on the scheduling: a tie goes to the earlier candidate.
func Auto(img image.Image, o Options) (*Result, Options, error) {
	cands := autoCandidates(o)
	next := o.counter("auto", len(cands))
	b := img.Bounds()
	results := make([]*Result, len(cands))
	scores := make([]float64, len(cands))
	errs := make([]error, len(cands))
	sem := make(chan struct{}, autoWorkers(len(cands), b.Dx()*b.Dy()))
	var wg sync.WaitGroup
	for i, c := range cands {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			r, err := Trace(img, c)
			if err != nil {
				errs[i] = err
				return
			}
			results[i], scores[i] = r, score(img, r, c)
			next()
		}()
	}
	wg.Wait()
	best := -1
	for i := range cands {
		if errs[i] != nil {
			return nil, Options{}, errs[i]
		}
		if best < 0 || scores[i] > scores[best] {
			best = i
		}
	}
	return results[best], cands[best], nil
}

// autoPixels is the image area, in pixels, that one candidate may have
// to itself: the memory of a trace is about proportional to it, and a
// sixteen-megapixel trace already takes most of two gigabytes.
const autoPixels = 4 << 20

// autoWorkers is how many of n candidates run at once on an image of
// the given pixel count: n on a small image, fewer as the image grows,
// at least one, and never more than par.Workers allows.
func autoWorkers(n, pixels int) int {
	return max(1, min(par.Workers(n), autoPixels*4/max(pixels, 1), n))
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

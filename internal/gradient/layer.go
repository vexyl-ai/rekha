package gradient

import (
	"math"
	"slices"
)

// A region shaded two ways at once (a sphere: a highlight over a hue
// drift, a rim light) fits no single field. The layered model paints a
// base ramp and then up to maxOverlays radial overlays over it, each a
// flat colour whose opacity falls off with the distance from its own
// centre, as SVG composites a path over another:
//
//	c = (1 − α₂)·((1 − α₁)·B(t) + α₁·H₁) + α₂·H₂
//
// The fit alternates: each overlay's opacity profile and colour by
// least squares given the rest, then the base given the overlays, on a
// fixed-stride sample of the region's interior pixels; an overlay's
// centre starts where the residual's gradients point through and is
// refined by coordinate descent on the sample RMS.
const (
	maxOverlays  = 2
	layerSamples = 4096 // interior pixels the alternating fit works on
	layerIters   = 8    // alternating rounds per fit
	layerGain    = 0.85 // an overlay stays when it cuts the RMS to this share
	layerGoal    = 0.5  // overlays are added until the RMS is this share of tol
	opacityBins  = 32
	refineTrials = 24  // centre moves tried per overlay
	showThrough  = 0.1 // least share of the base a pixel must show to vote on it
	peakShare    = 0.3 // share of the samples, by residual, that is the residual's peak
	seedReach    = 2.0 // how far past the residual an overlay's colour is seeded
	mirror       = 2.0 // where the peak's mirror seed lies, in peak offsets from the centroid
	ringSeeds    = 8   // centres tried on a ring outside the region, at mirror RMS radii
	refineSeeds  = 3   // seeds, best short fits first, that are refined
)

// overlay is one radial layer: colour over an opacity profile sampled
// by distance from (cx, cy) between tmin and tmax.
type overlay struct {
	cx, cy     float64
	colour     [3]float64
	tmin, tmax float64
	alpha      []float64 // per bin, non-increasing from the centre
}

// at is the overlay's opacity at (x, y): the profile interpolated
// between bin centres, held flat beyond the ends.
func (o *overlay) at(x, y float64) float64 {
	return o.alphaAt(math.Hypot(x-o.cx, y-o.cy))
}

func (o *overlay) alphaAt(r float64) float64 {
	nb := len(o.alpha)
	if nb == 0 {
		return 0
	}
	f := float64((r-o.tmin)/(o.tmax-o.tmin))*float64(nb) - 0.5
	if f <= 0 {
		return o.alpha[0]
	}
	if f >= float64(nb-1) {
		return o.alpha[nb-1]
	}
	i := int(f)
	g := f - float64(i)
	return o.alpha[i] + float64(g*(o.alpha[i+1]-o.alpha[i]))
}

// layered is a base ramp under overlays, with the RMS of the interior
// pixels against it.
type layered struct {
	fld        field
	tmin, tmax float64
	base       []rawStop
	over       []overlay
	rms        float64
}

func (l *layered) baseAt(x, y float64) [3]float64 {
	return colourAt(l.base, (l.fld.at(x, y)-l.tmin)/(l.tmax-l.tmin))
}

func (l *layered) at(x, y float64) [3]float64 {
	c := l.baseAt(x, y)
	for i := range l.over {
		c = mixColour(c, l.over[i].colour, l.over[i].at(x, y))
	}
	return c
}

func mixColour(a, b [3]float64, f float64) [3]float64 {
	return [3]float64{a[0] + float64(f*(b[0]-a[0])), a[1] + float64(f*(b[1]-a[1])), a[2] + float64(f*(b[2]-a[2]))}
}

// pixelSample is one interior pixel of the fit.
type pixelSample struct {
	x, y float64
	c    [3]float64
	t    float64 // base parameter
}

// ramp is the layered model as a Ramp: the base along the field (as
// rampFrom lays a linear ramp, through the centroid of m), with one
// overlay per layer whose stops, one per opacity bin that the model at
// tol keeps, carry the opacity in their alpha. nil when the base has no
// stops.
func (l *layered) ramp(m *moments, tol float64) *Ramp {
	var stops []Stop
	for _, s := range modelOf(l.base, tol) {
		stops = append(stops, Stop{Offset: s.offset, Colour: quantise(s.colour)})
	}
	if len(stops) < 2 {
		return nil
	}
	n := float64(m.n)
	cx, cy := m.sx/n, m.sy/n
	tc := l.fld.at(cx, cy)
	rp := &Ramp{
		X0: cx + float64((l.tmin-tc)*l.fld.u[0]), Y0: cy + float64((l.tmin-tc)*l.fld.u[1]),
		X1: cx + float64((l.tmax-tc)*l.fld.u[0]), Y1: cy + float64((l.tmax-tc)*l.fld.u[1]),
		Stops: stops,
	}
	for i := range l.over {
		o := &l.over[i]
		// The profile as samples in a grey ramp (the alpha in every
		// channel), simplified like a colour ramp, with the ends held
		// flat to the centre and the radius.
		nb := len(o.alpha)
		raw := []rawStop{{offset: 0, colour: [3]float64{o.alpha[0], o.alpha[0], o.alpha[0]}}}
		for j, a := range o.alpha {
			t := o.tmin + float64((float64(j)+0.5)/float64(nb)*(o.tmax-o.tmin))
			raw = append(raw, rawStop{offset: t / o.tmax, colour: [3]float64{a, a, a}})
		}
		last := o.alpha[nb-1]
		raw = append(raw, rawStop{offset: 1, colour: [3]float64{last, last, last}})
		colour := quantise(o.colour)
		var os []Stop
		for _, s := range modelOf(raw, tol) {
			c := colour
			c.A = uint8(math.Round(math.Max(0, math.Min(1, s.colour[0])) * 255))
			os = append(os, Stop{Offset: s.offset, Colour: c})
		}
		rp.Overlays = append(rp.Overlays, Overlay{CX: o.cx, CY: o.cy, R: o.tmax, Stops: os})
	}
	return rp
}

// fitLayered fits the layered model to the pixels pix (member says
// which pixels are the region's) over the base field fld. nil when the
// base alone cannot be sampled. Overlays are added while each pays
// (layerGain); the returned rms is over every interior pixel.
func fitLayered(rgb []uint8, pix []int, member func(p int) bool, w, h int, fld field, tol float64) *layered {
	inside := func(p int) bool {
		x, y := p%w, p/w
		return (x == 0 || member(p-1)) && (x == w-1 || member(p+1)) &&
			(y == 0 || member(p-w)) && (y == h-1 || member(p+w))
	}
	l := &layered{fld: fld, tmin: math.Inf(1), tmax: math.Inf(-1)}
	for _, p := range pix {
		t := fld.at(float64(p%w)+0.5, float64(p/w)+0.5)
		l.tmin = math.Min(l.tmin, t)
		l.tmax = math.Max(l.tmax, t)
	}
	if l.tmax-l.tmin < minLength {
		return nil
	}
	var interior []int
	for _, p := range pix {
		if inside(p) {
			interior = append(interior, p)
		}
	}
	stride := max(1, (len(interior)+layerSamples-1)/layerSamples)
	var samp []pixelSample
	for i := 0; i < len(interior); i += stride {
		p := interior[i]
		x, y := float64(p%w)+0.5, float64(p/w)+0.5
		samp = append(samp, pixelSample{x: x, y: y, c: colourOf(rgb, p), t: fld.at(x, y)})
	}
	if len(samp) < 3 {
		return nil
	}
	f := &layerFit{l: l, samp: samp, tol: tol}
	f.refitBase()
	if l.base == nil {
		return nil
	}
	rms := f.rms()
	for len(l.over) < maxOverlays && rms > layerGoal*tol {
		// Every seed gets a short fit; the best few are refined and the
		// best of those kept, when it pays.
		before := f.snapshot()
		type trial struct {
			rms float64
			l   layered
		}
		var trials []trial
		for _, o := range f.seeds(rgb, w, h, member) {
			*l = before
			l.over = append(slices.Clone(before.over), o)
			f.prime(len(l.over) - 1)
			f.alternate(layerIters / 2)
			trials = append(trials, trial{rms: f.rms(), l: f.snapshot()})
		}
		slices.SortStableFunc(trials, func(a, b trial) int { return cmpFloat(a.rms, b.rms) })
		var best *layered
		bestRMS := math.Inf(1)
		for i := range min(len(trials), refineSeeds) {
			*l = trials[i].l
			f.alternate(layerIters)
			f.refineCentre(len(l.over) - 1)
			if r := f.rms(); r < bestRMS {
				bestRMS = r
				snap := f.snapshot()
				best = &snap
			}
		}
		if best == nil || bestRMS > layerGain*rms {
			*l = before
			break
		}
		*l = *best
		rms = bestRMS
	}
	// The error over every interior pixel, not the pixelSample.
	var err float64
	for _, p := range interior {
		d := dist(l.at(float64(p%w)+0.5, float64(p/w)+0.5), colourOf(rgb, p))
		err += float64(d * d)
	}
	l.rms = math.Sqrt(err / float64(len(interior)))
	return l
}

// layerFit is the working state of a fit.
type layerFit struct {
	l    *layered
	samp []pixelSample
	tol  float64
}

func (f *layerFit) rms() float64 {
	var err float64
	for _, s := range f.samp {
		d := dist(f.l.at(s.x, s.y), s.c)
		err += float64(d * d)
	}
	return math.Sqrt(err / float64(len(f.samp)))
}

// refitBase samples the base along t from what the overlays leave of
// each pixel: c = F·B + G with F the product of the overlays'
// transparencies and G their paint over black, so B = (c − G)/F,
// weighted by F² and only where F shows at least showThrough.
func (f *layerFit) refitBase() {
	l := f.l
	b := newBins(l.tmin, l.tmax)
	for _, s := range f.samp {
		F, G := 1.0, [3]float64{}
		for i := range l.over {
			a := l.over[i].at(s.x, s.y)
			G = mixColour(G, l.over[i].colour, a)
			F *= 1 - a
		}
		if F < showThrough {
			continue
		}
		var tgt [3]float64
		for k := range tgt {
			tgt[k] = math.Max(0, math.Min(1, (s.c[k]-G[k])/F))
		}
		b.addWeighted(s.t, tgt, float64(F*F))
	}
	// The overlays may hide all but one bin; the base then stays.
	if raw := b.raw(); raw != nil || l.base == nil {
		l.base = raw
	}
}

// seeds proposes centres for the next overlay from the residual: where
// its gradients point through, the centroid of its peak (the pixels in
// the top peakShare of residual magnitude), and that peak mirrored
// beyond the region's far side (a rim light's centre lies outside the
// region). The colour is seeded on the far side of the residual at the
// peak, so the first least-squares round has something to lean on.
func (f *layerFit) seeds(rgb []uint8, w, h int, member func(p int) bool) []overlay {
	l := f.l
	n := len(f.samp)
	res := make([][3]float64, n)
	mag := make([]float64, n)
	var m moments
	var sx, sy float64
	for i, s := range f.samp {
		p := int(s.y)*w + int(s.x)
		res[i] = sub(s.c, l.at(s.x, s.y))
		mag[i] = dist(res[i], [3]float64{})
		sx += s.x
		sy += s.y
		x, y := int(s.x), int(s.y)
		if x > 0 && x < w-1 && y > 0 && y < h-1 && member(p-1) && member(p+1) && member(p-w) && member(p+w) {
			gx := half(sub(colourOf(rgb, p+1), l.at(s.x+1, s.y)), sub(colourOf(rgb, p-1), l.at(s.x-1, s.y)))
			gy := half(sub(colourOf(rgb, p+w), l.at(s.x, s.y+1)), sub(colourOf(rgb, p-w), l.at(s.x, s.y-1)))
			m.add(s.x, s.y, res[i])
			m.addGrad(s.x, s.y, gx, gy)
		}
	}
	sorted := slices.Clone(mag)
	slices.Sort(sorted)
	thresh := sorted[int(float64(n-1)*(1-peakShare))]
	if thresh <= 0 {
		return nil
	}
	var px, py, wsum float64
	var colour [3]float64
	for i, s := range f.samp {
		if mag[i] < thresh {
			continue
		}
		wt := float64(mag[i] * mag[i])
		px += float64(s.x * wt)
		py += float64(s.y * wt)
		wsum += wt
		far := l.at(s.x, s.y)
		for k := range far {
			colour[k] += float64(math.Max(0, math.Min(1, far[k]+seedReach*res[i][k])) * wt)
		}
	}
	px, py = px/wsum, py/wsum
	for k := range colour {
		colour[k] /= wsum
	}
	cx, cy := sx/float64(n), sy/float64(n)
	var out []overlay
	if gx, gy, ok := m.centre(); ok {
		out = append(out, overlay{cx: gx, cy: gy, colour: colour})
	}
	out = append(out,
		overlay{cx: px, cy: py, colour: colour},
		overlay{cx: cx + mirror*(px-cx), cy: cy + mirror*(py-cy), colour: colour})
	// A ring of centres outside the region, for a rim light whose
	// residual the base ramp has mostly absorbed.
	var sxx, syy float64
	for _, s := range f.samp {
		sxx += float64((s.x - cx) * (s.x - cx))
		syy += float64((s.y - cy) * (s.y - cy))
	}
	ring := mirror * math.Sqrt((sxx+syy)/float64(n))
	for i := range ringSeeds {
		a := 2 * math.Pi * float64(i) / ringSeeds
		out = append(out, overlay{cx: cx + ring*math.Cos(a), cy: cy + ring*math.Sin(a), colour: colour})
	}
	for i := range out {
		f.span(&out[i])
	}
	return out
}

// prime gives overlay j a first profile against a flat base (the base's
// mean colour): the base ramp fitted alone has already absorbed what it
// can of the overlay's shading, and least squares from there would give
// the overlay nothing. Against a flat base the overlay must own its
// shading, and the alternating fit then hands the ramp back to the base.
func (f *layerFit) prime(j int) {
	l := f.l
	saved := l.base
	var mean [3]float64
	for _, s := range f.samp {
		for k := range mean {
			mean[k] += s.c[k]
		}
	}
	for k := range mean {
		mean[k] /= float64(len(f.samp))
	}
	l.base = []rawStop{{offset: 0, colour: mean}, {offset: 1, colour: mean}}
	f.refitOverlay(j)
	l.base = saved
}

// snapshot copies the model so a trial can be undone.
func (f *layerFit) snapshot() layered {
	saved := *f.l
	saved.over = slices.Clone(f.l.over)
	for i := range saved.over {
		saved.over[i].alpha = slices.Clone(saved.over[i].alpha)
	}
	saved.base = slices.Clone(f.l.base)
	return saved
}

// span sets an overlay's distance range over the sample.
func (f *layerFit) span(o *overlay) {
	o.tmin, o.tmax = math.Inf(1), math.Inf(-1)
	for _, s := range f.samp {
		r := math.Hypot(s.x-o.cx, s.y-o.cy)
		o.tmin = math.Min(o.tmin, r)
		o.tmax = math.Max(o.tmax, r)
	}
	if o.tmax-o.tmin < 1 {
		o.tmax = o.tmin + 1
	}
}

// alternate runs rounds of: each overlay's profile and colour by least
// squares given the rest, then the base given the overlays.
func (f *layerFit) alternate(rounds int) {
	l := f.l
	for range rounds {
		for j := range l.over {
			f.refitOverlay(j)
		}
		f.refitBase()
	}
}

// refitOverlay solves overlay j's opacity per distance bin and then its
// colour, holding the rest. With Y the composite at α_j = 0, below the
// composite under j and F the transparency above it, c − Y = α_j·F·(H_j
// − below): linear in α_j per pixel, and in H_j once α_j is known.
func (f *layerFit) refitOverlay(j int) {
	l := f.l
	o := &l.over[j]
	type term struct {
		r        float64
		d, resid [3]float64 // F·(H − below), c − Y
		below    [3]float64
		f        float64
	}
	terms := make([]term, len(f.samp))
	for i, s := range f.samp {
		below := l.baseAt(s.x, s.y)
		for k := 0; k < j; k++ {
			below = mixColour(below, l.over[k].colour, l.over[k].at(s.x, s.y))
		}
		Y, F := below, 1.0
		for k := j + 1; k < len(l.over); k++ {
			a := l.over[k].at(s.x, s.y)
			Y = mixColour(Y, l.over[k].colour, a)
			F *= 1 - a
		}
		terms[i] = term{r: math.Hypot(s.x-o.cx, s.y-o.cy), d: scale(sub(o.colour, below), F), resid: sub(s.c, Y), below: below, f: F}
	}
	// Opacity per bin: α = Σ resid·d / Σ d·d, clipped, then
	// non-increasing from the centre.
	num := make([]float64, opacityBins)
	den := make([]float64, opacityBins)
	binOf := func(r float64) int {
		i := int(float64((r-o.tmin)/(o.tmax-o.tmin)) * opacityBins)
		return max(0, min(i, opacityBins-1))
	}
	for _, t := range terms {
		i := binOf(t.r)
		num[i] += dot(t.resid, t.d)
		den[i] += dot(t.d, t.d)
	}
	o.alpha = nonIncreasing(num, den)
	// Colour: resid = g·(H − below), g = α·F, so H = Σ g·(resid +
	// g·below) / Σ g², clipped to the gamut.
	var hn [3]float64
	var hd float64
	for _, t := range terms {
		g := o.alphaAt(t.r) * t.f
		for k := range hn {
			hn[k] += float64(g * (t.resid[k] + float64(g*t.below[k])))
		}
		hd += float64(g * g)
	}
	if hd > 0 {
		for k := range hn {
			o.colour[k] = math.Max(0, math.Min(1, hn[k]/hd))
		}
	}
}

// refineCentre moves overlay j's centre by coordinate descent on the
// sample RMS: a step of a quarter of the sample's radius, halved
// whenever no direction improves, until it is under a pixel or the
// trials run out. Each move is judged after a short alternating fit.
func (f *layerFit) refineCentre(j int) {
	l := f.l
	var sx, sy, sxx, syy float64
	for _, s := range f.samp {
		sx += s.x
		sy += s.y
		sxx += float64(s.x * s.x)
		syy += float64(s.y * s.y)
	}
	n := float64(len(f.samp))
	step := math.Sqrt(math.Max((sxx+syy)/n-float64(sx/n*sx/n)-float64(sy/n*sy/n), 1)) / 4
	best := f.rms()
	trials := 0
	for step >= 1 && trials < refineTrials {
		improved := false
		for _, d := range [4][2]float64{{step, 0}, {-step, 0}, {0, step}, {0, -step}} {
			if trials >= refineTrials {
				break
			}
			trials++
			saved := f.snapshot()
			o := &l.over[j]
			o.cx += d[0]
			o.cy += d[1]
			f.span(o)
			f.alternate(layerIters / 2)
			if r := f.rms(); r < best {
				best = r
				improved = true
				break
			}
			*l = saved
		}
		if !improved {
			step /= 2
		}
	}
}

// nonIncreasing is the weighted least-squares fit of a non-increasing
// sequence to the bin values num/den with weights den (pool adjacent
// violators), clipped to 0..1; a bin without weight takes its
// neighbours' pooled value.
func nonIncreasing(num, den []float64) []float64 {
	type block struct {
		n, d float64
		cnt  int
	}
	var blocks []block
	for i := range num {
		blocks = append(blocks, block{n: num[i], d: den[i], cnt: 1})
		// A later block may not exceed an earlier one: pool while it does.
		for len(blocks) > 1 {
			a, b := blocks[len(blocks)-2], blocks[len(blocks)-1]
			if a.d > 0 && b.d > 0 && b.n/b.d <= a.n/a.d {
				break
			}
			blocks = append(blocks[:len(blocks)-2], block{n: a.n + b.n, d: a.d + b.d, cnt: a.cnt + b.cnt})
		}
	}
	out := make([]float64, 0, len(num))
	for _, b := range blocks {
		v := 0.0
		if b.d > 0 {
			v = math.Max(0, math.Min(1, b.n/b.d))
		}
		for range b.cnt {
			out = append(out, v)
		}
	}
	return out
}

func cmpFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func sub(a, b [3]float64) [3]float64 { return [3]float64{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }
func scale(a [3]float64, f float64) [3]float64 {
	return [3]float64{float64(a[0] * f), float64(a[1] * f), float64(a[2] * f)}
}
func dot(a, b [3]float64) float64 {
	return float64(a[0]*b[0]) + float64(a[1]*b[1]) + float64(a[2]*b[2])
}

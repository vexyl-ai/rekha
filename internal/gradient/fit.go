package gradient

import (
	"image/color"
	"math"
	"sort"
)

// minPixels is the smallest region that takes part in a merge; anything
// smaller is a speck or a sliver the cleanup already judged.
const minPixels = 16

// minLength is the shortest extent, in pixels along its direction, a ramp
// may have: shorter than that a gradient is invisible and the region
// stays flat.
const minLength = 16

// minSpan is the least colour change end to end (sRGB distance, channels
// 0..1) a ramp may have whatever the tolerance: about two counts per
// channel, below which two stops print the same.
const minSpan = 0.0136

// sideSlope is the share of the union's slope each side of a merge must
// carry inside itself; flats carry none and are refused.
const sideSlope = 0.5

// Stop is one colour of a ramp at an offset in 0..1.
type Stop struct {
	Offset float64
	Colour color.NRGBA
}

// Ramp is a gradient in pixel coordinates (SVG userSpaceOnUse). Linear:
// the Stops run from (X0,Y0) at offset 0 to (X1,Y1) at offset 1. Radial:
// the Stops run from the centre (CX,CY) at offset 0 to the circle of
// radius R at offset 1 (the first stop is where the region begins, which
// may be well out from the centre).
type Ramp struct {
	X0, Y0, X1, Y1 float64
	Radial         bool
	CX, CY, R      float64
	Stops          []Stop
	// Overlays, when the region is shaded more than one way, are painted
	// over the ramp in order, each as another path of the region's
	// shape filled with its gradient.
	Overlays []Overlay
}

// Overlay is a gradient of one colour whose stops' alpha fades it; the
// region shows through where it is clear. Radial by default, from the
// centre (CX,CY) out to R; Linear, from (X0,Y0) to (X1,Y1).
type Overlay struct {
	CX, CY, R      float64
	Linear         bool
	X0, Y0, X1, Y1 float64
	Stops          []Stop
}

// field is a one-dimensional parametrisation of the plane: t = u·p for a
// linear ramp, t = |p − c| for a radial one. A ramp's colour is a
// function of t.
type field struct {
	radial bool
	u      [2]float64
	cx, cy float64
}

func (f field) at(x, y float64) float64 {
	if f.radial {
		return math.Hypot(x-f.cx, y-f.cy)
	}
	return float64(f.u[0]*x) + float64(f.u[1]*y)
}

// moments are the sums a least-squares plane per channel needs, over
// pixel centres, and the structure tensor of the colour gradient (the
// sum of ∇c·∇cᵀ over channels and pixels), whose principal axis is the
// direction the colour changes along whichever way it changes. With the
// tensor go the sums of k·∇c, k = x·∂c/∂y − y·∂c/∂x, from which the point
// every gradient points through, if there is one, is solved (centre).
// They add under a merge, so a union is never re-scanned.
type moments struct {
	n                     int
	sx, sy, sxx, sxy, syy float64
	sc, scx, scy, scc     [3]float64
	ng                    int // gradient samples (per channel)
	gxx, gxy, gyy         float64
	kgx, kgy              float64
}

func (m *moments) add(x, y float64, c [3]float64) {
	m.n++
	m.sx += x
	m.sy += y
	m.sxx += float64(x * x)
	m.sxy += float64(x * y)
	m.syy += float64(y * y)
	for i, v := range c {
		m.sc[i] += v
		m.scx[i] += float64(v * x)
		m.scy[i] += float64(v * y)
		m.scc[i] += float64(v * v)
	}
}

// addGrad accumulates the colour gradient (per channel, in colour units
// per pixel) at pixel centre (x, y) into the structure tensor and the
// centre sums.
func (m *moments) addGrad(x, y float64, gx, gy [3]float64) {
	for i := range gx {
		m.ng++
		m.gxx += float64(gx[i] * gx[i])
		m.gxy += float64(gx[i] * gy[i])
		m.gyy += float64(gy[i] * gy[i])
		k := float64(x*gy[i]) - float64(y*gx[i])
		m.kgx += float64(k * gx[i])
		m.kgy += float64(k * gy[i])
	}
}

// gslope is the RMS colour change per pixel over the gradient samples,
// whichever way each runs: a ring's slope, where a fit along one
// direction sees none.
func (m *moments) gslope() float64 {
	if m.ng == 0 {
		return 0
	}
	return math.Sqrt((m.gxx + m.gyy) / float64(m.ng))
}

// centreRatio bounds how far from the pixels' centroid a centre may lie,
// in multiples of their RMS radius: farther, the iso-lines are straight
// to the eye and the gradient is a linear one that happens not to fit.
const centreRatio = 8

// centre solves the point the colour gradients point through, by least
// squares on (p − c) × ∇c = 0 over the gradient samples. ok is false
// when the gradients are parallel (a linear ramp: the system is
// singular) or the point lies beyond centreRatio.
func (m *moments) centre() (cx, cy float64, ok bool) {
	if m.ng == 0 || m.n == 0 {
		return 0, 0, false
	}
	// Normal equations of Σ (k − cx·gy + cy·gx)²:
	//   cx·Σgy² − cy·Σgx·gy = Σk·gy
	//  −cx·Σgx·gy + cy·Σgx² = −Σk·gx
	a11, a12, a22 := m.gyy, -m.gxy, m.gxx
	b1, b2 := m.kgy, -m.kgx
	det := float64(a11*a22) - float64(a12*a12)
	if det <= 1e-6*float64(a11*a22) {
		return 0, 0, false
	}
	cx = (float64(b1*a22) - float64(b2*a12)) / det
	cy = (float64(a11*b2) - float64(a12*b1)) / det
	n := float64(m.n)
	mx, my := m.sx/n, m.sy/n
	rr := (m.sxx+m.syy)/n - float64(mx*mx) - float64(my*my)
	if float64((cx-mx)*(cx-mx))+float64((cy-my)*(cy-my)) > float64(centreRatio*centreRatio)*rr {
		return 0, 0, false
	}
	return cx, cy, true
}

func (m *moments) merge(o *moments) {
	m.n += o.n
	m.ng += o.ng
	m.gxx += o.gxx
	m.gxy += o.gxy
	m.gyy += o.gyy
	m.kgx += o.kgx
	m.kgy += o.kgy
	m.sx += o.sx
	m.sy += o.sy
	m.sxx += o.sxx
	m.sxy += o.sxy
	m.syy += o.syy
	for i := range m.sc {
		m.sc[i] += o.sc[i]
		m.scx[i] += o.scx[i]
		m.scy[i] += o.scy[i]
		m.scc[i] += o.scc[i]
	}
}

// spread is the RMS distance of the colours from their mean.
func (m *moments) spread() float64 {
	n := float64(m.n)
	var v float64
	for i := range m.sc {
		v += m.scc[i] - float64(m.sc[i]*m.sc[i])/n
	}
	return math.Sqrt(math.Max(v, 0) / n)
}

// mean is the average colour.
func (m *moments) mean() [3]float64 {
	n := float64(m.n)
	return [3]float64{m.sc[0] / n, m.sc[1] / n, m.sc[2] / n}
}

// fitted is a ramp model: colour = a + b·t along t = u·p, with the RMS
// residual of the pixels against it and the colour change per pixel.
type fitted struct {
	ok         bool
	u          [2]float64
	a, b       [3]float64
	rms, slope float64
	extent     float64 // pixels the fit spans along u (a uniform strip's width from the spread of t)
}

// fit chooses the direction u as the principal direction of the
// per-channel least-squares planes and fits colour against t = u·p in
// one dimension. ok is false when the pixels do not span a plane (fewer
// than three, or collinear).
func (m *moments) fit() fitted {
	if m.n < 3 {
		return fitted{}
	}
	n := float64(m.n)
	cxx := m.sxx - float64(m.sx*m.sx)/n
	cyy := m.syy - float64(m.sy*m.sy)/n
	cxy := m.sxy - float64(m.sx*m.sy)/n
	det := float64(cxx*cyy) - float64(cxy*cxy)
	if det <= 1e-9*float64(cxx*cyy) {
		return fitted{}
	}
	// Gradient of each channel's plane, and the 2×2 scatter of those.
	var g11, g12, g22 float64
	for i := range m.sc {
		ccx := m.scx[i] - float64(m.sx*m.sc[i])/n
		ccy := m.scy[i] - float64(m.sy*m.sc[i])/n
		b := (float64(ccx*cyy) - float64(ccy*cxy)) / det
		d := (float64(ccy*cxx) - float64(ccx*cxy)) / det
		g11 += float64(b * b)
		g12 += float64(b * d)
		g22 += float64(d * d)
	}
	return m.along(principal(g11, g12, g22))
}

// fitWithin is fit, except that when the planes' direction leaves a
// residual above tol (the colour bends, or rises then falls) and
// gradients were accumulated, the direction is the principal axis of
// the structure tensor instead, which ignores the sign of the change.
// The planes keep precedence while they fit because on a steep grainy
// ramp the tensor sees only the pixels whose grain cancels the slope
// and tilts.
func (m *moments) fitWithin(tol float64) fitted {
	f := m.fit()
	if f.ok && f.rms <= tol || m.gxx+m.gyy == 0 {
		return f
	}
	if t := m.along(principal(m.gxx, m.gxy, m.gyy)); t.ok {
		return t
	}
	return f
}

// along fits colour against t = u·p in one dimension for a given unit
// direction u. ok is false with fewer than three pixels or when the
// pixels do not spread along u.
func (m *moments) along(u [2]float64) fitted {
	var f fitted
	if m.n < 3 {
		return f
	}
	n := float64(m.n)
	ux, uy := u[0], u[1]
	st := float64(ux*m.sx) + float64(uy*m.sy)
	stt := float64(ux*ux*m.sxx) + 2*float64(ux*uy*m.sxy) + float64(uy*uy*m.syy)
	ctt := stt - float64(st*st)/n
	if ctt <= 1e-12*n {
		return f
	}
	f.ok = true
	f.u = u
	var rss, slope2 float64
	for i := range m.sc {
		sct := float64(ux*m.scx[i]) + float64(uy*m.scy[i])
		cct := sct - float64(st*m.sc[i])/n
		b := cct / ctt
		a := (m.sc[i] - float64(b*st)) / n
		f.a[i], f.b[i] = a, b
		// Residual about the mean, less what the slope explains: the
		// centred form loses far less to cancellation than Σc² − βᵀXᵀy.
		rss += m.scc[i] - float64(m.sc[i]*m.sc[i])/n - float64(b*cct)
		slope2 += float64(b * b)
	}
	f.rms = math.Sqrt(math.Max(rss, 0) / n)
	f.slope = math.Sqrt(slope2)
	f.extent = 2 * math.Sqrt(3*ctt/n)
	return f
}

// principal is the unit eigenvector of the larger eigenvalue of the
// symmetric matrix [[p q][q r]], oriented with a positive x (or, when x
// is zero, a positive y). A zero matrix gives (1,0).
func principal(p, q, r float64) [2]float64 {
	half := (p - r) / 2
	lambda := (p+r)/2 + math.Sqrt(float64(half*half)+float64(q*q))
	var x, y float64
	switch {
	case lambda <= 0:
		return [2]float64{1, 0}
	case q != 0:
		x, y = lambda-r, q
	case p >= r:
		x, y = 1, 0
	default:
		x, y = 0, 1
	}
	l := math.Sqrt(float64(x*x) + float64(y*y))
	x, y = x/l, y/l
	if x < 0 || (x == 0 && y < 0) {
		x, y = -x, -y
	}
	return [2]float64{x, y}
}

// binWidth is the length along u, in pixels, each colour sample of a ramp
// covers; maxBins caps the samples on a long ramp.
const (
	binWidth = 8
	maxBins  = 64
)

// bins collect the pixels of a ramp group by their projection t on its
// direction, between the group's tmin and tmax, so the colour along the
// ramp can be sampled.
type bins struct {
	tmin, tmax float64
	nb         int
	st         []float64    // Σt·weight per bin
	sc         [][3]float64 // Σcolour·weight per bin
	wt         []float64    // Σweight per bin
}

func newBins(tmin, tmax float64) *bins {
	nb := int(math.Ceil((tmax - tmin) / binWidth))
	nb = max(1, min(nb, maxBins))
	return &bins{tmin: tmin, tmax: tmax, nb: nb, st: make([]float64, nb), sc: make([][3]float64, nb), wt: make([]float64, nb)}
}

func (b *bins) add(t float64, c [3]float64) {
	b.addWeighted(t, c, 1)
}

// addWeighted adds a sample with weight w (a layered fit weighs a
// pixel by how much of its colour the base shows through).
func (b *bins) addWeighted(t float64, c [3]float64, w float64) {
	if w <= 0 {
		return
	}
	i := int(float64((t-b.tmin)/(b.tmax-b.tmin)) * float64(b.nb))
	i = max(0, min(i, b.nb-1))
	b.st[i] += float64(t * w)
	b.wt[i] += w
	for k, v := range c {
		b.sc[i][k] += float64(v * w)
	}
}

// rawStop is a colour sample along a ramp before quantisation.
type rawStop struct {
	offset float64
	colour [3]float64
}

// raw samples the ramp: one stop per non-empty bin, at the bin's mean
// projection (so an exactly linear ramp samples exactly on its line),
// and one at each end by linear extrapolation of the two nearest
// samples, clamped to 0..1. Fewer than two non-empty bins give nil.
func (b *bins) raw() []rawStop {
	span := b.tmax - b.tmin
	var out []rawStop
	for i := range b.wt {
		if b.wt[i] == 0 {
			continue
		}
		n := b.wt[i]
		s := rawStop{offset: (b.st[i]/n - b.tmin) / span}
		for k := range s.colour {
			s.colour[k] = b.sc[i][k] / n
		}
		out = append(out, s)
	}
	if len(out) < 2 {
		return nil
	}
	ends := func(p, q rawStop, at float64) rawStop {
		s := rawStop{offset: at}
		f := (at - p.offset) / (q.offset - p.offset)
		for k := range s.colour {
			v := p.colour[k] + float64(f*(q.colour[k]-p.colour[k]))
			s.colour[k] = math.Max(0, math.Min(1, v))
		}
		return s
	}
	first := ends(out[0], out[1], 0)
	last := ends(out[len(out)-2], out[len(out)-1], 1)
	return append(append([]rawStop{first}, out...), last)
}

// model simplifies the raw samples by Douglas-Peucker in colour at
// tol/2: a sample within tol/2 of the straight colour line between its
// kept neighbours is dropped. nil when the ramp cannot be sampled.
func (b *bins) model(tol float64) []rawStop {
	return modelOf(b.raw(), tol)
}

// modelOf simplifies raw samples by Douglas-Peucker in colour at tol/2.
func modelOf(raw []rawStop, tol float64) []rawStop {
	if raw == nil {
		return nil
	}
	keep := make([]bool, len(raw))
	keep[0], keep[len(raw)-1] = true, true
	simplify(raw, keep, 0, len(raw)-1, tol/2)
	out := raw[:0:0]
	for i, s := range raw {
		if keep[i] {
			out = append(out, s)
		}
	}
	return out
}

// stops quantises the model. span is the largest colour distance
// between any two bin samples, the extrapolated ends left out: a pocket
// of another colour in an end bin is amplified by the extrapolation and
// must not count as colour change.
func (b *bins) stops(tol float64) (stops []Stop, span float64) {
	raw := b.raw()
	if raw == nil {
		return nil, 0
	}
	for i := 1; i < len(raw)-1; i++ {
		for j := i + 1; j < len(raw)-1; j++ {
			span = math.Max(span, cdist(raw[i].colour, raw[j].colour))
		}
	}
	for _, s := range b.model(tol) {
		stops = append(stops, Stop{Offset: s.offset, Colour: quantise(s.colour)})
	}
	return stops, span
}

// colourAt interpolates a model at offset o in 0..1.
func colourAt(model []rawStop, o float64) [3]float64 {
	i := sort.Search(len(model), func(i int) bool { return model[i].offset >= o })
	if i == 0 {
		return model[0].colour
	}
	if i == len(model) {
		return model[len(model)-1].colour
	}
	p, q := model[i-1], model[i]
	f := (o - p.offset) / (q.offset - p.offset)
	var c [3]float64
	for k := range c {
		c[k] = p.colour[k] + float64(f*(q.colour[k]-p.colour[k]))
	}
	return c
}

func cdist(a, b [3]float64) float64 {
	var d2 float64
	for k := range a {
		e := a[k] - b[k]
		d2 += float64(e * e)
	}
	return math.Sqrt(d2)
}

// simplify marks, between kept samples lo and hi, the sample farthest in
// colour from the line through them when it is farther than eps, and
// recurses on both sides of it.
func simplify(raw []rawStop, keep []bool, lo, hi int, eps float64) {
	if hi-lo < 2 {
		return
	}
	p, q := raw[lo], raw[hi]
	best, far := -1, eps
	for i := lo + 1; i < hi; i++ {
		f := (raw[i].offset - p.offset) / (q.offset - p.offset)
		var d2 float64
		for k := range p.colour {
			e := raw[i].colour[k] - p.colour[k] - float64(f*(q.colour[k]-p.colour[k]))
			d2 += float64(e * e)
		}
		if d := math.Sqrt(d2); d > far {
			best, far = i, d
		}
	}
	if best < 0 {
		return
	}
	keep[best] = true
	simplify(raw, keep, lo, best, eps)
	simplify(raw, keep, best, hi, eps)
}

func quantise(c [3]float64) color.NRGBA {
	ch := func(v float64) uint8 { return uint8(math.Round(math.Max(0, math.Min(1, v)) * 255)) }
	return color.NRGBA{R: ch(c[0]), G: ch(c[1]), B: ch(c[2]), A: 255}
}

// rampFrom builds the ramp of a group from its moments (for the
// centroid), its field and its bins (for the stops). Linear: the line
// runs from projection tmin to tmax on u through the centroid. Radial:
// the circle is the centre and tmax, and the stops' offsets are their
// distances over tmax. nil when the stops cannot be sampled, or the
// sampled colour changes less than max(tol, minSpan) along the ramp.
func rampFrom(m *moments, f field, b *bins, tol float64) *Ramp {
	stops, span := b.stops(tol)
	if stops == nil || span < math.Max(tol, minSpan) {
		return nil
	}
	if f.radial {
		for i := range stops {
			stops[i].Offset = (b.tmin + float64(stops[i].Offset*(b.tmax-b.tmin))) / b.tmax
		}
		return &Ramp{Radial: true, CX: f.cx, CY: f.cy, R: b.tmax, Stops: stops}
	}
	n := float64(m.n)
	cx, cy := m.sx/n, m.sy/n
	tc := f.at(cx, cy)
	return &Ramp{
		X0: cx + float64((b.tmin-tc)*f.u[0]), Y0: cy + float64((b.tmin-tc)*f.u[1]),
		X1: cx + float64((b.tmax-tc)*f.u[0]), Y1: cy + float64((b.tmax-tc)*f.u[1]),
		Stops: stops,
	}
}

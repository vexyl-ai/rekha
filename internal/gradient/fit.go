package gradient

import (
	"image/color"
	"math"
)

// minPixels is the smallest region that takes part in a merge; anything
// smaller is a speck or a sliver the cleanup already judged.
const minPixels = 16

// Stop is one colour of a ramp at an offset in 0..1.
type Stop struct {
	Offset float64
	Colour color.NRGBA
}

// Ramp is a linear gradient in pixel coordinates (SVG userSpaceOnUse):
// colour Stops[0] at (X0,Y0) to Stops[1] at (X1,Y1).
type Ramp struct {
	X0, Y0, X1, Y1 float64
	Stops          []Stop
}

// moments are the sums a least-squares plane per channel needs, over
// pixel centres. They add under a merge, so a union is never re-scanned.
type moments struct {
	n                     int
	sx, sy, sxx, sxy, syy float64
	sc, scx, scy, scc     [3]float64
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

func (m *moments) merge(o *moments) {
	m.n += o.n
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
}

// fit chooses the direction u from the per-channel least-squares planes
// (the principal direction of their gradients), then fits colour against
// t = u·p in one dimension. ok is false when the pixels do not span a
// plane (fewer than three, or collinear).
func (m *moments) fit() fitted {
	var f fitted
	if m.n < 3 {
		return f
	}
	n := float64(m.n)
	cxx := m.sxx - float64(m.sx*m.sx)/n
	cyy := m.syy - float64(m.sy*m.sy)/n
	cxy := m.sxy - float64(m.sx*m.sy)/n
	det := float64(cxx*cyy) - float64(cxy*cxy)
	if det <= 1e-9*float64(cxx*cyy) {
		return f
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
	f.ok = true
	f.u = principal(g11, g12, g22)
	ux, uy := f.u[0], f.u[1]
	st := float64(ux*m.sx) + float64(uy*m.sy)
	stt := float64(ux*ux*m.sxx) + 2*float64(ux*uy*m.sxy) + float64(uy*uy*m.syy)
	ctt := stt - float64(st*st)/n
	var rss, slope2 float64
	for i := range m.sc {
		sct := float64(ux*m.scx[i]) + float64(uy*m.scy[i])
		cct := sct - float64(st*m.sc[i])/n
		b := 0.0
		if ctt > 1e-12*n {
			b = cct / ctt
		}
		a := (m.sc[i] - float64(b*st)) / n
		f.a[i], f.b[i] = a, b
		// Residual about the mean, less what the slope explains: the
		// centred form loses far less to cancellation than Σc² − βᵀXᵀy.
		rss += m.scc[i] - float64(m.sc[i]*m.sc[i])/n - float64(b*cct)
		slope2 += float64(b * b)
	}
	f.rms = math.Sqrt(math.Max(rss, 0) / n)
	f.slope = math.Sqrt(slope2)
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

// ramp builds the two-stop ramp of f between projections tmin and tmax
// on u, through the pixels' centroid.
func ramp(m *moments, f fitted, tmin, tmax float64) *Ramp {
	n := float64(m.n)
	cx, cy := m.sx/n, m.sy/n
	tc := float64(f.u[0]*cx) + float64(f.u[1]*cy)
	at := func(t float64) color.NRGBA {
		ch := func(i int) uint8 {
			v := f.a[i] + float64(f.b[i]*t)
			return uint8(math.Round(math.Max(0, math.Min(1, v)) * 255))
		}
		return color.NRGBA{R: ch(0), G: ch(1), B: ch(2), A: 255}
	}
	return &Ramp{
		X0: cx + float64((tmin-tc)*f.u[0]), Y0: cy + float64((tmin-tc)*f.u[1]),
		X1: cx + float64((tmax-tc)*f.u[0]), Y1: cy + float64((tmax-tc)*f.u[1]),
		Stops: []Stop{{Offset: 0, Colour: at(tmin)}, {Offset: 1, Colour: at(tmax)}},
	}
}

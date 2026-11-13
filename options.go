package rekha

import (
	"fmt"
	"image/color"
	"math"
	"sync/atomic"

	"github.com/vexyl-ai/rekha/internal/contour"
	"github.com/vexyl-ai/rekha/internal/refine"
)

// Mode selects the output geometry.
type Mode string

// Output modes.
const (
	ModePixel   Mode = "pixel"
	ModePolygon Mode = "polygon"
	ModeSpline  Mode = "spline"
)

// Preset selects a named bundle of defaults.
type Preset string

// Presets.
const (
	PresetBW     Preset = "bw"
	PresetPoster Preset = "poster"
	PresetPhoto  Preset = "photo"
)

// Hierarchical selects how colour shapes are layered in the output.
type Hierarchical string

// Layering modes.
const (
	// HierarchicalStacked paints every shape's outer contour, larger and
	// deeper shapes first, so each one covers what it encloses: no holes,
	// no seams between abutting colours.
	HierarchicalStacked Hierarchical = "stacked"
	// HierarchicalCutout paints each shape with its holes cut out, as
	// Phase 1 does for black on white.
	HierarchicalCutout Hierarchical = "cutout"
)

// TurnPolicy decides how a boundary walk resolves a 2×2 checkerboard.
type TurnPolicy = contour.TurnPolicy

// Turn policies.
const (
	TurnMinority = contour.Minority
	TurnMajority = contour.Majority
	TurnBlack    = contour.Black
	TurnWhite    = contour.White
)

// Options controls a trace. One struct backs the CLI, the HTTP API and the
// WebAssembly entry point; Validate is the single source of truth for ranges.
type Options struct {
	Preset         Preset
	Mode           Mode
	FilterSpeckle  int // pixels; 0..1024
	ColorPrecision int // bits; 1..8
	PathPrecision  int // decimals; 0..8
	MaxColors      int // palette cap; 0 = as many as ColorTolerance needs, up to 256

	Threshold         float64    // 0..1; pixels darker than this are foreground
	AutoThreshold     bool       // choose Threshold with Otsu's method
	TurnPolicy        TurnPolicy // checkerboard resolution
	SimplifyTolerance float64    // px; 0..10
	CornerThreshold   float64    // degrees; 0..180
	FitTolerance      float64    // px; 0.01..10

	ColorTolerance float64       // OKLab distance 0..1 at which colours merge (white to black is 1)
	Palette        []color.NRGBA // fixed palette, at most 256 distinct entries; empty = quantise
	Hierarchical   Hierarchical
	// ExternalMasks are regions the caller has already decided on (a
	// segmentation model's objects, a matte, a selection) that replace
	// the quantiser's verdict where they cover the image. See Mask.
	ExternalMasks []Mask

	Refine RefineOptions // spline mode only
	Stroke float64       // px, 0..4, default 0; polygon/spline colour shapes are stroked in their fill to hide hairline seams between fitted edges when viewed enlarged
	// Deprecated: use Stroke. CutoutStroke is read when Stroke is 0.
	CutoutStroke float64

	Gradients         bool    // colour presets: merge adjacent bands that form one linear ramp into a gradient fill
	GradientTolerance float64 // 0..1, default 0.03; RMS sRGB distance (channels 0..1) a ramp may leave

	// NoMark leaves out the generator mark: by default the SVG root
	// carries data-generator="rekha <version>" and a comment naming
	// Rekha and its repository. Neither changes how the image renders.
	NoMark bool

	// Format is the file format Write produces: SVG (the zero value),
	// PDF or EPS. WriteSVG, WritePDF and WriteEPS ignore it.
	Format Format

	// Scale says the image is this many times the size the output is
	// for: the SVG is written at image size ÷ Scale with the viewBox at
	// image size, and a PDF or EPS page is sized the same way. Zero
	// means 1; otherwise 0.25 to 8. A caller that upscales an image
	// before tracing (to keep thin lines) sets the factor here.
	Scale float64

	// Auto makes Trace and TraceReader race the candidate option sets
	// of Auto and keep the one closest to the image, ignoring every
	// field of these options but the output ones Auto lists. Segment
	// does not read it.
	Auto bool

	// Progress, when set, is told how far Segment and Finish have got:
	// stage names quantise, label, ramps (Segment; no count) and
	// outlines, nest, edges, refine, curves (Finish; done of total, total
	// 0 when a stage has no count). Under parallel work it may be called from several
	// goroutines at once and counts may arrive out of order. It must be
	// quick: a stage reports at most once per whole percent.
	Progress Progress
}

// RefineOptions controls the Phase 3 refinement of spline output: each
// fitted contour is optimised against the input's anti-aliasing (Yang et
// al. 2016), its segments merged and split, and circles recognised.
type RefineOptions struct {
	Enabled    bool
	Sweeps     int            // 1..10; passes over the contour's pieces
	MaxIter    int            // 1..200; L-BFGS iterations per piece
	Weights    refine.Weights // prior weights, finite and non-negative
	MergeSplit bool           // merge smooth joints, split bad fits
	Primitives bool           // replace near-circles by exact circles
}

// DefaultOptions returns the bw/spline defaults, PresetOptions(PresetBW).
func DefaultOptions() Options { return PresetOptions(PresetBW) }

// PresetOptions returns the defaults for preset p. An unknown preset
// returns the bw defaults with Preset set to p, which Validate rejects.
func PresetOptions(p Preset) Options {
	o := Options{
		Preset:         p,
		Mode:           ModeSpline,
		FilterSpeckle:  4,
		ColorPrecision: 6,
		PathPrecision:  2,
		MaxColors:      2,

		Threshold:         0.5,
		TurnPolicy:        TurnMinority,
		SimplifyTolerance: 0.75,
		CornerThreshold:   60,
		FitTolerance:      1.0,

		Hierarchical: HierarchicalCutout,

		GradientTolerance: 0.03,

		Refine: RefineOptions{Sweeps: 3, MaxIter: 30, Weights: refine.DefaultWeights, MergeSplit: true, Primitives: true},
	}
	switch p {
	case PresetPoster:
		o.MaxColors = 0
		o.ColorTolerance = 0.08
		o.Hierarchical = HierarchicalStacked
	case PresetPhoto:
		o.MaxColors = 0
		o.ColorTolerance = 0.04
		o.FilterSpeckle = 16
		o.FitTolerance = 1.5
		o.Hierarchical = HierarchicalStacked
	}
	return o
}

// Mask is one region supplied through Options.ExternalMasks. Coverage
// holds one value per pixel of the image, row-major: 0 outside, 255
// inside, between on a soft edge. A pixel covered by half or more takes
// the mask's colour; a softer edge is kept as the pixel's second colour
// and blend, which the refinement reads as coverage. Fill is the region's
// colour; the zero value takes the mean of the pixels it covers. Cut
// drops the region instead: its pixels become the page and are not
// drawn. Masks apply in order, so a later one wins where two overlap.
type Mask struct {
	Coverage []uint8
	Fill     color.NRGBA
	Cut      bool
}

// ValidationError reports the first invalid field.
type ValidationError struct {
	Field string
	Msg   string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Msg }

// Validate checks every field in struct order and returns the first failure
// as a *ValidationError, or nil.
// Progress receives Options.Progress reports.
type Progress func(stage string, done, total int)

// report sends a stage with no count.
func (o Options) report(stage string) {
	if o.Progress != nil {
		o.Progress(stage, 0, 0)
	}
}

// counter returns a function to call after each of total items of
// stage; it reports every whole percent and the end, and nothing when
// no Progress is set.
func (o Options) counter(stage string, total int) func() {
	if o.Progress == nil || total == 0 {
		return func() {}
	}
	var done atomic.Int64
	o.Progress(stage, 0, total)
	return func() {
		d := done.Add(1)
		if d == int64(total) || d*100/int64(total) != (d-1)*100/int64(total) {
			o.Progress(stage, int(d), total)
		}
	}
}

func (o Options) Validate() error {
	switch o.Preset {
	case PresetBW, PresetPoster, PresetPhoto:
	default:
		return &ValidationError{"Preset", fmt.Sprintf("unknown value %q", string(o.Preset))}
	}
	switch o.Mode {
	case ModePixel, ModePolygon, ModeSpline:
	default:
		return &ValidationError{"Mode", fmt.Sprintf("unknown value %q", string(o.Mode))}
	}
	if _, err := ParseFormat(string(o.Format)); err != nil {
		return err
	}
	if err := inRange("FilterSpeckle", o.FilterSpeckle, 0, 1024); err != nil {
		return err
	}
	if err := inRange("ColorPrecision", o.ColorPrecision, 1, 8); err != nil {
		return err
	}
	if err := inRange("PathPrecision", o.PathPrecision, 0, 8); err != nil {
		return err
	}
	if err := inRange("MaxColors", o.MaxColors, 0, 256); err != nil {
		return err
	}
	if err := inRangeF("Threshold", o.Threshold, 0, 1); err != nil {
		return err
	}
	switch o.TurnPolicy {
	case TurnMinority, TurnMajority, TurnBlack, TurnWhite:
	default:
		return &ValidationError{"TurnPolicy", fmt.Sprintf("unknown value %q", string(o.TurnPolicy))}
	}
	if err := inRangeF("SimplifyTolerance", o.SimplifyTolerance, 0, 10); err != nil {
		return err
	}
	if err := inRangeF("CornerThreshold", o.CornerThreshold, 0, 180); err != nil {
		return err
	}
	if err := inRangeF("FitTolerance", o.FitTolerance, 0.01, 10); err != nil {
		return err
	}
	if err := inRangeF("ColorTolerance", o.ColorTolerance, 0, 1); err != nil {
		return err
	}
	if len(o.Palette) > 256 {
		return &ValidationError{"Palette", fmt.Sprintf("%d entries, at most 256", len(o.Palette))}
	}
	for i, c := range o.Palette {
		for _, d := range o.Palette[:i] {
			if c == d {
				return &ValidationError{"Palette", fmt.Sprintf("duplicate entry #%02x%02x%02x", c.R, c.G, c.B)}
			}
		}
	}
	switch o.Hierarchical {
	case HierarchicalStacked, HierarchicalCutout:
	default:
		return &ValidationError{"Hierarchical", fmt.Sprintf("unknown value %q", string(o.Hierarchical))}
	}
	if o.Refine.Enabled {
		if err := o.Refine.validate(); err != nil {
			return err
		}
	}
	if err := inRangeF("Stroke", o.Stroke, 0, 4); err != nil {
		return err
	}
	if err := inRangeF("CutoutStroke", o.CutoutStroke, 0, 4); err != nil {
		return err
	}
	if err := inRangeF("GradientTolerance", o.GradientTolerance, 0, 1); err != nil {
		return err
	}
	if o.Scale != 0 {
		if err := inRangeF("Scale", o.Scale, 0.25, 8); err != nil {
			return err
		}
	}
	return nil
}

// validate checks the refinement ranges; only consulted when enabled, so
// a caller that never touches refinement keeps a valid zero value.
func (r RefineOptions) validate() error {
	if r.Sweeps < 1 || r.Sweeps > 10 {
		return &ValidationError{"Refine", fmt.Sprintf("Sweeps %d out of range 1..10", r.Sweeps)}
	}
	if r.MaxIter < 1 || r.MaxIter > 200 {
		return &ValidationError{"Refine", fmt.Sprintf("MaxIter %d out of range 1..200", r.MaxIter)}
	}
	for _, w := range []struct {
		name string
		v    float64
	}{{"Apt", r.Weights.Apt}, {"Hpt", r.Weights.Hpt}, {"Lpt", r.Weights.Lpt}} {
		if math.IsNaN(w.v) || math.IsInf(w.v, 0) || w.v < 0 {
			return &ValidationError{"Refine", fmt.Sprintf("weight %s %g must be finite and non-negative", w.name, w.v)}
		}
	}
	return nil
}

func inRangeF(field string, v, lo, hi float64) error {
	if v < lo || v > hi || v != v {
		return &ValidationError{field, fmt.Sprintf("%g out of range %g..%g", v, lo, hi)}
	}
	return nil
}

func inRange(field string, v, lo, hi int) error {
	if v < lo || v > hi {
		return &ValidationError{field, fmt.Sprintf("%d out of range %d..%d", v, lo, hi)}
	}
	return nil
}

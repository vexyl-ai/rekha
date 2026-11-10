package rekha

import (
	"fmt"
	"image/color"
	"math"

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

	Refine RefineOptions // spline mode only
	Stroke float64       // px, 0..4, default 0; polygon/spline colour shapes are stroked in their fill to hide hairline seams between fitted edges when viewed enlarged

	Gradients         bool    // colour presets: merge adjacent bands that form one linear ramp into a gradient fill
	GradientTolerance float64 // 0..1, default 0.03; RMS sRGB distance (channels 0..1) a ramp may leave
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

// ValidationError reports the first invalid field.
type ValidationError struct {
	Field string
	Msg   string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Msg }

// Validate checks every field in struct order and returns the first failure
// as a *ValidationError, or nil.
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
	if err := inRangeF("GradientTolerance", o.GradientTolerance, 0, 1); err != nil {
		return err
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

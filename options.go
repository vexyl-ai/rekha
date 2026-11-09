package rekha

import "fmt"

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

// Options controls a trace. One struct backs the CLI, the HTTP API and the
// WebAssembly entry point; Validate is the single source of truth for ranges.
type Options struct {
	Preset         Preset
	Mode           Mode
	FilterSpeckle  int // pixels; 0..1024
	ColorPrecision int // bits; 1..8
	PathPrecision  int // decimals; 0..8
	MaxColors      int // 0 = unlimited (bw forces 2); 0..256
}

// DefaultOptions returns the bw/spline defaults.
func DefaultOptions() Options {
	return Options{
		Preset:         PresetBW,
		Mode:           ModeSpline,
		FilterSpeckle:  4,
		ColorPrecision: 6,
		PathPrecision:  2,
		MaxColors:      0,
	}
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
	return nil
}

func inRange(field string, v, lo, hi int) error {
	if v < lo || v > hi {
		return &ValidationError{field, fmt.Sprintf("%d out of range %d..%d", v, lo, hi)}
	}
	return nil
}

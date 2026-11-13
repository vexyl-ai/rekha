// Package apiopts turns the option fields of the WebAssembly and HTTP
// entry points into rekha.Options: a JSON object (wasm) or string fields
// (HTTP form or query), seeded by the preset exactly as the CLI does.
package apiopts

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"github.com/vexyl-ai/rekha"
)

// Params is the JSON shape shared by the wasm and HTTP entry points. Every
// field is optional; the preset seeds the rest exactly as the CLI.
type Params struct {
	Auto              *bool      `json:"auto"` // race the candidates of rekha.Auto, ignoring the tracing fields
	Preset            *string    `json:"preset"`
	Mode              *string    `json:"mode"`
	FilterSpeckle     *int       `json:"filterSpeckle"`
	ColorPrecision    *int       `json:"colorPrecision"`
	PathPrecision     *int       `json:"pathPrecision"`
	MaxColors         *int       `json:"maxColors"`
	Threshold         *Threshold `json:"threshold"` // number or "auto"
	TurnPolicy        *string    `json:"turnPolicy"`
	Simplify          *float64   `json:"simplify"`
	CornerThreshold   *float64   `json:"cornerThreshold"`
	FitTolerance      *float64   `json:"fitTolerance"`
	ColorTolerance    *float64   `json:"colorTolerance"`
	Palette           *Palette   `json:"palette"` // "#rrggbb,#rrggbb" or ["#rrggbb", …]
	Hierarchical      *string    `json:"hierarchical"`
	Refine            *bool      `json:"refine"`
	RefinePasses      *int       `json:"refinePasses"`
	CutoutStroke      *float64   `json:"cutoutStroke"` // old name for stroke
	Stroke            *float64   `json:"stroke"`
	Gradients         *bool      `json:"gradients"`
	GradientTolerance *float64   `json:"gradientTolerance"`
	NoMark            *bool      `json:"noMark"` // leave out the data-generator mark and comment
	Format            *string    `json:"format"` // svg (default), pdf, eps
}

// Threshold is a foreground threshold: a number in 0..1 or "auto" (Otsu).
type Threshold struct {
	Auto  bool
	Value float64
}

// UnmarshalJSON accepts a JSON number or the string "auto".
func (t *Threshold) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		if s != "auto" {
			return fmt.Errorf("threshold: %q is not a number or auto", s)
		}
		*t = Threshold{Auto: true}
		return nil
	}
	var v float64
	if err := json.Unmarshal(b, &v); err != nil {
		return fmt.Errorf("threshold: %s is not a number or auto", string(b))
	}
	*t = Threshold{Value: v}
	return nil
}

// Palette is a fixed palette given as "#rrggbb,#rrggbb" or a JSON array
// of "#rrggbb" strings.
type Palette []color.NRGBA

// UnmarshalJSON accepts either form.
func (p *Palette) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		pal, err := ParsePalette(s)
		if err != nil {
			return fmt.Errorf("palette: %w", err)
		}
		*p = pal
		return nil
	}
	var list []string
	if err := json.Unmarshal(b, &list); err != nil {
		return fmt.Errorf("palette: %s is not a string or an array of strings", string(b))
	}
	pal, err := ParsePalette(strings.Join(list, ","))
	if err != nil {
		return fmt.Errorf("palette: %w", err)
	}
	*p = pal
	return nil
}

// ParsePalette parses "#rrggbb,#rrggbb,…".
func ParsePalette(s string) ([]color.NRGBA, error) {
	var pal []color.NRGBA
	for _, tok := range strings.Split(s, ",") {
		tok = strings.TrimSpace(tok)
		hex, ok := strings.CutPrefix(tok, "#")
		if !ok || len(hex) != 6 {
			return nil, fmt.Errorf("%q is not a #rrggbb colour", tok)
		}
		v, err := strconv.ParseUint(hex, 16, 24)
		if err != nil {
			return nil, fmt.Errorf("%q is not a #rrggbb colour", tok)
		}
		pal = append(pal, color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255})
	}
	return pal, nil
}

// Parse decodes a JSON object of Params (empty input is {}), seeds the
// options from its preset (bw when absent), applies the given fields and
// validates. Unknown keys are errors.
func Parse(jsonText []byte) (rekha.Options, error) {
	var p Params
	if len(bytes.TrimSpace(jsonText)) > 0 {
		dec := json.NewDecoder(bytes.NewReader(jsonText))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&p); err != nil {
			return rekha.Options{}, fmt.Errorf("options: %w", err)
		}
	}
	preset := rekha.PresetBW
	if p.Preset != nil {
		preset = rekha.Preset(*p.Preset)
	}
	o := rekha.PresetOptions(preset)
	setString := func(dst *string, v *string) {
		if v != nil {
			*dst = *v
		}
	}
	setInt := func(dst *int, v *int) {
		if v != nil {
			*dst = *v
		}
	}
	setFloat := func(dst *float64, v *float64) {
		if v != nil {
			*dst = *v
		}
	}
	if p.Auto != nil {
		o.Auto = *p.Auto
	}
	setString((*string)(&o.Mode), p.Mode)
	setInt(&o.FilterSpeckle, p.FilterSpeckle)
	setInt(&o.ColorPrecision, p.ColorPrecision)
	setInt(&o.PathPrecision, p.PathPrecision)
	setInt(&o.MaxColors, p.MaxColors)
	if p.Threshold != nil {
		o.AutoThreshold = p.Threshold.Auto
		if !p.Threshold.Auto {
			o.Threshold = p.Threshold.Value
		}
	}
	setString((*string)(&o.TurnPolicy), p.TurnPolicy)
	setFloat(&o.SimplifyTolerance, p.Simplify)
	setFloat(&o.CornerThreshold, p.CornerThreshold)
	setFloat(&o.FitTolerance, p.FitTolerance)
	setFloat(&o.ColorTolerance, p.ColorTolerance)
	if p.Palette != nil {
		o.Palette = []color.NRGBA(*p.Palette)
	}
	setString((*string)(&o.Hierarchical), p.Hierarchical)
	if p.Refine != nil {
		o.Refine.Enabled = *p.Refine
	}
	setInt(&o.Refine.Sweeps, p.RefinePasses)
	setFloat(&o.Stroke, p.CutoutStroke)
	setFloat(&o.Stroke, p.Stroke)
	if p.Gradients != nil {
		o.Gradients = *p.Gradients
	}
	setFloat(&o.GradientTolerance, p.GradientTolerance)
	if p.NoMark != nil {
		o.NoMark = *p.NoMark
	}
	if p.Format != nil {
		o.Format = rekha.Format(*p.Format)
	}
	if err := o.Validate(); err != nil {
		return rekha.Options{}, err
	}
	return o, nil
}

// FromStrings is Parse for string-valued fields (HTTP form or query): a
// value that parses as a number becomes a JSON number, true/false a
// boolean, anything else a JSON string.
func FromStrings(kv map[string]string) (rekha.Options, error) {
	obj := make(map[string]json.RawMessage, len(kv))
	for k, v := range kv {
		obj[k] = literal(v)
	}
	b, err := json.Marshal(obj)
	if err != nil {
		return rekha.Options{}, err
	}
	o, err := Parse(b)
	if err != nil {
		return rekha.Options{}, rewriteFieldError(err, kv)
	}
	return o, nil
}

// literal is v as a JSON literal.
func literal(v string) json.RawMessage {
	if v == "true" || v == "false" {
		return json.RawMessage(v)
	}
	if _, err := strconv.ParseFloat(v, 64); err == nil && json.Valid([]byte(v)) {
		return json.RawMessage(v)
	}
	b, _ := json.Marshal(v)
	return b
}

// rewriteFieldError names the offending key when the JSON decoder reports
// a type mismatch, so "maxColors=four" says maxColors.
func rewriteFieldError(err error, kv map[string]string) error {
	var ute *json.UnmarshalTypeError
	if errors.As(err, &ute) && ute.Field != "" {
		return fmt.Errorf("options: %s: %q is not a %s", ute.Field, kv[ute.Field], ute.Type)
	}
	return err
}

// SegmentKeys lists the Params keys that change the segmentation (and so
// need Segment to run again); every other key only changes Finish.
func SegmentKeys() []string {
	return []string{"auto", "preset", "threshold", "colorPrecision", "colorTolerance", "maxColors", "palette", "filterSpeckle", "gradients", "gradientTolerance"}
}

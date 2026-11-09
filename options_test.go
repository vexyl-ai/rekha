package rekha

import (
	"errors"
	"image/color"
	"math"
	"reflect"
	"testing"

	"github.com/vexyl-ai/rekha/internal/refine"
)

func TestDefaultOptionsValid(t *testing.T) {
	if err := DefaultOptions().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejects(t *testing.T) {
	cases := []struct {
		name  string
		mut   func(*Options)
		field string
	}{
		{"preset", func(o *Options) { o.Preset = "neon" }, "Preset"},
		{"mode", func(o *Options) { o.Mode = "curvy" }, "Mode"},
		{"speckle-neg", func(o *Options) { o.FilterSpeckle = -1 }, "FilterSpeckle"},
		{"speckle-big", func(o *Options) { o.FilterSpeckle = 1025 }, "FilterSpeckle"},
		{"colorprec-0", func(o *Options) { o.ColorPrecision = 0 }, "ColorPrecision"},
		{"colorprec-9", func(o *Options) { o.ColorPrecision = 9 }, "ColorPrecision"},
		{"pathprec-9", func(o *Options) { o.PathPrecision = 9 }, "PathPrecision"},
		{"maxcolors-257", func(o *Options) { o.MaxColors = 257 }, "MaxColors"},
		{"threshold-high", func(o *Options) { o.Threshold = 1.1 }, "Threshold"},
		{"threshold-neg", func(o *Options) { o.Threshold = -0.1 }, "Threshold"},
		{"turnpolicy", func(o *Options) { o.TurnPolicy = "spiral" }, "TurnPolicy"},
		{"simplify-big", func(o *Options) { o.SimplifyTolerance = 11 }, "SimplifyTolerance"},
		{"corner-big", func(o *Options) { o.CornerThreshold = 181 }, "CornerThreshold"},
		{"fit-zero", func(o *Options) { o.FitTolerance = 0 }, "FitTolerance"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := DefaultOptions()
			c.mut(&o)
			err := o.Validate()
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("want *ValidationError, got %v", err)
			}
			if ve.Field != c.field {
				t.Fatalf("Field = %q, want %q", ve.Field, c.field)
			}
		})
	}
}

func TestValidationErrorMessage(t *testing.T) {
	e := &ValidationError{Field: "Mode", Msg: "unknown value \"curvy\""}
	if e.Error() != "Mode: unknown value \"curvy\"" {
		t.Fatal(e.Error())
	}
}

func TestPresetOptionsTable(t *testing.T) {
	bw, poster, photo := PresetOptions(PresetBW), PresetOptions(PresetPoster), PresetOptions(PresetPhoto)
	if !reflect.DeepEqual(DefaultOptions(), bw) {
		t.Fatalf("DefaultOptions differs from PresetOptions(bw)")
	}
	checks := []struct {
		name string
		got  any
		want any
	}{
		{"bw.Hierarchical", bw.Hierarchical, HierarchicalCutout},
		{"bw.MaxColors", bw.MaxColors, 2},
		{"bw.Threshold", bw.Threshold, 0.5},
		{"bw.FilterSpeckle", bw.FilterSpeckle, 4},
		{"bw.FitTolerance", bw.FitTolerance, 1.0},
		{"poster.Hierarchical", poster.Hierarchical, HierarchicalStacked},
		{"poster.MaxColors", poster.MaxColors, 0},
		{"poster.ColorTolerance", poster.ColorTolerance, 0.08},
		{"poster.ColorPrecision", poster.ColorPrecision, 6},
		{"poster.FilterSpeckle", poster.FilterSpeckle, 4},
		{"poster.FitTolerance", poster.FitTolerance, 1.0},
		{"photo.Hierarchical", photo.Hierarchical, HierarchicalStacked},
		{"photo.ColorTolerance", photo.ColorTolerance, 0.04},
		{"photo.FilterSpeckle", photo.FilterSpeckle, 16},
		{"photo.FitTolerance", photo.FitTolerance, 1.5},
		{"photo.Mode", photo.Mode, ModeSpline},
		{"photo.CornerThreshold", photo.CornerThreshold, 60.0},
	}
	for _, c := range checks {
		if !reflect.DeepEqual(c.got, c.want) {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	for _, p := range []Preset{PresetBW, PresetPoster, PresetPhoto} {
		if err := PresetOptions(p).Validate(); err != nil {
			t.Errorf("%s preset invalid: %v", p, err)
		}
	}
}

func TestValidateNewFields(t *testing.T) {
	bad := func(field string, mutate func(*Options)) {
		t.Helper()
		o := PresetOptions(PresetPoster)
		mutate(&o)
		var ve *ValidationError
		if err := o.Validate(); !errors.As(err, &ve) || ve.Field != field {
			t.Errorf("%s: got %v", field, err)
		}
	}
	bad("ColorTolerance", func(o *Options) { o.ColorTolerance = 1.5 })
	bad("Palette", func(o *Options) { o.Palette = make([]color.NRGBA, 257) })
	bad("Palette", func(o *Options) {
		o.Palette = []color.NRGBA{{255, 0, 0, 255}, {255, 0, 0, 255}}
	})
	bad("Hierarchical", func(o *Options) { o.Hierarchical = "x" })
}

func TestPresetRefineDefaults(t *testing.T) {
	for _, p := range []Preset{PresetBW, PresetPoster, PresetPhoto} {
		r := PresetOptions(p).Refine
		want := RefineOptions{Enabled: false, Sweeps: 3, MaxIter: 30, Weights: refine.DefaultWeights, MergeSplit: true, Primitives: true}
		if !reflect.DeepEqual(r, want) {
			t.Errorf("%s: Refine = %+v, want %+v", p, r, want)
		}
	}
}

func TestValidateRefineRanges(t *testing.T) {
	bad := func(name string, mutate func(*RefineOptions)) {
		t.Helper()
		o := PresetOptions(PresetBW)
		o.Refine.Enabled = true
		mutate(&o.Refine)
		var ve *ValidationError
		if err := o.Validate(); !errors.As(err, &ve) || ve.Field != "Refine" {
			t.Errorf("%s: got %v", name, err)
		}
	}
	bad("Sweeps 0", func(r *RefineOptions) { r.Sweeps = 0 })
	bad("Sweeps 11", func(r *RefineOptions) { r.Sweeps = 11 })
	bad("MaxIter 0", func(r *RefineOptions) { r.MaxIter = 0 })
	bad("MaxIter 201", func(r *RefineOptions) { r.MaxIter = 201 })
	bad("negative weight", func(r *RefineOptions) { r.Weights.Apt = -0.1 })
	bad("NaN weight", func(r *RefineOptions) { r.Weights.Lpt = math.NaN() })
	o := PresetOptions(PresetBW)
	o.Refine.Enabled = true
	if err := o.Validate(); err != nil {
		t.Fatalf("defaults with refinement enabled invalid: %v", err)
	}
}

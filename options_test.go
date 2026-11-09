package rekha

import (
	"errors"
	"testing"
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

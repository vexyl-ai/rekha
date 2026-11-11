package apiopts

import (
	"image/color"
	"reflect"
	"strings"
	"testing"

	"github.com/vexyl-ai/rekha"
)

func mustParse(t *testing.T, s string) rekha.Options {
	t.Helper()
	o, err := Parse([]byte(s))
	if err != nil {
		t.Fatalf("Parse(%s): %v", s, err)
	}
	return o
}

func TestParsePresetSeedsDefaults(t *testing.T) {
	o := mustParse(t, `{"preset":"poster"}`)
	if want := rekha.PresetOptions(rekha.PresetPoster); !reflect.DeepEqual(o, want) {
		t.Fatalf("got %+v\nwant %+v", o, want)
	}
}

func TestParseOverrides(t *testing.T) {
	o := mustParse(t, `{"preset":"poster","refine":true,"refinePasses":5,"threshold":"auto","palette":"#000000,#ffffff","maxColors":3}`)
	if !o.Refine.Enabled || o.Refine.Sweeps != 5 || !o.AutoThreshold || len(o.Palette) != 2 || o.MaxColors != 3 {
		t.Fatalf("overrides not applied: %+v", o)
	}
	if o.Preset != rekha.PresetPoster || o.Hierarchical != rekha.HierarchicalStacked {
		t.Fatalf("preset defaults lost: %+v", o)
	}
}

func TestParseThresholdNumber(t *testing.T) {
	o := mustParse(t, `{"threshold":0.3}`)
	if o.Threshold != 0.3 || o.AutoThreshold {
		t.Fatalf("threshold %v auto %v", o.Threshold, o.AutoThreshold)
	}
}

func TestParsePaletteArray(t *testing.T) {
	o := mustParse(t, `{"preset":"poster","palette":["#ff0000","#00ff00"]}`)
	want := []color.NRGBA{{R: 255, A: 255}, {G: 255, A: 255}}
	if !reflect.DeepEqual(o.Palette, want) {
		t.Fatalf("palette %v", o.Palette)
	}
}

func TestParseUnknownKey(t *testing.T) {
	if _, err := Parse([]byte(`{"colour":1}`)); err == nil || !strings.Contains(err.Error(), "colour") {
		t.Fatalf("err %v", err)
	}
}

func TestParseInvalidValue(t *testing.T) {
	if _, err := Parse([]byte(`{"fitTolerance":99}`)); err == nil || !strings.Contains(err.Error(), "FitTolerance") {
		t.Fatalf("err %v", err)
	}
}

func TestParseEmpty(t *testing.T) {
	for _, s := range []string{"{}", ""} {
		if o := mustParse(t, s); !reflect.DeepEqual(o, rekha.DefaultOptions()) {
			t.Fatalf("%q: %+v", s, o)
		}
	}
}

func TestFromStrings(t *testing.T) {
	o, err := FromStrings(map[string]string{"preset": "poster", "refine": "true", "threshold": "auto", "maxColors": "4"})
	if err != nil {
		t.Fatal(err)
	}
	if o.Preset != rekha.PresetPoster || !o.Refine.Enabled || !o.AutoThreshold || o.MaxColors != 4 {
		t.Fatalf("%+v", o)
	}
}

func TestFromStringsBadNumber(t *testing.T) {
	if _, err := FromStrings(map[string]string{"maxColors": "four"}); err == nil || !strings.Contains(err.Error(), "maxColors") {
		t.Fatalf("err %v", err)
	}
}

func TestSegmentKeys(t *testing.T) {
	want := []string{"preset", "threshold", "colorPrecision", "colorTolerance", "maxColors", "palette", "filterSpeckle", "gradients", "gradientTolerance"}
	if got := SegmentKeys(); !reflect.DeepEqual(got, want) {
		t.Fatalf("%v", got)
	}
}

func TestParsePaletteErrors(t *testing.T) {
	_, err := ParsePalette("fff")
	if err == nil || err.Error() != `"fff" is not a #rrggbb colour` {
		t.Fatalf("err %v", err)
	}
}

func TestParseGradients(t *testing.T) {
	o := mustParse(t, `{"preset":"photo","gradients":true,"gradientTolerance":0.05}`)
	if !o.Gradients || o.GradientTolerance != 0.05 {
		t.Fatalf("got %+v", o)
	}
	keys := strings.Join(SegmentKeys(), ",")
	if !strings.Contains(keys, "gradients") || !strings.Contains(keys, "gradientTolerance") {
		t.Errorf("segment keys %s", keys)
	}
}

func TestParseStrokeAlias(t *testing.T) {
	if o := mustParse(t, `{"stroke":0.5}`); o.Stroke != 0.5 {
		t.Errorf("stroke: %+v", o.Stroke)
	}
	if o := mustParse(t, `{"cutoutStroke":0.3}`); o.Stroke != 0.3 {
		t.Errorf("cutoutStroke alias: %+v", o.Stroke)
	}
	if o := mustParse(t, `{"cutoutStroke":0.3,"stroke":0.5}`); o.Stroke != 0.5 {
		t.Errorf("stroke wins over the alias: %+v", o.Stroke)
	}
}

func TestParseNoMark(t *testing.T) {
	o, err := Parse([]byte(`{"noMark": true}`))
	if err != nil {
		t.Fatal(err)
	}
	if !o.NoMark {
		t.Fatal("noMark not applied")
	}
	o, err = FromStrings(map[string]string{"noMark": "true"})
	if err != nil {
		t.Fatal(err)
	}
	if !o.NoMark {
		t.Fatal("noMark not applied from strings")
	}
}

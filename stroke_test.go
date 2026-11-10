package rekha

import (
	"strings"
	"testing"
)

func TestStrokeStacked(t *testing.T) {
	o := PresetOptions(PresetPhoto)
	o.Hierarchical = HierarchicalStacked
	if out := traceSVG(t, "gradient-shapes", o); strings.Contains(out, "stroke") {
		t.Fatalf("Stroke 0 must emit no stroke attributes: %.300q", out)
	}
	o.Stroke = 0.5
	out := traceSVG(t, "gradient-shapes", o)
	if !strings.Contains(out, `stroke-width="0.5"`) {
		t.Fatalf("stacked output with Stroke 0.5 should carry stroke-width 0.5: %.300q", out)
	}
	o.Gradients = true
	out = traceSVG(t, "gradient-shapes", o)
	if !strings.Contains(out, `stroke="url(#g0)"`) {
		t.Errorf("a gradient-filled shape strokes in its gradient: %.400q", out)
	}
}

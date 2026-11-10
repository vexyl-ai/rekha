package rekha

import (
	"sync"
	"testing"
)

// progressLog records every Progress call, safe from any goroutine.
type progressLog struct {
	mu    sync.Mutex
	calls []progressCall
}

type progressCall struct {
	stage       string
	done, total int
}

func (l *progressLog) fn(stage string, done, total int) {
	l.mu.Lock()
	l.calls = append(l.calls, progressCall{stage: stage, done: done, total: total})
	l.mu.Unlock()
}

// stages returns the distinct stage names in order of first report.
func (l *progressLog) stages() []string {
	var out []string
	for _, c := range l.calls {
		if len(out) == 0 || out[len(out)-1] != c.stage {
			out = append(out, c.stage)
		}
	}
	return out
}

// last returns the last report of stage and whether there was one.
func (l *progressLog) last(stage string) (progressCall, bool) {
	var c progressCall
	ok := false
	for _, x := range l.calls {
		if x.stage == stage {
			c, ok = x, true
		}
	}
	return c, ok
}

func TestSegmentProgressStages(t *testing.T) {
	var log progressLog
	o := PresetOptions(PresetPhoto)
	o.Gradients = true
	o.Progress = log.fn
	if _, err := Segment(synth(t, "gradient-shapes"), o); err != nil {
		t.Fatal(err)
	}
	want := []string{"quantise", "label", "ramps"}
	got := log.stages()
	if len(got) != len(want) {
		t.Fatalf("stages %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("stages %v, want %v", got, want)
		}
	}
	for _, c := range log.calls {
		if c.total != 0 {
			t.Errorf("segment stages have no count: %+v", c)
		}
	}
}

func TestFinishProgressCounts(t *testing.T) {
	// Cutout spline output fits the planar map's arcs first (edges), then
	// every contour (curves); stacked output has no edges stage.
	var log progressLog
	o := PresetOptions(PresetPoster)
	o.Hierarchical = HierarchicalCutout
	o.Progress = log.fn
	s, err := Segment(synth(t, "gradient-shapes"), o)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Finish(o); err != nil {
		t.Fatal(err)
	}
	for _, stage := range []string{"edges", "curves"} {
		c, ok := log.last(stage)
		if !ok {
			t.Fatalf("stage %q never reported; got %v", stage, log.stages())
		}
		if c.total == 0 || c.done != c.total {
			t.Errorf("stage %q ends at %d of %d, want done == total > 0", stage, c.done, c.total)
		}
	}
	for _, c := range log.calls {
		if c.done > c.total || c.done < 0 {
			t.Errorf("report out of range: %+v", c)
		}
	}
	// Reports are rationed: at most one per whole percent plus the end.
	for _, stage := range []string{"edges", "curves"} {
		n := 0
		for _, c := range log.calls {
			if c.stage == stage {
				n++
			}
		}
		if n > 102 {
			t.Errorf("stage %q reported %d times", stage, n)
		}
	}
}

func TestFinishProgressStacked(t *testing.T) {
	var log progressLog
	o := PresetOptions(PresetPoster)
	o.Progress = log.fn
	s, err := Segment(synth(t, "gradient-shapes"), o)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Finish(o); err != nil {
		t.Fatal(err)
	}
	if _, ok := log.last("edges"); ok {
		t.Errorf("stacked output has no planar map and no edges stage")
	}
	if c, ok := log.last("curves"); !ok || c.total == 0 || c.done != c.total {
		t.Errorf("curves ends at %+v, want done == total > 0", c)
	}
	// Tracing the outlines of every colour comes first, counted by
	// colour, then nesting (no count), then the curve fits.
	want := []string{"quantise", "label", "outlines", "nest", "curves"}
	got := log.stages()
	if len(got) != len(want) {
		t.Fatalf("stages %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("stages %v, want %v", got, want)
		}
	}
	if c, _ := log.last("outlines"); c.total != len(s.Palette) || c.done != c.total {
		t.Errorf("outlines ends at %+v, want %d of %d", c, len(s.Palette), len(s.Palette))
	}
}

func TestProgressNilIsFine(t *testing.T) {
	o := PresetOptions(PresetPoster)
	if _, err := Trace(synth(t, "gradient-shapes"), o); err != nil {
		t.Fatal(err)
	}
}

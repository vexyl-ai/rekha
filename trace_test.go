package rekha

import (
	"bytes"
	"errors"
	"flag"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/vexyl-ai/rekha/internal/bitmap"
	"github.com/vexyl-ai/rekha/internal/corpus"
	"github.com/vexyl-ai/rekha/internal/rasttest"
)

var update = flag.Bool("update", false, "rewrite golden files")

func synth(t *testing.T, name string) *image.NRGBA {
	t.Helper()
	for _, s := range corpus.All() {
		if s.Name == name {
			return corpus.Render(s)
		}
	}
	t.Fatalf("no synthetic %q", name)
	return nil
}

func TestTracePresetNotImplemented(t *testing.T) {
	o := DefaultOptions()
	o.Preset = PresetPoster
	_, err := Trace(image.NewNRGBA(image.Rect(0, 0, 2, 2)), o)
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Field != "Preset" {
		t.Fatalf("got %v", err)
	}
}

func TestTraceEmptyImageNoPaths(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	r, err := Trace(img, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Paths) != 0 {
		t.Fatalf("got %d paths", len(r.Paths))
	}
	var buf bytes.Buffer
	if err := r.WriteSVG(&buf, DefaultOptions()); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("<svg")) || bytes.Contains(buf.Bytes(), []byte("<path")) {
		t.Fatalf("svg:\n%s", buf.String())
	}
}

func TestTracePixelModeRoundTrip(t *testing.T) {
	o := DefaultOptions()
	o.Mode = ModePixel
	o.FilterSpeckle = 0
	for _, s := range corpus.All() {
		img := corpus.Render(s)
		r, err := Trace(img, o)
		if err != nil {
			t.Fatal(err)
		}
		gray, w, h := bitmap.Gray(img)
		want := bitmap.FromGray(gray, w, h, 128)
		got := rasttest.Fill(w, h, r.Paths)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				if got.Get(x, y) != want.Get(x, y) {
					t.Fatalf("%s: pixel (%d,%d) got %v want %v", s.Name, x, y, got.Get(x, y), want.Get(x, y))
				}
			}
		}
	}
}

func TestTraceGolden(t *testing.T) {
	for _, s := range corpus.All() {
		for _, mode := range []Mode{ModePixel, ModePolygon, ModeSpline} {
			o := DefaultOptions()
			o.Mode = mode
			r, err := Trace(corpus.Render(s), o)
			if err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			if err := r.WriteSVG(&buf, o); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join("testdata", "golden", s.Name+"-"+string(mode)+".svg")
			if *update {
				if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
					t.Fatal(err)
				}
				continue
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%s: %v (run with -update)", path, err)
			}
			if !bytes.Equal(want, buf.Bytes()) {
				t.Errorf("%s differs from golden", path)
			}
		}
	}
}

func TestTraceDeterministic(t *testing.T) {
	img := synth(t, "star-5")
	var first []byte
	for i := 0; i < 3; i++ {
		r, err := Trace(img, DefaultOptions())
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		r.WriteSVG(&buf, DefaultOptions())
		if first == nil {
			first = buf.Bytes()
		} else if !bytes.Equal(first, buf.Bytes()) {
			t.Fatal("output differs between runs")
		}
	}
}

func TestTraceSplineClosesEveryPath(t *testing.T) {
	for _, s := range corpus.All() {
		r, err := Trace(corpus.Render(s), DefaultOptions())
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range r.Paths {
			for _, sp := range p.Subpaths {
				if len(sp.Segs) == 0 {
					t.Fatalf("%s: empty subpath", s.Name)
				}
				first := sp.Segs[0].P[0]
				last := sp.Segs[len(sp.Segs)-1].P[3]
				if d := last.Sub(first).Len(); d > 1e-9 {
					t.Fatalf("%s: subpath not closed, gap %g", s.Name, d)
				}
				for i := 1; i < len(sp.Segs); i++ {
					if d := sp.Segs[i].P[0].Sub(sp.Segs[i-1].P[3]).Len(); d > 1e-9 {
						t.Fatalf("%s: segment %d disconnected", s.Name, i)
					}
				}
			}
		}
	}
}

func FuzzTrace(f *testing.F) {
	f.Add(uint8(1), uint8(1), []byte{0xff})
	f.Add(uint8(2), uint8(2), []byte{0b1001})
	f.Add(uint8(3), uint8(3), []byte{0b11111111, 0b1})
	f.Fuzz(func(t *testing.T, w, h uint8, data []byte) {
		W, H := int(w)%64+1, int(h)%64+1
		img := image.NewNRGBA(image.Rect(0, 0, W, H))
		for y := 0; y < H; y++ {
			for x := 0; x < W; x++ {
				i := y*W + x
				bit := len(data) > 0 && data[(i/8)%len(data)]&(1<<(i%8)) != 0
				c := color.NRGBA{255, 255, 255, 255}
				if bit {
					c = color.NRGBA{0, 0, 0, 255}
				}
				img.SetNRGBA(x, y, c)
			}
		}
		for _, mode := range []Mode{ModePixel, ModePolygon, ModeSpline} {
			for _, pol := range []TurnPolicy{TurnMinority, TurnMajority, TurnBlack, TurnWhite} {
				o := DefaultOptions()
				o.Mode, o.TurnPolicy, o.FilterSpeckle = mode, pol, 0
				r, err := Trace(img, o)
				if err != nil {
					t.Fatal(err)
				}
				for _, p := range r.Paths {
					if len(p.Subpaths) == 0 {
						t.Fatal("path without subpaths")
					}
					for _, sp := range p.Subpaths {
						for _, sg := range sp.Segs {
							for _, pt := range sg.P {
								if math.IsNaN(pt.X) || math.IsNaN(pt.Y) {
									t.Fatal("NaN coordinate")
								}
							}
						}
					}
				}
				var buf bytes.Buffer
				if err := r.WriteSVG(&buf, o); err != nil {
					t.Fatal(err)
				}
			}
		}
	})
}

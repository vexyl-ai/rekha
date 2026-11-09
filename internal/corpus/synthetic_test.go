package corpus

import (
	"bytes"
	"testing"
)

func find(t *testing.T, name string) Synthetic {
	t.Helper()
	for _, s := range All() {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("no synthetic named %q", name)
	return Synthetic{}
}

func TestAllNamesUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range All() {
		if seen[s.Name] {
			t.Fatalf("duplicate name %q", s.Name)
		}
		seen[s.Name] = true
	}
	if len(seen) != 10 {
		t.Fatalf("got %d synthetics, want 10", len(seen))
	}
}

func TestRenderDeterministic(t *testing.T) {
	for _, s := range All() {
		a := Render(s)
		b := Render(s)
		if !bytes.Equal(a.Pix, b.Pix) {
			t.Fatalf("%s: two renders differ", s.Name)
		}
	}
}

func TestRenderSize(t *testing.T) {
	for _, s := range All() {
		img := Render(s)
		if img.Bounds().Dx() != 256 || img.Bounds().Dy() != 256 {
			t.Fatalf("%s: bounds %v, want 256x256", s.Name, img.Bounds())
		}
	}
}

func TestCircleAAHasFractionalEdge(t *testing.T) {
	aa := Render(find(t, "circle-aa"))
	hard := Render(find(t, "circle-hard"))
	frac := func(img interface{ PixOffset(x, y int) int }, pix []uint8) bool {
		for y := 0; y < 256; y++ {
			for x := 0; x < 256; x++ {
				r := pix[img.PixOffset(x, y)]
				if r > 0 && r < 255 {
					return true
				}
			}
		}
		return false
	}
	if !frac(aa, aa.Pix) {
		t.Fatal("circle-aa has no fractional edge pixel")
	}
	if frac(hard, hard.Pix) {
		t.Fatal("circle-hard has a fractional pixel")
	}
}

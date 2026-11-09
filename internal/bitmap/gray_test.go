package bitmap

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"math/rand"
	"testing"
	"time"
)

func TestGrayCompositesAlphaOnWhite(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 3, 1))
	img.SetNRGBA(0, 0, color.NRGBA{0, 0, 0, 0})
	img.SetNRGBA(1, 0, color.NRGBA{0, 0, 0, 255})
	img.SetNRGBA(2, 0, color.NRGBA{255, 0, 0, 255})
	g, w, h := Gray(img)
	if w != 3 || h != 1 {
		t.Fatalf("size %dx%d", w, h)
	}
	if g[0] != 255 || g[1] != 0 || g[2] != 54 {
		t.Fatalf("gray = %v, want [255 0 54]", g)
	}
}

func TestFromGrayThreshold(t *testing.T) {
	b := FromGray([]uint8{0, 127, 128, 255}, 4, 1, 128)
	want := []bool{true, true, false, false}
	for x, w := range want {
		if b.Get(x, 0) != w {
			t.Fatalf("x=%d got %v want %v", x, b.Get(x, 0), w)
		}
	}
}

func TestOtsuBimodal(t *testing.T) {
	g := make([]uint8, 200)
	for i := range g {
		if i < 100 {
			g[i] = 20
		} else {
			g[i] = 200
		}
	}
	th := Otsu(g)
	if th <= 20 || th > 200 {
		t.Fatalf("Otsu = %d, want in (20,200]", th)
	}
}

func TestOtsuFlat(t *testing.T) {
	g := []uint8{77, 77, 77}
	if th := Otsu(g); th != 77 {
		t.Fatalf("Otsu flat = %d, want 77", th)
	}
}

// referenceGray is the straightforward implementation: composite onto
// white through the generic draw path, then weigh the channels.
func referenceGray(img image.Image) []uint8 {
	r := image.Rect(0, 0, img.Bounds().Dx(), img.Bounds().Dy())
	flat := image.NewNRGBA(r)
	draw.Draw(flat, r, image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(flat, r, img, img.Bounds().Min, draw.Over)
	out := make([]uint8, r.Dx()*r.Dy())
	for i := range out {
		p := flat.Pix[i*4:]
		y := 0.2126*float64(p[0]) + 0.7152*float64(p[1]) + 0.0722*float64(p[2])
		out[i] = uint8(math.Round(y))
	}
	return out
}

func TestGrayMatchesReference(t *testing.T) {
	rnd := rand.New(rand.NewSource(1))
	const w, h = 37, 23
	rect := image.Rect(5, 7, 5+w, 7+h) // non-zero origin
	nrgba := image.NewNRGBA(rect)
	nrgbaAlpha := image.NewNRGBA(rect)
	rgba := image.NewRGBA(rect)
	gray := image.NewGray(rect)
	pal := make(color.Palette, 16)
	for i := range pal {
		pal[i] = color.NRGBA{uint8(rnd.Intn(256)), uint8(rnd.Intn(256)), uint8(rnd.Intn(256)), 255}
	}
	pal[3] = color.NRGBA{200, 10, 10, 90} // one translucent entry
	paletted := image.NewPaletted(rect, pal)
	rgba64 := image.NewRGBA64(rect)
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			c := color.NRGBA{uint8(rnd.Intn(256)), uint8(rnd.Intn(256)), uint8(rnd.Intn(256)), 255}
			nrgba.SetNRGBA(x, y, c)
			rgba.SetRGBA(x, y, color.RGBA{c.R, c.G, c.B, 255})
			gray.SetGray(x, y, color.Gray{Y: c.R})
			paletted.SetColorIndex(x, y, uint8(rnd.Intn(16)))
			rgba64.Set(x, y, c)
			c.A = uint8(rnd.Intn(256))
			nrgbaAlpha.SetNRGBA(x, y, c)
		}
	}
	cases := []struct {
		name string
		img  image.Image
		tol  int
	}{
		{"nrgba", nrgba, 0},
		{"rgba", rgba, 0},
		{"gray", gray, 0},
		{"paletted", paletted, 1},
		{"nrgba-alpha", nrgbaAlpha, 1},
		{"rgba64", rgba64, 0},
	}
	for _, c := range cases {
		got, gw, gh := Gray(c.img)
		if gw != w || gh != h {
			t.Fatalf("%s: size %dx%d, want %dx%d", c.name, gw, gh, w, h)
		}
		want := referenceGray(c.img)
		for i := range want {
			if d := int(got[i]) - int(want[i]); d > c.tol || d < -c.tol {
				t.Fatalf("%s: pixel %d = %d, reference %d", c.name, i, got[i], want[i])
			}
		}
	}
}

// Grey conversion was 40% of tracing time; opaque images must take the
// direct path, not a generic composite.
func TestGrayLargeOpaqueIsFast(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4000, 4000))
	for i := range img.Pix {
		img.Pix[i] = uint8(i * 7)
		if i%4 == 3 {
			img.Pix[i] = 255
		}
	}
	start := time.Now()
	Gray(img)
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("Gray took %v on 16 Mpx, want under 200ms", elapsed)
	}
}

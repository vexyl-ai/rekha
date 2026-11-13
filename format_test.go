package rekha

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestParseFormat(t *testing.T) {
	for in, want := range map[string]Format{"": FormatSVG, "svg": FormatSVG, "PDF": FormatPDF, "eps": FormatEPS, ".pdf": FormatPDF} {
		got, err := ParseFormat(in)
		if err != nil || got != want {
			t.Errorf("ParseFormat(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := ParseFormat("dxf"); err == nil || !strings.Contains(err.Error(), "dxf") {
		t.Errorf("dxf: %v", err)
	}
}

func TestOptionsValidateFormat(t *testing.T) {
	o := PresetOptions(PresetBW)
	o.Format = "dxf"
	if err := o.Validate(); err == nil || !strings.Contains(err.Error(), "Format") {
		t.Fatalf("got %v", err)
	}
}

func TestWriteDispatchesOnFormat(t *testing.T) {
	o := PresetOptions(PresetBW)
	r, err := Trace(synth(t, "star-5"), o)
	if err != nil {
		t.Fatal(err)
	}
	for format, prefix := range map[Format]string{FormatSVG: "<svg", FormatPDF: "%PDF-1.4", FormatEPS: "%!PS-Adobe-3.0 EPSF-3.0"} {
		o.Format = format
		var buf bytes.Buffer
		if err := r.Write(&buf, o); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(buf.String(), prefix) {
			t.Errorf("%s: starts %q", format, buf.String()[:20])
		}
	}
	// The mark and NoMark follow the SVG's rule in every format.
	o.Format = FormatPDF
	var buf bytes.Buffer
	r.Write(&buf, o)
	if !strings.Contains(buf.String(), "/Producer (Rekha ") {
		t.Error("PDF lacks the Producer mark")
	}
	o.NoMark = true
	buf.Reset()
	r.Write(&buf, o)
	if strings.Contains(buf.String(), "Rekha") {
		t.Error("PDF marked despite NoMark")
	}
}

// TestPDFAndEPSRenderLikeSVG rasterises the three outputs of a gradient
// trace with Ghostscript and rsvg-convert and asks the PDF and the EPS
// to match the SVG within a few dB; skipped where the tools are absent.
func TestPDFAndEPSRenderLikeSVG(t *testing.T) {
	gs, err1 := exec.LookPath("gs")
	rsvg, err2 := exec.LookPath("rsvg-convert")
	if err1 != nil || err2 != nil {
		t.Skip("needs gs and rsvg-convert")
	}
	o := PresetOptions(PresetPhoto)
	o.Gradients = true
	o.Stroke = 0.5
	r, err := Trace(synth(t, "gradient-shapes"), o)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Gradients)+len(r.Radials) == 0 {
		t.Fatal("fixture traced without gradients")
	}
	dir := t.TempDir()
	write := func(f Format) string {
		o.Format = f
		p := filepath.Join(dir, "out."+string(f))
		fh, err := os.Create(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := r.Write(fh, o); err != nil {
			t.Fatal(err)
		}
		fh.Close()
		return p
	}
	svgPNG := filepath.Join(dir, "svg.png")
	if out, err := exec.Command(rsvg, "-w", strconv.Itoa(r.Width), "-h", strconv.Itoa(r.Height), "-b", "white", write(FormatSVG), "-o", svgPNG).CombinedOutput(); err != nil {
		t.Fatalf("rsvg: %v %s", err, out)
	}
	ref := loadPNG(t, svgPNG)
	for _, f := range []Format{FormatPDF, FormatEPS} {
		out := filepath.Join(dir, string(f)+".png")
		args := []string{"-q", "-dBATCH", "-dNOPAUSE", "-dPDFSTOPONERROR", "-sDEVICE=png16m", "-r96", "-dGraphicsAlphaBits=4", "-dTextAlphaBits=4", "-sOutputFile=" + out}
		if f == FormatEPS {
			args = append(args, "-dEPSCrop")
		}
		if msg, err := exec.Command(gs, append(args, write(f))...).CombinedOutput(); err != nil {
			t.Fatalf("%s: gs: %v %s", f, err, msg)
		}
		got := loadPNG(t, out)
		if got.Bounds() != ref.Bounds() {
			t.Fatalf("%s: rendered %v, svg %v", f, got.Bounds(), ref.Bounds())
		}
		// Ghostscript and librsvg antialias edges differently and band
		// shadings a little: a faithful file scores 29–33 dB here, a
		// black stroke 22, paths run together 13.
		if d := psnr(ref, got); d < 28 {
			t.Errorf("%s: %.1f dB from the SVG rendering, want ≥ 28", f, d)
		}
	}
}

func loadPNG(t *testing.T, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

// psnr compares two images over white, in 8-bit sRGB.
func psnr(a, b image.Image) float64 {
	var sum float64
	bd := a.Bounds()
	n := 0
	for y := bd.Min.Y; y < bd.Max.Y; y++ {
		for x := bd.Min.X; x < bd.Max.X; x++ {
			ca := color.NRGBAModel.Convert(a.At(x, y)).(color.NRGBA)
			cb := color.NRGBAModel.Convert(b.At(x, y)).(color.NRGBA)
			for _, d := range []float64{float64(ca.R) - float64(cb.R), float64(ca.G) - float64(cb.G), float64(ca.B) - float64(cb.B)} {
				sum += d * d
			}
			n += 3
		}
	}
	if sum == 0 {
		return math.Inf(1)
	}
	return 10 * math.Log10(255*255/(sum/float64(n)))
}

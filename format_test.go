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
	"regexp"
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

func TestConvertSVGToPDFKeepsMarkAndEdits(t *testing.T) {
	o := PresetOptions(PresetPhoto)
	o.Gradients = true
	r, err := Trace(synth(t, "gradient-shapes"), o)
	if err != nil {
		t.Fatal(err)
	}
	var svgText bytes.Buffer
	if err := r.WriteSVG(&svgText, o); err != nil {
		t.Fatal(err)
	}
	// An edit in the playground: the first path removed.
	edited := strings.Replace(svgText.String(), "<path", "<!-- gone --><path", 1)
	edited = regexp.MustCompile(`(?s)<!-- gone --><path[^>]*/>\n`).ReplaceAllString(edited, "")
	for _, c := range []struct {
		format Format
		prefix string
		mark   string
	}{{FormatPDF, "%PDF-1.4", "/Producer (Rekha "}, {FormatEPS, "%!PS-Adobe-3.0", "%%Creator: Rekha "}, {FormatSVG, "<svg", "data-generator=\"rekha "}} {
		o.Format = c.format
		var out bytes.Buffer
		if err := Convert(strings.NewReader(edited), &out, o); err != nil {
			t.Fatalf("%s: %v", c.format, err)
		}
		if !strings.HasPrefix(out.String(), c.prefix) || !strings.Contains(out.String(), c.mark) {
			t.Errorf("%s: %q", c.format, out.String()[:60])
		}
	}
	o.Format = FormatSVG
	var again bytes.Buffer
	if err := Convert(strings.NewReader(edited), &again, o); err != nil || again.String() != edited {
		t.Errorf("svg round trip changed the document: %v", err)
	}
	o.NoMark = true
	again.Reset()
	Convert(strings.NewReader(edited), &again, o)
	if strings.Contains(again.String(), "generator") {
		t.Error("NoMark left the mark in place")
	}
	if err := Convert(strings.NewReader("<svg xmlns=\"http://www.w3.org/2000/svg\"><rect/></svg>"), &again, o); err == nil {
		t.Error("foreign SVG accepted")
	}
}

func TestConvertKeepsScale(t *testing.T) {
	img := synth(t, "star-5")
	o := PresetOptions(PresetBW)
	o.Scale = 2
	o.NoMark = true
	r, err := Trace(img, o)
	if err != nil {
		t.Fatal(err)
	}
	var svgBuf, pdf bytes.Buffer
	if err := r.WriteSVG(&svgBuf, o); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(svgBuf.String(), `width="128" height="128" viewBox="0 0 256 256"`) {
		t.Fatalf("svg root: %.120s", svgBuf.String())
	}
	o.Format = FormatPDF
	if err := Convert(bytes.NewReader(svgBuf.Bytes()), &pdf, o); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pdf.String(), "/MediaBox [0 0 96 96]") {
		t.Fatalf("pdf has no 96 pt page: %.300s", pdf.String())
	}
}

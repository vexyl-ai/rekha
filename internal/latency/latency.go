// Package latency times Segment and Finish per preset over a directory of
// images and prints a markdown table; the same code runs natively and
// under WebAssembly (go_js_wasm_exec maps os to Node's fs), so the two
// tables compare like for like.
package latency

import (
	"fmt"
	"image"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/vexyl-ai/rekha"

	_ "image/jpeg"
	_ "image/png"
)

// Case is one preset configuration measured per image.
type Case struct {
	Name string
	Opts rekha.Options
}

// Cases are the presets the baseline records.
func Cases() []Case {
	refine := rekha.PresetOptions(rekha.PresetPoster)
	refine.Refine.Enabled = true
	return []Case{
		{Name: "bw", Opts: rekha.PresetOptions(rekha.PresetBW)},
		{Name: "poster", Opts: rekha.PresetOptions(rekha.PresetPoster)},
		{Name: "poster --refine", Opts: refine},
	}
}

// Row is the timing of one image under one case.
type Row struct {
	Image         string
	Width, Height int
	Case          string
	SegmentMs     float64
	FinishMs      float64
}

// Measure times one trace; the image is decoded beforehand so only the
// tracer is measured.
func Measure(img image.Image, c Case) (Row, error) {
	t0 := time.Now()
	s, err := rekha.Segment(img, c.Opts)
	if err != nil {
		return Row{}, err
	}
	t1 := time.Now()
	if _, err := s.Finish(c.Opts); err != nil {
		return Row{}, err
	}
	t2 := time.Now()
	b := img.Bounds()
	return Row{Width: b.Dx(), Height: b.Dy(), Case: c.Name, SegmentMs: ms(t1.Sub(t0)), FinishMs: ms(t2.Sub(t1))}, nil
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

// Images lists the PNG and JPEG files under dir, sorted, plus extra paths.
func Images(dir string, extra ...string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ext := strings.ToLower(filepath.Ext(p)); !d.IsDir() && (ext == ".png" || ext == ".jpg" || ext == ".jpeg") {
			paths = append(paths, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	for _, e := range extra {
		if _, err := os.Stat(e); err == nil {
			paths = append(paths, e)
		}
	}
	return paths, nil
}

// Report measures every image under every case and writes a markdown
// table to w, with the platform in the heading.
func Report(w io.Writer, paths []string, cases []Case) error {
	fmt.Fprintf(w, "Platform: %s/%s, GOMAXPROCS %d, Go %s\n\n", runtime.GOOS, runtime.GOARCH, runtime.GOMAXPROCS(0), runtime.Version())
	fmt.Fprintln(w, "| Image | Size | Case | Segment ms | Finish ms | Total ms |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|")
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		img, _, err := image.Decode(f)
		f.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		for _, c := range cases {
			r, err := Measure(img, c)
			if err != nil {
				return fmt.Errorf("%s %s: %w", p, c.Name, err)
			}
			r.Image = strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
			fmt.Fprintf(w, "| %s | %d×%d | %s | %.0f | %.0f | %.0f |\n", r.Image, r.Width, r.Height, r.Case, r.SegmentMs, r.FinishMs, r.SegmentMs+r.FinishMs)
		}
	}
	return nil
}

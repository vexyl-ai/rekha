//go:build !js

package bench

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/image/bmp"

	"github.com/vexyl-ai/rekha/internal/metrics"
)

// Result is one image × tool measurement.
type Result struct {
	Image   string
	Tool    string
	PSNR    float64 // NaN when Err != "" or Skipped != ""
	Stats   metrics.SVGStats
	Millis  int64
	Skipped string // non-empty: "not installed" or "no reference"
	Err     string // non-empty on failure
}

// Runner executes tools over a corpus.
type Runner struct {
	Rasterize func(ctx context.Context, svgPath string, w, h int) (image.Image, error) // nil → RSVGConvert
	Timeout   time.Duration                                                            // 0 → 60s per tool invocation
	WorkDir   string                                                                   // "" → os.MkdirTemp
}

func (r *Runner) defaults() (func(context.Context, string, int, int) (image.Image, error), time.Duration, string, func(), error) {
	raster := r.Rasterize
	if raster == nil {
		raster = RSVGConvert
	}
	timeout := r.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	work := r.WorkDir
	cleanup := func() {}
	if work == "" {
		d, err := os.MkdirTemp("", "rekha-bench-")
		if err != nil {
			return nil, 0, "", nil, err
		}
		work = d
		cleanup = func() { os.RemoveAll(d) }
	}
	return raster, timeout, work, cleanup, nil
}

// Run walks corpusDir recursively for *.png files (case-insensitive), sorted
// by path, and runs every tool on each. Results are sorted by Image then Tool.
func (r *Runner) Run(ctx context.Context, corpusDir string, tools []Tool) ([]Result, error) {
	raster, timeout, work, cleanup, err := r.defaults()
	if err != nil {
		return nil, err
	}
	defer cleanup()

	images, err := listPNGs(corpusDir)
	if err != nil {
		return nil, err
	}
	installed := map[string]bool{}
	for _, t := range tools {
		if t.Dir != "" {
			installed[t.Name] = true
			continue
		}
		_, lookErr := exec.LookPath(t.Bin)
		installed[t.Name] = lookErr == nil
	}

	var results []Result
	for i, img := range images {
		for _, t := range tools {
			res := Result{Image: img, Tool: t.Name, PSNR: math.NaN()}
			if !installed[t.Name] {
				res.Skipped = "not installed"
				results = append(results, res)
				continue
			}
			dir := filepath.Join(work, fmt.Sprintf("%04d-%s", i, t.Name))
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, err
			}
			r.runOne(ctx, raster, timeout, dir, img, t, &res)
			results = append(results, res)
		}
	}
	sort.Slice(results, func(a, b int) bool {
		if results[a].Image != results[b].Image {
			return results[a].Image < results[b].Image
		}
		return results[a].Tool < results[b].Tool
	})
	return results, nil
}

func (r *Runner) runOne(ctx context.Context, raster func(context.Context, string, int, int) (image.Image, error), timeout time.Duration, dir, imgPath string, t Tool, res *Result) {
	src, err := readPNG(imgPath)
	if err != nil {
		res.Err = err.Error()
		return
	}
	// Errors must not leak the scratch directory: it changes every run and
	// would make recorded baselines non-reproducible.
	fail := func(err error) { res.Err = strings.ReplaceAll(err.Error(), dir+string(filepath.Separator), "") }

	if t.Dir != "" {
		if err := r.scoreReference(ctx, raster, dir, src, imgPath, t.Dir, res); err != nil {
			fail(err)
		}
		return
	}

	in, err := prepareInput(dir, src, t.Input)
	if err != nil {
		fail(err)
		return
	}
	// The tool runs inside dir with relative paths, so its own messages
	// name "in.png"/"out.svg" rather than the scratch path.
	const outName = "out.svg"
	out := filepath.Join(dir, outName)
	args := make([]string, len(t.Args))
	for i, a := range t.Args {
		args[i] = strings.NewReplacer("{in}", filepath.Base(in), "{out}", outName).Replace(a)
	}
	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	cmd := exec.CommandContext(tctx, t.Bin, args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	res.Millis = time.Since(start).Milliseconds()
	if runErr != nil {
		fail(fmt.Errorf("%s: %v: %s", t.Bin, runErr, strings.TrimSpace(stderr.String())))
		return
	}
	svg, err := os.ReadFile(out)
	if err != nil {
		fail(err)
		return
	}
	res.Stats = metrics.Stats(svg)
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	rendered, err := raster(ctx, out, w, h)
	if err != nil {
		fail(err)
		return
	}
	psnr, err := metrics.PSNR(src, rendered)
	if err != nil {
		fail(err)
		return
	}
	res.PSNR = psnr
}

// scoreReference scores a hand-downloaded output for imgPath found in
// refDir: <name>.svg is copied into dir and rasterised and measured like
// a tool's output (no time), <name>.png is compared directly, and
// neither marks the row "no reference". An SVG whose own size (viewBox,
// else width and height) is not the input's shape is an error: the
// rasteriser would stretch it to the input's size and score the stretch.
func (r *Runner) scoreReference(ctx context.Context, raster func(context.Context, string, int, int) (image.Image, error), dir string, src image.Image, imgPath, refDir string, res *Result) error {
	name := strings.TrimSuffix(filepath.Base(imgPath), filepath.Ext(imgPath))
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	if svg, err := os.ReadFile(filepath.Join(refDir, name+".svg")); err == nil {
		sw, sh, ok := svgSize(svg)
		if !ok {
			return fmt.Errorf("reference SVG has no viewBox or width/height")
		}
		if math.Abs(sw/sh-float64(w)/float64(h)) > sizeTol*float64(w)/float64(h) {
			return fmt.Errorf("reference SVG is %gx%g for a %dx%d input", sw, sh, w, h)
		}
		res.Stats = metrics.Stats(svg)
		local := filepath.Join(dir, "ref.svg")
		if err := os.WriteFile(local, svg, 0o644); err != nil {
			return err
		}
		rendered, err := raster(ctx, local, w, h)
		if err != nil {
			return err
		}
		res.PSNR, err = metrics.PSNR(src, rendered)
		if err != nil {
			res.PSNR = math.NaN()
		}
		return err
	}
	ref, err := readPNG(filepath.Join(refDir, name+".png"))
	if err != nil {
		if os.IsNotExist(err) {
			res.Skipped = "no reference"
			return nil
		}
		return err
	}
	res.PSNR, err = metrics.PSNR(src, ref)
	if err != nil {
		res.PSNR = math.NaN()
	}
	return err
}

// sizeTol is the relative aspect-ratio difference a reference SVG may
// have from the input before its row is an error.
const sizeTol = 0.01

var (
	svgTag    = regexp.MustCompile(`(?s)<svg\b[^>]*>`)
	viewBoxRe = regexp.MustCompile(`\bviewBox\s*=\s*["']\s*([-\d.eE+]+)[\s,]+([-\d.eE+]+)[\s,]+([-\d.eE+]+)[\s,]+([-\d.eE+]+)\s*["']`)
	widthRe   = regexp.MustCompile(`\bwidth\s*=\s*["']\s*([\d.eE+-]+)`)
	heightRe  = regexp.MustCompile(`\bheight\s*=\s*["']\s*([\d.eE+-]+)`)
)

// svgSize is the size the root <svg> element declares: its viewBox, else
// its width and height (units ignored; only the shape is compared).
func svgSize(svg []byte) (w, h float64, ok bool) {
	tag := svgTag.Find(svg)
	if tag == nil {
		return 0, 0, false
	}
	num := func(b []byte) (float64, bool) {
		v, err := strconv.ParseFloat(string(b), 64)
		return v, err == nil && v > 0
	}
	if m := viewBoxRe.FindSubmatch(tag); m != nil {
		if w, okw := num(m[3]); okw {
			if h, okh := num(m[4]); okh {
				return w, h, true
			}
		}
	}
	mw, mh := widthRe.FindSubmatch(tag), heightRe.FindSubmatch(tag)
	if mw == nil || mh == nil {
		return 0, 0, false
	}
	w, okw := num(mw[1])
	h, okh := num(mh[1])
	return w, h, okw && okh
}

func listPNGs(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.EqualFold(filepath.Ext(p), ".png") {
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

func readPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

// prepareInput writes src into dir in the format the tool wants and returns
// the path. BMP output is composited onto white because BMP has no alpha.
func prepareInput(dir string, src image.Image, format string) (string, error) {
	switch format {
	case "png":
		p := filepath.Join(dir, "in.png")
		f, err := os.Create(p)
		if err != nil {
			return "", err
		}
		defer f.Close()
		return p, png.Encode(f, src)
	case "bmp":
		p := filepath.Join(dir, "in.bmp")
		r := image.Rect(0, 0, src.Bounds().Dx(), src.Bounds().Dy())
		flat := image.NewRGBA(r)
		draw.Draw(flat, r, image.NewUniform(color.White), image.Point{}, draw.Src)
		draw.Draw(flat, r, src, src.Bounds().Min, draw.Over)
		f, err := os.Create(p)
		if err != nil {
			return "", err
		}
		defer f.Close()
		return p, bmp.Encode(f, flat)
	default:
		return "", fmt.Errorf("bench: unknown input format %q", format)
	}
}

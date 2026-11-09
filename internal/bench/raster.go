//go:build !js

package bench

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const rasterBin = "rsvg-convert"

// CheckRasterizer reports whether rsvg-convert is on PATH.
func CheckRasterizer() error {
	if _, err := exec.LookPath(rasterBin); err != nil {
		return fmt.Errorf("bench: %s not found on PATH; install with `brew install librsvg`", rasterBin)
	}
	return nil
}

// RSVGConvert rasterises svgPath to a w×h image using rsvg-convert.
func RSVGConvert(ctx context.Context, svgPath string, w, h int) (image.Image, error) {
	out := filepath.Join(filepath.Dir(svgPath), "raster.png")
	cmd := exec.CommandContext(ctx, rasterBin, "-w", fmt.Sprint(w), "-h", fmt.Sprint(h), svgPath, "-o", out)
	if b, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("%s: %v: %s", rasterBin, err, strings.TrimSpace(string(b)))
	}
	f, err := os.Open(out)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

// Command rekha traces raster images into SVG.
package main

import (
	"flag"
	"fmt"
	"image/color"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/vexyl-ai/rekha"
	"github.com/vexyl-ai/rekha/internal/cli"

	// Decoders for the formats TraceReader accepts.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

const version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("rekha", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: rekha [options] input... (use - for stdin)")
		fs.PrintDefaults()
	}

	// The preset seeds every default, so it is read before the flags are
	// defined; flags given on the command line then override it.
	o := rekha.PresetOptions(presetArg(args))
	var preset, mode, output, threshold, turnPolicy, palette, hierarchical string
	var showVersion bool
	fs.StringVar(&preset, "preset", string(o.Preset), "preset seeding the defaults: bw (cutout, 2 colours), poster (stacked, tolerance 0.08), photo (stacked, tolerance 0.04, speckle 16, fit 1.5)")
	fs.StringVar(&mode, "mode", string(o.Mode), "mode: pixel, polygon, spline")
	fs.IntVar(&o.FilterSpeckle, "filter-speckle", o.FilterSpeckle, "discard regions smaller than N pixels")
	fs.IntVar(&o.ColorPrecision, "color-precision", o.ColorPrecision, "colour bits per channel (1-8)")
	fs.IntVar(&o.PathPrecision, "path-precision", o.PathPrecision, "decimal places in path data (0-8)")
	fs.IntVar(&o.MaxColors, "max-colors", o.MaxColors, "maximum colours (0-256), 0 = as many as --color-tolerance needs")
	fs.Float64Var(&o.ColorTolerance, "color-tolerance", o.ColorTolerance, "OKLab distance at which colours merge (0-1)")
	fs.StringVar(&palette, "palette", "", "fixed palette as comma-separated #rrggbb colours")
	fs.StringVar(&hierarchical, "hierarchical", string(o.Hierarchical), "layering: stacked, cutout")
	fs.StringVar(&threshold, "threshold", strconv.FormatFloat(o.Threshold, 'g', -1, 64), "foreground threshold 0..1, or auto (Otsu)")
	fs.StringVar(&turnPolicy, "turn-policy", string(o.TurnPolicy), "checkerboard resolution: minority, majority, black, white")
	fs.Float64Var(&o.SimplifyTolerance, "simplify", o.SimplifyTolerance, "polygon simplification tolerance in px (0-10)")
	fs.Float64Var(&o.CornerThreshold, "corner-threshold", o.CornerThreshold, "turn angle in degrees above which a vertex is a corner (0-180)")
	fs.Float64Var(&o.FitTolerance, "fit-tolerance", o.FitTolerance, "curve fitting tolerance in px (0.01-10)")
	fs.BoolVar(&o.Refine.Enabled, "refine", o.Refine.Enabled, "optimise the fitted curves against the image's anti-aliasing (spline mode)")
	fs.IntVar(&o.Refine.Sweeps, "refine-passes", o.Refine.Sweeps, "refinement sweeps over each contour (1-10)")
	fs.Float64Var(&o.CutoutStroke, "cutout-stroke", o.CutoutStroke, "stroke width in px hiding seams between cutout shapes when enlarged (0-4, 0 = none; thickens edges at 1:1)")
	fs.StringVar(&output, "output", "", "output path (single input only; - for stdout)")
	fs.StringVar(&output, "o", "", "alias for --output")
	fs.BoolVar(&showVersion, "version", false, "print version and exit")

	if err := fs.Parse(args); err != nil {
		return cli.ExitUsage
	}
	if showVersion {
		fmt.Fprintln(stdout, "rekha "+version)
		return cli.ExitOK
	}
	inputs := fs.Args()
	if len(inputs) == 0 {
		fs.Usage()
		return cli.ExitUsage
	}
	if output != "" && len(inputs) > 1 {
		fmt.Fprintln(stderr, "rekha: --output requires exactly one input")
		return cli.ExitUsage
	}
	o.Preset = rekha.Preset(preset)
	o.Mode = rekha.Mode(mode)
	o.TurnPolicy = rekha.TurnPolicy(turnPolicy)
	o.Hierarchical = rekha.Hierarchical(hierarchical)
	if palette != "" {
		pal, err := parsePalette(palette)
		if err != nil {
			fmt.Fprintln(stderr, "rekha: --palette: "+err.Error())
			return cli.ExitUsage
		}
		o.Palette = pal
	}
	if threshold == "auto" {
		o.AutoThreshold = true
	} else {
		v, err := strconv.ParseFloat(threshold, 64)
		if err != nil {
			fmt.Fprintf(stderr, "rekha: --threshold: %q is not a number or auto\n", threshold)
			return cli.ExitUsage
		}
		o.Threshold = v
	}
	if err := o.Validate(); err != nil {
		fmt.Fprintln(stderr, "rekha: "+err.Error())
		return cli.ExitUsage
	}

	code := cli.ExitOK
	for _, in := range inputs {
		if err := traceOne(in, cli.OutputPath(in, output), o, stdin, stdout); err != nil {
			fmt.Fprintf(stderr, "rekha: %s: %v\n", in, err)
			code = cli.ExitError
		}
	}
	return code
}

// presetArg returns the value of --preset in args before parsing, or bw.
func presetArg(args []string) rekha.Preset {
	for i, a := range args {
		if a == "--" {
			break
		}
		name := strings.TrimLeft(a, "-")
		if name == a || name == "" {
			continue
		}
		if v, ok := strings.CutPrefix(name, "preset="); ok {
			return rekha.Preset(v)
		}
		if name == "preset" && i+1 < len(args) {
			return rekha.Preset(args[i+1])
		}
	}
	return rekha.PresetBW
}

// parsePalette parses "#rrggbb,#rrggbb,…".
func parsePalette(s string) ([]color.NRGBA, error) {
	var pal []color.NRGBA
	for _, tok := range strings.Split(s, ",") {
		tok = strings.TrimSpace(tok)
		hex, ok := strings.CutPrefix(tok, "#")
		if !ok || len(hex) != 6 {
			return nil, fmt.Errorf("%q is not a #rrggbb colour", tok)
		}
		v, err := strconv.ParseUint(hex, 16, 24)
		if err != nil {
			return nil, fmt.Errorf("%q is not a #rrggbb colour", tok)
		}
		pal = append(pal, color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255})
	}
	return pal, nil
}

// traceOne traces input ("-" for stdin) and writes the SVG to out ("-" for
// stdout). The output file is only created once tracing has succeeded.
func traceOne(in, out string, o rekha.Options, stdin io.Reader, stdout io.Writer) error {
	var r io.Reader = stdin
	if in != "-" {
		f, err := os.Open(in)
		if err != nil {
			return err
		}
		defer f.Close()
		r = f
	}
	res, err := rekha.TraceReader(r, o)
	if err != nil {
		return err
	}
	if out == "-" {
		return res.WriteSVG(stdout, o)
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	if err := res.WriteSVG(f, o); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

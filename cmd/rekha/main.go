// Command rekha traces raster images into SVG.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"

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

	o := rekha.DefaultOptions()
	var preset, mode, output, threshold, turnPolicy string
	var showVersion bool
	fs.StringVar(&preset, "preset", string(o.Preset), "preset: bw, poster, photo")
	fs.StringVar(&mode, "mode", string(o.Mode), "mode: pixel, polygon, spline")
	fs.IntVar(&o.FilterSpeckle, "filter-speckle", o.FilterSpeckle, "discard regions smaller than N pixels")
	fs.IntVar(&o.ColorPrecision, "color-precision", o.ColorPrecision, "colour bits per channel (1-8)")
	fs.IntVar(&o.PathPrecision, "path-precision", o.PathPrecision, "decimal places in path data (0-8)")
	fs.IntVar(&o.MaxColors, "max-colors", o.MaxColors, "maximum colours, 0 = unlimited")
	fs.StringVar(&threshold, "threshold", strconv.FormatFloat(o.Threshold, 'g', -1, 64), "foreground threshold 0..1, or auto (Otsu)")
	fs.StringVar(&turnPolicy, "turn-policy", string(o.TurnPolicy), "checkerboard resolution: minority, majority, black, white")
	fs.Float64Var(&o.SimplifyTolerance, "simplify", o.SimplifyTolerance, "polygon simplification tolerance in px (0-10)")
	fs.Float64Var(&o.CornerThreshold, "corner-threshold", o.CornerThreshold, "turn angle in degrees above which a vertex is a corner (0-180)")
	fs.Float64Var(&o.FitTolerance, "fit-tolerance", o.FitTolerance, "curve fitting tolerance in px (0.01-10)")
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

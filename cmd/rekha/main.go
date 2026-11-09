// Command rekha traces raster images into SVG.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/lonelypx/rekha"
	"github.com/lonelypx/rekha/internal/cli"
)

const version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("rekha", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: rekha [options] input... (use - for stdin)")
		fs.PrintDefaults()
	}

	o := rekha.DefaultOptions()
	var preset, mode, output string
	var showVersion bool
	fs.StringVar(&preset, "preset", string(o.Preset), "preset: bw, poster, photo")
	fs.StringVar(&mode, "mode", string(o.Mode), "mode: pixel, polygon, spline")
	fs.IntVar(&o.FilterSpeckle, "filter-speckle", o.FilterSpeckle, "discard regions smaller than N pixels")
	fs.IntVar(&o.ColorPrecision, "color-precision", o.ColorPrecision, "colour bits per channel (1-8)")
	fs.IntVar(&o.PathPrecision, "path-precision", o.PathPrecision, "decimal places in path data (0-8)")
	fs.IntVar(&o.MaxColors, "max-colors", o.MaxColors, "maximum colours, 0 = unlimited")
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
	if err := o.Validate(); err != nil {
		fmt.Fprintln(stderr, "rekha: "+err.Error())
		return cli.ExitUsage
	}
	for _, in := range inputs {
		_ = cli.OutputPath(in, output)
	}
	fmt.Fprintln(stderr, "rekha: tracing not implemented yet (phase 0)")
	return cli.ExitError
}

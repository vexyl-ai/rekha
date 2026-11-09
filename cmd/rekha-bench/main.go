//go:build !js

// Command rekha-bench runs reference tracers over the corpus and scores them.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vexyl-ai/rekha/internal/bench"
	"github.com/vexyl-ai/rekha/internal/cli"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("rekha-bench", flag.ContinueOnError)
	corpus := fs.String("corpus", "testdata/corpus", "corpus directory (walked recursively for *.png)")
	toolsFlag := fs.String("tools", "", "comma-separated tool names; default all builtin")
	out := fs.String("out", "bench/out", "output directory for results.md and results.csv")
	if err := fs.Parse(args); err != nil {
		return cli.ExitUsage
	}

	tools := bench.BuiltinTools()
	if *toolsFlag != "" {
		var unknown []string
		tools, unknown = bench.ByName(strings.Split(*toolsFlag, ","))
		if len(unknown) > 0 {
			fmt.Fprintf(os.Stderr, "rekha-bench: unknown tools: %s\n", strings.Join(unknown, ", "))
			return cli.ExitUsage
		}
	}
	if err := bench.CheckRasterizer(); err != nil {
		fmt.Fprintln(os.Stderr, "rekha-bench:", err)
		return cli.ExitError
	}

	r := &bench.Runner{}
	results, err := r.Run(context.Background(), *corpus, tools)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rekha-bench:", err)
		return cli.ExitError
	}
	if len(results) == 0 {
		fmt.Fprintln(os.Stderr, "rekha-bench: 0 images found under", *corpus)
	}

	if err := os.MkdirAll(*out, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "rekha-bench:", err)
		return cli.ExitError
	}
	if err := writeFile(filepath.Join(*out, "results.md"), func(f *os.File) error { return bench.WriteMarkdown(f, results) }); err != nil {
		fmt.Fprintln(os.Stderr, "rekha-bench:", err)
		return cli.ExitError
	}
	if err := writeFile(filepath.Join(*out, "results.csv"), func(f *os.File) error { return bench.WriteCSV(f, results) }); err != nil {
		fmt.Fprintln(os.Stderr, "rekha-bench:", err)
		return cli.ExitError
	}
	if err := bench.WriteMarkdown(os.Stdout, results); err != nil {
		return cli.ExitError
	}
	return cli.ExitOK
}

func writeFile(path string, write func(*os.File) error) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := write(f); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

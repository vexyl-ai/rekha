// Command gen writes the synthetic corpus as PNG files.
//
//	go run ./internal/corpus/gen -out testdata/corpus/synthetic
package main

import (
	"flag"
	"fmt"
	"image/png"
	"os"
	"path/filepath"

	"github.com/vexyl-ai/rekha/internal/corpus"
)

func main() {
	out := flag.String("out", "testdata/corpus/synthetic", "output directory")
	flag.Parse()
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(2)
	}
	for _, s := range corpus.All() {
		if err := write(filepath.Join(*out, s.Name+".png"), s); err != nil {
			fmt.Fprintln(os.Stderr, "gen:", err)
			os.Exit(2)
		}
	}
}

func write(path string, s corpus.Synthetic) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, corpus.Render(s))
}

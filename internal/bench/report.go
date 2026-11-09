//go:build !js

package bench

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

func fmtPSNR(v float64) string {
	switch {
	case math.IsNaN(v):
		return ""
	case math.IsInf(v, 1):
		return "inf"
	default:
		return strconv.FormatFloat(v, 'f', 2, 64)
	}
}

func note(r Result) string {
	if r.Skipped != "" {
		return r.Skipped
	}
	return r.Err
}

// cell flattens newlines and escapes pipes so a value cannot break a row.
func cell(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.ReplaceAll(s, "|", `\|`)
}

// WriteMarkdown writes rs as a GitHub-flavoured markdown table.
func WriteMarkdown(w io.Writer, rs []Result) error {
	if _, err := fmt.Fprintln(w, "| Image | Tool | PSNR dB | Paths | Nodes | Bytes | ms | Note |"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "|---|---|---:|---:|---:|---:|---:|---|"); err != nil {
		return err
	}
	for _, r := range rs {
		_, err := fmt.Fprintf(w, "| %s | %s | %s | %d | %d | %d | %d | %s |\n",
			cell(r.Image), cell(r.Tool), fmtPSNR(r.PSNR), r.Stats.Paths, r.Stats.Nodes, r.Stats.Bytes, r.Millis, cell(note(r)))
		if err != nil {
			return err
		}
	}
	return nil
}

// WriteCSV writes rs with a fixed header.
func WriteCSV(w io.Writer, rs []Result) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"image", "tool", "psnr", "paths", "nodes", "bytes", "ms", "skipped", "err"}); err != nil {
		return err
	}
	for _, r := range rs {
		rec := []string{
			r.Image, r.Tool, fmtPSNR(r.PSNR),
			strconv.Itoa(r.Stats.Paths), strconv.Itoa(r.Stats.Nodes), strconv.Itoa(r.Stats.Bytes),
			strconv.FormatInt(r.Millis, 10), r.Skipped, r.Err,
		}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

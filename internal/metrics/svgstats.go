package metrics

import "bytes"

// SVGStats summarises an SVG document's size.
type SVGStats struct {
	Paths int // number of <path elements
	Nodes int // number of path-data command letters across all d attributes
	Bytes int // len(svg)
}

const pathCommands = "MmLlHhVvCcSsQqTtAaZz"

// Stats counts <path elements and path-data command letters. It scans text
// rather than parsing XML, which is enough for tracer output.
func Stats(svg []byte) SVGStats {
	s := SVGStats{Bytes: len(svg), Paths: bytes.Count(svg, []byte("<path"))}
	rest := svg
	for {
		i := bytes.Index(rest, []byte(" d="))
		if i < 0 {
			break
		}
		rest = rest[i+3:]
		if len(rest) == 0 {
			break
		}
		q := rest[0]
		if q != '"' && q != '\'' {
			continue
		}
		rest = rest[1:]
		end := bytes.IndexByte(rest, q)
		if end < 0 {
			break
		}
		for _, c := range rest[:end] {
			if bytes.IndexByte([]byte(pathCommands), c) >= 0 {
				s.Nodes++
			}
		}
		rest = rest[end+1:]
	}
	return s
}

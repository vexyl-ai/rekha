package metrics

import "bytes"

// SVGStats summarises an SVG document's size.
type SVGStats struct {
	Paths int // number of <path elements
	Nodes int // number of path segments across all d attributes (see Stats)
	Bytes int // len(svg)
}

// arity is the number of coordinates each path command consumes per
// segment. Implicit repeats (numbers after a command's first segment) count
// as further segments of the same command, or as L/l after M/m.
var arity = map[byte]int{
	'M': 2, 'L': 2, 'T': 2, 'H': 1, 'V': 1, 'C': 6, 'S': 4, 'Q': 4, 'A': 7, 'Z': 0,
	'm': 2, 'l': 2, 't': 2, 'h': 1, 'v': 1, 'c': 6, 's': 4, 'q': 4, 'a': 7, 'z': 0,
}

// Stats counts <path elements and path segments. A segment is one drawing
// command instance, explicit or implicit, so "M0 0 1 1 2 2" is three segments
// (a move and two implicit lines) and Potrace-style "l1 1 2 2" is two. Z
// counts as one. The scan is textual, not XML, which is enough for tracer
// output.
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
		s.Nodes += countSegments(rest[:end])
		rest = rest[end+1:]
	}
	return s
}

func countSegments(d []byte) int {
	n := 0
	var cmd byte
	need := 0 // coordinates still needed to complete the current segment
	i := 0
	for i < len(d) {
		c := d[i]
		if a, ok := arity[c]; ok {
			cmd = c
			need = a
			if a == 0 {
				n++
			}
			i++
			continue
		}
		if isNumStart(c) {
			j := scanNumber(d, i)
			i = j
			if cmd == 0 {
				continue // numbers before any command are ignored
			}
			need--
			if need == 0 {
				n++
				// Implicit repeat: M/m continue as L/l, others repeat.
				switch cmd {
				case 'M':
					cmd = 'L'
				case 'm':
					cmd = 'l'
				}
				need = arity[cmd]
			}
			continue
		}
		i++ // whitespace, commas, anything else
	}
	return n
}

func isNumStart(c byte) bool {
	return (c >= '0' && c <= '9') || c == '-' || c == '+' || c == '.'
}

// scanNumber returns the index just past the number starting at i. It
// handles signs, decimals and exponents. A sign or a second '.' starts a new
// number, matching SVG's compact path syntax ("1.5-2.5", "1.5.5").
func scanNumber(d []byte, i int) int {
	j := i
	if d[j] == '-' || d[j] == '+' {
		j++
	}
	seenDot := false
	for j < len(d) {
		c := d[j]
		switch {
		case c >= '0' && c <= '9':
			j++
		case c == '.' && !seenDot:
			seenDot = true
			j++
		case (c == 'e' || c == 'E') && j+1 < len(d) && (isDigit(d[j+1]) || ((d[j+1] == '-' || d[j+1] == '+') && j+2 < len(d) && isDigit(d[j+2]))):
			j += 2
			for j < len(d) && isDigit(d[j]) {
				j++
			}
			return j
		default:
			return j
		}
	}
	return j
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

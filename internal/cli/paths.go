// Package cli holds helpers shared by the rekha command-line entry point.
package cli

import "strings"

// OutputPath maps an input path to its output path. "-" maps to "-".
// Otherwise the extension is replaced by "." + ext; a file with no
// extension gets it appended. If explicit != "" it is returned unchanged.
func OutputPath(input, explicit, ext string) string {
	if explicit != "" {
		return explicit
	}
	if input == "-" {
		return "-"
	}
	base := input
	if i := strings.LastIndexByte(input, '/'); i >= 0 {
		base = input[i+1:]
	}
	if dot := strings.LastIndexByte(base, '.'); dot > 0 {
		return input[:len(input)-len(base)+dot] + "." + ext
	}
	return input + "." + ext
}

// FormatOf picks the output format: the --format flag when given, else
// the output path's extension when it names a format, else svg.
func FormatOf(flag, output string) string {
	if flag != "" {
		return strings.ToLower(flag)
	}
	if dot := strings.LastIndexByte(output, '.'); dot >= 0 && !strings.Contains(output[dot:], "/") {
		switch ext := strings.ToLower(output[dot+1:]); ext {
		case "svg", "pdf", "eps":
			return ext
		}
	}
	return "svg"
}

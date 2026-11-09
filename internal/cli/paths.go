// Package cli holds helpers shared by the rekha command-line entry point.
package cli

import "strings"

// OutputPath maps an input path to its output path. "-" maps to "-".
// Otherwise the extension is replaced by ".svg"; a file with no extension
// gets ".svg" appended. If explicit != "" it is returned unchanged.
func OutputPath(input, explicit string) string {
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
		return input[:len(input)-len(base)+dot] + ".svg"
	}
	return input + ".svg"
}

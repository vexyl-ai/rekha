//go:build !js

// Package bench runs external tracers as black boxes over a corpus and scores
// their SVG output against the input pixels.
package bench

// Tool describes how to invoke one tracer.
type Tool struct {
	Name  string
	Bin   string   // looked up with exec.LookPath
	Args  []string // tokens; "{in}" and "{out}" are substituted
	Input string   // "png" or "bmp"; bmp inputs are converted from the PNG
}

// BuiltinTools returns the reference tracers the harness knows about.
func BuiltinTools() []Tool {
	return []Tool{
		{Name: "potrace", Bin: "potrace", Args: []string{"--svg", "-o", "{out}", "{in}"}, Input: "bmp"},
		{Name: "vtracer", Bin: "vtracer", Args: []string{"--input", "{in}", "--output", "{out}"}, Input: "png"},
	}
}

// RekhaTool returns the tool entry for a rekha binary at bin, run with the
// bw preset so the comparison with the bilevel reference tools is fair.
func RekhaTool(bin string) Tool {
	return Tool{Name: "rekha", Bin: bin, Args: []string{"--preset", "bw", "-o", "{out}", "{in}"}, Input: "png"}
}

// ByName returns the builtin tools whose names appear in names, in the
// order given. "rekha" resolves to RekhaTool("rekha"); the caller replaces
// its Bin with the configured path. Unknown names are returned in the
// second value.
func ByName(names []string) ([]Tool, []string) {
	all := append(BuiltinTools(), RekhaTool("rekha"))
	var tools []Tool
	var unknown []string
	for _, n := range names {
		found := false
		for _, t := range all {
			if t.Name == n {
				tools = append(tools, t)
				found = true
				break
			}
		}
		if !found {
			unknown = append(unknown, n)
		}
	}
	return tools, unknown
}

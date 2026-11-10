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
	// Dir, when set, names a directory of hand-downloaded outputs instead
	// of a binary: for corpus image <name>.png the harness scores
	// <Dir>/<name>.svg (PSNR, paths, nodes, bytes; no time) or else
	// <Dir>/<name>.png (PSNR only). Neither present gives the row the
	// note "no reference". Bin, Args and Input are ignored.
	Dir string
}

// Reference directories of the web tools (see docs/benchmark.md). Their
// contents are git-ignored except the README.
const (
	VectorMagicDir  = "bench/reference/vectormagic"
	VectorizerAIDir = "bench/reference/vectorizer-ai"
)

// BuiltinTools returns the reference tracers the harness knows about.
func BuiltinTools() []Tool {
	return []Tool{
		{Name: "potrace", Bin: "potrace", Args: []string{"--svg", "-o", "{out}", "{in}"}, Input: "bmp"},
		{Name: "vtracer", Bin: "vtracer", Args: []string{"--input", "{in}", "--output", "{out}"}, Input: "png"},
		{Name: "vectormagic", Dir: VectorMagicDir},
		{Name: "vectorizer-ai", Dir: VectorizerAIDir},
	}
}

// RekhaTool returns the tool entry for a rekha binary at bin, run with the
// bw preset so the comparison with the bilevel reference tools is fair.
func RekhaTool(bin string) Tool {
	return rekhaTool(bin, "rekha", "bw")
}

// RekhaTools returns one tool per preset for the rekha binary at bin:
// rekha (bw), rekha-poster and rekha-photo.
func RekhaTools(bin string) []Tool {
	return []Tool{
		RekhaTool(bin),
		rekhaTool(bin, "rekha-poster", "poster"),
		rekhaTool(bin, "rekha-photo", "photo"),
		rekhaTool(bin, "rekha-refine", "bw", "--refine"),
		rekhaTool(bin, "rekha-poster-refine", "poster", "--refine"),
	}
}

// rekhaTool is a rekha invocation under preset with extra flags before the
// output and input placeholders.
func rekhaTool(bin, name, preset string, extra ...string) Tool {
	args := append([]string{"--preset", preset}, extra...)
	args = append(args, "-o", "{out}", "{in}")
	return Tool{Name: name, Bin: bin, Args: args, Input: "png"}
}

// ByName returns the builtin tools whose names appear in names, in the
// order given. The rekha tools resolve with Bin "rekha"; the caller
// replaces it with the configured path. Unknown names are returned in the
// second value.
func ByName(names []string) ([]Tool, []string) {
	all := append(BuiltinTools(), RekhaTools("rekha")...)
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

//go:build !js

package bench

import (
	"strings"
	"testing"
)

func TestRekhaToolsRefine(t *testing.T) {
	tools, unknown := ByName([]string{"rekha-refine", "rekha-poster-refine"})
	if len(unknown) != 0 || len(tools) != 2 {
		t.Fatalf("tools %v unknown %v", tools, unknown)
	}
	want := map[string]string{
		"rekha-refine":        "--preset bw --refine -o {out} {in}",
		"rekha-poster-refine": "--preset poster --refine -o {out} {in}",
	}
	for _, tool := range tools {
		if got := strings.Join(tool.Args, " "); got != want[tool.Name] || tool.Input != "png" {
			t.Fatalf("%s args %q input %q", tool.Name, got, tool.Input)
		}
	}
}

func TestBuiltinToolsIncludeReferences(t *testing.T) {
	want := map[string]string{
		"vectormagic":   "bench/reference/vectormagic",
		"vectorizer-ai": "bench/reference/vectorizer-ai",
	}
	for _, tool := range BuiltinTools() {
		if dir, ok := want[tool.Name]; ok {
			if tool.Dir != dir || tool.Bin != "" {
				t.Fatalf("%s: dir %q bin %q", tool.Name, tool.Dir, tool.Bin)
			}
			delete(want, tool.Name)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing reference tools %v", want)
	}
	tools, unknown := ByName([]string{"vectormagic", "vectorizer-ai"})
	if len(tools) != 2 || len(unknown) != 0 {
		t.Fatalf("ByName: %v unknown %v", tools, unknown)
	}
}

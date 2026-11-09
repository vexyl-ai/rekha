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

package bench

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestSmallGlyphsBeatsPotrace is the Phase 3b small-feature exit test:
// the bw preset in plain spline mode scores at least Potrace's PSNR on
// small-glyphs with no more nodes, measured by the same harness and
// rasteriser as the benchmark. It skips where potrace or rsvg-convert is
// not installed.
func TestSmallGlyphsBeatsPotrace(t *testing.T) {
	for _, bin := range []string{"potrace", "rsvg-convert", "go"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not installed", bin)
		}
	}
	dir := t.TempDir()
	rekha := filepath.Join(dir, "rekha")
	build := exec.Command("go", "build", "-o", rekha, "../../cmd/rekha")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	corpus := filepath.Join(dir, "corpus")
	if err := os.MkdirAll(corpus, 0o755); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile("../../testdata/corpus/synthetic/small-glyphs.png")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(corpus, "small-glyphs.png"), src, 0o644); err != nil {
		t.Fatal(err)
	}
	tools := append([]Tool{RekhaTool(rekha)}, BuiltinTools()[0])
	rs, err := (&Runner{}).Run(context.Background(), corpus, tools)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]Result{}
	for _, r := range rs {
		by[r.Tool] = r
	}
	p, r := by["potrace"], by["rekha"]
	if p.Err != "" || r.Err != "" || p.Skipped != "" || r.Skipped != "" {
		t.Fatalf("potrace %+v rekha %+v", p, r)
	}
	t.Logf("potrace %.2f dB %d nodes; rekha %.2f dB %d nodes", p.PSNR, p.Stats.Nodes, r.PSNR, r.Stats.Nodes)
	if r.PSNR < p.PSNR {
		t.Errorf("rekha %.2f dB below potrace %.2f dB", r.PSNR, p.PSNR)
	}
	if r.Stats.Nodes > p.Stats.Nodes {
		t.Errorf("rekha %d nodes above potrace %d", r.Stats.Nodes, p.Stats.Nodes)
	}
}

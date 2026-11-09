package metrics

import "testing"

func TestStatsCountsPathsAndNodes(t *testing.T) {
	svg := []byte(`<svg><path d="M0 0L1 1 2 2Z"/><path d="m1,1c1 1 2 2 3 3z"/></svg>`)
	got := Stats(svg)
	want := SVGStats{Paths: 2, Nodes: 6, Bytes: len(svg)}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestStatsEmpty(t *testing.T) {
	got := Stats([]byte("<svg/>"))
	want := SVGStats{Paths: 0, Nodes: 0, Bytes: 6}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

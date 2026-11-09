package metrics

import "testing"

func TestStatsCountsPathsAndNodes(t *testing.T) {
	svg := []byte(`<svg><path d="M0 0L1 1 2 2Z"/><path d="m1,1c1 1 2 2 3 3z"/></svg>`)
	got := Stats(svg)
	// M, L, implicit L, Z = 4; m, c, z = 3.
	want := SVGStats{Paths: 2, Nodes: 7, Bytes: len(svg)}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestStatsCountsImplicitSegments(t *testing.T) {
	cases := []struct {
		d    string
		want int
	}{
		{"M0 0 10 10 20 20Z", 4},                    // M + 2 implicit L + Z
		{"m5,5l-115 -332 -357 -9z", 4},              // m, l, implicit l, z
		{"M0 0c1 1 2 2 3 3 4 4 5 5 6 6", 3},         // M + 2 cubics
		{"M0 0h10v10h-10z", 5},                      // M h v h z
		{"M0 0 a5 5 0 0 1 10 0 5 5 0 0 1 -10 0", 3}, // M + 2 arcs
		{"M1.5-2.5e1L3,4", 2},                       // signs, exponents, commas
		{"M0 0 S1 1 2 2 Q3 3 4 4 T5 5", 4},          // M S Q T
		{"M 0 0 L 1 1 M 2 2 L 3 3 z", 5},            // two subpaths
	}
	for _, c := range cases {
		svg := []byte(`<svg><path d="` + c.d + `"/></svg>`)
		if got := Stats(svg).Nodes; got != c.want {
			t.Errorf("Nodes(%q) = %d, want %d", c.d, got, c.want)
		}
	}
}

func TestStatsEmpty(t *testing.T) {
	got := Stats([]byte("<svg/>"))
	want := SVGStats{Paths: 0, Nodes: 0, Bytes: 6}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

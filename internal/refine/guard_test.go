package refine

import "testing"

func TestWithinBudgetRelativeClause(t *testing.T) {
	if !withinBudget(100, 101, 1000, 10) {
		t.Fatal("1 % rise on a rough loop must pass")
	}
	if withinBudget(100, 109, 1000, 10) {
		t.Fatal("9 % on a rough loop, 900 per px, must fail")
	}
}

func TestWithinBudgetAbsoluteClause(t *testing.T) {
	// Pokéball ring: E 2.73 → 2.98 over the whole loop (l0 == length).
	if !withinBudget(2.73, 2.98, 5530, 5530) {
		t.Fatal("+0.25 per boundary px must pass")
	}
	// A wrong tangent candidate: 2.73 → 7.69.
	if withinBudget(2.73, 7.69, 5530, 5530) {
		t.Fatal("+4.96 per boundary px must fail")
	}
}

func TestWithinBudgetRelativeCap(t *testing.T) {
	// A quarter ellipse snapped to a circle on a noise-free mask: tenfold
	// rise, 0.13 per boundary pixel. The floor must not admit it.
	if withinBudget(0.0019, 0.0197, 230, 30.8) {
		t.Fatal("a tenfold rise must fail whatever it costs per pixel")
	}
	// A corner refitted off its true radius: +85 %, 0.011 per pixel.
	if withinBudget(0.0049, 0.0091, 358.6, 141.4) {
		t.Fatal("+85 % must fail")
	}
	// Within the cap but over the floor on a long loop: fails.
	if withinBudget(2.73, 3.5, 5530, 5530) {
		t.Fatal("+28 % must fail")
	}
}

# Primitives v2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `--refine` primitive snapping fire on real logos: whole circles, arcs with any kind of end, mixed radii, no polygonal arcs, sharp corners, under a guard that admits a near-perfect fit becoming an exact circle.

**Architecture:** In `internal/refine`, split-and-fit *pieces* (line / arc / free, whole refined segments between hard cuts) replace today's `arcRuns`; each arc end is typed (point or tangent) from the joint it sits on and the circle is solved per trial radius; arcs are rebuilt first, then corner fillets dropped, then line pieces collapsed; every snap passes a two-clause guard (2 % relative rise, or an absolute floor of `primAbs` energy per boundary pixel). `SnapCircle` gains a radius refit. Five synthetic shapes pin the behaviour; a bench file records the real-logo numbers.

**Tech Stack:** Go 1.26, `CGO_ENABLED=0`, only `golang.org/x/image`. Existing helpers reused: `kasa`, `Circle`, `arcPieces`, `replaceRun`, `tangentCircle`, `throughAndTangent`, `centreAt`, `fineDataEnergy`, `windowAround`, `segmentsBox`, `fit.FitLine`, `fit.Intersect`, `AnglePrior`, `straightCubic`.

**Spec:** `docs/superpowers/specs/2026-11-14-primitives-v2-design.md`

## Global Constraints

- Go 1.26, `CGO_ENABLED=0`, no new dependencies, wasm must build.
- `internal/refine` and `internal/fit`: keyed `geom.Vec2` literals; wrap float products in `float64()` where two products are summed (FMA determinism between arm64 and amd64, see existing code such as `tangentCircle`).
- Pixel and polygon goldens byte-identical. Spline goldens change only with a PSNR justification (`go test ./ -run Golden -update` after checking). Refined goldens compare as geometry (nodes within 10 %, 36 dB).
- Output identical for `GOMAXPROCS=1` and `8`; `go test -fuzz` targets stay clean; no panic, no NaN.
- Before every public commit on the integration branch: `make lint test wasm-check wasm wasm-test web-test` and `GOARCH=amd64 go test ./ ./internal/gradient/` green. Per-task commits need `go test ./internal/refine/ ./` green.
- `git commit -s`; never add a `Co-Authored-By` or any AI attribution trailer; never `git add -A` (untracked `tests/`, `adobe-express-output/`, `Logo.jpg`, `8_1sasa11.jpg`, `out*.svg` must stay out). Delete any `zz_debug*` file before gates.
- Constants from the spec: `tol = 0.5·FitTolerance` (caller), `circleRelTol = 0.1`, `circleSamples = 8`, `arcJointTurn = 5°`, `pieceSweep = π/2·1.06`, `radiusSearch = 0.1`, `radiusSteps = 16`, `primRise = 0.02`, `primAbs = 1.0`, `cornerFillet = 3`, `cornerReach = 6`, `lineTol = 0.25` (trace.go, unchanged).
- Real-logo images live in `/Users/anilmathew/Documents/AICoding/rekha-cloud/spike-in/` (private); never copy them into the public repo. Never read any `key.txt`.

## Review Focus

Inputs the spec implies but the tasks' tests only partly cover; each gets a pinned test in the task named.

1. A loop whose every joint is fixed and every segment frozen (a fully shared boundary in cutout mode) reaches `SnapPieces`: nothing moves, no index goes out of range. → Task 5 `TestSnapPiecesAllFixedUnchanged`.
2. An arc piece whose point ends are further apart than twice any trial radius (a perturbed near-semicircle whose Kåsa radius is slightly under half the chord): the piece is left alone, no NaN enters the bezigon. → Task 5 `TestSnapPiecesChordBeyondDiameter`.
3. A loop of two segments (`b.N() == 2`) or one: `pieces` returns at most one free piece and `SnapPieces` returns the loop unchanged. → Task 3 `TestPiecesTinyLoops`.
4. A 500-segment loop (a 2048-px poster outline) must not make split-and-fit quadratic in wall time: `BenchmarkRefineBigPoster` within 1.2× of main. → Task 8 measurement; Task 3 `TestPiecesSplitDepthBounded` asserts a 512-segment noisy loop finishes under 200 ms.
5. A corner fillet beside a fixed joint (a junction of three colours at a sharp corner) must never be dropped: the junction anchor cannot move. → Task 6 `TestSharpenCornerKeepsFixedJoint`.

---

### Task 1: Synthetic shapes `half-disc`, `annular-sector`, `fillet-arc`, `triangle`, `ring`

**Files:**
- Modify: `internal/corpus/synthetic.go` (the `All()` list, new helpers)
- Modify: `internal/corpus/synthetic_test.go`
- Modify: `colour_test.go:84-90` (refine golden case list)
- Create: `testdata/corpus/synthetic/{half-disc,annular-sector,fillet-arc,triangle,ring}.png` (generated)
- Create: goldens for the five under every family the golden tests cover (generated with `-update`)

**Interfaces:**
- Produces: `corpus.TriangleCorners() [3]geom.Vec2`, `corpus.FilletDGeometry() (centre geom.Vec2, r, fillet, chordX float64)`, `corpus.RingRadii() (outer, inner float64)`; shape names as above, 256 px, 4×4 samples, black on white.

Geometry (all centred on (128,128) unless said):
- `half-disc`: `disc(128,128,80)` ∩ `x ≥ 128`.
- `annular-sector`: `60 ≤ |p−c| ≤ 100` and the angle of `p−c` within ±60° of the +x axis (120° sweep).
- `fillet-arc`: `disc(128,128,100)` ∩ `x ≥ 60`, the two chord corners rounded by r = 12 fillets tangent to the chord and internally tangent to the circle: fillet centres `(72, 128 ± 67.882)` (because `(72−128)² + dy² = 88²`); a point with `x < 72` and `|y−128| > 67.882` is inside only if inside its fillet disc.
- `triangle`: corners `(38,196)`, `(218,196)`, `(105.5,96.78)` (sides 180, 150, 120; angles 55.8°, 41.4°, 82.8°).
- `ring`: `disc(128,128,110)` minus `disc(128,128,80)`.

- [ ] **Step 1: Failing test for the exported geometry helpers**

In `internal/corpus/synthetic_test.go`:

```go
func TestNewPrimitiveShapesRender(t *testing.T) {
	for _, name := range []string{"half-disc", "annular-sector", "fillet-arc", "triangle", "ring"} {
		s, ok := byName(name) // small helper over All()
		if !ok { t.Fatalf("%s missing", name) }
		img := Render(s)
		if img.Bounds().Dx() != 256 { t.Fatalf("%s: %v", name, img.Bounds()) }
	}
	tc := TriangleCorners()
	if d := tc[1].Sub(tc[0]).Len(); math.Abs(d-180) > 1e-9 { t.Fatalf("AB = %v", d) }
	if o, i := RingRadii(); o != 110 || i != 80 { t.Fatal(o, i) }
	c, r, f, x := FilletDGeometry()
	if c != (geom.Vec2{X: 128, Y: 128}) || r != 100 || f != 12 || x != 60 { t.Fatal(c, r, f, x) }
}
```

- [ ] **Step 2: Run, expect FAIL** `go test ./internal/corpus/ -run TestNewPrimitiveShapesRender` → undefined: TriangleCorners.
- [ ] **Step 3: Add the five shapes to `All()` and the three helpers.** Use `fill(img, inside, black, 4)` with closures built from `disc`, half-planes and the fillet rule above.
- [ ] **Step 4: Run, expect PASS.** Also `go test ./internal/corpus/` (determinism test still green).
- [ ] **Step 5: Regenerate images and goldens**

```bash
go run ./internal/corpus/gen -out testdata/corpus/synthetic
go test ./ -run 'Golden' -update
go test ./
```
Expected: only the five new names appear as new files under `testdata/golden/` and `testdata/corpus/synthetic/`; `git status` shows no modified existing golden (byte-identical for the others).

- [ ] **Step 6: Add the five to the refine golden list** in `colour_test.go` (`PresetBW` each), run `go test ./ -run Golden -update` again, then `go test ./`.
- [ ] **Step 7: Commit**

```bash
git add internal/corpus testdata/corpus/synthetic testdata/golden colour_test.go
git commit -s -m "corpus: half-disc, annular-sector, fillet-arc, triangle and ring for the primitive pass"
```

---

### Task 2: The guard and the whole-circle radius refit

**Files:**
- Create: `internal/refine/guard.go`
- Modify: `internal/refine/primitives.go` (`budget`, `withinRise`, `guardedSnap` call the new guard)
- Modify: `internal/refine/circle.go` (`SnapCircle`)
- Test: `internal/refine/guard_test.go`, `internal/refine/circle_test.go`

**Interfaces:**
- Produces: `const primAbs = 1.0`; `func withinBudget(before, after, l0, length float64) bool` (spec §5: `after−before ≤ primRise·before || (after−before)·l0/length ≤ primAbs`); `func segmentsLength(b Bezigon, from, to int) float64` (polyline length of segments from..to cyclic, Flatten chords); `withinRise(b, cand, m, img, bFrom, bTo, cFrom, cTo)` keeps its signature and now uses `withinBudget` with `length = segmentsLength(b, bFrom, bTo)`.
- `SnapCircle(b, m, img, tol) (Bezigon, bool)` keeps its signature; it golden-sections the radius within `radiusSearch` of the chord-corrected Kåsa radius (16 steps) on `fineDataEnergy` over the loop's window, then guards the best.

- [ ] **Step 1: Failing guard tests**

```go
func TestWithinBudgetRelativeClause(t *testing.T) {
	if !withinBudget(100, 101, 1000, 10) { t.Fatal("1 % rise on a rough loop must pass") }
	if withinBudget(100, 109, 1000, 10) { t.Fatal("9 % on a rough loop, 900 per px, must fail") }
}
func TestWithinBudgetAbsoluteClause(t *testing.T) {
	// Pokéball ring: E 2.73 → 2.98 over the whole loop (l0 == length).
	if !withinBudget(2.73, 2.98, 5530, 5530) { t.Fatal("+0.25 per boundary px must pass") }
	// A wrong tangent candidate: 2.73 → 7.69.
	if withinBudget(2.73, 7.69, 5530, 5530) { t.Fatal("+4.96 per boundary px must fail") }
}
```

- [ ] **Step 2: Run, expect FAIL** (undefined: withinBudget).
- [ ] **Step 3: Write `guard.go`** (`primAbs`, `withinBudget`, `segmentsLength`); replace `budget()` in `withinRise` and `guardedSnap` (`guardedSnap` passes `length = segmentsLength(b, run.first-lo, run.first+run.count-1+hi)`); delete `budget`.
- [ ] **Step 4: Run, expect PASS**, and `go test ./internal/refine/` still green (existing primitive tests rely on the relative clause and must not change behaviour).
- [ ] **Step 5: Failing whole-circle test** in `circle_test.go`

```go
// TestSnapCircleAcceptsNearPerfectRing: an 8-cubic outline of a circle
// of r = 110, each anchor nudged ±0.15 px radially, refined against the
// exact circle's mask, becomes four cubics even though the free outline
// fits the mask a little better than any circle (the Pokéball case).
func TestSnapCircleAcceptsNearPerfectRing(t *testing.T) {
	exact := Circle(v(128, 128), 110, v(238, 128), true)
	m := supersampled(exact, Window{0, 0, 256, 256})
	b := nudgedCircle(v(128, 128), 110, 8, 0.15) // 8 arc cubics, anchors alternately in/out
	out, ok := SnapCircle(b, m, Window{0, 0, 256, 256}, 0.5)
	if !ok || out.N() != 4 { t.Fatalf("ok=%v N=%d", ok, out.N()) }
	if r := out.Pts[0].Sub(v(128, 128)).Len(); math.Abs(r-110) > 0.1 { t.Fatalf("r = %.3f", r) }
}
```
(`nudgedCircle` is a test helper: `arcSeg` pieces of 45° with each anchor moved radially by ±d, handles rebuilt from the moved anchors.) Before the fix this test documents today's behaviour: run it once with the old guard to confirm it fails for the right reason (reverted, `ok == false`), not because `FitCircle` rejects.

- [ ] **Step 6: Run, expect FAIL** with `ok=false N=8`.
- [ ] **Step 7: Implement the radius refit in `SnapCircle`** (golden section over `fineDataEnergy`, window = `windowAround(img, segmentsBox(b,0,N-1), segmentsBox(cand,0,3))`, `l0 = max(b.Length(),1)`, then `withinBudget(before, best, l0, l0)`).
- [ ] **Step 8: Run, expect PASS**; `go test ./internal/refine/ ./` green (`TestRefineCircleAAIsFourCubics`, `TestFitCircleRejectsSmallSquare` unchanged).
- [ ] **Step 9: Commit**

```bash
git add internal/refine/guard.go internal/refine/guard_test.go internal/refine/primitives.go internal/refine/circle.go internal/refine/circle_test.go
git commit -s -m "refine: the primitive guard gains an absolute floor, and a whole circle refits its radius"
```

---

### Task 3: Pieces: hard cuts, split-and-fit, re-merge, classify

**Files:**
- Create: `internal/refine/pieces.go`
- Test: `internal/refine/pieces_test.go`

**Interfaces:**
- Produces:
  ```go
  type pieceKind uint8
  const (pieceFree pieceKind = iota; pieceLine; pieceArc)
  type piece struct {
      first, count int        // segments first..first+count-1, never wrapping
      kind         pieceKind
      centre       geom.Vec2  // arc
      r            float64    // arc
      ccw          bool       // arc: increasing angle along the piece
      p, dir       geom.Vec2  // line: a point and unit direction, from fit.FitLine
  }
  func hardCuts(b Bezigon, o Options) []bool            // nil when no joint is a cut
  func fitSpan(b Bezigon, first, count int, tol float64) (piece, bool)
  func loopPieces(b Bezigon, tol float64, o Options) (Bezigon, Options, []piece)
  ```
  `hardCuts`: joint `a` is a cut when `o.fixed(a)`, `o.frozen(a)`, `o.frozen(a−1)`, or `AnglePrior(b, a) ≥ arcJointTurn·π/180`. `fitSpan`: samples as `samples(b, first, count)`; circle fits when Kåsa succeeds and `onCircle(pts, c, r, min(tol, circleRelTol·r))`; line fits when every sample is within `tol` of `fit.FitLine(pts)`; kind is `pieceLine` when the line fits and the circle's sagitta over the span's chord (`r − sqrt(r² − (chord/2)²)`, or ∞ when the circle does not fit or chord > 2r) is under `tol`, else `pieceArc` when the circle fits, else `ok == false`. Frozen segments are always their own `pieceFree`. `loopPieces`: rotates `b` (and `o`, as `rotateToCut` does) to start at a hard cut; when there is none, cuts at the joint nearest the sample furthest from the loop's Kåsa circle and rotates there; then split-and-fit each span recursively (split at the joint nearest the sample of greatest radial deviation from the span's Kåsa circle, or at the middle joint when Kåsa fails), re-merge adjacent non-free pieces whose union `fitSpan`s, and return the pieces in segment order covering every segment exactly once (free pieces fill the gaps).

- [ ] **Step 1: Failing tests** (helpers `v`, `ln`, `arcSeg`, `cubic`, `FromSegments` from existing test files)

```go
// filletD: line (10,10)→(110,10), arc r=100 centre (110,110) −90°→0°,
// fillet r=12 centre (198,110) 0°→90°, line (198,122)→(10,122), line up.
// Every joint but (10,10) and (10,122) is smooth.
func TestPiecesSplitMixedRadii(t *testing.T) {
	_, _, ps := loopPieces(filletD(), 0.5, Options{})
	kinds := kindsOf(ps) // e.g. "line arc arc line line" after rotation to (10,10)
	// expect 5 pieces: line, arc(r≈100), arc(r≈12), line, line
	// assert arc radii within 1 % and the arcs' counts 1 each
}
func TestPiecesMergeHealsSplit(t *testing.T) {
	// 180° arc r=80 as four 45° cubics, then a 2-cubic fillet r=8 flowing
	// smoothly into a line, closed by lines with corners: expect exactly
	// one piece with r≈80 and count 4, and one with r≈8 and count 2.
}
func TestPiecesShortArcLeftoverIsNotLine(t *testing.T) {
	// arc r=700 sweep 10° as five 2° cubics between two radial lines
	// (corners at both ends): one pieceArc with count 5, no pieceLine
	// among the five, even though each cubic alone sits within 0.11 px
	// of its chord.
}
func TestPiecesUncutLoopGetsFirstCut(t *testing.T) {
	// rounded rect 160×160 r=24 as 4 lines + 4 quarter arcs, all smooth:
	// hardCuts == nil; loopPieces returns 8 pieces, 4 line and 4 arc.
}
func TestPiecesRespectFixedAndFrozen(t *testing.T) {
	// the 180° arc with Fixed at its middle joint → two arc pieces;
	// with Frozen on one of its cubics → that cubic is pieceFree.
}
func TestPiecesTinyLoops(t *testing.T) {
	// N == 1 and N == 2: at most one piece, kind pieceFree, no panic.
}
func TestPiecesSplitDepthBounded(t *testing.T) {
	// 512 segments of a jittered circle (±1 px): loopPieces under 200 ms.
}
```

- [ ] **Step 2: Run, expect FAIL** (undefined: loopPieces).
- [ ] **Step 3: Write `pieces.go`** per the Interfaces block. Recursion: `split(b, first, count, tol, out []piece) []piece`; `merge(b, ps, tol) []piece` loops until no merge happened.
- [ ] **Step 4: Run, expect PASS**; `go vet ./internal/refine/`.
- [ ] **Step 5: Commit**

```bash
git add internal/refine/pieces.go internal/refine/pieces_test.go
git commit -s -m "refine: a loop is cut into line, arc and free pieces by split-and-fit"
```

---

### Task 4: Arc ends and the per-radius circle solve

**Files:**
- Create: `internal/refine/joints.go`
- Test: `internal/refine/joints_test.go`

**Interfaces:**
- Consumes: `piece`, `pieceKind`, `loopPieces` (Task 3); `tangentCircle`, `throughAndTangent`, `centreAt`, `AnglePrior`, `straightCubic` (existing).
- Produces:
  ```go
  type endKind uint8
  const (endPoint endKind = iota; endTangent)
  type arcEnd struct {
      kind endKind
      at   geom.Vec2   // endPoint: the point the circle passes through
      far  geom.Vec2   // endTangent: the line's other anchor (line far→at or at→far)
      seg  int         // the neighbouring segment this end touches
      line bool        // the neighbour is a line piece (its end is rebuilt when the anchor moves)
  }
  func arcEnds(b Bezigon, o Options, ps []piece, i int, tol float64) [2]arcEnd
  func solveArc(ends [2]arcEnd, r float64, near geom.Vec2) (centre geom.Vec2, ok bool)
  ```
  `arcEnds` types each end of arc piece `ps[i]` per spec §4.2: fixed joint → point at the anchor; neighbour line piece with turn < `arcJointTurn` → tangent (far = the line piece's other end anchor); neighbour line piece with larger turn → point at the anchor's radial projection onto `ps[i]`'s circle; neighbour arc piece → point at the mean of the anchor's radial projections onto both circles; neighbour free piece (or no neighbour: the piece spans the whole rotated loop) → point at the anchor. `solveArc`: both tangent → `tangentCircle`; one tangent → `throughAndTangent`; none → `centreAt`.

- [ ] **Step 1: Failing tests**

```go
func TestArcEndsTangentBothSides(t *testing.T) {
	// quarterTwice() (existing helper): the arc piece's ends are both
	// endTangent; solveArc at r=20 returns centre (50,30) within 1e-6.
}
func TestArcEndsCornerOnLine(t *testing.T) {
	// half-disc bezigon: vertical line + 180° arc r=80 as 4 cubics with
	// anchors nudged 0.2 px: both ends endPoint, each within 0.2 px of
	// the true corners (128,48) and (128,208).
}
func TestArcEndsFixedIsPoint(t *testing.T) {
	// quarterTwice with Options{Fixed: ...} at the arc's first joint:
	// ends[0].kind == endPoint at the anchor exactly, ends[1] tangent.
}
func TestArcEndsArcArcMeanProjection(t *testing.T) {
	// fillet-arc loop (Task 3's filletD): the big arc's end beside the
	// fillet is endPoint; its `at` is within 1e-9 of the mean of the
	// anchor's projections on the two circles.
}
func TestSolveArcChordBeyondDiameter(t *testing.T) {
	// two points 100 apart, r = 40: ok == false.
}
```

- [ ] **Step 2: Run, expect FAIL.**
- [ ] **Step 3: Write `joints.go`.** Turn at a joint from `AnglePrior(b, joint)`. The tangent-fallback-to-point rule (a tangent solve that cannot keep the arc's samples within `lim`) lives in Task 5's snap, not here: `solveArc` only solves.
- [ ] **Step 4: Run, expect PASS.**
- [ ] **Step 5: Commit**

```bash
git add internal/refine/joints.go internal/refine/joints_test.go
git commit -s -m "refine: an arc's ends are typed from their joints and its circle solved per radius"
```

---

### Task 5: `SnapPieces` replaces `FitArcs`; `CommonRadius` over pieces

**Files:**
- Modify: `internal/refine/primitives.go` (remove `arcRun`, `arcRuns`, `cutJoints`, `curved`, `fitRun`, `runEnd`, `runEnds`, `rotateToCut`, `snapRun`, `FitArcs`; add `SnapPieces`, `snapPiece`, `rebuildLines`; adapt `guardedSnap`, `CommonRadius`)
- Modify: `internal/refine/primitives_test.go` (existing `FitArcs` tests call `SnapPieces`)
- Modify: `trace.go:95-109` (`refine.SnapPieces` in place of `refine.FitArcs`)
- Modify: `internal/refine/fuzz_test.go` if it names `FitArcs`

**Interfaces:**
- Consumes: Tasks 2–4.
- Produces:
  ```go
  func SnapPieces(b Bezigon, m Mask, img Window, tol float64, o Options) (Bezigon, Options)
  func CommonRadius(b Bezigon, m Mask, img Window, tol float64, o Options) (Bezigon, Options) // unchanged signature
  ```
  `SnapPieces`: `loopPieces`; arc pieces later-first through `guardedSnap` (now taking `ps []piece, i int` instead of `arcRun`; the golden-section search calls `snapPiece(b, o, ps, i, r, pieces, tol)` per trial radius; `snapPiece` = `arcEnds` → `solveArc` → when an end is tangent and the arc's samples leave `lim`, retype that end as a point at the anchor's radial projection and solve again → reject the trial when the point ends are unreachable → `arcPieces` + `replaceRun`, rebuilding a tangent end's line neighbour with `lineCubic` to the foot and moving a point end's anchor (and a line neighbour's end) to `at`); then (Task 6 hook) corner sharpening; then `rebuildLines`: each line piece becomes one `lineCubic` between its current end anchors, a line–line joint first moved to `fit.Intersect` of the two fitted lines when that is within `min(tol, 0.1·shorter chord)` of the anchor, each under `withinRise`. A rejected arc keeps its old control points and anchors. After a snap the pieces are renumbered (`replaceRun` already renumbers `Fixed`/`Frozen`; piece indices after `i` are unaffected because later pieces go first; piece `i` itself is updated to the new count).
  `CommonRadius`: `loopPieces` on the snapped loop, members among arc pieces within `radiusAgree` of the mean, each refitted at the mean through `guardedSnap` with `pieces = count` and `search = false`.

- [ ] **Step 1: Port the existing tests.** Rename calls `FitArcs(...)` → `SnapPieces(...)` in `primitives_test.go` (quarter circle, fixed ends, energy-rise revert, beside bowed line, tiny loops, fine energy). Run `go test ./internal/refine/`: FAIL with undefined `SnapPieces`.
- [ ] **Step 2: Failing new tests**

```go
func TestSnapPiecesHalfDisc(t *testing.T) {
	// vertical line + 180° arc r=80 (4 cubics, anchors nudged 0.2 px),
	// mask from the exact half-disc: result has 3 segments, 1 straight,
	// 2 arc cubics whose anchors lie within 0.1 px of the circle and
	// whose end anchors lie on the line x = 128 within 0.1 px.
}
func TestSnapPiecesAnnularSector(t *testing.T) {
	// radii 60/100, 120° sweep, radial lines, nudged 0.2 px: 6 segments,
	// 2 straight, 4 arcs; the two arcs' centres within 0.2 px of each
	// other; radii within 0.2 px of 60 and 100.
}
func TestSnapPiecesCollapsesCollinear(t *testing.T) {
	// a square whose top edge is three collinear segments: 4 segments out.
}
func TestSnapPiecesRejectedStaysContinuous(t *testing.T) {
	// two adjacent arc pieces; the mask agrees with the first and
	// disagrees with the second (drawn from a different radius): after
	// SnapPieces every segment's end equals the next segment's start.
}
func TestSnapPiecesAllFixedUnchanged(t *testing.T) {
	// quarterTwice with every joint Fixed and every segment Frozen:
	// output Pts equal input Pts.
}
func TestSnapPiecesChordBeyondDiameter(t *testing.T) {
	// a near-semicircle whose Kåsa radius is under half its chord after
	// nudging; output has no NaN and the piece's anchors are unchanged.
}
```

- [ ] **Step 3: Run, expect FAIL.**
- [ ] **Step 4: Implement `SnapPieces`, `snapPiece`, `rebuildLines`, adapt `guardedSnap` and `CommonRadius`, delete the old run code, switch `trace.go`.**
- [ ] **Step 5: Run `go test ./internal/refine/ ./`, expect PASS**, including `TestRoundedRectIsLinesAndArcs`, `TestRectRotFourNodes`, `TestPrimitivesNeverLoseOnGoalImages`, `TestRefineCircleAAIsFourCubics`. If a refine golden moves, check it renders within 36 dB of the old one (the golden test does) and note which in the commit body.
- [ ] **Step 6: Commit**

```bash
git add internal/refine/primitives.go internal/refine/primitives_test.go internal/refine/fuzz_test.go trace.go testdata/golden
git commit -s -m "refine: arcs are snapped per piece with ends typed by their joints, and collinear lines collapse"
```

---

### Task 6: Corner sharpening

**Files:**
- Create: `internal/refine/corners.go`
- Modify: `internal/refine/primitives.go` (`SnapPieces` calls `sharpenCorners` between arcs and lines)
- Test: `internal/refine/corners_test.go`

**Interfaces:**
- Consumes: `piece`, `withinRise`, `fit.Intersect`, `fit.FitLine`.
- Produces: `const cornerFillet = 3.0; const cornerReach = 2 * cornerFillet`; `func sharpenCorners(b Bezigon, o Options, ps []piece, m Mask, img Window, tol float64) (Bezigon, Options, []piece)`: for each piece (any kind) with chord ≤ `cornerFillet`, whose two joints are not fixed and whose segments are not frozen, between two `pieceLine` pieces of chord > `cornerFillet`, where `fit.Intersect` of the two lines lies within `cornerReach` of the fillet's midpoint: remove the piece's segments, end the previous line and start the next at the intersection (`lineCubic`), judged by `withinRise` over the fillet plus both neighbours; pieces renumbered. Later pieces first.

- [ ] **Step 1: Failing tests**

```go
func TestSharpenCornerDropsSmallFillet(t *testing.T) {
	// the Task 1 triangle as a bezigon whose corner at (38,196) is a
	// 2 px fillet cubic; mask from the sharp triangle: 3 segments out,
	// the corner within 0.15 px of (38,196).
}
func TestSharpenCornerKeepsLargeFillet(t *testing.T) {
	// same triangle with a 10 px fillet: 4 segments out, fillet kept.
}
func TestSharpenCornerKeepsFixedJoint(t *testing.T) {
	// the 2 px fillet with Fixed set on its first joint: 4 segments out.
}
```

- [ ] **Step 2: Run, expect FAIL.**
- [ ] **Step 3: Write `corners.go`; call it from `SnapPieces` after the arc pass and before `rebuildLines`.**
- [ ] **Step 4: Run, expect PASS**; `go test ./internal/refine/ ./`.
- [ ] **Step 5: Commit**

```bash
git add internal/refine/corners.go internal/refine/corners_test.go internal/refine/primitives.go
git commit -s -m "refine: a fillet shorter than three pixels between two lines becomes their corner"
```

---

### Task 7: Integration tests on the synthetic shapes and the arc-share helper

**Files:**
- Modify: `refine_test.go` (root)
- Modify: `testdata/golden/*-refine-spline.svg` as justified

**Interfaces:**
- Consumes: Task 1 shapes and helpers; the pipeline through `trace.go`.
- Produces: `func isArc(c fit.Cubic) bool` (samples at t = ¼, ½, ¾ within 0.02 px of the circle through the ends and the t = ½ point, or within 1e-4·r) and `func arcShare(segs []fit.Segment) (curved, arcs int)` test helpers; `func bwRefinedPaths(t, name) *Result` for multi-subpath shapes.

- [ ] **Step 1: Failing tests**

```go
func TestHalfDiscIsLineAndArcs(t *testing.T)      // 3 segments: 1 line, 2 arcs; arc radius 80 ± 0.2
func TestAnnularSectorIsLinesAndArcs(t *testing.T) // 6 segments: 2 lines, 4 arcs, every curved segment isArc, the two radii 60/100 ± 0.3, centres within 0.3 px
func TestFilletArcRadii(t *testing.T)             // kinds: 1 line, 2 fillet arcs within 2 % of 12, large arc pieces within 2 % of 100 (two cubics for 180°+)
func TestTriangleThreeNodes(t *testing.T)         // exactly 3 segments, all lines, each node within 0.15 px of corpus.TriangleCorners()
func TestRingIsTwoCircles(t *testing.T)           // one path, two subpaths, 4 cubics each, radii 110/80 ± 0.1
```
Each via `bwRefined` / `bwRefinedPaths` (bw preset, `Refine.Enabled`).

- [ ] **Step 2: Run, expect FAIL** (today's code: half-disc arc not snapped, triangle 6 nodes, ring 8-cubic loops).
- [ ] **Step 3: Make them pass.** Expect this to be tuning, not new code: the constants in Global Constraints move only with a ledgered ruling and a line in the bench results file. If `fillet-arc` needs the merged fillet+arc cubic split (spec non-goal), ledger it and relax that test to "the large arc is an arc" rather than add the feature.
- [ ] **Step 4: Regenerate justified refine goldens** (`go test ./ -run Golden -update` only after the geometry comparison says which changed, and only for shapes whose change the new tests explain). `go test ./` green, `GOARCH=amd64 go test ./ ./internal/gradient/` green.
- [ ] **Step 5: Commit**

```bash
git add refine_test.go testdata/golden
git commit -s -m "test: primitives land on half-disc, annular-sector, fillet-arc, triangle and ring"
```

---

### Task 8: Bench, docs, gates

**Files:**
- Create: `bench/results/2026-11-14-primitives-v2.md`
- Modify: `CHANGELOG.md`, `README.md:37` (Geometric primitives bullet)

- [ ] **Step 1: Build main and branch binaries** (`git worktree` of main at `/tmp/...` or the scratchpad `rekha-base`), run the 21-image bench (`--preset poster --color-tolerance 0.06 --gradients --refine --no-mark`, PSNR over white by `rsvg-convert`, path count) and, for the nine `*-logo.png`, nodes and arc share (a throwaway Go program in the scratchpad reusing `isArc` logic over the SVG, or the Python `arcshare.py` already in the scratchpad `prim/`). Record every number in the results file in the format of `bench/results/2026-11-14-wu-groups.md`.
  Expected (spec §2): Pokéball ≥ 90 % arcs and ≤ 70 nodes; G ≥ 40 %; no image loses > 0.5 dB; mean not down by more than 0.1 dB.
- [ ] **Step 2: `go test ./ -run xxx -bench RefineBigPoster -count 5`** on main and branch; record; expected branch ≤ 1.2× main.
- [ ] **Step 3: Docs.** CHANGELOG under Unreleased → Changed: one entry naming the four blockers fixed and the new shapes. README bullet: add "half-discs, pie wedges and concentric bands come out as arcs with corner ends; sharp corners are single nodes".
- [ ] **Step 4: Gates**

```bash
make lint test wasm-check wasm wasm-test web-test
GOARCH=amd64 go test ./ ./internal/gradient/
```
Expected: all green; `git status` clean of `zz_debug*`.

- [ ] **Step 5: Commit**

```bash
git add bench/results/2026-11-14-primitives-v2.md CHANGELOG.md README.md
git commit -s -m "docs: primitives v2 bench on the logo set, changelog and README"
```

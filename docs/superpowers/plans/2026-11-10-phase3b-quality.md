# Phase 3b Quality Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Seam-free colour cutout through a shared-boundary planar map, Potrace-level small-feature fidelity in plain spline mode, arc and rectangle primitives, a 3× faster refiner, and a benchmark that scores hand-downloaded Vector Magic / Vectorizer.AI outputs.

**Architecture:** A new `internal/planar` package cuts the per-colour crack loops at junctions into arcs shared by both sides; `trace.go` fits open arcs with the existing corner/straight-edge machinery; `internal/refine` learns fixed anchors and frozen segments so an arc is refined once by its owner in one parallel Jacobi round. `internal/fit` gets a radius-relative line tolerance and a split floor. Primitives (arcs, common radius, rectangles) and the speed work (self-intersection as a step constraint, mod-3 parallel pieces, sweep early exit) live in `internal/refine`. `internal/bench` gains directory-backed reference tools.

**Tech Stack:** Go 1.26, standard library + `golang.org/x/image` only, `CGO_ENABLED=0`, wasm-buildable. Tools for the baseline: potrace 1.16, vtracer 1.0.0-alpha.4, rsvg-convert.

**Spec:** `docs/superpowers/specs/2026-11-10-phase3b-quality-design.md`

## Global Constraints

- Module `github.com/vexyl-ai/rekha`, Go 1.26, `CGO_ENABLED=0`, Apache-2.0; `GOOS=js GOARCH=wasm go build ./...` must pass; no new dependencies.
- Every commit `git commit -s` (DCO). No `Co-Authored-By: Claude` or other Claude attribution trailer, ever.
- Keyed `geom.Vec2{X:, Y:}` literals; every float product `x*y` that feeds a sum in `internal/refine` and `internal/fit` wrapped `float64(x*y)` (FMA fusion on arm64 vs amd64).
- No Potrace/gotrace source read, ported or paraphrased. Potrace and VTracer only as black-box binaries in `make bench`.
- `make lint test wasm-check` exit 0 and `GOARCH=amd64 go test ./...` green (Rosetta) before any task is called complete that touches `internal/refine`, `internal/fit`, `trace.go` or goldens.
- Output deterministic for any `GOMAXPROCS`.
- Pixel and polygon goldens (`testdata/golden/*-pixel.svg`, `*-polygon.svg`) byte-identical throughout. Spline goldens may be regenerated only after the §4 PSNR check (new golden's 4× `rasttest` render not more than 0.3 dB below the old's against the corpus PNG). Refine goldens are compared geometrically (`TestTraceGoldenRefine`).
- `Logo.jpg`, `out*.svg`, `s1.png` in the repo root are the user's files: never `git add` them (use explicit paths, never `git add -A`).

## Review Focus

1. A colour cutout image whose white page is omitted and which has a shape touching the image border: border arcs have no partner and the omitted loop owns nothing; the shape must still be fitted and refined along its border edge and emit a closed path. Test in Task 5 (`TestSharedRefineBorderShape`).
2. A label image with a saddle vertex where the two diagonal colours are both emitted (poster `checkerboard-2px`): every arc is one edge long, 40 960 nodes; the map and fitting must stay linear-time and produce exactly the Phase 2 pixel sets. Test in Task 3 (`TestPlanarCheckerboard`) and Task 4 (checkerboard poster golden time under 2 s).
3. A loop with fewer than three segments after merge/split (a two-cubic island, a single-line arc): primitives, mod-3 classes and the Jacobi round must not index out of range. Test in Task 6 (`TestRefineTinyLoops`) and Task 7 (`TestPrimitivesSkipTinyLoops`).
4. A rectangle that is really a rhombus or a trapezoid within the 2° test: the corner-distance acceptance and the energy guard must reject it, not snap it. Test in Task 7 (`TestRectangleRejectsTrapezoid`).
5. A reference directory holding an SVG whose `viewBox` differs from the input size, or a PNG of the wrong size: the row must carry an error, not a wrong PSNR. Test in Task 8 (`TestReferenceToolSizeMismatch`).

---

### Task 1: Corpus additions (`small-glyphs`, `rect-rot`, `big-poster`)

**Files:**
- Modify: `internal/corpus/synthetic.go` (add `Size int` to `Synthetic`, three images, `ellipse` and `stemFlag` helpers)
- Modify: `internal/corpus/gen/main.go` only if it hard-codes 256
- Create: `testdata/corpus/synthetic/small-glyphs.png`, `rect-rot.png`, `big-poster.png` (generated)
- Test: `internal/corpus/synthetic_test.go`, `colour_test.go` (`TestCorpusHasColourSynthetics` list)

**Interfaces:**
- Consumes: nothing new.
- Produces: `Synthetic.Size int` (0 → 256); `Render(s)` honours it; names `small-glyphs`, `rect-rot`, `big-poster` in `All()`.

- [ ] **Step 1: Write the failing tests**

```go
func TestSyntheticSizes(t *testing.T) {
    want := map[string]int{"small-glyphs": 256, "rect-rot": 256, "big-poster": 2048, "circle-aa": 256}
    for _, s := range All() {
        if w, ok := want[s.Name]; ok {
            if b := Render(s).Bounds(); b.Dx() != w || b.Dy() != w { t.Errorf("%s: %v", s.Name, b) }
        }
    }
}
func TestSmallGlyphsLayout(t *testing.T) {
    // three rows; the 5-px counter of each ring is white at its centre and the ring is black 8 px left of it
}
func TestRectRotCorners(t *testing.T) {
    // the four true corners of a 120×80 rectangle rotated 20° about (128,128) are exported as RectRotCorners() for Task 7
}
```

- [ ] **Step 2: Run them** — `go test ./internal/corpus/ -run 'Synthetic|SmallGlyphs|RectRot'` — Expected: FAIL (unknown names / missing field).
- [ ] **Step 3: Implement** `Size int` on `Synthetic`; `Render` uses `s.Size` when non-zero; `small-glyphs`: rows at heights 24, 32, 40 px, each row a ring (outer ellipse ratio 0.8, inner disc r = 5), a stem of width h/4 with a diagonal flag from the top-left (polygon), a rounded rectangle 1.5h × h with radius h/5, all `fill(..., black, 4)`; `rect-rot`: `polygon` of the 120×80 rectangle rotated 20° about (128, 128), `black, 4`; `big-poster` (2048): deterministic pseudo-random (seeded `math/rand/v2` with seed 3) placement of about 300 discs (r 20–60), 100 rings and 100 rounded rectangles in six colours, `samples 4`. Export `func RectRotCorners() [4]geom.Vec2`.
- [ ] **Step 4: Regenerate PNGs** — `go run ./internal/corpus/gen -out testdata/corpus/synthetic` (check the generator's flag) — Expected: three new files, the existing twelve byte-identical (`git status` shows only additions).
- [ ] **Step 5: Add the three names to `TestCorpusHasColourSynthetics`** (or its bw sibling) and run `go test ./internal/corpus/ . -run 'Synthetic|SmallGlyphs|RectRot|CorpusHas'` — Expected: PASS.
- [ ] **Step 6: Commit** — `git add internal/corpus testdata/corpus/synthetic/small-glyphs.png testdata/corpus/synthetic/rect-rot.png testdata/corpus/synthetic/big-poster.png colour_test.go && git commit -s -m "feat(corpus): small-glyphs, rect-rot and 2048-px big-poster synthetics"`

### Task 2: Small-feature fitting (relative line tolerance, split floor)

**Files:**
- Modify: `internal/fit/schneider.go` (`fitCubic`, `straight`), constants `lineRel = 0.05`, `lineFloor = 0.25`, `minSplitPoints = 6`
- Test: `internal/fit/schneider_test.go`, `trace_test.go` (`TestSmallGlyphsBeatsPotrace`), goldens `testdata/golden/*-spline.svg`, `*-poster-spline.svg`, `*-photo-spline.svg`

**Interfaces:**
- Consumes: nothing new.
- Produces: `fitCubic(pts, t0, t1, errSq, out)` unchanged signature; `straight(pts []geom.Vec2, errSq float64, t0, t1 geom.Vec2) bool` (new tangent parameters).

- [ ] **Step 1: Record Potrace on `small-glyphs`** — `make build && ./bin/rekha-bench -corpus testdata/corpus/synthetic -tools potrace -out /tmp/sg` (check the binary's flags) — note Potrace's PSNR and node count for `small-glyphs` from `/tmp/sg/results.md`; they become constants `potraceSmallGlyphsPSNR`, `potraceSmallGlyphsNodes` in `trace_test.go` with a comment naming the run.
- [ ] **Step 2: Write the failing tests**

```go
// internal/fit: a 45° arc of radius 8 with exact end tangents must be a cubic, not a line.
func TestSmallArcIsCubic(t *testing.T) {
    pts := arcPoints(geom.Vec2{X: 0, Y: 0}, 8, 0, math.Pi/4, 12) // 12 points, radius 8
    segs := FitOpen(pts, tangentAt(0), tangentAt(math.Pi/4).Scale(-1), 1.5)
    if len(segs) != 1 || segs[0].Line { t.Fatalf("got %+v", segs) }
}
// a 6-point run that misses tolerance is accepted as one cubic, not split
func TestSixPointRunNotSplit(t *testing.T) { ... len(segs) == 1 ... }
// a long straight run still comes out as one line
func TestLongStraightStillLine(t *testing.T) { ... }
// trace_test.go
func TestSmallGlyphsBeatsPotrace(t *testing.T) {
    r := traceSynthetic(t, "small-glyphs", PresetBW) // plain spline
    psnr := superPSNR(...); nodes := totalNodes(r)
    if psnr < potraceSmallGlyphsPSNR || nodes > potraceSmallGlyphsNodes { t.Errorf(...) }
}
```

- [ ] **Step 3: Run them** — `go test ./internal/fit/ . -run 'SmallArc|SixPoint|LongStraight|SmallGlyphs'` — Expected: `TestSmallArcIsCubic` FAIL (line), `TestSmallGlyphsBeatsPotrace` FAIL; the others may pass (note which in the ledger).
- [ ] **Step 4: Implement** in `fitCubic`: compute `R` from chord `L` and turn `Δθ = angle(t0, -t1)`; `lineTol = max(lineFloor, min(sqrt(errSq), lineRel·R))`; call `straight(pts, lineTol², t0, t1)`; never split when `n < minSplitPoints` (append the best cubic). `straight` keeps its chord test but at the new tolerance.
- [ ] **Step 5: Run the fit and root tests** — `go test ./internal/fit/ . -run 'SmallArc|SixPoint|LongStraight|SmallGlyphs'` — Expected: PASS. If `TestSmallGlyphsBeatsPotrace` still fails, diagnose on the fixture (render both at 8×, as the Logo investigation did) and ledger the ruling; the stem notch is the likely second cause (corner move in `refineCorners`).
- [ ] **Step 6: Golden check** — `go test . -run 'TestTraceGolden'` lists the spline goldens that changed. For each, before regenerating: `go run ./cmd/golden-check` (write this small helper under `internal/corpus/goldencheck/` if none exists: renders old and new golden at 4× with `rasttest.Paint`, prints PSNR of each against the corpus PNG) — Expected: no image more than 0.3 dB below its old golden. Regenerate with the goldens' documented `UPDATE_GOLDEN=1` (check `trace_test.go`) only the spline families; `git diff --stat testdata/golden` must show no `-pixel.svg` or `-polygon.svg`.
- [ ] **Step 7: Full suite both arches** — `go test ./... && GOARCH=amd64 go test ./... && make lint wasm-check` — Expected: all exit 0.
- [ ] **Step 8: Commit** — `git add internal/fit trace_test.go testdata/golden && git commit -s -m "feat(fit): radius-relative line tolerance and split floor for small features"`

### Task 3: Planar map (`internal/planar`)

**Files:**
- Create: `internal/planar/planar.go`, `internal/planar/planar_test.go`
- Modify: none (reads `contour.TraceFunc`, `geom`)

**Interfaces:**
- Consumes: `contour.TraceFunc(b *bitmap.Bitmap, turnLeft func(x, y int) bool) []Contour`, `contour.BuildShapes`.
- Produces (exact):

```go
package planar
const None uint8 = 255
type Arc struct { Pts []geom.Point; Left, Right uint8; Closed bool }
type Ref struct { Arc int; Reverse bool }
type Loop struct { Colour uint8; Area2 int; Refs []Ref }
type Map struct { Arcs []Arc; Loops []Loop }
func Build(labels []uint8, w, h int, colours []uint8, saddle func(c uint8) func(x, y int) bool) Map
func (m Map) Assemble(l Loop) []geom.Point   // the loop's crack polygon, as TraceFunc produced it
func (m Map) Shapes() [][]int                // per colour-shape: indices into Loops, outer first then holes (BuildShapes order)
```

`colours` lists the labels to trace (all palette entries for colour presets); loop order is colour, then `Pts[0]` (y, x) as `Segmentation.layers` produces it.

- [ ] **Step 1: Write the failing tests**

```go
func TestPlanarEveryArcHasOnePartner(t *testing.T)   // 3-colour 8×8 image: each arc's edge set appears in exactly 2 loops, or Left == None
func TestPlanarAssembleMatchesTrace(t *testing.T)    // for every loop, Assemble equals contour.TraceFunc's loop point for point and Area2
func TestPlanarJunctions(t *testing.T)               // a T of three colours: the T vertex ends 3 arcs; a saddle vertex ends 4
func TestPlanarIsland(t *testing.T)                  // a disc inside a square: one Closed arc referenced by the disc's loop and the square's hole, Reverse on one
func TestPlanarCheckerboard(t *testing.T)            // 64×64 2-px checkerboard: arcs all 1 edge long, Build under 50 ms
func FuzzPlanar(f *testing.F)                        // ≤16×16, ≤4 labels: no panic; the two invariants above hold
```

- [ ] **Step 2: Run** — `go test ./internal/planar/` — Expected: build failure (package missing).
- [ ] **Step 3: Implement `Build`**: per colour, `TraceFunc` with `saddle(c)`; junction test per crack vertex from the four surrounding labels (outside = `None`); cut each loop at junction indices (a loop without junctions is one `Closed` arc); dedupe with `map[[3]int]int` keyed `(minEdge, maxEdge, count)` where `edgeID(x, y, dir)` is the spec's undirected id; the second occurrence gets `Reverse: true` after checking its first point equals the stored arc's last point. `Assemble` concatenates, reversing where marked, dropping each arc's first point after the first arc. `Shapes` groups loops of one colour with `contour.BuildShapes` on the assembled contours.
- [ ] **Step 4: Run** — `go test ./internal/planar/ && go test ./internal/planar/ -run FuzzPlanar -fuzz FuzzPlanar -fuzztime 20s` — Expected: PASS, no crashers.
- [ ] **Step 5: Commit** — `git add internal/planar && git commit -s -m "feat(planar): shared-boundary map of a label image"`

### Task 4: Arc fitting and cutout assembly (plain mode)

**Files:**
- Modify: `trace.go` (`spline` → `fitLoop`, new `fitArc`), `internal/geom/simplify.go` (`RemoveStaircasesOpenIndices`), `internal/geom/rdp.go` (`RDPIndices` already open: verify it keeps both ends), `finish.go` (cutout spline path builds the map, fits arcs, assembles shapes)
- Test: `trace_test.go`, `finish_test.go`, `internal/geom/simplify_test.go`

**Interfaces:**
- Consumes: Task 3 `planar.Build/Assemble/Shapes`; existing `segments(pts, o)`.
- Produces: `fitLoop(dense []geom.Vec2, o Options) []fit.Segment` (today's `spline` body), `fitArc(dense []geom.Vec2, o Options) []fit.Segment` (open), `reverseSegs(segs []fit.Segment) []fit.Segment`, and in `finish.go` `type arcFit struct{ segs []fit.Segment }` stored per `planar.Arc`; `(*Segmentation).finishCutout(o Options) *Result` used when `o.Hierarchical == cutout && o.Preset != bw && o.Mode == spline`.

- [ ] **Step 1: Write the failing tests**

```go
func TestFitArcEndsExact(t *testing.T)        // quarter-circle crack polyline: first/last segment points equal dense[0]/dense[last]; max deviation ≤ 1.5 px
func TestFitArcReverseIdentical(t *testing.T) // reverseSegs(fitArc(d)) numbers equal fitArc(d) reversed; and fitArc(reverse(d)) renders within 0.1 px of it
func TestFitLoopEqualsSpline(t *testing.T)    // fitLoop on circle-aa's loop == old golden circle-aa-spline.svg byte for byte
func TestRemoveStaircasesOpenKeepsEnds(t *testing.T)
func TestCutoutSharedPathData(t *testing.T)   // aa-abut poster cutout plain: the two shapes' shared arc appears with identical coordinates (reversed) in both paths
func TestCutoutPixelSetsUnchanged(t *testing.T) // rasttest.Fill of each cutout path == Phase 2's per-colour pixel set on logo-holes, poster-discs
func TestCutoutCheckerboardFast(t *testing.T) // checkerboard-2px poster cutout spline under 2 s
```

- [ ] **Step 2: Run** — `go test . ./internal/geom -run 'FitArc|FitLoop|StaircasesOpen|CutoutShared|CutoutPixel|CutoutCheckerboard'` — Expected: FAIL (undefined).
- [ ] **Step 3: Implement** `fitArc`: open variants of the index pipeline (`MergeCollinearIndices` is already open-safe for ends? verify; `RemoveStaircasesOpenIndices` never drops index 0 or n−1; `RDPIndices` keeps ends), corners/bends/straight edges without wrap, end vertices at `dense[0]`/`dense[n-1]` with `tOut`/`tIn` from `fit.FitLine` over `flankWindow(n)` points; runs < 4 points → line. `finishCutout`: build map with the segmentation's saddle, fit each arc in parallel (`Closed` → `fitLoop`), assemble each shape's outer and holes by concatenating `arcFit.segs` (reversed per `Ref.Reverse`), emit as today (colour then scan order, one path per shape).
- [ ] **Step 4: Run the tests** — same command — Expected: PASS; `go test ./...` green; pixel/polygon goldens untouched (`git status testdata/golden` empty).
- [ ] **Step 5: Commit** — `git add trace.go finish.go internal/geom trace_test.go finish_test.go internal/geom/simplify_test.go && git commit -s -m "feat(cutout): fit each shared boundary once from the planar map"`

### Task 5: Shared refinement (fixed anchors, frozen segments, Jacobi round)

**Files:**
- Modify: `internal/refine/refine.go` (`Options.Fixed []bool`, `Options.Frozen []bool`, `refinePiece` honours them, `clampAll`), `internal/refine/mergesplit.go` (never merge a fixed joint, never touch a frozen segment), `internal/refine/circle.go` (`FitCircle` caller checks), `finish.go` (`finishCutout` refine round, write-back), `mask.go` (unchanged API)
- Test: `internal/refine/refine_test.go`, `internal/refine/mergesplit_test.go`, `refine_test.go`

**Interfaces:**
- Consumes: Task 4 `finishCutout`, `arcFit`; `refine.Refine(b, m, img, o)`, `refine.MergeSplit`.
- Produces: `refine.Options{Sweeps, MaxIter, Weights, Fixed []bool, Frozen []bool}`; `refine.FromSegments` unchanged; helper `arcOwner(m planar.Map, loops []layerLoop) []int` in `finish.go` (owner loop per arc: emitted before omitted, lower colour, then loop order; `-1` when both omitted).

- [ ] **Step 1: Write the failing tests**

```go
// internal/refine
func TestRefineFixedAnchorStays(t *testing.T)      // perturbed circle, anchor 0 Fixed: after Refine b.Pts[0] unchanged to 1e-12, others move
func TestRefineFrozenSegmentUnchanged(t *testing.T) // segment 2 Frozen: its 4 points unchanged; piece (2,3) moves only segment 3's handles
func TestMergeSplitRespectsFixed(t *testing.T)     // a merge that would remove a Fixed joint is not taken; a Frozen segment is never split
// root
func TestSharedRefineIdenticalBothSides(t *testing.T) // aa-abut poster cutout --refine: shared arc path data identical reversed
func TestSharedRefineBorderShape(t *testing.T)        // synthetic with a shape on the border and white page omitted: path closed, border arc refined (points differ from plain)
func TestCutoutWithin3dBOfStacked(t *testing.T)       // aa-abut, three-layer-stack, logo-holes, poster-discs: superPSNR(cutout refine, stroke 0) ≥ superPSNR(stacked refine) − 3
```

- [ ] **Step 2: Run** — `go test ./internal/refine . -run 'Fixed|Frozen|RespectsFixed|SharedRefine|Within3dB'` — Expected: FAIL (fields undefined / PSNR short).
- [ ] **Step 3: Implement**: variable vector excludes fixed/frozen coordinates (trust box and `clampAll` skip them); a piece with both segments frozen returns early; `mergePass` skips joints with `Fixed[j]` or either segment frozen; `splitPass` skips frozen segments; `FitCircle` attempted only when `Fixed`/`Frozen` are all false. In `finishCutout`: per loop assemble a bezigon from `arcFit`s with `Fixed` at every arc end and `Frozen` on non-owned arcs' segments (track segment ranges per arc), refine all loops in `parallel`, then copy owned arcs' segment ranges back into `arcFit`, re-assemble, emit. Mask: `s.mask(colour, assembledPoly, hole, nil)` as today.
- [ ] **Step 4: Run** — same command, then `go test ./... && GOARCH=amd64 go test ./...` — Expected: PASS. If `TestCutoutWithin3dBOfStacked` misses on one image, measure with stroke 0.1 and ledger the number; the test threshold stays 3 dB and the baseline records the miss.
- [ ] **Step 5: Commit** — `git add internal/refine finish.go refine_test.go && git commit -s -m "feat(refine): fixed anchors and frozen segments; shared arcs refined once by their owner"`

### Task 6: Refinement speed

**Files:**
- Modify: `internal/refine/energy.go` (remove `Weights.Spt`; keep `SelfIntersectionPrior` for merge/split), `internal/refine/refine.go` (crossing count at piece start, `ok` callback rejects increases; mod-3 classes run in `parallel`; `sweepStop = 0.005`), `internal/refine/lbfgs.go` (no change expected), `options.go` (validation list drops `Spt`), `README.md` (one line)
- Test: `internal/refine/refine_test.go` (`BenchmarkRefineBigPoster`, `TestRefineParallelClassesDeterministic`, `TestRefineNoNewCrossings`, `TestRefineTinyLoops`, `TestSweepEarlyExit`), `refine_test.go` (`TestRefineDeterministicAcrossProcs` extended to `big-poster`)

**Interfaces:**
- Consumes: Task 1 `big-poster.png`; Task 5 `Fixed`/`Frozen`.
- Produces: `refine.Weights{Apt, Hpt, Lpt, Opt}`; `refine.Options` unchanged otherwise.

- [ ] **Step 1: Phase 3 baseline number** — in a temporary worktree of `main` (`git worktree add /tmp/p3 main`), copy `testdata/corpus/synthetic/big-poster.png` and a copy of the benchmark test file in, run `go test ./internal/refine -run xxx -bench BenchmarkRefineBigPoster -benchtime 3x` and record ns/op and the refined PSNR it logs; remove the worktree. Write both into the ledger; they go into the baseline results file in Task 8.
- [ ] **Step 2: Write the failing tests**

```go
func TestRefineParallelClassesDeterministic(t *testing.T) // GOMAXPROCS 1 vs 8 on a 200-segment loop: identical Pts
func TestRefineNoNewCrossings(t *testing.T)               // a loop refined against a mask that tempts a crossing: crossing count never rises per sweep
func TestRefineTinyLoops(t *testing.T)                    // N = 1, 2, 3, 4 bezigons: no panic, output finite
func TestSweepEarlyExit(t *testing.T)                     // a loop already at its minimum stops after one sweep (count via a test hook or timing)
func BenchmarkRefineBigPoster(b *testing.B)               // Segment+Finish of big-poster under poster stacked --refine; logs superPSNR once
```

- [ ] **Step 3: Run** — `go test ./internal/refine -run 'Parallel|NoNewCrossings|TinyLoops|EarlyExit'` — Expected: FAIL / compile error on `Spt` removal in tests.
- [ ] **Step 4: Implement** per spec §6: crossing count of the piece's polyline at piece start; `ok(x, xn)` returns false when the count rises; `energy` drops the Spt term; classes `r = 0, 1, 2` with the wrap rule (the piece whose moving points overlap the class's first piece is deferred to the next class); each class runs through `parallel` with a scratch per worker and writes results after the class; sweep loop breaks when the sweep's energy drop is under 0.5 % of its start.
- [ ] **Step 5: Run and measure** — `go test ./internal/refine -run 'Parallel|NoNewCrossings|TinyLoops|EarlyExit' && go test ./internal/refine -run xxx -bench BenchmarkRefineBigPoster -benchtime 3x` — Expected: tests PASS; ns/op ≤ ⅓ of Step 1; logged PSNR within 0.2 dB of Step 1. If short of 3×, profile (`-cpuprofile`) and ledger what remains; the goal is the spec's, not the first measurement.
- [ ] **Step 6: Goal images and full suite** — `go test ./... && GOARCH=amd64 go test ./... && make lint wasm-check`; `TestTraceGoldenRefine` geometric compare still passes (regenerate refine goldens only if it fails and the PSNR check holds).
- [ ] **Step 7: Commit** — `git add internal/refine options.go README.md refine_test.go && git commit -s -m "perf(refine): crossings as a step constraint, mod-3 parallel pieces, sweep early exit"`

### Task 7: Primitives (arcs, common radius, rectangles, energy guard)

**Files:**
- Create: `internal/refine/primitives.go` (`FitArcs`, `CommonRadius`, `FitRectangle`, `arcCubics`), `internal/refine/primitives_test.go`
- Modify: `internal/refine/circle.go` (energy guard on circles), `trace.go` (`refineSegments` order: circle, arcs, common radius, rectangle)
- Test: `refine_test.go` (`TestRoundedRectIsLinesAndArcs`, `TestRectRotFourNodes`, `TestPrimitivesNeverLoseOnGoalImages`)

**Interfaces:**
- Consumes: Task 1 `RectRotCorners()`; Task 5 `Fixed`/`Frozen`; `localDataEnergy`, `windowAround`, `segmentsBox`.
- Produces:

```go
func FitArcs(b Bezigon, m Mask, img Window, tol float64, fixed, frozen []bool) Bezigon
func CommonRadius(b Bezigon, m Mask, img Window, fixed []bool) Bezigon
func FitRectangle(b Bezigon, m Mask, img Window, tol float64) (Bezigon, bool)
const primRise = 0.02; rectAngleTol = 2.0 (degrees); arcJointTurn = 5.0 (degrees); radiusAgree = 0.10
```

- [ ] **Step 1: Write the failing tests**

```go
func TestFitArcsQuarterCircle(t *testing.T)        // a 2-cubic 90° arc between two lines → 1 cubic with handle ratio 4/3·tan(π/8) ± 0.002
func TestFitArcsKeepsFixedEnds(t *testing.T)       // fixed end anchors unchanged
func TestFitArcsRevertsOnEnergyRise(t *testing.T)  // a mask that disagrees: bezigon unchanged
func TestCommonRadiusFourCorners(t *testing.T)     // radii 23.5, 24, 24.4, 24.1 → all 24.0
func TestFitRectangleRotated(t *testing.T)         // 4 noisy corners of the 20° rectangle → within 0.1 px of RectRotCorners()
func TestRectangleRejectsTrapezoid(t *testing.T)   // a 2° trapezoid whose corners would move 1 px: ok == false
func TestRectangleAxisSnap(t *testing.T)           // orientation 0.6° → exactly axis-aligned
func TestPrimitivesSkipTinyLoops(t *testing.T)     // N = 1, 2: unchanged, no panic
// root
func TestRoundedRectIsLinesAndArcs(t *testing.T)   // rounded-rect bw --refine: 8 segments, 4 Line, 4 cubics ratio 0.5523 ± 0.002, radius 24 ± 0.2 (from handle length / 0.5523)
func TestRectRotFourNodes(t *testing.T)            // rect-rot bw --refine: 4 nodes within 0.1 px of RectRotCorners()
func TestPrimitivesNeverLoseOnGoalImages(t *testing.T) // Primitives on vs off on the seven goal images: no loss over 0.2 dB, total nodes not up
```

- [ ] **Step 2: Run** — `go test ./internal/refine . -run 'FitArcs|CommonRadius|FitRectangle|Rectangle|PrimitivesSkip|RoundedRect|RectRot|PrimitivesNever'` — Expected: FAIL (undefined).
- [ ] **Step 3: Implement** per spec §5: arc runs = maximal runs of curved segments (not `straightCubic`) between straight segments or fixed joints with interior joint turns under `arcJointTurn`; Kåsa on `circleSamples` per segment; accept when all samples within `min(tol, 0.1·r)` and both end anchors within that of the circle (else skip); replace with `arcCubics(centre, r, a0, a1, ccw)` pieces ≤ 90°, handle `4/3·tan(θ/4)·r`; guard with `localDataEnergy` over `windowAround(segmentsBox(before), segmentsBox(after))`. Common radius: group arcs by radius within `radiusAgree` of the mean, refit at the mean, guard each. Rectangle: as spec. Wire into `refineSegments` after `MergeSplit`, under `o.Refine.Primitives`, with `tol = 0.5·o.FitTolerance`.
- [ ] **Step 4: Run** — same command, then full suite both arches — Expected: PASS; refine goldens that fail the geometric compare are regenerated after the PSNR check (`rounded-rect` will change).
- [ ] **Step 5: Commit** — `git add internal/refine trace.go refine_test.go testdata/golden && git commit -s -m "feat(refine): circular-arc, common-radius and rectangle primitives under an energy guard"`

### Task 8: Reference tools, docs, baseline

**Files:**
- Modify: `internal/bench/tool.go` (`Tool.Dir`, `BuiltinTools` adds `vectormagic`, `vectorizer-ai`), `internal/bench/run.go` (`runOne` branch for `Dir`), `docs/benchmark.md` (procedure), `README.md` (status line, Phase 3b summary, `Weights` note), `.gitignore` (`/bench/reference/*/` except `README.md`), `design/roadmap.md` (Phase 3 row: items done)
- Create: `bench/reference/vectormagic/README.md`, `bench/reference/vectorizer-ai/README.md`, `bench/results/<date>-phase3b.md`
- Test: `internal/bench/tool_test.go`, `internal/bench/run_test.go`

**Interfaces:**
- Consumes: Task 1–7.
- Produces: `bench.Tool{Name, Bin, Args, Input, Dir}`.

- [ ] **Step 1: Write the failing tests**

```go
func TestReferenceToolSVG(t *testing.T)          // temp Dir with <name>.svg: row has PSNR, Stats, Millis 0
func TestReferenceToolPNG(t *testing.T)          // temp Dir with <name>.png: PSNR set, Stats zero
func TestReferenceToolMissing(t *testing.T)      // neither file: Skipped == "no reference"
func TestReferenceToolSizeMismatch(t *testing.T) // PNG of the wrong size: Err non-empty, PSNR NaN
func TestBuiltinToolsIncludeReferences(t *testing.T)
```

- [ ] **Step 2: Run** — `go test ./internal/bench -run 'Reference|BuiltinTools'` — Expected: FAIL.
- [ ] **Step 3: Implement** per spec §7; `runOne` with `Dir` set copies/reads the file instead of executing; PNG path uses `metrics.PSNR` directly on the decoded image after a size check.
- [ ] **Step 4: Run** — `go test ./internal/bench` — Expected: PASS.
- [ ] **Step 5: Docs** — procedure in `docs/benchmark.md`; READMEs in the two reference directories (what to drop there, naming rule); README status "Phase 3b"; roadmap row updated; `.gitignore` entries.
- [ ] **Step 6: Baseline** — `make bench` (rekha, rekha-refine, rekha-poster, rekha-poster-refine, potrace, vtracer; reference rows show `no reference`); write `bench/results/<date>-phase3b.md` with: the goal table (cutout vs stacked per image with stroke 0 and 0.1; `small-glyphs` vs Potrace; `rounded-rect`/`rect-rot` segment and node counts; `big-poster` Phase 3 vs 3b ns/op and PSNR from Task 6; 256² timings; GOMAXPROCS determinism), a Logo.jpg paragraph (plain, `--refine`, cutout `--refine`, times, by hand, not committed), and the full results table. Every goal gets pass/fail.
- [ ] **Step 7: Full suite both arches, lint, wasm** — `go test ./... && GOARCH=amd64 go test ./... && make lint wasm-check` — Expected: exit 0.
- [ ] **Step 8: Commit** — `git add internal/bench docs/benchmark.md README.md .gitignore design/roadmap.md bench/reference bench/results && git commit -s -m "feat(bench): reference tools from hand-downloaded outputs; Phase 3b baseline and docs"`

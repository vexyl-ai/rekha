# Phase 3 Refinement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `--refine` moves fitted control points to match the input's anti-aliasing through a differentiable coverage rasteriser with Yang 2016 priors, merges and splits segments, detects circles, and a stroke stopgap hides cutout seams; every preset's output is unchanged with the flag off.

**Architecture:** New package `internal/refine` holds the bezigon type, an exact-area coverage rasteriser, the energy (mask data term plus four priors), a small L-BFGS, piecewise sweeps, merge/split and circle fitting. The root package builds a per-contour `refine.Mask` from `Segmentation.Labels/Second/Blend` and the contour's own fill, and calls refinement from `Finish` after the Phase 1 spline fit. `svg.Path` gains optional stroke attributes for cutout.

**Tech Stack:** Go 1.26, standard library, `golang.org/x/image` only.

**Spec:** `docs/superpowers/specs/2026-11-09-phase3-refine-design.md`

## Global Constraints

- Module `github.com/vexyl-ai/rekha`, Go 1.26, `CGO_ENABLED=0`, `GOOS=js GOARCH=wasm go build ./...` passes; no new dependencies.
- Apache-2.0, DCO: `git commit -s`, trailer `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>`.
- `make lint test wasm-check` exits 0; `geom.Point`/`geom.Vec2` literals outside `geom` are keyed; float products feeding goldens wrapped in `float64()`.
- No Potrace/gotrace-derived code.
- Output independent of `GOMAXPROCS` and map order; refinement parallel across contours, serial within one.
- With `Refine.Enabled == false` and `CutoutStroke == 0`, every existing golden is byte-identical.
- Refinement never returns a bezigon whose energy is above its input's; no NaN/Inf leaves the package.

## Review Focus

1. A bezigon with `N == 1` or `N == 2`, or all points coincident, under `--refine`: returned unchanged or refined, never panics or NaNs (Task 3 `FuzzRefine`, `TestRefineDegenerate`).
2. A contour whose window at the image edge is clipped to zero area, or whose control points sit outside the image: points clamped, piece skipped (Task 3 `TestRefineClampsToImage`).
3. A `--refine` run where the fit is already perfect (`two-colour-abut`): every piece skipped by the residual floor, output identical to unrefined within path precision (Task 6 `TestRefineLeavesPerfectFitAlone`).
4. A 1-px thin line under `--refine`: the short-handle and length priors must not collapse it to zero area; `thin-lines` PSNR not below unrefined (Task 6 integration PSNR table includes `thin-lines`).
5. Cutout stroke under `--mode pixel`: stroke widens pixel-exact shapes; the pixel-mode oracle (`rasttest.Paint`) must ignore strokes, and `CutoutStroke` must not apply in pixel mode (Task 7 `TestCutoutStrokeNotInPixelMode`).

---

### Task 1: Bezigon and coverage rasteriser

**Files:**
- Create: `internal/refine/bezigon.go`, `internal/refine/coverage.go`, `internal/refine/bezigon_test.go`, `internal/refine/coverage_test.go`

**Interfaces:**
- Consumes: `fit.Segment{Line bool; P fit.Cubic}`, `fit.Cubic [4]geom.Vec2` with `At(t)`, `geom.Vec2`.
- Produces: `type Bezigon struct{ Pts []geom.Vec2 }` (3N entries); `func FromSegments(segs []fit.Segment) Bezigon` (a line's handles at 1/3 and 2/3 of the chord); `func (b Bezigon) N() int`; `func (b Bezigon) Segment(j int) fit.Cubic` (indices mod 3N); `func (b Bezigon) Segments(lineTol float64) []fit.Segment` (cubic → `Line` when both handles within `lineTol` of the chord); `const Flatten = 16`; `func (b Bezigon) Polyline() []geom.Vec2` (Flatten points per segment, uniform t, closed, no duplicate of the start); `func (b Bezigon) Length() float64` (polyline length); `type Window struct{ X0, Y0, W, H int }`; `func Coverage(b Bezigon, win Window, out []float32)` (exact-area accumulation, `min(|v|,1)`); `func AccumulateEdges(pts []geom.Vec2, closed bool, win Window, acc []float32)` (adds the signed area/cover of a polyline's edges into `acc` of size `W*H`, the primitive that lets fixed edges be accumulated once); `func Resolve(acc []float32, win Window, out []float32)` (prefix sum per row, abs, clamp).

- [ ] **Step 1: Write failing tests**

```go
func TestFromSegmentsRoundTrip(t *testing.T)   // 4 segments (2 lines, 2 cubics) → Bezigon.N()==4, Segments(0.1) returns the lines as Line and the cubics exactly
func TestSegmentsCollapsesStraightCubic(t *testing.T) // cubic with handles 0.05 px off the chord → Line; 0.5 px off → not Line
func TestCoverageSquareExact(t *testing.T)     // square (2,2)-(6,6) in a 8×8 window: inside 1 (±1e-6), outside 0; shifted by 0.5 in x: column x=1 is 0.5, x=5 is 0.5
func TestCoverageCircleArea(t *testing.T)      // 4-cubic circle r=20 centred (32,32), 64×64 window: Σcov within 0.5 % of π·400
func TestCoverageContinuity(t *testing.T)      // move one vertex of the square by 1e-3: max |Δcov| < 1e-2
func TestAccumulateThenResolveEqualsCoverage(t *testing.T) // AccumulateEdges over the polyline + Resolve == Coverage
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/refine/`
Expected: FAIL, build error: undefined `Bezigon`, `Coverage`.

- [ ] **Step 3: Implement `bezigon.go` and `coverage.go`**

Accumulation: for each edge, walk the scanlines it crosses; within a scanline, walk the cells; per cell add `dy·(1 − cover_x)` style area to the cell and `dy` to the cell to its right (the font-rasteriser signed-area-plus-cover scheme), with `dy` signed by edge direction. `Resolve` prefix-sums along x. Edges entirely outside the window clip in y and clamp in x so coverage for cells left of the window is still carried.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/refine/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/refine
git commit -s -m "feat(refine): bezigon type and exact-area coverage rasteriser"
```

---

### Task 2: Energy and priors

**Files:**
- Create: `internal/refine/energy.go`, `internal/refine/energy_test.go`

**Interfaces:**
- Consumes: Task 1.
- Produces: `type Mask interface{ At(x, y int) (cov, weight float32) }`; `type Weights struct{ Spt, Apt, Hpt, Lpt float64 }`; `var DefaultWeights = Weights{Spt: 1.0, Apt: 0.08, Hpt: 0.1, Lpt: 0.1}`; `func DataEnergy(cov []float32, m Mask, win Window, l0 float64) float64` (`Σ weight·(cov−mask)² / l0`); `func AnglePrior(b Bezigon, joints ...int) float64` (sum of absolute joint angles via clamped `acos`; all joints when none given); `func HandlePrior(b Bezigon, segs ...int) float64` (Σ 1/len, len floored at 1e-6); `func LengthPrior(b Bezigon, segs ...int) float64` (polyline length of the segments); `func SelfIntersectionPrior(b Bezigon) float64` (shorter-arc sum over polyline crossings, bbox-pruned per edge pair); `func (w Weights) Prior(b Bezigon) float64`.

- [ ] **Step 1: Write failing tests**

```go
func TestAnglePriorSquare(t *testing.T)        // square as 4 line-cubics: AnglePrior ≈ 2π (±1e-6); smooth 4-cubic circle: < 1e-3
func TestHandlePriorNearZero(t *testing.T)     // a handle 1e-9 from its anchor → HandlePrior > 1e5; normal square → < 10
func TestLengthPriorEqualsPolyline(t *testing.T) // LengthPrior(b) == b.Length() for the circle (±1e-9)
func TestSelfIntersectionFigureEight(t *testing.T) // figure-eight bezigon → Spt > 0; circle → 0
func TestDataEnergyZeroOnOwnCoverage(t *testing.T) // mask = Coverage(b) → DataEnergy 0; mask = Coverage of the circle shifted by 1 px → > 0
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/refine/ -run 'Prior|DataEnergy'`
Expected: FAIL, undefined symbols.

- [ ] **Step 3: Implement `energy.go`**

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/refine/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/refine
git commit -s -m "feat(refine): data energy over a coverage mask and the four Yang priors"
```

---

### Task 3: L-BFGS and piecewise refinement

**Files:**
- Create: `internal/refine/lbfgs.go`, `internal/refine/refine.go`, `internal/refine/refine_test.go`, `internal/refine/fuzz_test.go`

**Interfaces:**
- Consumes: Tasks 1–2.
- Produces: `type Options struct{ Sweeps, MaxIter int; Weights Weights }`; `var DefaultOptions = Options{Sweeps: 3, MaxIter: 30, Weights: DefaultWeights}`; `func Refine(b Bezigon, m Mask, img Window, o Options) Bezigon` (`img` is the image rectangle for clamping); `func minimize(x []float64, f func([]float64) float64, maxIter int) []float64` (L-BFGS m=5, central differences h=0.01, Armijo c=1e-4 halving ≤ 20, stop on relative decrease < 1e-4 or max move < 0.005; never returns a point with higher f than the start; NaN/Inf step rejected); `func (b Bezigon) piece(j int) []int` (indices 3j+1..3j+5 mod 3N); `const windowPad = 3`, `const skipResidual = 1e-4` (per pixel).

- [ ] **Step 1: Write failing tests**

```go
func TestMinimizeQuadratic(t *testing.T)        // f = Σ(x_i − i)² from zeros → within 1e-3 of (0,1,2,…,9) in ≤ 30 iterations
func TestRefineRecoversPerturbedBezigon(t *testing.T) // random smooth 8-segment bezigon (seeded) in a 64×64 image, mask = its own Coverage, each point perturbed up to 1.5 px → after Refine every point within 0.05 px
func TestRefineRecoversAgainstSupersampledMask(t *testing.T) // same, mask = rasttest.Fill at 4× box-downsampled → within 0.1 px
func TestRefineRemovesFigureEightCrossing(t *testing.T) // figure-eight refined against the convex mask of its outline → SelfIntersectionPrior == 0
func TestRefineKeepsSquareCorners(t *testing.T) // square against a sharp square mask → AnglePrior within 0.05 of 2π, points within 0.05 px
func TestRefineLengthensShortHandle(t *testing.T) // circle with one handle shrunk to 1e-3 → that handle > 1 px after Refine
func TestRefineDegenerate(t *testing.T)         // N=1, N=2, all points equal, zero-area line → returns without panic, no NaN, N unchanged
func TestRefineClampsToImage(t *testing.T)      // points placed at (−5,−5) and beyond the image → clamped inside img padded by 1; no panic
func TestRefineSkipsConvergedPieces(t *testing.T) // mask = own Coverage: output == input exactly (every piece skipped)
func FuzzRefine(f *testing.F)                   // N 1..12, coords in 32×32 with coincidences, random mask: no panic, no NaN, energy(out) ≤ energy(in)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/refine/ -run 'Minimize|Refine'`
Expected: FAIL, undefined `Refine`, `minimize`.

- [ ] **Step 3: Implement `lbfgs.go` and `refine.go`**

Per piece: window = bbox of segments `J`, `J+1` padded by `windowPad`, clamped to `img`; accumulate all other segments' edges once into `fixed`; evaluation = copy `fixed`, `AccumulateEdges` of the two segments' polylines, `Resolve`, `DataEnergy` + priors for joints `J..J+2`, segments `J, J+1`, plus `Spt`. `l0 = max(b.Length(), 1)` once. Skip when start `DataEnergy·l0/(W·H) < skipResidual`. Sweep early-exit on max move < 0.01.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/refine/ && go test ./internal/refine/ -run '^$' -fuzz FuzzRefine -fuzztime 30s`
Expected: PASS; fuzz exits 0.

- [ ] **Step 5: Commit**

```bash
git add internal/refine
git commit -s -m "feat(refine): L-BFGS piecewise bezigon refinement"
```

---

### Task 4: Merge and split

**Files:**
- Create: `internal/refine/mergesplit.go`, `internal/refine/mergesplit_test.go`
- Modify: `internal/fit/schneider.go` (export `func FitOne(pts []geom.Vec2, t0, t1 geom.Vec2) Cubic` wrapping `generateBezier` with chord-length parameters)

**Interfaces:**
- Consumes: Task 3 `Refine`, piece machinery.
- Produces: `func MergeSplit(b Bezigon, m Mask, img Window, o Options) Bezigon` (one merge pass, one split pass, one more sweep); `const mergeAngle = 0.5` degrees, `const mergeRise = 0.01`, `const splitFactor = 3.0`.

- [ ] **Step 1: Write failing tests**

```go
func TestFitOneMatchesGenerateBezier(t *testing.T) // fit package: FitOne on points sampled from a cubic returns it within 1e-6
func TestMergeReducesOverSegmentedCircle(t *testing.T) // circle sampled into 8 cubics (split each ideal cubic at t=0.5), mask = ideal circle coverage → N ≤ 4, DataEnergy within 1 % of the 8-segment value
func TestMergeNeverBelowTwo(t *testing.T)       // 2-segment smooth loop → N stays 2
func TestSplitAtCorner(t *testing.T)            // one cubic spanning a right angle of a square (3 segments total) against the square mask → N becomes 4 and residual drops
func TestMergeSplitKeepsSharpJoints(t *testing.T) // square: no merge happens (angles 90°)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/refine/ ./internal/fit/ -run 'Merge|Split|FitOne'`
Expected: FAIL, undefined.

- [ ] **Step 3: Implement `MergeSplit` and `fit.FitOne`**

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/refine/ ./internal/fit/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/refine internal/fit
git commit -s -m "feat(refine): segment merge and split after the sweeps"
```

---

### Task 5: Circle detection

**Files:**
- Create: `internal/refine/circle.go`, `internal/refine/circle_test.go`

**Interfaces:**
- Produces: `func FitCircle(b Bezigon, tol float64) (centre geom.Vec2, r float64, ok bool)` (Kåsa algebraic fit on 8 samples per segment; `ok` when `N ≥ 3` and every sample's radial deviation ≤ tol); `func Circle(centre geom.Vec2, r float64, start geom.Vec2, ccw bool) Bezigon` (4 cubics, handle `0.5523·r`, first anchor at the circle point nearest `start`); `const kappa = 0.5523`.

- [ ] **Step 1: Write failing tests**

```go
func TestFitCircleOnIdealCircle(t *testing.T)   // Circle(c=(50,40), r=17) → FitCircle ok, centre within 1e-6, r within 1e-6
func TestFitCircleRejectsRoundedRect(t *testing.T) // rounded-rect bezigon (4 lines + 4 quarter arcs, 60×40, r 10) → !ok at tol 0.5
func TestFitCircleRejectsTwoSegments(t *testing.T) // N=2 → !ok
func TestCircleStartAndOrientation(t *testing.T) // Circle(..., start=(67,40), ccw) → Pts[0] == (67,40) ±1e-9; Area2 sign matches ccw
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/refine/ -run Circle`
Expected: FAIL, undefined.

- [ ] **Step 3: Implement `circle.go`**

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/refine/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/refine
git commit -s -m "feat(refine): circle detection and replacement"
```

---

### Task 6: Masks, options and Finish integration

**Files:**
- Create: `mask.go`, `mask_test.go`, `refine_test.go`
- Modify: `options.go` (RefineOptions, Validate, PresetOptions), `segment.go:42-60` (bw Second/Blend), `finish.go:100-116` (refine after spline fit), `colour_test.go` (goldens `<name>-refine-spline.svg`), `trace.go` (`refineSegments`)

**Interfaces:**
- Consumes: Tasks 1–5; `Segmentation{Labels, Second, Blend}`; `contour.FillOuter`; Phase 2 `layer{colour, contours, shape}`; `rasttest.Fill`.
- Produces: `type RefineOptions struct{ Enabled bool; Sweeps, MaxIter int; Weights refine.Weights; MergeSplit, Primitives bool }`; `Options.Refine RefineOptions`; `type coverageMask struct` implementing `refine.Mask` built by `func (s *Segmentation) mask(c uint8, poly []geom.Point, hole bool, band func(p int) bool) *coverageMask` (`band` reports extension-band pixels, nil when none); `func refineSegments(segs []fit.Segment, m refine.Mask, img refine.Window, o Options) []fit.Segment` (FromSegments → Refine → MergeSplit if set → FitCircle/Circle if Primitives, `tol = 0.5·FitTolerance` → `Segments(0.1)`); `Finish` calls it for each contour when `o.Refine.Enabled && o.Mode == ModeSpline`.

- [ ] **Step 1: Write failing tests**

```go
func TestSegmentBWFillsBlend(t *testing.T)       // segment_test: grey 64 pixel on white → Labels 1 (black), Second 0, Blend round(255·(191/255)) = 191; grey 200 → Labels 0, Blend 200; white → 255
func TestMaskCircleAABoundary(t *testing.T)      // mask_test: circle-aa bw, outer contour of the disc: an interior pixel → (1,1); an exterior pixel → (0,1); a boundary pixel with grey g → cov == (255−g)/255 ± 1/255
func TestMaskHoleIsComplement(t *testing.T)      // logo-holes cutout: hole contour of the red ring: inside the hole → 1, in the ring body boundary pixel → 1 − share_red
func TestMaskExtensionBandIsHard(t *testing.T)   // aa-abut stacked: a band pixel (labelled blue, inside red's extended region) → (1,1)
func TestPresetRefineDefaults(t *testing.T)      // every preset: Enabled false, Sweeps 3, MaxIter 30, Weights DefaultWeights, MergeSplit true, Primitives true
func TestValidateRefineRanges(t *testing.T)      // Sweeps 0 and 11 rejected, MaxIter 0 and 201 rejected, negative or NaN weight rejected
func TestRefineImprovesPSNR(t *testing.T)        // refine_test: for circle-aa (bw) and aa-abut (poster): PSNR of rasttest-painted 4× supersampled spline output vs input, refined ≥ unrefined + 1 dB; for thin-lines (bw): refined ≥ unrefined − 0.1 dB
func TestRefineLeavesPerfectFitAlone(t *testing.T) // two-colour-abut poster: refined SVG == unrefined SVG
func TestRefineCircleAAIsFourCubics(t *testing.T) // circle-aa bw --refine: one path, 4 segments, handle/radius 0.5523 ± 0.001, centre within 0.1 px of (128,128)
func TestRefineDeterministicAcrossProcs(t *testing.T) // GOMAXPROCS 1 vs 8 on poster-discs --refine: identical bytes
func TestTraceGoldenRefine(t *testing.T)         // colour_test: `<name>-refine-spline.svg` for the seven goal images (bw for the three bilevel, poster for the four colour), -update flag
// FuzzSegment: add a refine run on spline output through the Phase 1 sanity oracle
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./ -run 'Blend|Mask|RefineDefaults|ValidateRefine|RefineImproves|RefineLeaves|RefineCircle|RefineDeterministic|GoldenRefine'`
Expected: FAIL, undefined `Refine` field, `mask`.

- [ ] **Step 3: Implement masks, options, bw blend and the Finish hook**

In `Finish`, after `segments()` per job, when refining: build the mask from the contour's points, `hole = c.Area2 < 0`, `band` = membership in the layer's extension region (stacked, extended layers only; `extendSiblings` records the band pixels it added per layer as `layer.band []int32`), then `refineSegments`. Existing goldens must stay byte-identical with `Enabled == false`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./ -run TestTraceGoldenRefine -update && go test ./... && go test ./ -run '^$' -fuzz FuzzSegment -fuzztime 60s`
Expected: PASS; existing goldens unchanged (`git status testdata/golden` shows only new `*-refine-*` files); fuzz exits 0.

- [ ] **Step 5: Commit**

```bash
git add options.go segment.go finish.go trace.go mask.go mask_test.go refine_test.go segment_test.go colour_test.go testdata/golden
git commit -s -m "feat: --refine option, soft coverage masks and refinement in Finish"
```

---

### Task 7: Cutout stroke stopgap

**Files:**
- Modify: `internal/svg/writer.go` (Path.Stroke, Path.StrokeWidth, attributes), `internal/svg/writer_test.go`, `options.go` (CutoutStroke, Validate 0..4, presets 0 / 0.5 / 0.5), `finish.go` (set stroke on cutout colour paths in polygon/spline modes), `finish_test.go`, `internal/rasttest/raster.go` (ignore strokes: no change needed, add test)

**Interfaces:**
- Produces: `svg.Path{Fill string; Stroke string; StrokeWidth float64; Subpaths []Subpath}`; writer emits ` stroke="…" stroke-width="…" stroke-linejoin="round"` only when `Stroke != ""`; `Options.CutoutStroke float64`.

- [ ] **Step 1: Write failing tests**

```go
func TestWriteStrokeAttributes(t *testing.T)     // svg: Path with Stroke "#ff0000", StrokeWidth 0.5 → `stroke="#ff0000" stroke-width="0.5" stroke-linejoin="round"`; without Stroke → no stroke attribute
func TestPresetCutoutStroke(t *testing.T)        // bw 0, poster 0.5, photo 0.5; Validate rejects −1 and 5
func TestCutoutPathsCarryStroke(t *testing.T)    // aa-abut poster cutout spline: every path Stroke == Fill, StrokeWidth 0.5; stacked: Stroke ""; bw cutout: Stroke ""
func TestCutoutStrokeNotInPixelMode(t *testing.T) // poster cutout pixel: Stroke "" (pixel oracle stays exact)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/svg/ ./ -run 'Stroke'`
Expected: FAIL, unknown field `Stroke`.

- [ ] **Step 3: Implement**

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./...`
Expected: PASS; `git status testdata/golden` clean (bw goldens and stacked colour goldens unchanged; cutout colour goldens do not exist).

- [ ] **Step 5: Commit**

```bash
git add internal/svg options.go finish.go finish_test.go
git commit -s -m "feat(cutout): stroke stopgap hides hairline seams between abutting colours"
```

---

### Task 8: CLI, bench tools, baseline and docs

**Files:**
- Modify: `cmd/rekha/main.go` (`--refine`, `--refine-passes`, `--cutout-stroke`), `cmd/rekha/main_test.go`, `internal/bench/tool.go` (`rekha-refine`, `rekha-poster-refine`), `internal/bench/tool_test.go`, `README.md`, `docs/benchmark.md`
- Create: `bench/results/2026-11-09-phase3.md`

**Interfaces:**
- Consumes: Tasks 6–7 options.
- Produces: `RekhaTools(bin)` returns five tools; `rekhaTool(bin, name, preset string, extra ...string) Tool`.

- [ ] **Step 1: Write failing tests**

```go
func TestRunRefineFlag(t *testing.T)             // main_test: `--refine --refine-passes 2 --preset bw` on circle-aa → exit 0, SVG has exactly 4 "C" commands in its one path
func TestRunCutoutStrokeFlag(t *testing.T)       // `--preset poster --hierarchical cutout --cutout-stroke 0` → no stroke attribute; default → stroke-width="0.5"
func TestRekhaToolsRefine(t *testing.T)          // bench: ByName("rekha-refine") args contain "--refine" and "--preset" "bw"; "rekha-poster-refine" has "--preset" "poster" "--refine"
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/rekha ./internal/bench -run 'Refine|CutoutStroke'`
Expected: FAIL.

- [ ] **Step 3: Implement flags and tools; run `make bench`; write the baseline**

Baseline header: per goal image unrefined vs refined PSNR, nodes, ms (bw tools for `circle-aa`, `rounded-rect`, `star-5`; poster tools for `poster-discs`, `aa-abut`, `logo-holes`, `three-layer-stack`), mean ΔdB, max loss, node ratio, verdict against the spec goals (+2 dB mean, no image −0.5 dB, nodes ≤ 1.1×, < 1 s each); cutout with and without stroke on `aa-abut` and `three-layer-stack` (run the CLI by hand, note the commands). README: `--refine`, `--refine-passes`, `--cutout-stroke` in the flags paragraph and a sentence on what refinement does and costs. `docs/benchmark.md`: the two new tools.

- [ ] **Step 4: Run tests and verify**

Run: `make lint test wasm-check && make bench`
Expected: exit 0; `bench/out/results.md` has `rekha-refine` and `rekha-poster-refine` rows.

- [ ] **Step 5: Commit**

```bash
git add cmd/rekha internal/bench README.md docs/benchmark.md bench/results/2026-11-09-phase3.md
git commit -s -m "feat(cli): --refine, --refine-passes, --cutout-stroke; bench tools; Phase 3 baseline"
```

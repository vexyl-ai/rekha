# Phase 3 design: refinement and quality

Status: design, 2026-11-09. Derived from `design/refine-bezigon-optimisation.md` (the algorithm notes, read in full), `design/roadmap.md` (Phase 3 row) and `Go SVG image tracing tool.md` (Phase 3 row, "sub-pixel boundaries from anti-aliasing values", "whole-shape fitting of circles", "Layering"). User opted to skip design questions; every choice below is the author's call. Phase 1 and Phase 2 (`2026-11-09-phase1-bilevel-design.md`, `2026-11-09-phase2-colour-design.md`) are the base and stay the initialiser: segmentation, contour tracing, stacking and Schneider fitting are unchanged. Everything here is additive and off by default.

## Intent

Close the gap from "VTracer parity" to "Vector Magic grade" on flat art by going back to the pixels after fitting. The Phase 1 fitter never looks at the raster again once it has the crack polygon; it fits a polygon, not the image, so anti-aliasing is thrown away, curves bias inward on circles, and a few dozen changed boundary pixels can swing a fit by 3 dB (`noisy-disc`, `circle-aa`). Phase 3 adds a refinement stage that treats the fitted curves as an initial guess and moves their control points to minimise the difference between an exact-area rasterisation of each curve and a soft coverage mask built from the anti-aliasing in the input, with the four priors of Yang et al. 2016 to keep curves smooth, short, un-knotted and un-kinked. Alongside it: segment merge and split so the node count falls rather than rises, circle detection, straight-cubic collapse, and a stroke stopgap that hides cutout seams.

## Goals

- `rekha --refine in.png` (any preset) runs the refinement after spline fitting and writes the same kind of SVG.
- **Exit test.** On the anti-aliased synthetics (`circle-aa`, `rounded-rect`, `star-5` under bw; `poster-discs`, `aa-abut`, `logo-holes`, `three-layer-stack` under poster), spline mode with `--refine` scores a **mean PSNR at least 2 dB above** the same preset without it, **no single image loses more than 0.5 dB**, and the total node count is **no more than 1.1×** the unrefined count. Recorded in `bench/results/<date>-phase3.md`.
- Refinement of a 256×256 synthetic takes **under 1 s** on the development machine at default `GOMAXPROCS`; the time column of the baseline records it.
- Cutout mode with the stroke stopgap scores within 1 dB of stacked mode on `aa-abut` and `three-layer-stack` (today cutout shows hairlines).
- Output is deterministic regardless of `GOMAXPROCS`; refinement is parallel across contours, serial within one.
- No panic and no NaN on any input: fuzzed images, one-segment bezigons, zero-area shapes, coincident control points.
- `--refine` off: Phase 1 and Phase 2 goldens are byte-identical. `--refine` on under bw+cutout: the SVG differs only in path data.
- Builds for `GOOS=js GOARCH=wasm`, `CGO_ENABLED=0`, no new dependencies.

## Non-goals

- Shared-boundary planar map for cutout (both sides of an edge referencing one curve). The stroke stopgap ships instead; the planar map is Phase 3b if the benchmark or users demand it.
- Wavelet rasterisation and analytic gradients (notes §3.3 B). Exact-area accumulation with finite differences first; the synthetic recovery test is the gate.
- Joint optimisation of all bezigons against the RGB composite (notes §3.2 "Phase 3b variant").
- Colour refinement: palette colours stay as Phase 2 set them.
- Interactive refinement, external masks (Phase 6), ellipses and rotated rectangles as primitives.
- Turning `--refine` on by default. Promotion is a separate decision after the baseline; this phase leaves every preset's output unchanged unless the flag is given.
- Real-image corpus additions.

## Anti-goals

- Any Potrace/gotrace-derived code (unchanged).
- Output that changes with thread count or map iteration order.
- Refinement that can make a result worse than its input on the corpus without being caught: the energy must never be accepted higher than the start value, and the benchmark gate checks every image.
- A refinement path that bypasses the Phase 1 oracle: pixel and polygon modes are untouched.

## Constraints (carried)

Module `github.com/vexyl-ai/rekha`, Go 1.26, `CGO_ENABLED=0`, Apache-2.0, DCO sign-off, keyed `geom` literals, `float64()`-wrapped float products where goldens must be byte-exact across architectures, `make lint test wasm-check` green. Patent status of Yang 2016 remains an open lawyer item (`design/roadmap.md`); the code stays optional and isolated in one package so it can be removed if that answer is bad.

## Pipeline

```
Segment (Phase 2, unchanged) → Finish:
  layers, nesting, extension (Phase 2, unchanged)
  per contour (parallel): segments() → spline fit            [Phase 1]
      → if o.Refine.Enabled && o.Mode == spline:
          mask  := coverage mask for this contour (§1)
          bez   := refine.FromSegments(segs)
          bez    = refine.Refine(bez, mask, o.Refine)          [§2–§5]
          bez    = merge/split pass, one more sweep            [§6]
          segs   = bez.Segments(): circle (§7) or cubics with straight ones as lines
  cutout + CutoutStroke > 0: paths carry stroke = fill          [§8]
  SVG writer: optional stroke attributes
```

### 1. Soft coverage masks

The segmentation already stores, per pixel, `Labels` (nearest palette entry), `Second` (second nearest) and `Blend` (128..255, share of the nearest colour in the pixel, from the sRGB projection between the two). Phase 3 reads them; nothing is added to `Segmentation` except that the **bw preset now fills `Second` and `Blend` too**: `Second` is the other entry and `Blend = clamp(round(255 · share), 128, 255)` with `share = (255 − grey)/255` for a foreground pixel and `grey/255` for a background pixel. Neither field reaches the SVG, so bw goldens are unaffected.

Each contour to be refined is given a `Mask`:

```go
// Mask is the per-pixel target a contour is refined against: the
// coverage the curve should have at (x, y), in [0, 1], and a weight
// (0 = don't care). Pixels outside the image have coverage 0, weight 1.
type Mask interface{ At(x, y int) (cov, weight float32) }
```

The mask for a contour with colour `c` is built from `S`, the even-odd fill of the contour's own polygon (the same `contour.FillOuter`), cropped to the contour's bounding box plus two pixels:

- interior of `S`: coverage 1; outside `S`: coverage 0; weight 1 everywhere except as below;
- **boundary pixels** (a pixel in `S` with a 4-neighbour outside, or outside with a 4-neighbour in `S`) take the share of `c` at that pixel, `s_c = Blend/255` when `Labels == c`, `1 − Blend/255` when `Second == c`, otherwise the hard value `[p ∈ S]`. For an **outer** contour the coverage is `s_c`; for a **hole** (cutout mode) it is `1 − s_c`, since the curve encloses the other side;
- in stacked mode the pixels of the **extension band** (labelled a later sibling, inside `S`) are hard 1 with weight 1, so the refined outline keeps running along the dilated band and the seam-free guarantee of Phase 2 survives refinement. The optimiser fits the band edge, which is an offset of a smooth curve and costs no extra nodes in practice.

The mask is transient per contour: one bitmap of the cropped box plus pointers into `Labels`/`Second`/`Blend`.

### 2. Bezigons and the rasteriser

`internal/refine`:

```go
// Bezigon is a closed chain of N cubic Béziers: Pts has 3N entries,
// Pts[3j] the start anchor of segment j, Pts[3j+1], Pts[3j+2] its
// handles, Pts[3(j+1) mod 3N] its end anchor.
type Bezigon struct{ Pts []geom.Vec2 }

func FromSegments(segs []fit.Segment) Bezigon   // lines become cubics with handles at 1/3 and 2/3
func (b Bezigon) Segments(lineTol float64) []fit.Segment // cubics whose handles lie within lineTol of the chord become lines
```

Rasteriser: **exact-area coverage accumulation** (the signed-area-and-cover technique of font rasterisers, notes §3.3 A). Every segment is flattened to a fixed **16** uniform-`t` lines; each line adds its exact signed area and cover to the cells it crosses; a prefix sum per scanline yields signed coverage; the result is `min(|v|, 1)`. Coverage is the exact area of the flattened polygon inside each pixel, so it is continuous in every vertex, and it is **additive in edges**: the contribution of the segments that do not move during one piece's optimisation is accumulated once, copied per evaluation, and only the two moving segments (32 lines) are added before the prefix sum. Evaluation happens over a **window**: the bounding box of the two segments at the start of the piece, padded by 3 px, clamped to the image. Control points are also clamped to the image rectangle padded by 1 px.

```go
// Coverage accumulates b's flattened edges into acc (w×h window at x0, y0)
// and returns coverage per pixel in out.
func Coverage(b Bezigon, x0, y0, w, h int, out []float32)
```

### 3. Energy

As the notes (§2.2–2.4), with the data term over the mask:

    E = E_data + λ_spt·E_spt + λ_apt·E_apt + λ_hpt·E_hpt + λ_lpt·E_lpt
    E_data = (1 / l_0) · Σ_window weight · (cov − mask)²

`l_0` is the arc length of the **initial** bezigon (flattened polyline), fixed for the run. Priors, unnormalised, from the paper: angle variation (sum of absolute joint angles, `arccos` of the clamped dot product), short handles (inverse barrier, floor 1e-6 on handle length), curve length (flattened polyline length), self-intersection (sum of the shorter arc for every crossing of the flattened polyline, bounding-box pruned). Per piece only the terms touching segments `J`, `J+1` are recomputed, except SPT which is tested for the piece's lines against the rest.

Default weights `λ_spt 1.0, λ_apt 0.08, λ_hpt 0.1, λ_lpt 0.1` in `refine.Weights`, exposed on `Options` but not on the CLI.

### 4. Optimiser

Per piece `J` (segments `J`, `J+1`; free points `Pts[3J+1 .. 3J+5]`, ten scalars; `Pts[3J]`, `Pts[3J+6]` fixed): **L-BFGS**, memory 5, gradient by **central finite differences with h = 0.01 px** (20 window rasterisations), Armijo backtracking (c = 1e-4, halving, at most 20 halvings), at most **30 iterations**, stop when the relative energy decrease is below 1e-4 or no point moved more than 0.005 px. The piece's energy is never allowed to increase: if a step fails the line search the piece is left as it was. Own implementation, about a hundred lines, no dependency.

Sweeps: `J = 0 .. N−1` in order, up to `Sweeps` (default **3**) sweeps, stopping early when a sweep moved no point more than 0.01 px. A piece whose starting window residual `E_data` is below 1e-4 per pixel is skipped. Bezigons with `N < 2` are returned unchanged (a single cubic has no piece). No global pass.

Determinism: fixed sweep order, serial within a bezigon, parallel across contours with results stored by index.

### 5. Guards

- Any `NaN` or `Inf` in an evaluation makes that step rejected; a bezigon whose final energy is `NaN` is returned as its input.
- `arccos` arguments clamped to [−1, 1]; handle lengths floored; `l_0` floored at 1.
- Windows clamped to the image; a window of zero area skips the piece.

### 6. Merge and split (notes §3.6)

After the sweeps, one pass, then one more sweep:

- **Merge.** For each joint whose angle is below 0.5° and whose two adjacent segments have handles collinear with the joint within 0.5°, replace the two segments by one cubic fitted (Schneider's single-cubic least squares, `fit.generateBezier` exposed as `fit.Cubic1`) to their flattened points, re-optimise the piece containing it, accept if the window's `E_data` rises by less than 1 %. Never merge below `N = 2`.
- **Split.** For each segment whose window `E_data` per pixel exceeds 3× the bezigon's mean, split at `t = 0.5` by de Casteljau and re-optimise the two pieces containing the new joint.

`Options.Refine.MergeSplit` (default true when refinement is enabled). The node count goal depends on this pass.

### 7. Primitives and straight segments

After merge/split:

- **Circle.** Sample the bezigon at 8 points per segment, fit a circle algebraically (Kåsa), accept when every sample lies within `0.5 · FitTolerance` of the circle and `N ≥ 3`; replace by the standard four-cubic circle (handle length `0.5523 · r`) starting at the sample nearest the original start point, orientation preserved. `Options.Refine.Primitives` (default true when enabled).
- **Straight cubics.** `Segments(lineTol)` with `lineTol = 0.1 px`: a cubic both of whose handles lie within `lineTol` of the chord becomes a `Line` segment, so the writer emits `L`. Rectangles therefore stay four lines.

### 8. Cutout stroke stopgap

`Options.CutoutStroke float64` (px; 0 = none; `poster`/`photo` default **0.5**, `bw` 0). In cutout mode each colour path is written with `stroke="<fill>" stroke-width="<CutoutStroke>" stroke-linejoin="round"`, so abutting shapes overlap by half the width and no background shows through hairlines. `svg.Path` gains `Stroke string` and `StrokeWidth float64`; the writer emits the attributes only when `Stroke != ""`. The bw preset and stacked mode never set them, so Phase 1 and Phase 2 goldens are unchanged. The benchmark's rasteriser (rsvg-convert) honours strokes, so the gain is measured.

### 9. Public API (root package)

```go
type RefineOptions struct {
    Enabled    bool
    Sweeps     int            // 1..10, default 3
    MaxIter    int            // 1..200, default 30, per piece
    Weights    refine.Weights // {Spt 1.0, Apt 0.08, Hpt 0.1, Lpt 0.1}
    MergeSplit bool           // default true
    Primitives bool           // default true
}

type Options struct {
    // existing fields unchanged
    Refine       RefineOptions
    CutoutStroke float64       // px, 0..4; cutout only
}
```

`PresetOptions` sets `Refine{Enabled: false, Sweeps: 3, MaxIter: 30, Weights: refine.DefaultWeights, MergeSplit: true, Primitives: true}` for every preset and `CutoutStroke` 0 (bw) or 0.5 (poster, photo). `Validate` checks the ranges and that weights are finite and non-negative. Refinement applies only in `ModeSpline`; other modes ignore `Refine`.

`internal/refine` exports `Bezigon`, `FromSegments`, `(Bezigon).Segments`, `Mask`, `Weights`, `DefaultWeights`, `Options` (sweeps, iterations, weights), `Refine(b Bezigon, m Mask, o Options) Bezigon`, `MergeSplit(b Bezigon, m Mask, o Options) Bezigon`, `FitCircle(b Bezigon, tol float64) (centre geom.Vec2, r float64, ok bool)`, `Circle(centre geom.Vec2, r float64, start geom.Vec2, ccw bool) Bezigon`, `Coverage`.

### 10. CLI

`--refine` (bool), `--refine-passes N` (sweeps, default 3), `--cutout-stroke px` (default from preset). Help text says refinement applies to spline mode and roughly how much slower it is once measured.

### 11. Benchmark

`internal/bench` adds `rekha-refine` (bw + `--refine`) and `rekha-poster-refine` (poster + `--refine`); `make bench` runs them. The baseline `bench/results/<date>-phase3.md` records, per goal image, unrefined and refined PSNR, nodes and time, the exit-test verdict, and cutout with and without the stroke on `aa-abut` and `three-layer-stack`.

## Testing

- **Rasteriser.** Coverage of an axis-aligned square with integer corners is exactly 1 inside and 0 outside; a half-pixel shift gives exactly 0.5 on the shifted edge; the total coverage of a 4-cubic circle of radius 20 is within 0.5 % of πr²; moving a vertex by 1e-3 px changes no pixel by more than 1e-2 (continuity).
- **Synthetic recovery.** A random smooth 8-segment bezigon rasterised to a mask, control points perturbed by up to 1.5 px, refined: every point back within 0.05 px. Repeated with the mask rendered by `rasttest` (even-odd scanline, a different rasteriser) to check tolerance to rasteriser mismatch.
- **Priors.** A figure-eight has `E_spt > 0` and refinement against its own convex mask removes the crossing; a square with four right-angle joints keeps `E_apt ≈ 2π` and its corners after refinement against a sharp square mask; a near-zero handle gives a large `E_hpt` and refinement lengthens it.
- **Masks.** On `circle-aa` under bw the mask on a boundary pixel equals the pixel's grey share; a hole mask is the complement; extension band pixels are hard 1.
- **Merge/split.** A circle fitted by Phase 1 into 6 cubics merges to 4 or fewer with `E_data` within 1 %; a cubic fitted across a corner splits.
- **Primitives.** `circle-aa` refined becomes exactly four cubics with handle ratio 0.5523 ± 0.001 and centre within 0.1 px of (128, 128); `rounded-rect` is not a circle; straight cubics come out as `L`.
- **Integration.** `--refine` spline PSNR on `circle-aa` (bw) and `aa-abut` (poster) at least 1 dB above unrefined, measured in-test with `rasttest` coverage against the input; goldens `<name>-refine-spline.svg` for the seven goal images; determinism `GOMAXPROCS=1` vs `8`; bw and colour goldens unchanged with refinement off; cutout paths carry the stroke attributes under poster and not under bw or stacked.
- **Fuzz.** `FuzzRefine` over random bezigons (`N` 1..12, coordinates in a 32×32 box, including coincident points and zero area) against random masks: no panic, no NaN, energy not above start; `FuzzSegment` extended to run `--refine` on spline output and pass the Phase 1 sanity oracle.
- **Exit test.** `make bench`; the header of the Phase 3 baseline checks the goals.

## Left to the builder

- Accumulation buffer layout and the line-to-cells walk; whether coverage is float32 or float64 internally.
- L-BFGS two-loop recursion details and the Armijo constants beyond those stated.
- How windows are padded when a piece's segments move during optimisation (the window is fixed per piece; padding of 3 px is the stated default).
- Flattening point count for the SPT test (16 per segment is the default; fewer is acceptable if profiling demands).
- Kåsa fit implementation and the sampling density for straight-cubic detection.
- Worker-pool reuse of accumulation buffers.

## As built (2026-11-09)

Constants the implementation settled by measurement where the design above gave a starting value; the ledger of the executing plan records each ruling.

- Data term scaled by `DataScale = 8` (squared coverage error ×64) and the handle prior weighted 0.5 (design: 0.1): with the published weights the data term could not outbid the barrier once a handle hooked onto a displaced anchor, and corners rounded.
- Length prior normalised by the initial length `l0`, like the data term: the raw prior shrank thin shapes (`thin-lines` lost 5 dB; now gains 4).
- Each sweep moves a piece's points at most 2 px (a per-point trust box) and evaluates the data term over the rows the piece can reach, not its whole window; the self-intersection prior tests the piece's edges against the edges whose box meets that reach, with arc lengths tabulated once per piece; the merge pass measures the energy change over the window of the segments it touched. Together these keep refinement linear in the outline length (a 3.2-megapixel logo: 14 s, from 140 s).
- Segments straighter than 0.25 px are written as lines (`lineTol`).
- A circle is accepted only when every sample is within `min(0.5·FitTolerance, 0.1·r)` of it: an absolute tolerance alone took every 2-px square for a circle (`checkerboard-2px` fell from exact to 20 dB).
- A hole mask complements only measured shares; a boundary pixel with no measure of the colour keeps its hard value. Complementing the hard value zeroed fully-covered pixels on hole edges and cost cutout refinement 7 dB on `logo-holes`.
- The cutout stroke goal (within 1 dB of stacked on `aa-abut` and `three-layer-stack`) is not met at any width: two independently refined curves of one boundary leave a sub-pixel seam that the 1:1 PSNR sees, and a 0.5 px stroke thickens every edge (2–7 dB). The default is therefore 0 (the stroke is opt-in, and cutout output without the flag is the Phase 2 output, as the non-goals require); shared boundaries remain the real fix.
- Review fix pass (same day): the short-handle barrier is shifted to zero beyond a third of the chord, and a handle-order prior (`Opt` 0.1, linear in the pixels by which a handle's projection leaves the chord or passes the other's; on the goal images it cuts the total violation from 107 px to 2 px at the same PSNR) stops crossed, reversed and overshooting handles that knotted the caps of a 1-px line; a merge must also hold over its own window (mergeRise of the local energy plus an RMS edge error of 0.1 px), since a long outline's 1 % budget paid for rounding corners; the merge budget is fixed at the pass's start. The refined goldens are compared as geometry (same paths, node counts within 10 %, renderings within 40 dB) because the plain fit already differs by ulps between amd64 and arm64 and the optimiser turns ulps into different merge decisions; the refine arithmetic is nonetheless wrapped against FMA fusion.


# Phase 3b design: shared boundaries, small features, primitives, speed, references

Status: design, 2026-11-10. Follows `2026-11-09-phase3-refine-design.md` (Phase 3, merged to main as f36a1f1) and the Phase 3 roadmap row in `design/roadmap.md`, whose items "cutout mode: shared-boundary planar map", "primitive detection (lines, arcs, circles, rectangles)" and "post-fit simplification" Phase 3 left open. The user opted to skip design questions; every choice below is the author's call. Phase 1–3 stay the base: segmentation, contour tracing, stacking, Schneider fitting and the Yang refinement are unchanged except where a section below says otherwise.

## Intent

Phase 3 measured where Rekha stands: it beats Potrace and VTracer on the anti-aliased synthetics with `--refine`, ties VTracer on a 3.2-megapixel colour logo with a fortieth of the paths, and loses in three places that a Vector Magic user notices at once:

1. **Cutout seams.** Each colour's outline is fitted and refined on its own, so two shapes that meet along one pixel boundary carry two slightly different curves and the page shows through the gap. The Phase 3 stroke stopgap failed its goal at every width (9–15 dB behind stacked on `aa-abut`, `three-layer-stack`, `logo-holes`).
2. **Small features in plain mode.** On 35-px glyphs (stroke 8 px) the unrefined fit turns an oval counter into a hexagon and notches a stem; Potrace keeps the oval. The cause is a fixed absolute tolerance (1 px plus half a pixel of quantisation) that is 20 % of the radius on an 8-px arc, so a 45° arc "is" a straight line, and splitting makes it worse by producing shorter runs that pass the straight test trivially.
3. **No rectangles or arcs.** Vector Magic's signature on flat art is that a rounded rectangle comes out as four lines and four quarter circles and a rotated rectangle as four points. Phase 3 recognises only whole circles.

Two more items make the rest usable: refinement of a 3.2-megapixel logo takes 14 s (VTracer 0.36 s), and the claim "Vector Magic grade" has never been measured because Vector Magic has no CLI. Phase 3b fixes the three quality gaps, cuts refinement time by at least three times, and gives the benchmark a way to score reference outputs that a person downloads by hand.

## Goals

- **Shared boundaries (colour cutout).** In `--hierarchical cutout` under the colour presets, every boundary between two emitted shapes is one curve, fitted once and refined once; both shapes' path data contain the same coordinates (one reversed). Exit test on `aa-abut`, `three-layer-stack`, `logo-holes` and `poster-discs` under poster with `--refine`, cutout without stroke at 1:1: PSNR **within 3 dB of stacked** on each (Phase 3 gaps: 13.1, 9.2, 15.1, 1.9 dB). Also, each of the four is at least 3 dB above its Phase 3 cutout value, except `poster-discs` where the stacked ceiling is 1.9 dB away.
- **Small features (plain spline, any preset).** A new synthetic `small-glyphs` (rings with 5-px counters, stems with flags, small rounded rectangles at 24–40 px) traced with the bw preset and no `--refine` scores a **PSNR at least that of Potrace** on the same image with **no more nodes than Potrace** (both from `make bench`; the Go test pins the numbers the first baseline records). On the seven Phase 3 goal images, plain spline loses no more than 0.3 dB and the total node count is at most 1.1× Phase 3's.
- **Primitives (with `--refine`).** `rounded-rect` comes out as exactly 8 segments: 4 lines and 4 quarter-circle cubics with handle ratio 0.5523 ± 0.002 and a common radius within 0.2 px of 24. A new synthetic `rect-rot` (a 120×80 rectangle rotated 20°, anti-aliased) comes out as exactly 4 nodes, each within 0.1 px of the true corner. No goal image loses more than 0.2 dB from the primitive pass; total nodes on the goal set do not rise.
- **Speed.** Measured under poster, stacked (the default hierarchy, where §6 is the only change). A new 2048×2048 synthetic `big-poster` (several hundred discs, rings and rounded rectangles in six colours, about 2 000 segments after fitting) refines **at least 3× faster** than Phase 3's code (`BenchmarkRefineBigPoster`, Phase 3 value recorded in the baseline) with PSNR within 0.2 dB of Phase 3's refined result. Each 256×256 goal image still traces with `--refine` in under 0.5 s. Output is identical for `GOMAXPROCS=1` and `8`.
- **Reference tools.** `make bench` scores SVG or PNG outputs that a person downloaded from Vector Magic or Vectorizer.AI into `bench/reference/<tool>/`, in the same table and with the same metrics as the binaries, and `docs/benchmark.md` says exactly how to produce those files. The harness has a fixture test; the comparison itself runs when the files exist.
- Everything else holds: pixel and polygon goldens stay byte-identical; spline goldens of every preset may move only under the §4 PSNR check; determinism across `GOMAXPROCS`; no panic or NaN under fuzz; `CGO_ENABLED=0`, wasm build, no new dependencies; `make lint test wasm-check` green on arm64 and under `GOARCH=amd64`.

## Non-goals

- Sub-pixel placement of junction vertices (where three colours meet). They stay on the crack vertex; incident arcs end there exactly. Phase 3c if the benchmark shows it matters.
- Shared boundaries in the bw preset (only the foreground is emitted, nothing abuts) or in stacked mode (the extension band already hides joins). Both keep the Phase 1/2 code path.
- Ellipses, rotated rectangles with rounded corners as single primitives, general snapping of lines to the axes, output optimisation levels (a size knob), analytic gradients, GPU, SIMD.
- Turning `--refine` on by default; changing any preset default (`cutout` stays the default for `bw`, `stacked` for `poster`/`photo`).
- Fetching reference outputs automatically (no CLI exists; uploading the corpus to a third-party site is the user's action).
- Junction-aware merging across arcs; an arc's end joints are never merged.

## Anti-goals

- Any Potrace/gotrace-derived code. The small-feature work must not read Potrace's source; the Potrace score is a black-box target.
- Output that changes with thread count, map iteration order or scheduling.
- A primitive that is accepted on geometry alone and lowers fidelity: every snap is checked against the data energy and reverted if it rises.
- A planar map that disagrees with the per-colour contours: the set of pixels inside each cutout shape must be the same as Phase 2 traces today.

## Constraints (carried)

Module `github.com/vexyl-ai/rekha`, Go 1.26, `CGO_ENABLED=0`, Apache-2.0, DCO sign-off, no Claude attribution trailer, keyed `geom` literals, `float64()`-wrapped float products in `internal/refine` and `internal/fit`, refined goldens compared as geometry (same paths and fills, nodes within 10 %, 4× renders within 36 dB), always `GOARCH=amd64 go test ./...` before pushing.

## 1. Planar map for colour cutout

New package `internal/planar`.

```go
type Arc struct {
    Pts    []geom.Point // crack vertices, first and last are junctions (equal when Closed)
    Left   uint8        // label on the walker's left; None outside the image
    Right  uint8        // label on the walker's right: the colour whose loop owns this direction
    Closed bool         // an island: one loop with no junction
}
type Ref struct{ Arc int; Reverse bool }
type Loop struct {
    Colour uint8
    Area2  int   // of the assembled polygon
    Refs   []Ref // in walk order; assembling them reproduces contour.TraceFunc's loop
}
type Map struct {
    Arcs  []Arc
    Loops []Loop // in the order Segmentation.layers would produce the contours (colour, then y, then x)
}
const None uint8 = 255
func Build(labels []uint8, w, h int, saddle func(c uint8) func(x, y int) bool) Map
```

- `Build` runs `contour.TraceFunc` per colour with the segmentation's saddle rule (so every pixel lies inside exactly one shape, as Phase 2), then cuts each loop at **junction vertices**: a crack vertex whose four surrounding pixels (outside the image counts as `None`) carry three or more distinct labels, or carry two labels in the diagonal (saddle) pattern. Image-border vertices are junctions only by the first rule.
- Every unit crack edge between two different labels is walked exactly twice, once per side, so every arc has a partner with the same edge set in reverse. Arcs are deduplicated by the key (lowest undirected edge id, highest undirected edge id, edge count); undirected edge id = `2·(y·(w+1)+x) + {0: east, 1: south}` of the edge's lower-left vertex. The partner's loop references the arc with `Reverse: true`. A loop without junctions is one `Closed` arc, shared with the enclosing shape's hole.
- Edges on the image border have one side; their arcs have `Left == None` and no partner.
- `Loop.Refs` assembled (reversing where marked, dropping the repeated junction point) equals the contour `TraceFunc` produced, point for point; `Area2` is the same. A test asserts this on every corpus image and on fuzzed label images.

`Finish` under colour cutout in spline mode builds the map once, with or without `--refine`, fits every arc once (§2), refines owned arcs once when refining (§3), then assembles each shape's outer and holes from the arcs' final segments. Stacked mode, the bw preset, and pixel and polygon modes do not build the map (author's call: polygon cutout keeps its per-contour seams; pixel mode is exact already).

## 2. Fitting arcs

`spline(pts, o)` in `trace.go` becomes `fitLoop(dense, o)` and gains an open variant `fitArc(dense []geom.Vec2, o) []fit.Segment` for arcs with junction ends:

- The ends are fixed vertices at the junction crack points. Their tangents are least-squares flank directions over `flankWindow(len)` dense points (the existing `flankDir`/`fit.FitLine` machinery), as a corner vertex gets today.
- Interior corners, corner refinement, bends and straight-edge breakpoints work as in `spline`, on the open polyline (no wrap). The staircase and RDP steps use the open-polyline variants (`geom.RemoveStaircasesIndices` and `RDPClosedIndices` gain open counterparts; the open RDP keeps both ends).
- Output starts exactly at `dense[0]` and ends exactly at `dense[last]`.
- A `Closed` arc goes through `fitLoop`, unchanged from today, so island shapes fit as before.
- An arc shorter than 4 dense points is a single line.

Both sides then take the same `[]fit.Segment`; the reverse side reverses the slice and each cubic's control points. The SVG writer prints the same numbers in both paths. A test asserts the reversed path data of a shared arc equals the forward one, number for number, on `aa-abut` and `three-layer-stack`.

## 3. Refining shared arcs

- **Ownership.** Each arc is owned by one loop: an emitted loop before an omitted one (the white page owns nothing), then the loop whose `Colour` is lower, ties broken by loop order. Border arcs are owned by their only loop. An arc between two omitted loops is not refined.
- **Frozen geometry.** `refine.Options` gains `Fixed []bool` (per anchor: it does not move and its joint is never merged) and `Frozen []bool` (per segment: its control points do not move, it is never split or merged, and a piece whose two segments are both frozen is skipped). A piece with one frozen segment optimises only the other segment's free points; the shared anchor between an owned and a frozen segment is a junction and therefore `Fixed`. `clampAll` and the trust boxes leave fixed points where they are. `FitCircle` is attempted only when nothing is fixed (island loops).
- **Jacobi round.** All loops refine in parallel in one round: each loop's bezigon is assembled from its arcs, owned arcs are free, every other arc is frozen at its state at the start of the round, and the loop's mask is the Phase 3 coverage mask of its own colour (boundary shares are symmetric between the two colours, so refining from the owner's side is the same target as from the other). After the round, each arc's refined segments are copied back from the owner, and every loop re-assembles from the arcs. One round; a second is left to the builder to measure and is off unless it gains 0.5 dB on the goal set.
- **Merge/split** runs within the owner's loop with the fixed joints respected. **Primitives** (§5) run on assembled loops with the same `Fixed`/`Frozen` constraints: an arc run is snapped only if every joint it moves is free.
- The stroke stopgap stays available (`--cutout-stroke`, default 0). The baseline records cutout with stroke 0 and 0.1.

## 4. Small features in plain fitting (`internal/fit`, `trace.go`)

- **Relative line tolerance.** In `fitCubic`, the straight-line shortcut uses `lineTol = max(lineFloor, min(maxErr, lineRel·R))` where `R = L / (2·sin(Δθ/2))` is the radius of the circular arc with the run's chord `L` and the turn `Δθ` between the given end tangents (`Δθ < 1e-3` gives `R = ∞`), `lineRel = 0.05` and `lineFloor = 0.25` px (half the quantisation noise). A run that fails the straight test at the relative tolerance but would have passed at `maxErr` is fitted as a cubic, which `maxErr` still governs; it is not split for that reason alone.
- **Split floor.** `fitCubic` never splits a run with fewer than `minSplitPoints = 6` points; it accepts the best cubic. Runs with fewer than 4 points stay lines as today.
- **Short-run tangents.** `FitRun`'s tangent window (`k = max(2, min(4, n/3))`) is unchanged; the arc fitter supplies junction tangents, so no new estimator is needed.
- Anything else the `small-glyphs` fixture reveals (the stem notch suggests corner refinement moving a vertex off a short flank) is diagnosed on the fixture and recorded as a ruling; the exit test is the judge, not this list.
- §4 applies to every preset and both hierarchies whenever spline fitting runs (author's call: there is no flag to gate it and the small-feature fix is wanted everywhere). Spline goldens of all families may therefore change; each is regenerated only after a PSNR check against the input (the 4× `rasttest` render of the new golden is not more than 0.3 dB below the old golden's), and colour cutout spline goldens additionally change because of §1–3. Pixel and polygon goldens stay byte-identical.

## 5. Primitives (`internal/refine`, after merge/split, under `o.Refine.Primitives`)

- **Circular arcs.** Maximal runs of one or more consecutive curved segments bounded by straight segments or by the loop's fixed joints, not the whole loop (author's call: a 24-px corner is often one cubic after merge/split, so a single segment is a candidate), whose `circleSamples` points per segment lie within `min(0.5·FitTolerance, 0.1·r)` of one Kåsa-fitted circle and whose interior joints are smooth (turn under 5°). The run is replaced by arc cubics: pieces of at most 90° with handle ratio `4/3·tan(θ/4)`, interior anchors moved onto the circle, end anchors kept (they may be fixed junctions), the centre and radius refitted through the end anchors when they are not on the circle within the tolerance; a run whose end anchors cannot be kept on a circle within tolerance is left alone.
- **Rectangles.** A loop of exactly four segments after merge/split, all straight (`straightCubic` within `lineTol`), with every turn within `rectAngleTol = 2°` of 90°, becomes the least-squares rectangle: orientation is the circular mean of the four edge directions modulo 90°, centre the mean of the corners, extents the means of opposite sides; an orientation within 1° of the axes snaps to exactly 0°; accepted when every original corner is within `min(0.5·FitTolerance, 0.1·shortest side)` of its new position. Rounded rectangles arise from lines plus arcs, not as a primitive.
- **Energy guard.** After each snap the loop's data energy over `windowAround` the changed segments must not rise by more than `primRise = 2 %` of its value before the snap (the merge budget's rate); otherwise the snap is reverted. Circles keep the Phase 3 rule and gain the same guard.
- **Common radius.** After the arc pass, arcs on one loop whose radii agree within 10 % of their mean are refitted at the mean radius, each under the energy guard, so a rounded rectangle's four corners share one radius.
- Order: circles, then arcs, then common radius, then rectangles.

## 6. Refinement speed (`internal/refine`)

- **Self-intersection out of the energy.** `Weights.Spt` is removed. The self-intersection count of a piece's polyline against the candidate edges is computed once when the piece starts, and the L-BFGS admissibility callback rejects any step that raises it. This removes the prior from every finite-difference gradient evaluation (24 energy evaluations per iteration) and runs it once per line-search trial; the merge/split pass checks the count of the whole loop once per candidate as before.
- **Parallel pieces.** Within one loop, pieces `j ≡ r (mod 3)` share no moving point (a piece moves anchors `j+1` and the handles of segments `j` and `j+1`), so each residue class is optimised in parallel on its own scratch, reading the loop state at the start of the class, and the class's results are written back together. The sweep order is class 0, 1, 2. Results do not depend on worker count because every piece reads the same snapshot.
- **Early exit.** A sweep that lowers the loop energy by less than `sweepStop = 0.5 %` of its start value ends the sweeps.
- Loops whose segment count is not a multiple of 3 wrap: the last piece of a class may share a point with the first piece of the same class, so that piece joins the next class instead (the builder settles the exact rule; the invariant is that no two pieces optimised together share a moving point).
- `Sweeps` and `MaxIter` defaults stay 3 and 30. `Weights` validation loses `Spt`; the CLI has no new flag.

## 7. Reference tools (`internal/bench`, `docs/benchmark.md`)

- `bench.Tool` gains `Dir string`. A tool with `Dir` set runs no binary: for input `<cat>/<name>.png` it looks for `<Dir>/<name>.svg` (scored as any SVG: PSNR via the rasteriser, paths, nodes, bytes; time empty) or else `<Dir>/<name>.png` (a raster the site rendered at the input size; PSNR only, paths/nodes/bytes empty). Missing both gives a row with the note `no reference`. `BuiltinTools` adds `vectormagic` (`bench/reference/vectormagic`) and `vectorizer-ai` (`bench/reference/vectorizer-ai`); the directories are git-ignored except for a README each.
- `docs/benchmark.md` gains the procedure: for each `testdata/corpus/**/*.png`, upload to the site, download the SVG (paid tier) or save the preview PNG at the input's pixel size (free), name it after the input, drop it in the directory, run `make bench`.
- A fixture test drops a known SVG and a known PNG into a temporary reference directory and asserts the rows.

## 8. Corpus additions (`internal/corpus`)

`Synthetic` gains `Size int` (0 means 256). New images: `small-glyphs` (256), `rect-rot` (256), `big-poster` (2048). `small-glyphs` holds, in three rows at heights 24, 32 and 40 px: a ring whose outer is an ellipse (ratio 0.8) and whose counter is a 5-px-radius disc; a stem 1/4 as wide as it is tall with a diagonal flag from the top-left; a rounded rectangle with corner radius a fifth of its height. All anti-aliased with 4×4 samples, black on white.

## 9. Public API and CLI

No new `Options` fields and no new flags. `RefineOptions.Weights` loses `Spt` (a breaking change to a struct nobody outside the repo uses yet; noted in the README). `bench` gains the reference tools.

## Testing

- **Planar.** On every corpus image and on fuzzed label images: every arc has exactly one partner or `Left == None`; assembling each loop reproduces `contour.TraceFunc`'s contour and `Area2`; the junction rule finds every vertex with three labels and every saddle; the pixel set inside each assembled shape equals Phase 2's.
- **Arc fitting.** An open quarter-circle arc with junction ends fits within tolerance, starts and ends exactly at the junctions, and its reverse has identical numbers; a closed arc goes through `fitLoop` and matches `spline`'s output byte for byte.
- **Shared refinement.** On `aa-abut` with `--refine`, the two shapes' shared arc has identical reversed path data; fixed anchors do not move; frozen segments are unchanged; no joint at a junction is merged; the cutout PSNR goal is asserted in-test with `rasttest` at the 3 dB margin.
- **Small features.** `small-glyphs` under bw plain: PSNR and node count against the pinned Potrace numbers; a 45° arc of radius 8 px with tangents fits as a cubic, not a line; a 6-point run is not split; thin-line and checkerboard goldens unchanged.
- **Primitives.** `rounded-rect` 8 segments; `rect-rot` 4 nodes within 0.1 px; a rectangle whose data energy would rise is reverted; a run of arcs with a fixed junction keeps the junction.
- **Speed.** `BenchmarkRefineBigPoster` with the Phase 3 number in the results file; identical output for `GOMAXPROCS=1` and `8` on `big-poster`; self-intersection never increases through a sweep.
- **Reference tools.** Fixture test as in §7.
- **Fuzz.** `FuzzPlanar` over random label images (≤ 16×16, ≤ 4 labels): no panic, invariants above; `FuzzSegmentRefine` extended to cutout.
- **Exit test.** `make bench`; `bench/results/<date>-phase3b.md` checks every goal above, and records Logo.jpg (not in the repo) times by hand.

## Left to the builder

- The data structure for edge-to-arc lookup during deduplication (a map keyed by the triple is the default).
- How `fitArc` shares code with `fitLoop` (one function with a `closed` flag is the default).
- The order of write-back after the Jacobi round and the scratch allocation per worker.
- Kåsa fit through two fixed end anchors (constrained least squares or projection; projection is the default).
- The circular-mean implementation for rectangle orientation.
- Exact composition of `big-poster` beyond "about 2 000 segments in six colours".

## As built

Filled in by the executing plan's final message and ledger.

# Phase 1 design: bilevel tracing MVP

Status: design, 2026-11-09. Derived from `Go SVG image tracing tool.md` (Phase 1 row, component choices) and the Phase 0 baseline in `bench/results/2026-11-09-baseline.md`. User opted to skip design questions; every choice below is the author's call.

## Intent

First real tracing: a black-and-white image in, a clean SVG of filled paths out, through the same `rekha.Options` that will later drive colour. Quality target is Potrace on clean bilevel shapes, measured by the Phase 0 harness.

## Goals

- `rekha in.png` produces a valid SVG with one `<path>` per black shape, holes included, in `pixel`, `polygon` or `spline` mode.
- On the synthetic corpus (`star-5`, `circle-aa`, `circle-hard`, `rounded-rect`, `thin-lines`), spline mode is within **2 dB PSNR of Potrace** and within **1.5× Potrace's node count**.
- Deterministic output: same input and options give byte-identical SVG, any GOMAXPROCS.
- No panic on any input: fuzzed bitmaps, 1×1 images, all-black, all-white, 2-px checkerboard.
- Builds for `GOOS=js GOARCH=wasm` and with `CGO_ENABLED=0`.

## Non-goals

- Colour, layers, quantisation (Phase 2).
- Direct bezigon refinement, primitive detection, cutout mode (Phase 3).
- Centerline tracing, PDF/EPS output (Phase 5).
- Optimal-polygon dynamic programming in the Potrace manner (Phase 5, legal gate).
- Sub-pixel boundaries from anti-aliasing (Phase 2 soft masks). Phase 1 thresholds first.

## Anti-goals

- Any resemblance to Potrace or gotrace source: structure, identifiers, algorithm details beyond what the `potrace(1)` man page describes as user-visible behaviour (turn policies, threshold semantics). Nobody reads that code.
- A dependency outside the standard library and `golang.org/x/image`.

## Constraints (carried from Phase 0)

Module `github.com/vexyl-ai/rekha`, Go 1.26, `CGO_ENABLED=0`, Apache-2.0, DCO sign-off, VTracer flag vocabulary, exit codes 0/1/2, `-` for stdio.

## Pipeline

```
decode → grey → threshold → Bitmap → contours (crack walk, turn policy)
      → hierarchy (outer + holes) → despeckle (by area)
      → [pixel mode: emit]
      → simplify (collinear merge, staircase pass, RDP) → [polygon mode: emit]
      → corners (turn angle) → Schneider fit per run on dense points → [spline mode: emit]
      → SVG writer
```

### 1. Decode and grey

Standard library decoders (PNG, JPEG, GIF; first GIF frame). Composite onto white, then luminance `Y = 0.2126 R + 0.7152 G + 0.0722 B` on 8-bit sRGB values (no linearisation; matches what thresholding tools do). Result: `[]uint8` of W×H.

### 2. Threshold

`Options.Threshold float64` in `0..1`, default `0.5`; pixel is foreground (black) when `Y/255 < Threshold`. `Options.AutoThreshold bool` (CLI `--threshold auto`) runs Otsu's method on the grey histogram and overrides `Threshold`. Adaptive local thresholding (Bradley mean-window) is listed in the report but deferred to the first real scan corpus; Otsu covers logos and clean scans.

### 3. Bitmap

`internal/bitmap.Bitmap{W, H int; bits []uint64}` packed rows, `Get(x, y) bool` returning false outside bounds so border handling needs no padding. `FromGray(gray []uint8, w, h int, threshold uint8) Bitmap`.

### 4. Contour tracing (crack following)

Own design. Paths run along pixel edges (cracks), so coordinates are integers in `[0..W]×[0..H]`.

- An **edge** is a unit segment between two lattice points that separates a foreground pixel from a background pixel (or the outside). Direct every edge so that foreground is on its **left** when walking in image coordinates (x right, y down). With this convention outer boundaries come out clockwise on screen and holes counter-clockwise, and the SVG `nonzero` fill rule renders holes correctly without any extra bookkeeping.
- Walk: pick the lowest unused edge in raster order, follow edges end-to-start until returning to the start vertex. At each vertex the next edge is unique except at a **saddle** (2×2 block with diagonal foreground), where two outgoing edges exist. `Options.TurnPolicy` picks: `minority` (default; turn toward the colour that is rarer in the whole image), `majority`, `black` (keep foreground connected: turn so the walk hugs black), `white`. Names and meanings follow the `potrace(1)` man page's user-visible description; the implementation is ours.
- Each closed walk is a `Contour{Pts []Point; Area int}` with signed area from the shoelace formula; positive is outer, negative is hole, by the convention above.

### 5. Hierarchy and despeckle

- Assign each hole to the smallest-area outer contour that contains its first vertex (point-in-polygon on the outer's integer polygon). Holes with no parent are dropped (cannot happen for valid walks; guard anyway).
- Despeckle: drop any outer whose `|Area| < Options.FilterSpeckle` together with its holes; drop any hole whose `|Area| < FilterSpeckle`. Area is in pixels, so the VTracer flag meaning carries over.
- Result: `[]Shape{Outer Contour; Holes []Contour}`, sorted by outer's first vertex (y, then x) for determinism.

### 6. Simplify (polygon mode)

Three passes over each contour's integer polyline, in order:

1. **Collinear merge:** drop any vertex whose incoming and outgoing edges have the same direction.
2. **Staircase pass:** a maximal run of alternating ±90° turns where every segment is ≤ 2 px is a diagonal; replace the run by its two endpoints. This leaves real 1-px-deep notches alone when they are isolated (a single turn pair is not a run; the minimum run is 3 turns).
3. **Ramer–Douglas–Peucker** on the closed polygon with tolerance `Options.SimplifyTolerance` (default `0.75` px). Closed-polygon RDP: split at the two vertices farthest apart, simplify each half, rejoin.

Output stays on integer-or-half coordinates only incidentally; the polygon vertices are a subset of the original lattice points.

### 7. Corners

On the simplified polygon, a vertex is a **corner** when the absolute turn angle between incoming and outgoing segments exceeds `Options.CornerThreshold` degrees (default `60`, the VTracer default for the same flag). Corner indices are mapped back to indices in the dense (unsimplified, collinear-merged) contour, since fitting runs on dense points.

### 8. Curve fitting (spline mode)

Schneider 1990 (Graphics Gems, "An Algorithm for Automatically Fitting Digitized Curves"), implemented from the paper's description:

- For each run of dense points between consecutive corners (closed loop when a contour has no corner: split at the vertex farthest from the centroid, fit as one run whose end tangents are averaged so the join is G1):
  1. Chord-length parameterise.
  2. End tangents from a least-squares line through the first/last `k` points (`k = min(4, n/3)`, AutoTrace-style, per the report's warning that two-point tangents are noisy). At a corner the tangent is one-sided.
  3. Solve the 2×2 system for the two handle lengths; fall back to Wu/Barsky heuristic (handle = chord/3) when the system is near-singular.
  4. Compute max squared distance from points to curve. If ≤ `Options.FitTolerance²` (default `1.0` px) accept. Else if ≤ 4× tolerance² run up to 4 Newton–Raphson reparameterisation iterations and retry. Else split at the point of max error with a shared tangent and recurse.
- Degenerate runs (< 4 points) become a line segment, emitted as `L`.
- Each shape becomes a list of subpaths of `Segment{Kind Line|Cubic; P [4]Vec2}`.

### 9. SVG writer

`internal/svg.Writer`. Emits:

```
<svg xmlns="http://www.w3.org/2000/svg" width="W" height="H" viewBox="0 0 W H">
<path fill="#000000" d="..."/>
...
</svg>
```

- One `<path>` per shape; outer first, then holes as further `M` subpaths; relies on default `nonzero` with the orientation guarantee from §4.
- Coordinates formatted with `Options.PathPrecision` decimals, trailing zeros and trailing dot trimmed, `-` serving as separator where possible, no separator before a negative number. Pixel mode uses `H`/`V`; polygon uses `L`; spline uses `C` and `L`. Absolute commands throughout (relative would shrink bytes; it is a later optimisation and complicates golden tests).
- Deterministic: shapes already sorted; `strconv.FormatFloat` with fixed precision.

### 10. Public API (root package)

```go
type Options struct {               // existing fields kept
    ...
    Threshold         float64 // 0..1, default 0.5
    AutoThreshold     bool    // Otsu
    TurnPolicy        TurnPolicy // minority|majority|black|white, default minority
    SimplifyTolerance float64 // px, default 0.75
    CornerThreshold   float64 // degrees, default 60
    FitTolerance      float64 // px, default 1.0
}
type Result struct { Width, Height int; Shapes []Shape }
func Trace(img image.Image, o Options) (*Result, error)
func TraceReader(r io.Reader, o Options) (*Result, error)      // decodes, then Trace
func (r *Result) WriteSVG(w io.Writer, o Options) error
```

`Validate` extends to the new fields. Phase 1 ignores `Preset` other than `bw`, `ColorPrecision` and `MaxColors` (validated, unused); `poster`/`photo` presets return a `ValidationError{"Preset", "not implemented until Phase 2"}` from `Trace`, not from `Validate`, so option plumbing in CLI/server/WASM can already accept them.

### 11. CLI

`cmd/rekha` wires inputs → `TraceReader` → `WriteSVG` → `OutputPath`. New flags: `--threshold <0..1|auto>`, `--turn-policy`, `--simplify`, `--corner-threshold`, `--fit-tolerance`. Decode errors and write errors exit 2; the message names the input file.

### 12. Benchmark integration

`internal/bench.BuiltinTools` gains `rekha` with `Bin` resolved from a new `rekha-bench -rekha <path>` flag (default `bin/rekha`, made absolute), `Args: ["--preset","bw","-o","{out}","{in}"]`, `Input: "png"`. Baseline re-recorded as `bench/results/<date>-phase1.md`.

## Testing

- Unit tests per package with hand-built bitmaps: single pixel (one square contour, area 1), 2×2 checkerboard (turn policy decides 1 vs 2 contours), ring (outer + hole, hole orientation negative), touching-corner shapes.
- Golden SVGs under `testdata/golden/<image>-<mode>.svg` for the synthetic corpus; byte-exact; regenerated only via `go test ./... -update`.
- Property: pixel mode rasterised back equals the thresholded bitmap exactly (test-only tiny scanline rasteriser for axis-aligned polygons, since pixel mode is all `H`/`V`).
- Determinism: trace under `GOMAXPROCS=1` and `8`, compare bytes (Phase 1 is single-threaded per image, so this guards future parallelism).
- Fuzz: `FuzzTrace` on random W×H ≤ 64 bitmaps, all modes, all turn policies; no panic, every contour closed, every hole has a parent.
- Exit test: `make bench` with `rekha` added; goals above checked by hand from the table and recorded in the baseline file's header.

## Left to the builder

- Exact RDP closed-polygon split heuristic.
- Newton iteration count and singularity epsilon in Schneider.
- Internal package boundaries beyond those named (`bitmap`, `contour`, `geom`, `fit`, `svg`).
- Whether `thin-lines` (1-px lines) needs special handling; measure first.

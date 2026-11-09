# Phase 2 design: colour tracing with stacked layers

Status: design, 2026-11-09. Derived from `Go SVG image tracing tool.md` (Phase 2 row, "component choices", "Layering: stack by default", "One Options struct") and `design/roadmap.md` (Phase 2 row). User opted to skip design questions; every choice below is the author's call. Phase 1 (`2026-11-09-phase1-bilevel-design.md`) is the base: its pipeline from bitmap to SVG is reused unchanged per colour layer.

## Intent

A colour image in, a stacked SVG of filled paths out: one layer per colour region, painted bottom to top so that shapes extend beneath the shapes above them and no shape needs holes. The same `rekha.Options` and the same `rekha` CLI drive it through the `poster` and `photo` presets. Quality target is VTracer 1.0.0-alpha.4 on the colour part of the synthetic corpus, measured by the Phase 0 harness.

## Goals

- `rekha --preset poster in.png` produces a valid SVG whose paths, painted in document order, reproduce the quantised image.
- On the colour synthetics (`two-colour-abut`, `three-layer-stack`, and the new `poster-discs`, `aa-abut`, `logo-holes`), poster preset in spline mode is within **2 dB PSNR of VTracer** and within **1.5× VTracer's node count**. On `gradient-h`, photo preset is within 2 dB of VTracer at no more than 2× its paths.
- Phase 1 behaviour and goldens unchanged under `--preset bw`.
- Deterministic output regardless of `GOMAXPROCS`; layers are traced in parallel.
- No panic on any input: fuzzed colour images, 1×1, single colour, 256 distinct colours, fully transparent.
- Builds for `GOOS=js GOARCH=wasm` and with `CGO_ENABLED=0`; still only `golang.org/x/image` outside the standard library.
- Expensive segmentation and cheap finishing are separate public steps (`Segment`, `Finish`) so a UI can re-fit when only a smoothing slider moves (report §API).

## Non-goals

- Seam-free cutout with shared boundaries, primitive detection, post-fit simplification, bezigon refinement (Phase 3). A naive `cutout` mode (shapes with holes, no stacking) is included because it is nearly free.
- Sub-pixel boundaries from the coverage masks. Phase 2 computes and stores per-pixel soft coverage; Phase 3 consumes it.
- Semantic segmentation, external masks (`Options.ExternalMasks`, Phase 6 hook). The `Segmentation` type is designed so that hook can later replace the quantiser.
- Real-image corpus additions. The provenance rule needs sourced CC0 images; this phase adds synthetics only.
- Dithering detection, gradient meshes, strokes.

## Anti-goals

- Any Potrace/gotrace-derived code (unchanged).
- Output that changes with thread count or map iteration order.
- A per-layer memory footprint proportional to palette size × image size: layers are traced from one label map, not from K stored bitmaps.

## Constraints (carried)

Module `github.com/vexyl-ai/rekha`, Go 1.26, `CGO_ENABLED=0`, Apache-2.0, DCO sign-off, VTracer flag vocabulary where it fits, exit codes 0/1/2, `-` for stdio, keyed `geom` literals, `make lint test wasm-check` green.

## Pipeline

```
decode → flat sRGB (composite on white)
  → bw preset: grey → threshold → Phase 1 unchanged
  → colour: RGB cells at ColorPrecision bits → OKLab
      → palette: fixed (Options.Palette) or Wu-style variance splitting + weighted k-means,
                 size bounded by MaxColors and ColorTolerance
      → nearest/second-nearest lookup table → Labels, Second, Blend per pixel
      → region cleanup: despeckle small components, absorb anti-aliasing slivers, compact palette
  → per colour (parallel): bitmap of that label → contour.Trace(turn policy) → shapes
  → stacked: nesting tree over all shapes, depth order, outer contours only
    cutout: shapes with holes, colour order
  → per shape (parallel): Phase 1 segments() in pixel/polygon/spline mode
  → paths with Fill = palette hex; merge consecutive same-colour paths; omit a white full-canvas bottom layer
  → SVG writer (unchanged)
```

### 1. Flat sRGB

`bitmap.Gray` already composites onto white. Add `bitmap.Flatten(img) (rgb []uint8, w, h int)` returning 3 bytes per pixel, same fast paths (opaque NRGBA/RGBA, Gray, Paletted, else RGBA draw). Transparent pixels become white.

### 2. Colour space

`internal/colour`: `OKLab{L, A, B float64}`, `FromSRGB8(r, g, b uint8) OKLab` (sRGB decode to linear, Ottosson's LMS matrices and cube roots), `(OKLab) ToSRGB8() (r, g, b uint8)` (clamped), `Dist(a, b OKLab) float64` (Euclidean). OKLab is the report's recommended space pending measurement; distances here are what `ColorTolerance` is expressed in (white to black is 1.0, a just-noticeable difference is about 0.02).

### 3. Cells and histogram

Pixels are binned to `ColorPrecision` bits per channel (default 6: 262 144 cells). A pass over the flat RGB counts pixels per occupied cell; each occupied cell gets the OKLab of its centre and its weight. Everything downstream of this pass works on cells, so cost is bounded by the cell count, not the pixel count. `ColorPrecision` 8 is allowed (16.7 M cells; the lookup table below is then 50 MB); document it as the slow setting.

### 4. Palette

`internal/quant`:

- `Fixed(palette []colour.OKLab)`: no quantisation; cells map to the nearest entry.
- `Wu(cells []Cell, maxK int, tol float64) []colour.OKLab`: variance-minimising box splitting in the manner of Wu (1991) over a 32×32×32 histogram of OKLab (L in [0,1], a and b in [−0.4, 0.4]) with cumulative moment arrays (weight, ΣL, Σa, Σb, Σ(L²+a²+b²)). Repeatedly split the box with the largest sum of squared error along the axis and plane that minimise the resulting total error. Stop when the box count reaches `maxK`, or when the largest box's RMS radius `sqrt(SSE/weight)` is below `tol`, or no box can be split. `maxK` is `MaxColors`, or 256 when `MaxColors` is 0 (so 0 means "as many as `ColorTolerance` needs", not unlimited). Box centroids seed the palette.
- `KMeans(cells, palette, iters)`: weighted Lloyd iterations on cells (assign each cell to its nearest entry, recompute weighted centroids), at most 10 iterations, stopping early when no cell changes entry. Celebi's comparison puts Wu-initialised k-means first; k-means alone is sensitive to initialisation and Wu alone leaves centroids off-centre.
- Both deterministic: fixed iteration order over cells sorted by cell index; no maps; no randomness.

### 5. Lookup and soft coverage

A lookup table indexed by cell (2^(3·ColorPrecision) entries) stores for each cell its nearest palette index, second-nearest index and `Blend`: the fraction of the cell's colour attributable to the nearest entry when the colour is projected onto the OKLab segment between the two, as `uint8` in 128..255 (255 = entirely the nearest colour). A second pass over the pixels fills `Labels`, `Second` and `Blend` (3 bytes per pixel). `Blend` is the soft coverage mask the roadmap asks for; Phase 2 stores it and exposes it but only uses `Labels`.

### 6. Region cleanup

Connected components of equal label under 4-connectivity, labelled in one scan with union–find, processed in ascending area then scan order:

- **Despeckle.** A component with fewer than `FilterSpeckle` pixels is relabelled to the neighbouring label sharing the longest 4-adjacent border with it (ties: lower label). Repeat the pass until nothing changes, at most four passes.
- **Anti-aliasing slivers.** A component whose mean thickness `area / (perimeter / 2)` is below 1.5 px (perimeter counted in 4-adjacent pairs with other labels or the image edge) is absorbed into its longest-border neighbour **only if** its palette colour lies within `ColorTolerance` of the OKLab segment between the colours of its two longest-border neighbours. The condition keeps genuine thin lines (a yellow line between red and blue is far from the red–blue segment) and removes the intermediate bands that anti-aliased edges quantise into (report: "thin intermediate bands that become sliver shapes"). A component with a single neighbour label is left alone.
- **Compaction.** Palette entries with no pixels are dropped and labels renumbered in order of first appearance in scan order. `Second`/`Blend` are recomputed against the final palette with one more table pass.

### 7. Per-colour contours

For each palette index, build a `bitmap.Bitmap` of pixels with that label and run `contour.Trace` with `Options.TurnPolicy`, then `contour.BuildShapes(…, 0)` (despeckling is already done at the label level). Colours run in a worker pool of `runtime.GOMAXPROCS(0)` goroutines; each worker owns one bitmap buffer; results are stored by colour index so order never depends on scheduling.

### 8. Stacking

`Options.Hierarchical`: `stacked` (default for `poster`, `photo`) or `cutout` (default for `bw`; for colour presets it emits each shape with its holes and no ordering guarantees beyond colour index then scan order, so abutting shapes may show hairline seams, as the report warns).

Stacked order is a nesting tree over every shape of every colour: sort shapes by outer area ascending; each shape's parent is the smallest shape of larger area whose outer polygon contains the shape's first interior pixel (bounding-box prefilter, then the Phase 1 `containsPixel`). Depth is the parent's depth plus one; shapes with no parent are depth 0. Output order is depth, then outer first point (y, x). Every shape is emitted as its **outer contour only**: the regions inside its holes are exactly its descendants, which are painted on top later, so dropping the holes loses nothing and removes the seams cutout output would have along hole edges.

A depth-0 shape whose outer contour is the full canvas rectangle and whose colour is `#ffffff` is omitted: SVG's default background is transparent, which renders white on a white page and composites to white in the benchmark, and a logo traced from a transparent PNG then stays transparent.

### 9. Fitting and paths

Each emitted contour goes through the existing `segments()` (pixel, polygon or spline) in a parallel pool, stored by shape index. `svg.Path.Fill` is the palette colour as lowercase `#rrggbb`. Consecutive paths in output order with the same fill are merged into one `<path>` with several subpaths (all outers, same orientation, nonzero fill keeps them disjoint). The SVG writer is unchanged.

### 10. Public API (root package)

```go
type Hierarchical string                         // "stacked" | "cutout"

type Options struct {
    // existing fields unchanged; Preset, ColorPrecision, MaxColors now used
    ColorTolerance float64      // OKLab distance 0..1 at which colours merge; see presets
    Palette        []color.NRGBA // fixed palette, empty = quantise
    Hierarchical   Hierarchical
}

func PresetOptions(p Preset) Options             // defaults per preset
func DefaultOptions() Options                    // = PresetOptions(PresetBW), unchanged values

type Segmentation struct {
    Width, Height int
    Palette       []color.NRGBA  // final palette after cleanup, bw: {white, black}
    Labels        []uint8        // palette index per pixel
    Second        []uint8        // second-nearest index per pixel
    Blend         []uint8        // 128..255, share of Labels' colour in the pixel
}

func Segment(img image.Image, o Options) (*Segmentation, error)
func (s *Segmentation) Finish(o Options) (*Result, error)   // contours, stacking, fitting, paths
func Trace(img image.Image, o Options) (*Result, error)      // Segment then Finish
```

Preset defaults:

| Field | bw | poster | photo |
|---|---|---|---|
| Hierarchical | cutout | stacked | stacked |
| MaxColors | 2 | 0 (tolerance-bounded, cap 256) | 0 |
| ColorTolerance | — | 0.08 | 0.04 |
| ColorPrecision | 6 | 6 | 6 |
| FilterSpeckle | 4 | 4 | 16 |
| FitTolerance | 1.0 | 1.0 | 1.5 |
| CornerThreshold | 60 | 60 | 60 |
| Mode | spline | spline | spline |

Tolerance values are starting points; the benchmark settles them, and the chosen values are recorded in the baseline header.

`Validate` adds: `ColorTolerance` in 0..1, `Palette` length ≤ 256 with no duplicates, `Hierarchical` in its two values. `Segment` under `bw` thresholds exactly as Phase 1 and produces a two-entry palette; `Finish` under `bw`+`cutout` reproduces Phase 1 output byte for byte.

### 11. CLI

`--preset` is read first and seeds every default from `PresetOptions`; other flags override only when given on the command line (`flag.FlagSet.Visit`). New flags: `--color-tolerance <0..1>`, `--palette "#rrggbb,#rrggbb,…"`, `--hierarchical stacked|cutout`. `--max-colors 0` keeps the documented meaning "bounded by tolerance". Help text names the presets' defaults.

### 12. Corpus and benchmark

New synthetics in `internal/corpus`: `poster-discs` (five overlapping anti-aliased discs in five flat colours on white), `aa-abut` (red and blue meeting along an anti-aliased diagonal), `logo-holes` (two letter-like shapes with holes in two colours on a third background colour). Regenerated into `testdata/corpus/synthetic`.

`internal/bench` adds tools `rekha-poster` and `rekha-photo` (same binary, `--preset` differs) alongside `rekha` (bw). The baseline is re-recorded as `bench/results/<date>-phase2.md` with the goals above checked in its header.

## Testing

- `colour`: OKLab against Ottosson's reference values (white → L 1, a 0, b 0; sRGB red → L 0.628, a 0.225, b 0.126 within 0.001); round trip sRGB → OKLab → sRGB exact on every grey level and on 4096 sampled colours within 1.
- `quant`: three well-separated weighted clusters recovered exactly by Wu; k-means moves a deliberately offset seed onto the cluster mean; tolerance stop yields 2 colours for a two-colour image with anti-aliased edges; `MaxColors` cap honoured; fixed palette maps every cell to its nearest entry; determinism across two runs.
- Region cleanup: a 2-px speck absorbed into the longest-border neighbour; a 1-px purple band between red and blue absorbed; a 1-px yellow line between red and blue kept; palette compaction renumbers in scan order.
- Stacking: `logo-holes` nesting depths (background 0, letters 1, hole contents 2); property test that painting the pixel-mode stacked output in order reproduces `Labels` exactly, for every synthetic and for fuzzed images; same property for cutout.
- `Finish` under bw equals Phase 1 goldens byte for byte (existing `TestTraceGolden` plus colour goldens for poster/photo).
- Determinism: `GOMAXPROCS=1` and `8` produce identical bytes on every synthetic.
- Fuzz: `FuzzSegment` over random ≤ 32×32 colour images and presets poster/photo: no panic, label map covers the image, palette size ≤ cap, stacked pixel-mode raster equals labels, spline output passes the Phase 1 sanity oracle.
- Exit test: `make bench`; goals from the header of this document checked by hand and recorded in the baseline.

## Left to the builder

- Wu moment-array layout and the exact split search; whether boxes are kept in a heap or rescanned (≤ 256 boxes, either is fine).
- k-means convergence epsilon.
- Union–find implementation and component statistics layout.
- Worker-pool shape and buffer reuse.
- Whether `Second`/`Blend` are computed in the first table pass or only in the final one.

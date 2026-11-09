# Phase 2 Colour Tracing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Colour images trace to stacked SVG layers through the `poster` and `photo` presets, with a public `Segment`/`Finish` split, while `bw` output stays byte-identical to Phase 1.

**Architecture:** New packages `internal/colour` (OKLab), `internal/quant` (cells, Wu splitting, k-means, lookup table) and `internal/region` (label-map cleanup) feed a `Segmentation` (palette + per-pixel labels and soft blend). `Finish` traces each colour's bitmap with the Phase 1 contour tracer in parallel, orders shapes by a nesting tree for stacked output (outer contours only) or keeps holes for cutout, fits with the Phase 1 `segments()` and writes paths with palette fills.

**Tech Stack:** Go 1.26, standard library, `golang.org/x/image` only.

**Spec:** `docs/superpowers/specs/2026-11-09-phase2-colour-design.md`

## Global Constraints

- Module `github.com/vexyl-ai/rekha`, Go 1.26, `CGO_ENABLED=0`, `GOOS=js GOARCH=wasm go build ./...` passes.
- Only `golang.org/x/image` outside the standard library.
- Apache-2.0, DCO: `git commit -s`, trailer `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>`.
- `make lint test wasm-check` exits 0; `geom.Point`/`geom.Vec2` literals outside `geom` are keyed.
- No Potrace/gotrace-derived code.
- Output independent of `GOMAXPROCS` and map iteration order; no randomness.
- `bw` preset output byte-identical to Phase 1 goldens.
- Memory per layer must not scale with palette size × pixels; one label map, per-worker bitmap buffers.

## Review Focus

1. A fully transparent PNG under `poster`: one white layer, full-canvas, omitted → empty but valid SVG, no panic (Task 6 test `TestFinishAllWhiteIsEmpty`).
2. An image with more than 256 distinct colours under `photo` with `MaxColors 0`: palette capped at 256, labels fit in `uint8` (Task 3 `TestWuCapsAt256`, Task 5 `TestSegmentManyColours`).
3. A 1×1 or 1×N image under `poster`: segmentation and stacking survive degenerate components (Task 7 fuzz seeds).
4. A colour shape touching the canvas edge whose colour equals the background: two depth-0 shapes of the same colour, merged into one `<path>`, never treated as nested (Task 6 `TestStackedSameColourBorderShapes`).
5. A fixed palette containing colours absent from the image: unused entries dropped, labels compacted, fills still correct (Task 5 `TestSegmentFixedPaletteDropsUnused`).

---

### Task 1: OKLab colour space

**Files:**
- Create: `internal/colour/oklab.go`, `internal/colour/oklab_test.go`

**Interfaces:**
- Produces: `type OKLab struct{ L, A, B float64 }`; `func FromSRGB8(r, g, b uint8) OKLab`; `func (c OKLab) ToSRGB8() (r, g, b uint8)` (clamps out-of-gamut); `func Dist(a, b OKLab) float64` (Euclidean); `func (c OKLab) Hex() string` (lowercase `#rrggbb` via ToSRGB8).

- [ ] **Step 1: Write failing tests**

```go
func TestOKLabReferenceValues(t *testing.T)   // white → L 1, A 0, B 0 (±1e-3); red(255,0,0) → 0.628, 0.225, 0.126 (±1e-3); black → 0,0,0
func TestOKLabRoundTripGreys(t *testing.T)    // every grey g: FromSRGB8(g,g,g).ToSRGB8() == (g,g,g)
func TestOKLabRoundTripSampled(t *testing.T)  // 16×16×16 grid of sRGB: round trip within 1 per channel
func TestOKLabDistWhiteBlack(t *testing.T)    // Dist(white, black) == 1 ± 1e-6; Hex of (255,0,0) == "#ff0000"
```

- [ ] **Step 2: Run, expect failure** — Run: `go test ./internal/colour/` Expected: build failure, undefined symbols.
- [ ] **Step 3: Implement** sRGB decode (piecewise gamma), Ottosson's M1/M2 matrices and cube roots, inverse with cubes, encode, clamp to [0,255].
- [ ] **Step 4: Run** — Expected: PASS.
- [ ] **Step 5: Commit** — `git add internal/colour && git commit -s -m "feat(colour): OKLab conversion and distance"`

---

### Task 2: Flat sRGB buffer

**Files:**
- Modify: `internal/bitmap/gray.go`, `internal/bitmap/gray_test.go`

**Interfaces:**
- Produces: `func Flatten(img image.Image) (rgb []uint8, w, h int)` — 3 bytes per pixel, composited onto white, same fast paths as `Gray` (opaque NRGBA/RGBA, Gray, Paletted; else RGBA draw). `Gray` may be re-expressed over `Flatten` only if goldens stay identical.

- [ ] **Step 1: Write failing tests**

```go
func TestFlattenMatchesReference(t *testing.T)  // reuse referenceGray's compositing: for the same six images as TestGrayMatchesReference, each RGB byte equals the NRGBA composite (exact for opaque, ±1 for alpha/paletted)
func TestFlattenTransparentIsWhite(t *testing.T) // NRGBA{0,0,0,0} → 255,255,255
```

- [ ] **Step 2: Run, expect failure.** - [ ] **Step 3: Implement.** - [ ] **Step 4: Run, PASS; `go test ./...` still green.**
- [ ] **Step 5: Commit** — `git commit -s -m "feat(bitmap): Flatten to composited sRGB bytes"`

---

### Task 3: Quantiser

**Files:**
- Create: `internal/quant/cells.go`, `internal/quant/wu.go`, `internal/quant/kmeans.go`, `internal/quant/lookup.go`, tests alongside.

**Interfaces:**
- Consumes: `colour.OKLab`, `colour.FromSRGB8`, `colour.Dist`.
- Produces:
  - `type Cell struct{ Index int; Lab colour.OKLab; Weight int }`
  - `func Cells(rgb []uint8, bits int) []Cell` — occupied cells at `bits` per channel, ascending `Index` (`Index = r>>s<<2b | g>>s<<b | b>>s`, `s = 8-bits`), `Lab` of the cell centre.
  - `func Wu(cells []Cell, maxK int, tol float64) []colour.OKLab` — spec §4; `maxK ≤ 256`.
  - `func KMeans(cells []Cell, palette []colour.OKLab, maxIter int) []colour.OKLab` — weighted Lloyd, stops when assignments stop changing; an entry that loses every cell keeps its position.
  - `type Lookup struct{ Bits int; Nearest, Second, Blend []uint8 }`; `func NewLookup(palette []colour.OKLab, bits int) *Lookup` — one entry per cell index, `Blend` 128..255 from projection onto the nearest–second segment (255 when palette has one entry); `func (l *Lookup) Map(rgb []uint8, labels, second, blend []uint8)` fills per-pixel arrays.

- [ ] **Step 1: Write failing tests**

```go
func TestCellsCountsAndOrder(t *testing.T)        // 4 pixels, 2 colours, bits 6 → 2 cells ascending Index, weights 3 and 1; Lab of cell centre
func TestWuRecoversThreeClusters(t *testing.T)    // cells at red/green/blue (weights 100,50,25), maxK 8, tol 0.01 → 3 entries each within 0.02 of a cluster
func TestWuToleranceStopsAtTwo(t *testing.T)      // red and blue cells plus 2% weight of their midpoint, tol 0.08 → 2 entries
func TestWuCapsAt256(t *testing.T)                // 4096 distinct greys, maxK 0 (→256), tol 0 → len 256
func TestKMeansMovesSeed(t *testing.T)            // cluster at L 0.5, seed at 0.6 → centroid within 1e-9 of 0.5 after ≤10 iters
func TestLookupNearestSecondBlend(t *testing.T)   // palette {black, white}; cell of grey 64 → Nearest black, Second white, Blend ≈ 255-64 (±2); grey 255 → Blend 255
func TestWuDeterministic(t *testing.T)            // two runs on noisy-disc cells identical
```

- [ ] **Step 2: Run, expect failure.** - [ ] **Step 3: Implement** (Wu: 33³ cumulative moments over OKLab bins, split box with largest SSE along the best axis/plane; boxes rescanned, ≤256). - [ ] **Step 4: Run, PASS.**
- [ ] **Step 5: Commit** — `git commit -s -m "feat(quant): cells, Wu splitting, k-means, lookup table"`

---

### Task 4: Region cleanup

**Files:**
- Create: `internal/region/components.go`, `internal/region/cleanup.go`, tests.

**Interfaces:**
- Consumes: `colour.OKLab`, `colour.Dist`.
- Produces:
  - `type Components struct{ Label []int32; Area []int; Perimeter []int; Border map-free: Adjacent func(comp int) []Neighbour }` — the builder picks the layout; required operations: component id per pixel, area, 4-adjacent perimeter (other labels or image edge), neighbour labels with shared-border length in deterministic order (by length desc, then label asc).
  - `func Cleanup(labels []uint8, w, h int, palette []colour.OKLab, minArea int, tol float64) []colour.OKLab` — in place on `labels`: despeckle (< minArea, ≤4 passes), sliver absorption (thickness < 1.5 px and colour within `tol` of the segment between the two longest-border neighbours), compaction by first appearance in scan order; returns the compacted palette.

- [ ] **Step 1: Write failing tests**

```go
func TestCleanupAbsorbsSpeck(t *testing.T)        // 8×8 red with a 2-px blue speck, minArea 4 → all red, palette {red}
func TestCleanupAbsorbsAABand(t *testing.T)       // red | 1-px purple column | blue, purple = OKLab midpoint, tol 0.08 → purple gone, column joins longer-border side (ties → lower label)
func TestCleanupKeepsThinLine(t *testing.T)       // red | 1-px yellow column | blue → yellow kept
func TestCleanupSingleNeighbourSliverKept(t *testing.T) // 1-px band along image edge with one neighbour → kept
func TestCleanupCompactsInScanOrder(t *testing.T) // labels {3,1} used → renumbered 0,1 by first appearance; palette reordered
```

- [ ] **Step 2–5** as above; commit `feat(region): component despeckle, sliver absorption, palette compaction`.

---

### Task 5: Options, presets and Segment

**Files:**
- Modify: `options.go`, `options_test.go`
- Create: `segment.go`, `segment_test.go`

**Interfaces:**
- Consumes: Tasks 1–4; `bitmap.Gray`, `bitmap.Otsu`, `bitmap.FromGray`.
- Produces (spec §10): `type Hierarchical string` with `HierarchicalStacked = "stacked"`, `HierarchicalCutout = "cutout"`; `Options` gains `ColorTolerance float64`, `Palette []color.NRGBA`, `Hierarchical Hierarchical`; `func PresetOptions(p Preset) Options` with the spec table; `DefaultOptions()` = `PresetOptions(PresetBW)` (values unchanged from Phase 1 plus `Hierarchical: cutout`, `MaxColors: 2`); `Validate` rules for the new fields; `type Segmentation struct{ Width, Height int; Palette []color.NRGBA; Labels, Second, Blend []uint8 }`; `func Segment(img image.Image, o Options) (*Segmentation, error)`.
- bw: grey + threshold → palette `{white, black}`, `Labels` 1 for foreground, `Second` the other, `Blend` 255. Colour: Flatten → Cells → palette (Fixed/Wu+KMeans) → Lookup.Map → Cleanup → Lookup rebuilt on the final palette → Second/Blend.

- [ ] **Step 1: Write failing tests**

```go
func TestPresetOptionsTable(t *testing.T)            // every value of the spec table; DefaultOptions() == PresetOptions(PresetBW)
func TestValidateNewFields(t *testing.T)             // ColorTolerance 1.5 → error; 257-entry Palette → error; duplicate Palette → error; Hierarchical "x" → error
func TestSegmentBWMatchesThreshold(t *testing.T)     // star-5: Labels == FromGray bitmap, Palette {white, black}
func TestSegmentTwoColourAbut(t *testing.T)          // poster: 2 palette entries within 0.02 of red and blue; labels split at x=128 exactly
func TestSegmentThreeLayerStack(t *testing.T)        // poster: 3 entries (blue, red, white); anti-aliased disc edge yields no fourth entry
func TestSegmentManyColours(t *testing.T)            // 64×64 image with 4096 colours, photo → len(Palette) ≤ 256
func TestSegmentFixedPaletteDropsUnused(t *testing.T)// Palette {red, blue, green} on two-colour-abut → Palette {red, blue} in scan order
func TestSegmentDeterministic(t *testing.T)          // two runs equal
```

- [ ] **Step 2–5**; commit `feat: presets, Segmentation and Segment for bw and colour`.

---

### Task 6: Finish — layers, stacking, fitting

**Files:**
- Create: `internal/contour/nest.go`, `internal/contour/nest_test.go`, `finish.go`, `finish_test.go`
- Modify: `trace.go` (Trace = Segment + Finish; `segments()` kept)

**Interfaces:**
- Consumes: `contour.Trace`, `contour.BuildShapes`, `segments()`, `Segmentation`.
- Produces: `func Nest(shapes []Shape) []int` in `contour` — parent index per shape (−1 for roots) by smallest enclosing outer (bbox prefilter + `containsPixel` on the shape's first interior pixel); `func (s *Segmentation) Finish(o Options) (*Result, error)`; `Trace` calls both; `Result.Paths[i].Fill` is the palette hex.
- Parallelism: worker pool of `runtime.GOMAXPROCS(0)` over colours (one bitmap buffer per worker) and over shapes for fitting; results indexed.

- [ ] **Step 1: Write failing tests**

```go
func TestNestRingDepths(t *testing.T)                 // contour: disc with inner disc → parent of inner is outer; two side-by-side discs → both roots
func TestFinishBWEqualsPhase1(t *testing.T)           // every synthetic, bw: Finish output bytes == Trace Phase 1 goldens (existing TestTraceGolden passes unchanged)
func TestStackedPixelModeReproducesLabels(t *testing.T) // every synthetic, poster, ModePixel: painting paths in order (rasttest.Fill per path with its fill) == Labels
func TestCutoutPixelModeReproducesLabels(t *testing.T)  // same, Hierarchical cutout
func TestStackedOuterOnly(t *testing.T)               // three-layer-stack: no path has more than one subpath per shape; order blue, red, white; 3 paths
func TestStackedSameColourBorderShapes(t *testing.T)  // two red squares touching opposite canvas edges on white: one red <path> with two subpaths, depth 0
func TestFinishAllWhiteIsEmpty(t *testing.T)          // fully transparent 16×16, poster → 0 paths, valid SVG
func TestFinishDeterministicAcrossProcs(t *testing.T) // GOMAXPROCS 1 vs 8, poster-discs: identical bytes
```

`rasttest` gains `Paint(w, h int, paths []svg.Path, palette map[string]uint8) []uint8` (fills each path's pixels with its colour index in order) for the label-reproduction tests.

- [ ] **Step 2–5**; commit `feat: Finish with per-colour contours, nesting tree, stacked and cutout output`.

---

### Task 7: Colour synthetics, goldens, fuzz

**Files:**
- Modify: `internal/corpus/synthetic.go`, `testdata/corpus/synthetic/*.png` (regenerate), `trace_test.go`, `testdata/golden/*`

**Interfaces:**
- Produces: synthetics `poster-discs`, `aa-abut`, `logo-holes` (spec §12); goldens `<name>-poster-<mode>.svg` and `gradient-h-photo-spline.svg`; `FuzzSegment`.

- [ ] **Step 1: Write failing tests**

```go
func TestCorpusHasColourSynthetics(t *testing.T)   // All() contains the three names; logo-holes has exactly 3 colours after poster Segment
func TestTraceGoldenColour(t *testing.T)           // poster presets for all synthetics in all modes, photo for gradient-h; -update regenerates; svgClose compare
func TestLogoHolesDepths(t *testing.T)             // background 0, letters 1, hole contents 2 (via Nest on Segment+contours)
func FuzzSegment(f *testing.F)                     // random ≤32×32 RGB, poster and photo, cutout and stacked: no panic, palette ≤ 256, labels < len(palette), pixel-mode paint == labels, spline passes checkTraceSane adapted to colour (segment budget per subpath, raster agreement per layer)
```

- [ ] **Step 2–5**; regenerate PNGs with `go run ./internal/corpus/gen -out testdata/corpus/synthetic`; commit `test: colour synthetics, goldens and segmentation fuzz`.

---

### Task 8: CLI

**Files:**
- Modify: `cmd/rekha/main.go`, `cmd/rekha/main_test.go`

**Interfaces:**
- Consumes: `PresetOptions`, new Options fields.
- Behaviour: pre-scan args for `--preset`/`-preset` value, seed `o := PresetOptions(preset)`, define flags with those defaults, parse, then `fs.Visit` to apply only given flags; new flags `--color-tolerance`, `--palette` (comma-separated `#rrggbb`, parse error → exit 1), `--hierarchical`.

- [ ] **Step 1: Write failing tests**

```go
func TestRunPosterPresetDefaults(t *testing.T)     // --preset poster on three-layer-stack → exit 0, 3 <path> elements, fills #1e3cdc, #dc1e1e, #ffffff (quantised values within the goldens)
func TestRunPresetSeedsDefaults(t *testing.T)      // --preset photo without --filter-speckle → FilterSpeckle 16 used (observable: a 9-px speck disappears)
func TestRunPaletteFlag(t *testing.T)              // --palette "#ff0000,#0000ff" → fills exactly those; "--palette zz" → exit 1
func TestRunHierarchicalCutout(t *testing.T)       // --hierarchical cutout on logo-holes → a path with 2 subpaths exists
```

- [ ] **Step 2–5**; commit `feat(cli): presets seed defaults; colour-tolerance, palette, hierarchical flags`.

---

### Task 9: Benchmark and docs

**Files:**
- Modify: `internal/bench/tool.go`, `internal/bench/run_test.go`, `cmd/rekha-bench/main.go`, `docs/benchmark.md`, `README.md`
- Create: `bench/results/2026-11-09-phase2.md`

**Interfaces:**
- Produces: `RekhaTools(bin string) []Tool` returning `rekha`, `rekha-poster`, `rekha-photo` (Args differ in `--preset`); `ByName` knows all three; `rekha-bench` substitutes `Bin` for every tool whose name starts with `rekha`.

- [ ] **Step 1: Write failing tests** — `TestRekhaToolsPresets` (three tools, preset args), `TestByNameKnowsRekhaPoster`.
- [ ] **Step 2–4**; run `make bench`; write the baseline with the goal table (spec Goals) and preset values used; update README status and usage, `docs/benchmark.md` tool list.
- [ ] **Step 5: Commit** — `bench: Phase 2 colour baseline; docs`.

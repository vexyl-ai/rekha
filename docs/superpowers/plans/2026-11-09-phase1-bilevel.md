# Phase 1 Bilevel Tracing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `rekha in.png` emit a valid SVG of black filled paths (with holes) in `pixel`, `polygon` and `spline` modes, within 2 dB PSNR and 1.5× node count of Potrace on the synthetic bilevel corpus.

**Architecture:** Five internal packages in pipeline order (`bitmap` → `contour` → `geom` → `fit` → `svg`), orchestrated by `Trace` in the root package. Contours are walked along pixel edges with integer coordinates; simplification and corner detection run on the integer polygon; Schneider fitting runs on the dense boundary points between corners. The CLI and benchmark gain the wiring.

**Tech Stack:** Go 1.26 standard library (`image`, `image/png|jpeg|gif`, `math`, `strconv`). No new dependencies.

**Spec:** `docs/superpowers/specs/2026-11-09-phase1-bilevel-design.md`

## Global Constraints

- Module `github.com/vexyl-ai/rekha`, `go 1.26`, `CGO_ENABLED=0`; everything must build with `GOOS=js GOARCH=wasm` (no `//go:build !js` in any package this plan creates).
- No dependency outside the standard library and `golang.org/x/image`.
- Clean-room rule: no reading, porting or paraphrasing Potrace/gotrace source. Turn-policy names come from the `potrace(1)` man page's user-visible description only.
- Orientation convention: edges are directed with foreground on the **left** in image coordinates (x right, y down); outer contours have positive shoelace area, holes negative.
- Deterministic output: byte-identical SVG for identical input and options; shapes sorted by outer first vertex (y, then x).
- Defaults from spec: `Threshold 0.5`, `TurnPolicy minority`, `SimplifyTolerance 0.75`, `CornerThreshold 60`, `FitTolerance 1.0`, `FilterSpeckle 4`, `PathPrecision 2`.
- Exit codes 0/1/2; `-` for stdin/stdout; `git commit -s`.

## Review Focus

1. An image whose foreground touches the border (e.g. all-black 4×4): the contour walk must close along the image edge and emit one shape covering the canvas. Test in Task 3.
2. A shape 1 px wide (`thin-lines`): collinear merge then staircase pass must not collapse it to zero area; spline mode must still emit a closed path. Test in Task 7 (golden) and Task 4 (staircase on 1-px rectangle keeps 4 vertices).
3. A contour with fewer than 4 dense points between corners (tiny notch): fitter must emit `L`, not panic on an underdetermined system. Test in Task 5.
4. JPEG input with grey noise around edges: threshold produces specks; `FilterSpeckle` must remove both specks and pinholes. Test in Task 3 (hole below area threshold is dropped).
5. `PathPrecision 0`: coordinates print as integers with no trailing dot, and `-0` never appears. Test in Task 6.

---

### Task 1: Geometry primitives

**Files:**
- Create: `internal/geom/geom.go`, `internal/geom/geom_test.go`, `internal/geom/rdp.go`, `internal/geom/rdp_test.go`

**Interfaces:**
- Produces:
  ```go
  package geom
  type Point struct{ X, Y int }       // lattice point
  type Vec2 struct{ X, Y float64 }
  func (p Point) Vec() Vec2
  func (a Vec2) Add(b Vec2) Vec2; Sub(b Vec2) Vec2; Scale(k float64) Vec2; Dot(b Vec2) float64; Len() float64; Norm() Vec2 // Norm of zero vector returns zero vector
  func Area2(poly []Point) int        // twice the signed shoelace area; positive = clockwise on screen (y down)
  func Contains(poly []Point, p Point) bool // even-odd point-in-polygon; points exactly on an edge count as inside
  func PerpDist(p, a, b Vec2) float64 // distance from p to infinite line ab; a==b gives |p-a|
  func RDP(pts []Vec2, tol float64) []Vec2        // open polyline; keeps first and last
  func RDPClosed(pts []Vec2, tol float64) []Vec2  // closed; splits at index 0 and the vertex farthest from pts[0], simplifies both halves, rejoins without duplicating the split points
  ```

- [ ] **Step 1: Write failing tests**

`geom_test.go`:
```go
func TestArea2Square(t *testing.T)       // (0,0)(1,0)(1,1)(0,1) → +2 ; reversed → -2
func TestContains(t *testing.T)          // unit square: (0,0) inside (on edge), (2,2) outside; 3x3 ring polygon: centre inside
func TestNormZero(t *testing.T)          // Vec2{}.Norm() == Vec2{}
func TestPerpDistDegenerate(t *testing.T)// PerpDist((3,4),(0,0),(0,0)) == 5
```
`rdp_test.go`:
```go
func TestRDPKeepsCorners(t *testing.T)   // L-shape (0,0)(5,0)(5,5) with collinear fillers every 1px, tol 0.5 → exactly 3 points
func TestRDPRemovesStaircase(t *testing.T) // 1px stairs from (0,0) to (8,8), tol 0.75 → 2 points
func TestRDPClosedSquare(t *testing.T)   // square perimeter sampled every 1px (side 10), tol 0.5 → 4 points, each a corner
```

- [ ] **Step 2: Run, expect compile failure**

Run: `go test ./internal/geom/`
Expected: FAIL, undefined symbols.

- [ ] **Step 3: Implement per Interfaces**

RDP: recursive, find max `PerpDist`, split when > tol.

- [ ] **Step 4: Run, expect pass**

Run: `go test ./internal/geom/ -v`

- [ ] **Step 5: Commit**

```bash
git add internal/geom && git commit -s -m "feat(geom): add lattice/vector primitives and RDP simplification"
```

---

### Task 2: Bitmap and thresholding

**Files:**
- Create: `internal/bitmap/bitmap.go`, `internal/bitmap/bitmap_test.go`, `internal/bitmap/gray.go`, `internal/bitmap/gray_test.go`

**Interfaces:**
- Produces:
  ```go
  package bitmap
  type Bitmap struct { W, H int; bits []uint64 }   // row-major, 64 px per word
  func New(w, h int) *Bitmap
  func (b *Bitmap) Get(x, y int) bool               // false outside [0,W)×[0,H)
  func (b *Bitmap) Set(x, y int, v bool)            // no-op outside
  func (b *Bitmap) Count() int                      // foreground pixels
  func Gray(img image.Image) ([]uint8, int, int)   // composite on white, Y = round(0.2126R+0.7152G+0.0722B) on 8-bit; returns gray, w, h
  func FromGray(gray []uint8, w, h int, threshold uint8) *Bitmap // foreground where gray < threshold
  func Otsu(gray []uint8) uint8                     // Otsu threshold on 256-bin histogram; all-equal input returns that value
  func Parse(rows []string) *Bitmap                 // test helper usable from other packages: '#' = fg, '.' = bg
  ```

- [ ] **Step 1: Write failing tests**

```go
func TestGetOutsideIsFalse(t *testing.T)
func TestSetGetRoundTrip(t *testing.T)           // 70x3 bitmap crosses a word boundary; set (65,2), Count()==1
func TestParse(t *testing.T)                     // Parse([]string{"#.", ".#"}) → Get(0,0)&&Get(1,1), !Get(1,0)
func TestGrayCompositesAlphaOnWhite(t *testing.T) // transparent pixel → 255; opaque (0,0,0) → 0; (255,0,0) → 54
func TestFromGrayThreshold(t *testing.T)         // gray [0,127,128,255], threshold 128 → fg,fg,bg,bg
func TestOtsuBimodal(t *testing.T)               // 100 pixels at 20 and 100 at 200 → threshold in [20,200], splits them
func TestOtsuFlat(t *testing.T)                  // all 77 → 77
```

- [ ] **Step 2: Run, expect compile failure**

- [ ] **Step 3: Implement**

Threshold value passed to `FromGray` by callers is `uint8(math.Round(o.Threshold*255))`; that conversion lives in the root package, not here.

- [ ] **Step 4: Run, expect pass**

Run: `go test ./internal/bitmap/ -v`

- [ ] **Step 5: Commit**

```bash
git add internal/bitmap && git commit -s -m "feat(bitmap): add packed bitmap, grey conversion, fixed and Otsu thresholds"
```

---

### Task 3: Contour tracing, hierarchy, despeckle

**Files:**
- Create: `internal/contour/trace.go`, `internal/contour/trace_test.go`, `internal/contour/shapes.go`, `internal/contour/shapes_test.go`

**Interfaces:**
- Consumes: `bitmap.Bitmap`, `bitmap.Parse`; `geom.Point`, `geom.Area2`, `geom.Contains`.
- Produces:
  ```go
  package contour
  type TurnPolicy string
  const ( Minority TurnPolicy = "minority"; Majority = "majority"; Black = "black"; White = "white" )
  type Contour struct { Pts []geom.Point; Area2 int }   // closed; Pts[0] is the lowest (y,x) lattice point of the loop; no repeated last point
  func Trace(b *bitmap.Bitmap, policy TurnPolicy) []Contour // all closed crack loops, sorted by Pts[0] (y, then x)
  type Shape struct { Outer Contour; Holes []Contour }
  func BuildShapes(cs []Contour, minArea int) []Shape   // assigns holes (Area2<0) to smallest containing outer; drops outers with |Area2|/2 < minArea and their holes; drops holes with |Area2|/2 < minArea; sorted by Outer.Pts[0]
  ```
  Saddle rule: at a vertex with two outgoing edges, `Black` turns so the walk keeps foreground connected (the two diagonal fg pixels become one shape), `White` keeps background connected (two shapes), `Minority` behaves as `Black` when fg pixels < bg pixels in the image else as `White`, `Majority` the reverse.

- [ ] **Step 1: Write failing tests**

`trace_test.go`:
```go
func TestSinglePixel(t *testing.T)          // Parse{"#"} → 1 contour, Area2 == 2, 4 points, Pts[0]==(0,0)
func TestAllBlackTouchesBorder(t *testing.T)// 4x4 all '#' → 1 contour, Area2 == 32  (Review Focus 1)
func TestRingHasHole(t *testing.T)          // 5x5 ring (1-px wall) → 2 contours: Area2 +50 and -18
func TestCheckerboardPolicies(t *testing.T) // Parse{"#.", ".#"}: Black → 1 contour; White → 2; Minority → 2 (2 fg vs 2 bg is a tie; tie is not fg<bg, so behaves as White); Majority → 1
func TestEveryContourClosed(t *testing.T)   // for random 16x16 bitmaps (seeded), every contour: consecutive points unit distance apart, last→first unit distance
```
`shapes_test.go`:
```go
func TestHoleAssignedToSmallestOuter(t *testing.T) // big square containing small ring: hole attaches to the ring, not the square
func TestDespeckleDropsSmallOuterAndHole(t *testing.T) // 10x10 block with a 1-px pinhole plus a lone 1-px speck, minArea 4 → 1 shape, 0 holes (Review Focus 4)
func TestShapesSorted(t *testing.T)
```

- [ ] **Step 2: Run, expect compile failure**

- [ ] **Step 3: Implement**

Edge representation: for each lattice vertex store up to 2 outgoing edge directions derived on the fly from the 2×2 pixel neighbourhood of the vertex (`Get` returns false outside, so no padding). Keep a visited set keyed by (vertex, direction). Start loops from the lowest unvisited edge in raster order so `Pts[0]` is the lowest (y,x) vertex.

- [ ] **Step 4: Run, expect pass**

Run: `go test ./internal/contour/ -v`

- [ ] **Step 5: Commit**

```bash
git add internal/contour && git commit -s -m "feat(contour): add crack-following tracer with turn policies, hole hierarchy and despeckle"
```

---

### Task 4: Polygon simplification and corner detection

**Files:**
- Create: `internal/geom/simplify.go`, `internal/geom/simplify_test.go`, `internal/geom/corners.go`, `internal/geom/corners_test.go`

**Interfaces:**
- Produces:
  ```go
  package geom
  func MergeCollinear(poly []Point) []Point              // closed; drops vertices whose in/out directions are equal; result has ≥ 4 points for any valid crack loop
  func RemoveStaircases(poly []Point) []Point            // closed; a maximal run of ≥3 consecutive alternating ±90° turns whose segments are all ≤ 2 px is replaced by its two endpoints
  func Simplify(poly []Point, tol float64) []Vec2        // MergeCollinear → RemoveStaircases → RDPClosed(tol)
  func Corners(poly []Vec2, thresholdDeg float64) []int  // indices of vertices whose absolute turn angle > threshold; sorted ascending
  func NearestIndex(dense []Point, p Vec2) int           // index of dense point closest to p (ties → lower index); used to map corner indices back to dense points
  ```

- [ ] **Step 1: Write failing tests**

```go
func TestMergeCollinearSquare(t *testing.T)   // 10x10 crack loop (40 points) → 4 points
func TestStaircaseBecomesDiagonal(t *testing.T) // 8-step staircase loop closed by two straight sides → staircase side reduced to 2 endpoints
func TestStaircaseKeepsOnePxRect(t *testing.T) // 1x5 rectangle loop after MergeCollinear has 4 points and RemoveStaircases keeps all 4 (Review Focus 2)
func TestStaircaseKeepsIsolatedNotch(t *testing.T) // square with one 1-px notch (2 turns) keeps the notch
func TestCornersSquare(t *testing.T)          // 4 vertices, threshold 60 → [0,1,2,3]
func TestCornersOctagonNone(t *testing.T)     // regular octagon (45° turns), threshold 60 → []
func TestNearestIndex(t *testing.T)
```

- [ ] **Step 2: Run, expect compile failure**

- [ ] **Step 3: Implement**

Turn angle at vertex i: angle between `poly[i]-poly[i-1]` and `poly[i+1]-poly[i]`, via `math.Atan2(cross, dot)`, absolute value in degrees.

- [ ] **Step 4: Run, expect pass**

Run: `go test ./internal/geom/ -v`

- [ ] **Step 5: Commit**

```bash
git add internal/geom && git commit -s -m "feat(geom): add collinear merge, staircase removal, closed simplify and corner detection"
```

---

### Task 5: Schneider curve fitting

**Files:**
- Create: `internal/fit/cubic.go`, `internal/fit/schneider.go`, `internal/fit/schneider_test.go`

**Interfaces:**
- Consumes: `geom.Vec2`.
- Produces:
  ```go
  package fit
  type Cubic [4]geom.Vec2
  func (c Cubic) At(t float64) geom.Vec2
  type Segment struct { Line bool; P Cubic }   // Line: P[0]→P[3] straight, P[1],P[2] unused
  // FitOpen fits pts with given unit end tangents (t0 pointing into the curve at pts[0], t1 pointing into the curve at pts[last]).
  // maxErr is the max allowed point-to-curve distance. Fewer than 4 points → one Line segment.
  func FitOpen(pts []geom.Vec2, t0, t1 geom.Vec2, maxErr float64) []Segment
  // FitRun fits a run between two corners: tangents estimated by least-squares line through the first/last k points, k = max(2, min(4, len/3)).
  func FitRun(pts []geom.Vec2, maxErr float64) []Segment
  // FitClosed fits a loop with no corners: splits at the point farthest from the centroid, estimates a single tangent there from both sides averaged, and fits as one run so the join is G1.
  func FitClosed(pts []geom.Vec2, maxErr float64) []Segment
  ```
  Algorithm (spec §8): chord-length parameterisation; 2×2 least squares for handle lengths with fallback to chord/3 when |det| < 1e-9 or a length ≤ 0; accept when max error² ≤ maxErr²; else if ≤ 4·maxErr² do up to 4 Newton–Raphson reparameterisation passes and retry; else split at the max-error point with the tangent there taken as the normalised average of neighbouring chords, recurse.

- [ ] **Step 1: Write failing tests**

```go
func TestFitRecoversCubic(t *testing.T)     // sample a known Cubic at 50 t-values, FitOpen with its exact end tangents, maxErr 0.1 → 1 segment, control points within 0.5 of original
func TestFitLineIsSingleSegment(t *testing.T) // 20 collinear points → 1 segment, max deviation < 1e-6
func TestFitFewPointsIsLine(t *testing.T)   // 3 points → 1 Line segment (Review Focus 3)
func TestFitCircleWithinTolerance(t *testing.T) // 200 points on radius-50 circle, FitClosed maxErr 0.5 → ≤ 8 segments, every sample within 0.5 of some segment
func TestFitSplitsOnSharpError(t *testing.T) // V-shape of two lines fitted as one run with maxErr 0.1 → ≥ 2 segments
```

- [ ] **Step 2: Run, expect compile failure**

- [ ] **Step 3: Implement**

Point-to-curve distance for tests and for error evaluation: evaluate at the point's parameter (Schneider's definition), not the true nearest point.

- [ ] **Step 4: Run, expect pass**

Run: `go test ./internal/fit/ -v`

- [ ] **Step 5: Commit**

```bash
git add internal/fit && git commit -s -m "feat(fit): add Schneider least-squares cubic fitting with reparameterisation and splitting"
```

---

### Task 6: SVG writer

**Files:**
- Create: `internal/svg/writer.go`, `internal/svg/writer_test.go`

**Interfaces:**
- Consumes: `geom.Vec2`, `fit.Segment`.
- Produces:
  ```go
  package svg
  type Subpath struct { Segs []fit.Segment }      // closed implicitly; first point is Segs[0].P[0]
  type Path struct { Fill string; Subpaths []Subpath }  // Fill like "#000000"
  func FormatNum(v float64, prec int) string      // fixed prec decimals, trailing zeros and dot trimmed, "-0" → "0"
  func PathData(p Path, prec int) string          // "M x y" then "L x y" / "C x1 y1 x2 y2 x y" / "H x" / "V y" (H/V chosen automatically when a Line changes only one coordinate), "Z" per subpath; separator rules: a space between numbers except before a negative number
  func Write(w io.Writer, width, height int, paths []Path, prec int) error // spec §9 document shape; newline after <svg …> and after each <path/>
  ```

- [ ] **Step 1: Write failing tests**

```go
func TestFormatNum(t *testing.T)   // (1.5,2)→"1.5"; (2,2)→"2"; (-0.001,2)→"0"; (-0.004,2)→"0"; (-0.006,2)→"-0.01"; (3,0)→"3"  (Review Focus 5)
func TestPathDataSquareUsesHV(t *testing.T) // 4 lines around unit square → "M0 0H1V1H0Z"
func TestPathDataNegativeNoSpace(t *testing.T) // line to (-1,-2) → "M0 0L-1-2Z"
func TestPathDataCubic(t *testing.T)
func TestWriteDocument(t *testing.T) // exact bytes for a 2x2 canvas with one square path
```

- [ ] **Step 2: Run, expect compile failure**

- [ ] **Step 3: Implement**

- [ ] **Step 4: Run, expect pass**

Run: `go test ./internal/svg/ -v`

- [ ] **Step 5: Commit**

```bash
git add internal/svg && git commit -s -m "feat(svg): add compact deterministic SVG path writer"
```

---

### Task 7: `Trace`, modes, options, golden and fuzz tests

**Files:**
- Modify: `options.go`, `options_test.go`
- Create: `trace.go`, `trace_test.go`, `testdata/golden/*.svg`, `internal/rasttest/raster.go` (test-only axis-aligned rasteriser), `internal/rasttest/raster_test.go`

**Interfaces:**
- Consumes: everything above.
- Produces (root package):
  ```go
  type TurnPolicy = contour.TurnPolicy  // re-exported alias; constants TurnMinority, TurnMajority, TurnBlack, TurnWhite
  // Options gains: Threshold float64; AutoThreshold bool; TurnPolicy TurnPolicy; SimplifyTolerance float64; CornerThreshold float64; FitTolerance float64
  // DefaultOptions sets 0.5, false, TurnMinority, 0.75, 60, 1.0
  // Validate adds: Threshold 0..1 inclusive; TurnPolicy one of four; SimplifyTolerance 0..10; CornerThreshold 0..180; FitTolerance 0.01..10
  type Result struct { Width, Height int; Paths []svg.Path }
  func Trace(img image.Image, o Options) (*Result, error)   // Validate first; Preset != bw → &ValidationError{"Preset", "not implemented until Phase 2"}
  func TraceReader(r io.Reader, o Options) (*Result, error) // image.Decode then Trace; decode error wrapped as "rekha: decode: %w"
  func (r *Result) WriteSVG(w io.Writer, o Options) error  // svg.Write with o.PathPrecision
  ```
  Mode mapping: `pixel` → each contour's raw `Pts` as Lines; `polygon` → `geom.Simplify` as Lines; `spline` → corners on simplified polygon mapped to dense indices via `NearestIndex`, dense run between consecutive corners → `fit.FitRun`, no corners → `fit.FitClosed`. Dense points = `MergeCollinear(Pts)` converted to `Vec2`.
  `internal/rasttest.Fill(w, h int, paths []svg.Path) *bitmap.Bitmap` — test-only even-odd scanline fill for paths made of H/V/L segments with integer endpoints; samples pixel centres.

- [ ] **Step 1: Write failing tests**

`options_test.go` additions: new fields rejected out of range (`Threshold 1.1`, `TurnPolicy "spiral"`, `FitTolerance 0`).
`trace_test.go`:
```go
func TestTracePresetNotImplemented(t *testing.T) // Preset poster → *ValidationError Field "Preset"
func TestTraceEmptyImageNoPaths(t *testing.T)    // 8x8 white → 0 paths, valid SVG with no <path
func TestTracePixelModeRoundTrip(t *testing.T)   // for each synthetic via corpus.Render: Trace pixel mode FilterSpeckle 0; rasttest.Fill(result) equals the thresholded bitmap pixel-for-pixel
func TestTraceGolden(t *testing.T)               // for each synthetic × mode: WriteSVG bytes == testdata/golden/<name>-<mode>.svg; -update flag rewrites
func TestTraceDeterministic(t *testing.T)        // star-5 spline traced 3 times → identical bytes
func TestTraceSplineClosesEveryPath(t *testing.T)// every subpath's last segment end equals its first point within 1e-9
func FuzzTrace(f *testing.F)                     // seed corpus: 1x1, 2x2 checker, 3x3 ring; fuzz w,h ≤ 64 and byte slice → bitmap; all 3 modes × 4 policies; must not panic; every path has ≥1 subpath
```
`rasttest/raster_test.go`: `TestFillSquare` (2x2 square path fills exactly 4 pixels), `TestFillRingEvenOdd`.

- [ ] **Step 2: Run, expect compile failure**

- [ ] **Step 3: Implement `trace.go`, extend `options.go`, write `rasttest`**

- [ ] **Step 4: Generate goldens, inspect two by eye, run all**

Run: `go test ./ -run TestTraceGolden -update && go test ./... 2>&1 | tail -12 && go test ./ -run FuzzTrace -fuzz=FuzzTrace -fuzztime=20s`
Expected: all `ok`; fuzz finds no crash. Open `testdata/golden/star-5-spline.svg` and `three-layer-stack-spline.svg` with `rsvg-convert` to PNG and view: star has 10 corners and straight edges, stack shows the disc and the inner square as a hole.

- [ ] **Step 5: Commit**

```bash
git add options.go options_test.go trace.go trace_test.go internal/rasttest testdata/golden
git commit -s -m "feat: add Trace with pixel/polygon/spline modes, golden and fuzz tests"
```

---

### Task 8: CLI wiring

**Files:**
- Modify: `cmd/rekha/main.go`
- Create: `cmd/rekha/main_test.go`

**Interfaces:**
- Consumes: `rekha.TraceReader`, `Result.WriteSVG`, `cli.OutputPath`, `cli.Exit*`.
- Behaviour: new flags `--threshold` (string; `auto` sets `AutoThreshold`, else parsed float), `--turn-policy`, `--simplify`, `--corner-threshold`, `--fit-tolerance`. For each input: open (or stdin), trace, write to `OutputPath` (or stdout). First failure prints `rekha: <input>: <err>` and exits 2 after attempting remaining inputs. Refactor `run(args []string, stdin io.Reader, stdout, stderr io.Writer) int` so it is testable.

- [ ] **Step 1: Write failing tests**

```go
func TestRunTracesPNGToSVG(t *testing.T)   // write star-5 PNG to temp; run([]string{path}) → exit 0; <path>.svg exists, starts with "<svg", contains "<path"
func TestRunStdinStdout(t *testing.T)      // run([]string{"-"}, pngBytes, &out) → exit 0, out starts with "<svg"
func TestRunBadInputExit2(t *testing.T)    // nonexistent file → 2, stderr contains the filename
func TestRunThresholdAuto(t *testing.T)    // --threshold auto accepted; --threshold 1.5 → exit 1
```

- [ ] **Step 2: Run, expect failure**

- [ ] **Step 3: Implement**

- [ ] **Step 4: Run tests and a manual smoke**

Run: `go test ./cmd/rekha/ -v && make build && ./bin/rekha -o /tmp/star.svg testdata/corpus/synthetic/star-5.png && rsvg-convert /tmp/star.svg -o /tmp/star.png && echo ok`
Expected: PASS, `ok`; view `/tmp/star.png`.

- [ ] **Step 5: Commit**

```bash
git add cmd/rekha && git commit -s -m "feat(cli): wire tracing, threshold/turn-policy/simplify/corner/fit flags"
```

---

### Task 9: Benchmark integration and Phase 1 baseline

**Files:**
- Modify: `internal/bench/tool.go`, `internal/bench/run_test.go`, `cmd/rekha-bench/main.go`, `docs/benchmark.md`, `README.md`
- Create: `bench/results/2026-11-09-phase1.md`

**Interfaces:**
- `bench.RekhaTool(bin string) Tool` → `Tool{Name:"rekha", Bin: bin, Args: []string{"--preset","bw","-o","{out}","{in}"}, Input:"png"}`; `rekha-bench -rekha <path>` (default `bin/rekha`, made absolute with `filepath.Abs`) appends it to the tool list unless `-tools` excludes it; `ByName` recognises `rekha`.

- [ ] **Step 1: Write failing test**

```go
func TestRekhaToolArgs(t *testing.T) // RekhaTool("/x/rekha").Args == ["--preset","bw","-o","{out}","{in}"], Bin "/x/rekha"
```

- [ ] **Step 2: Run, expect compile failure**

- [ ] **Step 3: Implement; update docs/benchmark.md (rekha row in Add-a-tool section, `-rekha` flag) and README status to "Phase 1: bilevel tracing works"**

- [ ] **Step 4: Run baseline and check goals**

Run: `make bench 2>&1 | grep -E 'star-5|circle|rounded-rect|thin-lines' && cp bench/out/results.md bench/results/2026-11-09-phase1.md`
Expected: for star-5, circle-aa, circle-hard, rounded-rect, thin-lines: rekha PSNR ≥ potrace PSNR − 2 and rekha Nodes ≤ 1.5 × potrace Nodes. Prepend a header to the results file: date, tool versions, pass/fail per goal image. If a goal fails, record it in the header as a known gap and open the fix as a follow-up; do not tune blindly inside this task.

- [ ] **Step 5: Full verification and commit**

Run: `make lint test wasm-check > /tmp/make.log 2>&1; echo exit=$?`
Expected: `exit=0`.

```bash
git add internal/bench cmd/rekha-bench docs/benchmark.md README.md bench/results
git commit -s -m "bench: add rekha as a tool and record Phase 1 baseline"
git push origin main
```

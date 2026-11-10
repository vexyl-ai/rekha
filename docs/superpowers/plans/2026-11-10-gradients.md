# Gradient Fills Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Merge adjacent quantised bands that form one linear colour ramp into a single region filled by an SVG `<linearGradient>`, behind an opt-in `--gradients` option, with flat art byte-identical to today.

**Architecture:** A new package `internal/gradient` works on the label map after cleanup: components, incremental plane moments, a greedy merge gated by residual and boundary step, relabel with ramps per label. `Segment` calls it when `Options.Gradients` is set and remaps `Second`/`Blend`; `Finish` fills ramp labels with `url(#gN)`; `internal/svg` writes a `<defs>` block. CLI, apiopts, demo and docs expose the two options.

**Tech Stack:** Go 1.26 standard library; `rsvg-convert` (optional) for PSNR tests; Node 20 for the protocol test.

**Spec:** `docs/superpowers/specs/2026-11-10-gradients-design.md`

## Global Constraints

- Go 1.26, `CGO_ENABLED=0`, only `golang.org/x/image`.
- `make lint test wasm-check wasm wasm-test web-test` green; `GOARCH=amd64 go test ./...` green.
- Every existing golden byte-identical; with `Gradients` off no output byte changes.
- `float64()`-wrapped float products in `internal/gradient`.
- DCO `git commit -s`; never an AI attribution trailer; never `git add -A` (user files in repo root).
- Keyed struct literals. Sentence-case copy in the demo.

## Review Focus

1. A JPEG with blocking noise: components of a ramp still merge (noise RMS < 0.03) and a real edge with a step of 0.05 does not (Task 4 `TestMergeNoisyRampStillMerges`, `TestMergeHardEdgeRefused`).
2. Two near-identical flat colours (step 0.02) abutting: they may merge, but the result is a flat fill, never a near-constant gradient (Task 4 `TestMergeNearFlatStaysFlat`).
3. 256 labels already in use: a merge that would need a 257th label is refused, no `uint8` wrap (Task 4 `TestMergeLabelCap`).
4. A ramp region at depth 0 holding most of the page: never omitted as the white page (Task 5 `TestRampPageNotOmitted`).
5. `Second` pointing at a label fully absorbed into a ramp: `Blend` 255, `Second` = own label, no index past the palette (Task 5 `TestSecondRemapAfterMerge`).

---

### Task 1: `<defs>` in the SVG writer

**Files:**
- Modify: `internal/svg/writer.go`
- Test: `internal/svg/writer_test.go`

**Interfaces:**
- Produces: `type Stop struct{ Offset float64; Colour string }`, `type LinearGradient struct{ ID string; X1, Y1, X2, Y2 float64; Stops []Stop }`, `type Document struct{ Width, Height int; Gradients []LinearGradient; Paths []Path }`, `func WriteDocument(w io.Writer, d Document, prec int) error`. `Write` unchanged in signature and bytes.

- [ ] **Step 1: Write the failing tests**

```go
func TestWriteDocumentDefs(t *testing.T) {
	d := Document{Width: 4, Height: 4,
		Gradients: []LinearGradient{{ID: "g0", X1: 0.5, Y1: 1, X2: 3.5, Y2: 1, Stops: []Stop{{Offset: 0, Colour: "#000000"}, {Offset: 1, Colour: "#ffffff"}}}},
		Paths: []Path{{Fill: "url(#g0)", Subpaths: []Subpath{{Segs: square()}}}}}
	var b bytes.Buffer
	if err := WriteDocument(&b, d, 2); err != nil { t.Fatal(err) }
	want := "<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"4\" height=\"4\" viewBox=\"0 0 4 4\">\n<defs><linearGradient id=\"g0\" gradientUnits=\"userSpaceOnUse\" x1=\"0.5\" y1=\"1\" x2=\"3.5\" y2=\"1\"><stop offset=\"0\" stop-color=\"#000000\"/><stop offset=\"1\" stop-color=\"#ffffff\"/></linearGradient></defs>\n<path fill=\"url(#g0)\" d=\"M0 0H4V4H0Z\"/>\n</svg>\n"
	if b.String() != want { t.Errorf("got\n%s\nwant\n%s", b.String(), want) }
}

func TestWriteDocumentNoDefsMatchesWrite(t *testing.T) {
	paths := []Path{{Fill: "#000000", Subpaths: []Subpath{{Segs: square()}}}}
	var a, b bytes.Buffer
	_ = Write(&a, 4, 4, paths, 2)
	_ = WriteDocument(&b, Document{Width: 4, Height: 4, Paths: paths}, 2)
	if a.String() != b.String() { t.Errorf("WriteDocument without gradients differs from Write") }
}
```
`square()` returns the four line segments of the unit square scaled to 4 (reuse an existing helper in the test file if one exists).

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/svg/ -run 'TestWriteDocument' -v`
Expected: FAIL, "undefined: Document".

- [ ] **Step 3: Implement** `Stop`, `LinearGradient`, `Document`, `WriteDocument` in `writer.go`; `Write` calls `WriteDocument`. Defs are one line, emitted only when `len(d.Gradients) > 0`; numbers through `FormatNum(v, prec)`; offsets through `FormatNum(v, 3)`.

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/svg/ -v && go test ./ -run TestTraceGolden`
Expected: PASS; goldens unchanged.

- [ ] **Step 5: Commit**

```bash
git add internal/svg/writer.go internal/svg/writer_test.go
git commit -s -m "feat(svg): document writer with linearGradient defs"
```

---

### Task 2: Fixture `gradient-shapes`

**Files:**
- Modify: `internal/corpus/synthetic.go` (add to `All()` and to `Small()`), `internal/corpus/synthetic_test.go`
- Goldens: `testdata/golden/gradient-shapes-{pixel,polygon,spline}.svg` and any preset goldens the existing golden tests generate per `Small()` entry (run `-update` once, inspect, commit).

**Interfaces:**
- Produces: `Synthetic{Name: "gradient-shapes"}` drawn as spec §5; helper `ramp(img, inside, x0, y0, x1, y1 float64, c0, c1 color.NRGBA)` that fills `inside` with colour interpolated in sRGB per channel along the projection onto (x0,y0)→(x1,y1), anti-aliased by the existing sampler (4 samples).

- [ ] **Step 1: Write the failing test**

```go
func TestGradientShapesPixels(t *testing.T) {
	img := Render(byName(t, "gradient-shapes"))
	// disc top is blue, bottom is yellow; red rectangle is flat red
	top, bottom := img.NRGBAAt(72, 76), img.NRGBAAt(72, 180)
	if top.B < 180 || bottom.R < 200 { t.Errorf("disc ramp: top %v bottom %v", top, bottom) }
	if c := img.NRGBAAt(192, 186); c != (color.NRGBA{220, 30, 30, 255}) { t.Errorf("flat red: %v", c) }
	if c := img.NRGBAAt(150, 134); c.R > 80 { t.Errorf("grey strip left should be dark: %v", c) }
}
```
`byName` looks the synthetic up in `All()` and fails if missing.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/corpus/ -run TestGradientShapesPixels`
Expected: FAIL, "gradient-shapes not in All()".

- [ ] **Step 3: Implement** the shape and `ramp` helper.

- [ ] **Step 4: Run; then regenerate goldens for the new fixture only**

Run: `go test ./internal/corpus/ && go test ./ -run 'TestTraceGolden|Golden' -update && git status --short testdata/golden`
Expected: PASS; only new `gradient-shapes-*.svg` files appear, no existing golden modified (`git diff --stat testdata/golden` empty).

- [ ] **Step 5: Commit**

```bash
git add internal/corpus/synthetic.go internal/corpus/synthetic_test.go testdata/golden/gradient-shapes-*.svg
git commit -s -m "test(corpus): gradient-shapes synthetic with ramps and a hard edge"
```

---

### Task 3: `internal/gradient` plane moments and ramp fit

**Files:**
- Create: `internal/gradient/fit.go`, `internal/gradient/fit_test.go`, `internal/gradient/doc.go`

**Interfaces:**
- Produces:
  - `type moments struct { n int; sx, sy, sxx, sxy, syy float64; sc, scx, scy [3]float64; scc float64 }` with `func (m *moments) add(x, y float64, c [3]float64)`, `func (m *moments) merge(o *moments)`.
  - `type fitted struct { ok bool; u [2]float64; a, b [3]float64; rms, slope float64 }` from `func (m *moments) fit() fitted` (spec §2 steps 2).
  - `func (m *moments) mean() [3]float64`.
  - `type Stop`, `type Ramp` as spec §2; `func ramp(m *moments, f fitted, tmin, tmax float64) *Ramp` builds the two-stop ramp along `u` through the centroid between projections `tmin` and `tmax`.
  - `const minPixels = 16`.

- [ ] **Step 1: Write the failing tests**

```go
func TestFitHorizontalRamp(t *testing.T) {
	var m moments
	for y := 0; y < 8; y++ { for x := 0; x < 64; x++ { v := float64(x) / 63; m.add(float64(x)+0.5, float64(y)+0.5, [3]float64{v, v, v}) } }
	f := m.fit()
	if !f.ok || f.rms > 1e-9 { t.Fatalf("rms %g ok %v", f.rms, f.ok) }
	if math.Abs(math.Abs(f.u[0])-1) > 1e-9 { t.Errorf("direction %v, want ±x", f.u) }
	if math.Abs(f.slope-1.0/63*math.Sqrt(3)) > 1e-6 { t.Errorf("slope %g", f.slope) }
}

func TestFitFlatIsZeroSlope(t *testing.T) {
	var m moments
	for p := 0; p < 100; p++ { m.add(float64(p%10)+0.5, float64(p/10)+0.5, [3]float64{0.3, 0.3, 0.3}) }
	if f := m.fit(); !f.ok || f.slope > 1e-12 || f.rms > 1e-12 { t.Errorf("%+v", f) }
}

func TestFitCollinearNotOK(t *testing.T) {
	var m moments
	for x := 0; x < 10; x++ { m.add(float64(x)+0.5, 3.5, [3]float64{float64(x) / 9, 0, 0}) }
	if m.fit().ok { t.Error("a single row of pixels must not fit a plane") }
}

func TestMergeMomentsEqualsAdding(t *testing.T) { /* add 50 pixels to a and 50 to b, merge, compare each field with a moments built from all 100: equal within 1e-9 */ }

func TestRampStops(t *testing.T) {
	var m moments
	for y := 0; y < 8; y++ { for x := 0; x < 64; x++ { v := float64(x) / 63; m.add(float64(x)+0.5, float64(y)+0.5, [3]float64{v, v, v}) } }
	f := m.fit()
	r := ramp(&m, f, 0.5, 63.5)
	if r.Stops[0].Colour != (color.NRGBA{0, 0, 0, 255}) || r.Stops[1].Colour != (color.NRGBA{255, 255, 255, 255}) { t.Errorf("stops %+v", r.Stops) }
	if math.Abs(r.X0-0.5) > 1e-6 || math.Abs(r.X1-63.5) > 1e-6 || math.Abs(r.Y0-4) > 1e-6 { t.Errorf("ends %+v", r) }
}
```
(`ramp` orients `u` so that `X0 ≤ X1`, ties by `Y0 ≤ Y1`, so stop order is deterministic.)

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/gradient/`
Expected: FAIL to build, "undefined: moments".

- [ ] **Step 3: Implement** `fit.go`: moments, 3×3 solve by Cramer's rule with determinant below `1e-9·n³` meaning not ok, eigenvector of the 2×2 matrix (closed form; when both eigenvalues are 0 the fit is flat with `u = (1,0)` and `slope = 0`), 1-D refit along `u` from the same moments, `rms`, `slope`, `mean`, `ramp` with clamping to 0..255 by rounding.

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/gradient/ -v`
Expected: PASS 5/5.

- [ ] **Step 5: Commit**

```bash
git add internal/gradient/
git commit -s -m "feat(gradient): plane moments, principal direction, two-stop ramp fit"
```

---

### Task 4: `gradient.Merge`

**Files:**
- Create: `internal/gradient/merge.go`, `internal/gradient/merge_test.go`

**Interfaces:**
- Consumes: Task 3 `moments`, `fitted`, `ramp`, `minPixels`.
- Produces: `type Result struct{ Labels []uint8; N int; Ramps []*Ramp; OldToNew []int }`, `func Merge(rgb []uint8, labels []uint8, w, h, n int, tol float64) Result` exactly as spec §2 steps 1, 3, 4, 5. `rgb` is 3 bytes per pixel as `bitmap.Flatten` returns.

- [ ] **Step 1: Write the failing tests**

Helpers: `rampImage(w, h int, bands int)` returns `rgb` of a horizontal grey ramp and `labels` quantised into `bands` equal-width bands; `twoFlats(w, h, a, b uint8)` returns the left half grey `a`, right half grey `b`, labels 0 and 1.

```go
func TestMergeRampBandsJoin(t *testing.T) {
	rgb, labels := rampImage(128, 16, 8)
	r := Merge(rgb, labels, 128, 16, 8, 0.03)
	if r.N != 1 || r.Ramps[0] == nil { t.Fatalf("N %d ramps %v", r.N, r.Ramps) }
	if r.Ramps[0].Stops[0].Colour.R > 8 || r.Ramps[0].Stops[1].Colour.R < 247 { t.Errorf("stops %+v", r.Ramps[0].Stops) }
}

func TestMergeHardEdgeRefused(t *testing.T) {
	rgb, labels := twoFlats(64, 16, 100, 113) // step 13/255 ≈ 0.051
	r := Merge(rgb, labels, 64, 16, 2, 0.03)
	if r.N != 2 || r.Ramps[0] != nil || r.Ramps[1] != nil { t.Errorf("N %d ramps %v", r.N, r.Ramps) }
}

func TestMergeNearFlatStaysFlat(t *testing.T) {
	rgb, labels := twoFlats(64, 16, 100, 104) // step ≈ 0.016: may merge, must stay flat
	r := Merge(rgb, labels, 64, 16, 2, 0.03)
	for _, rp := range r.Ramps { if rp != nil { t.Errorf("near-flat pair produced a ramp %+v", rp) } }
}

func TestMergeNoisyRampStillMerges(t *testing.T) {
	rgb, labels := rampImage(128, 16, 8)
	rnd := rand.New(rand.NewPCG(1, 2))
	for i := range rgb { rgb[i] = clamp(int(rgb[i]) + rnd.IntN(9) - 4) } // ±4/255 noise
	r := Merge(rgb, labels, 128, 16, 8, 0.03)
	if r.N != 1 || r.Ramps[0] == nil { t.Errorf("N %d", r.N) }
}

func TestMergeTinyComponentIgnored(t *testing.T) { /* a 3×3 blob of label 1 inside a flat label 0: untouched, N == 2 */ }

func TestMergeLabelCap(t *testing.T) {
	// 256 labels: label i fills column i of a 256×4 image with grey i; the ramp would merge them all into one.
	// Make two separate ramps (columns 0..127 and 128..255 with a 60-grey jump between) so a merge of the right half
	// needs a fresh label while all 256 are in use: the merge must be refused rather than wrap.
	...
	if r.N > 256 { t.Fatalf("N %d", r.N) }
	for _, l := range r.Labels { if int(l) >= r.N { t.Fatalf("label %d past N %d", l, r.N) } }
}

func TestMergeOldToNew(t *testing.T) {
	rgb, labels := rampImage(128, 16, 4)
	// add a flat label 4 strip at the bottom
	...
	r := Merge(rgb, labels, 128, 20, 5, 0.03)
	if r.OldToNew[4] < 0 { t.Error("flat label must keep a new label") }
	for i := 0; i < 4; i++ { if r.OldToNew[i] != -1 { t.Errorf("band %d should be absorbed", i) } }
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/gradient/ -run TestMerge`
Expected: FAIL to build, "undefined: Merge".

- [ ] **Step 3: Implement** `merge.go`: union–find components in scan order; per-component `moments` and first pixel; adjacency map keyed by `[2]int32{min, max}` with `pairs int` and `step float64`; rounds as spec §2 step 3 (candidates rebuilt per round from live groups, sorted by mean-colour distance then first pixel); after the rounds, assign labels (spec step 5), compute `tmin`/`tmax` by one pass over pixels projecting onto each ramp group's `u`, build ramps, and `OldToNew`. The label cap: a merge is refused when `unmergedLabelsStillUsed + mergedGroups + 1 > 256`, computed conservatively as `n + groups + 1 > 256`.

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/gradient/ -v -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/gradient/merge.go internal/gradient/merge_test.go
git commit -s -m "feat(gradient): merge adjacent bands that fit one linear ramp"
```

---

### Task 5: Pipeline: options, Segment hook, Finish fills, WriteSVG

**Files:**
- Modify: `options.go` (fields, presets, validation), `segment.go` (`Ramps` field, hook, remap), `finish.go` (fills, ids, omit rule), `trace.go` (`Result.Gradients`, `WriteSVG`)
- Test: `gradient_test.go` (new, root package), `options_test.go`
- Goldens: `testdata/golden/gradient-h-photo-gradients-spline.svg`, `testdata/golden/gradient-shapes-photo-gradients-spline.svg`

**Interfaces:**
- Consumes: Task 4 `gradient.Merge`, `gradient.Ramp`; Task 1 `svg.Document`, `svg.LinearGradient`.
- Produces: `Options.Gradients bool`, `Options.GradientTolerance float64` (default 0.03 in every preset; `Validate`: 0..1), `Segmentation.Ramps []*gradient.Ramp`, `Result.Gradients []svg.LinearGradient`.

- [ ] **Step 1: Write the failing tests** (root package)

```go
func photoGradients() Options { o := PresetOptions(PresetPhoto); o.Gradients = true; return o }

func TestGradientHCollapses(t *testing.T) {
	svg := traceSVG(t, "gradient-h", photoGradients())
	if n := strings.Count(svg, "<path"); n > 3 { t.Errorf("%d paths", n) }
	if !strings.Contains(svg, "<linearGradient") { t.Error("no gradient") }
}

func TestGradientShapesFewerPaths(t *testing.T) {
	flat := traceSVG(t, "gradient-shapes", PresetOptions(PresetPhoto))
	grad := traceSVG(t, "gradient-shapes", photoGradients())
	if a, b := strings.Count(flat, "<path"), strings.Count(grad, "<path"); b*3 > a { t.Errorf("flat %d gradients %d", a, b) }
	for _, s := range []string{flat, grad} { if !strings.Contains(s, "fill=\"#dc1e1e\"") { t.Error("red rectangle lost") } }
}

func TestGradientsFlatArtUnchanged(t *testing.T) {
	for _, s := range corpus.Small() {
		if s.Name == "gradient-h" || s.Name == "gradient-shapes" { continue }
		off := PresetOptions(PresetPoster); on := off; on.Gradients = true
		if a, b := traceSVG(t, s.Name, off), traceSVG(t, s.Name, on); a != b { t.Errorf("%s changed with gradients on", s.Name) }
	}
}

func TestGradientsDeterministic(t *testing.T) { /* GOMAXPROCS 1 then 8: identical traceSVG on gradient-shapes */ }

func TestRampPageNotOmitted(t *testing.T) {
	// 64×64 image: a white-to-light-grey ramp over the whole page. With gradients the single region must be emitted.
	...
	if len(res.Paths) != 1 || !strings.HasPrefix(res.Paths[0].Fill, "url(#") { t.Errorf("paths %+v", res.Paths) }
}

func TestSecondRemapAfterMerge(t *testing.T) {
	s, _ := Segment(corpus.Render(byName("gradient-h")), photoGradients())
	for p := range s.Labels {
		if int(s.Second[p]) >= len(s.Palette) { t.Fatalf("second %d past palette %d", s.Second[p], len(s.Palette)) }
		if s.Ramps[s.Labels[p]] != nil && (s.Blend[p] != 255 || s.Second[p] != s.Labels[p]) { t.Fatalf("pixel %d inside a ramp: second %d blend %d", p, s.Second[p], s.Blend[p]) }
	}
}

func TestGradientOptionsValidate(t *testing.T) { o := DefaultOptions(); o.GradientTolerance = 1.5; if o.Validate() == nil { t.Error("1.5 accepted") } }
```
`traceSVG(t, name, o)` renders the synthetic, traces and writes; add it to the test file. Add the two new goldens to `TestTraceGolden`'s loop (a `-photo-gradients-spline` case for the two fixtures only).

- [ ] **Step 2: Run to verify failure**

Run: `go test ./ -run 'Gradient|RampPage|SecondRemap'`
Expected: FAIL to build, "o.Gradients undefined".

- [ ] **Step 3: Implement** per spec §1, §3, §4, §6 bullet 1. In `segmentColour`, after the second `Map` and the `scratch` loop and before the palette is built: when `o.Gradients`, call `gradient.Merge(rgb, s.Labels, w, h, len(final), o.GradientTolerance)`, replace `s.Labels`, remap `Second`/`Blend`, set `s.Ramps`, and build `final`-sized palette for `meanColours` from `r.N` (fixed palette: `fixedEntry` for `OldToNew` labels, mean for ramps). In `Finish`: `gradientIDs map[int]int` per palette index; `fill = "url(#g" + id + ")"`; `res.Gradients` appended on first use with the ramp's ends and stops (`colour.Hex` style `#rrggbb`); the omit rule gets `&& s.Ramps == nil || s.Ramps[l.colour] == nil`. `WriteSVG` writes `svg.Document{...}`.

- [ ] **Step 4: Run; generate the two goldens; whole suite**

Run: `go test ./ -run 'TestTraceGolden' -update && git status --short testdata/golden && go test ./... && GOARCH=amd64 go test ./ ./internal/gradient/`
Expected: only the two new goldens appear; PASS everywhere.

- [ ] **Step 5: Commit**

```bash
git add options.go segment.go finish.go trace.go gradient_test.go options_test.go trace_test.go testdata/golden/gradient-h-photo-gradients-spline.svg testdata/golden/gradient-shapes-photo-gradients-spline.svg
git commit -s -m "feat: --gradients merges linear ramps into linearGradient fills"
```

---

### Task 6: CLI, API keys, demo control, docs

**Files:**
- Modify: `cmd/rekha/main.go`, `cmd/rekha/main_test.go`, `internal/apiopts/apiopts.go`, `internal/apiopts/apiopts_test.go`, `web/protocol.js`, `web/protocol.test.js`, `web/index.html`, `web/app.js`, `README.md`, `docs/wasm.md`, `docs/server.md`, `design/roadmap.md`

**Interfaces:**
- Consumes: Task 5 options.
- Produces: flags `--gradients`, `--gradient-tolerance`; `Params.Gradients *bool` (`json:"gradients"`), `Params.GradientTolerance *float64` (`json:"gradientTolerance"`); both in `SegmentKeys()` and `SEGMENT_KEYS`; demo ids `gradients` (checkbox), `gradientTolerance` (number).

- [ ] **Step 1: Write the failing tests**

```go
// apiopts_test.go
func TestParseGradients(t *testing.T) {
	o, err := Parse([]byte(`{"preset":"photo","gradients":true,"gradientTolerance":0.05}`))
	if err != nil || !o.Gradients || o.GradientTolerance != 0.05 { t.Fatalf("%+v %v", o, err) }
	if !slices.Contains(SegmentKeys(), "gradients") || !slices.Contains(SegmentKeys(), "gradientTolerance") { t.Error("segment keys") }
}
// main_test.go
func TestFlagGradients(t *testing.T) { /* run with --preset photo --gradients --gradient-tolerance 0.05 on a gradient PNG written to t.TempDir(); output contains "<linearGradient" */ }
```
```js
// protocol.test.js
test('gradient keys re-segment', () => {
  assert.ok(SEGMENT_KEYS.includes('gradients') && SEGMENT_KEYS.includes('gradientTolerance'));
  assert.equal(needsResegment({ gradients: false }, { gradients: true }), true);
});
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/apiopts/ ./cmd/rekha/ && node --test web/`
Expected: FAIL: "o.Gradients undefined" / "unknown option gradients" / assertion on SEGMENT_KEYS.

- [ ] **Step 3: Implement** the flags (`fs.BoolVar(&o.Gradients, "gradients", …, "merge adjacent colour bands that form one linear ramp into a gradient fill (colour presets)")`, `fs.Float64Var(&o.GradientTolerance, "gradient-tolerance", …, "RMS colour error 0..1 a ramp may leave (sRGB units)")`), the two `Params` fields and `apply` lines, `SegmentKeys`, `SEGMENT_KEYS`, the demo checkbox "Smooth gradients" and number "Gradient tolerance" in the Colours section (`readOptions` adds `gradients`, `gradientTolerance`; `COMMON` adds `gradients: false, gradientTolerance: 0.03`; both ids in the change listener list), README usage line `bin/rekha --preset photo --gradients art.png` and one sentence in the options paragraph, `docs/wasm.md`/`docs/server.md` option lists, roadmap Phase 5 row "gradient fills (delivered 2026-11-10)".

- [ ] **Step 4: Run to verify pass**

Run: `make lint test web-test wasm-check`
Expected: all green.

- [ ] **Step 5: Commit**

```bash
git add cmd/rekha/main.go cmd/rekha/main_test.go internal/apiopts/apiopts.go internal/apiopts/apiopts_test.go web/protocol.js web/protocol.test.js web/index.html web/app.js README.md docs/wasm.md docs/server.md design/roadmap.md
git commit -s -m "feat(cli,api,web): --gradients and --gradient-tolerance everywhere"
```

---

### Task 7: Bench gate and baseline

**Files:**
- Modify: `gradient_test.go` (PSNR test, bird report), `internal/bench/tool.go` (add `rekha-photo-gradients` tool), `docs/benchmark.md`
- Create: `bench/results/2026-11-10-gradients.md`

**Interfaces:**
- Consumes: `bench.RSVGConvert`, `bench.CheckRasterizer`, `metrics.PSNR`, `metrics.Stats`.

- [ ] **Step 1: Write the failing tests**

```go
func TestGradientsPSNR(t *testing.T) {
	if err := bench.CheckRasterizer(); err != nil { t.Skip(err) }
	for _, name := range []string{"gradient-h", "gradient-shapes"} {
		img := corpus.Render(byName(name))
		flat := psnrOf(t, img, PresetOptions(PresetPhoto))
		grad := psnrOf(t, img, photoGradients())
		if grad < flat-0.05 { t.Errorf("%s: gradients %.2f dB below flat %.2f dB", name, grad, flat) }
	}
}

func TestBirdReport(t *testing.T) {
	path := os.Getenv("REKHA_BIRD")
	if path == "" { t.Skip("REKHA_BIRD not set") }
	// decode, trace with photo and photo+gradients, print paths, bytes, millis and (with rsvg) PSNR for each; no assertion
}
```
`psnrOf` writes the SVG to `t.TempDir()`, rasterises at the image size and returns `metrics.PSNR`.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./ -run 'TestGradientsPSNR|TestBirdReport' -v`
Expected: FAIL to build, "undefined: psnrOf".

- [ ] **Step 3: Implement** the helpers, the bench tool row `rekhaTool(bin, "rekha-photo-gradients", "photo", "--gradients")`, then run

```bash
go test ./ -run 'TestGradientsPSNR' -v
REKHA_BIRD=8_1sasa11.jpg go test ./ -run TestBirdReport -v
make bench
```
and write `bench/results/2026-11-10-gradients.md`: the two fixtures and the bird, paths, nodes, bytes, PSNR, time, flat vs gradients; a verdict on whether `photo` should default to gradients (it flips only if no corpus image loses PSNR or gains paths).

- [ ] **Step 4: Run to verify pass**

Run: `make lint test && GOARCH=amd64 go test ./...`
Expected: green.

- [ ] **Step 5: Commit**

```bash
git add gradient_test.go internal/bench/tool.go docs/benchmark.md bench/results/2026-11-10-gradients.md
git commit -s -m "bench(gradients): PSNR gate, bird report, baseline"
```

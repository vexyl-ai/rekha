# Gradient Fills v2 (multi-stop) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Curved colour ramps become one `<linearGradient>` with as many stops as they need, so merged feathers show no seams.

**Architecture:** `internal/gradient` gains a 1-D fit along a given direction, piecewise gates that only ask each side to be locally linear along the shared direction, and a bin-and-simplify stop builder. The SVG writer, `Segment` and `Finish` already handle any number of stops.

**Tech Stack:** Go 1.26 standard library; rsvg-convert for the PSNR gate.

**Spec:** `docs/superpowers/specs/2026-11-10-gradients-design.md` §2a

## Global Constraints

- As the v1 plan: Go 1.26, `CGO_ENABLED=0`, every existing golden byte-identical (gradient-h gradient golden included), flat art byte-identical with the option on, `float64()`-wrapped products in `internal/gradient`, DCO sign-off, no AI trailer, never `git add -A`.

## Review Focus

1. JPEG noise on a curved ramp: bins average it out; a band's `rms` along `u` stays under tol (Task 2 `TestMergeNoisyCurvedRampJoins`).
2. A ramp that rises then falls along `u`: one region, stops go up then down (Task 2 `TestMergeRiseAndFall`).
3. A bin with no interior pixels (a thin tip): skipped, the stops interpolate across it (Task 1 `TestStopsSkipEmptyBins`).
4. Stop count stays ≤ 66 on a 4000-px ramp (Task 1 `TestStopsCapped`).
5. Flat art byte identity under both presets and tolerance 0 (existing root tests, rerun).

---

### Task 1: Fit along a direction; stops from bins

**Files:**
- Modify: `internal/gradient/fit.go`, `internal/gradient/fit_test.go`

**Interfaces:**
- Produces: `func (m *moments) along(u [2]float64) fitted` (same fields as `fit`, `ok` when `n ≥ minPixels` and the spread along u is non-zero); `fit()` now computes `u` from the plane and returns `m.along(u)`; `type bins struct{ tmin, tmax float64; sum [][3]float64; cnt []int }`, `func newBins(tmin, tmax float64) *bins` (nb per spec), `func (b *bins) add(t float64, c [3]float64)`, `func (b *bins) stops(tol float64) []Stop` (per spec: centres, extrapolated ends, Douglas-Peucker at tol/2, nil when fewer than two non-empty bins), `func rampFrom(m *moments, f fitted, b *bins, tol float64) *Ramp` (ends at tmin/tmax through the centroid as before; nil when stops are nil or span < max(tol, minSpan)). Remove `ramp`.

- [ ] **Step 1: Write the failing tests** — `TestAlongMatchesFit` (along(f.u) equals fit on the horizontal ramp), `TestRampStopsLinearIsTwo` (exact 0..255 ramp over 256 px, bins at 8 px: stops exactly `{0 #000000} {1 #ffffff}`, ends 0.5 and 255.5), `TestStopsCurvedKeepsBends` (quadratic ramp: ≥ 3 stops, every stop within tol of `v = (t/255)²`), `TestStopsSkipEmptyBins` (ramp with pixels missing from bins 3–4: stops still monotone, count = non-empty bins + 2 before simplification), `TestStopsCapped` (4000-px ramp: ≤ 66 stops before simplification).
- [ ] **Step 2: Run** `go test ./internal/gradient/ -run 'Along|Stops'` — Expected: FAIL to build.
- [ ] **Step 3: Implement.** Douglas-Peucker on points `(offset, colour)`: distance of a stop from the segment between two kept stops is measured in colour only (interpolate the segment's colour at the stop's offset). Deterministic: bins in scan order, DP recursion on index ranges.
- [ ] **Step 4: Run** `go test ./internal/gradient/` — Expected: PASS (update `TestRampStops` to the new builder).
- [ ] **Step 5: Commit** `feat(gradient): fit along a direction; multi-stop ramps from bins`

---

### Task 2: Piecewise gates and the bin pass

**Files:**
- Modify: `internal/gradient/merge.go`, `internal/gradient/merge_test.go`

**Interfaces:**
- Consumes: Task 1 `along`, `bins`, `rampFrom`.
- Produces: `Merge` unchanged in signature; gates per spec §2a; `sidesFollow(a, b *group, u [2]float64) (ok bool, rho float64)`.

- [ ] **Step 1: Write the failing tests** — `TestMergeCurvedRampJoins` (quadratic grey ramp, 8 bands, 256×32: N = 1, ≥ 3 stops), `TestMergeNoisyCurvedRampJoins` (same with ±4 noise), `TestMergeRiseAndFall` (grey `255·sin(πx/w)`, 8 bands: N = 1), keep every v1 test.
- [ ] **Step 2: Run** — Expected: `TestMergeCurvedRampJoins` FAILS (N > 1 today).
- [ ] **Step 3: Implement.** Candidate evaluation: `m := union moments; f := m.fit()` for `u` only; `fa, fb := ga.m.along(f.u), gb.m.along(f.u)`; gates per spec. After the passes, for every ramp candidate group build `bins` over its interior pixels (one pass over pixels, projecting on the group's `u`) and call `rampFrom`.
- [ ] **Step 4: Run** `go test ./internal/gradient/ -race` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(gradient): merge whole band chains; stops follow the curve`

---

### Task 3: Goldens, bench, bird, docs

**Files:**
- Modify: `testdata/golden/gradient-shapes-photo-gradients-spline.svg` (if stops changed; gradient-h must not change), `bench/results/2026-11-10-gradients.md`, `README.md` (two-stop → multi-stop sentence), `docs/superpowers/specs/…` already amended.

- [ ] **Step 1: Run** `go test ./ -run 'TestTraceGoldenColour'` — Expected: gradient-h golden passes unchanged; gradient-shapes may differ → inspect, `-update`, confirm only that file changed.
- [ ] **Step 2: Run** `make lint test web-test && GOARCH=amd64 go test ./ ./internal/gradient/` — Expected: green.
- [ ] **Step 3: Run** the photo bench over the corpus and the bird report; render the bird's wing/tail/head crops; confirm no seam inside a feather. Update the baseline's bird table and add a v2 paragraph.
- [ ] **Step 4: Commit** `bench(gradients): v2 multi-stop baseline`

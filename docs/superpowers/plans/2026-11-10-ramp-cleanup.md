# Ramp Cleanup (v3) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A dissolved join keeps its good members, islands inside a ramp take the ramp's label, and stacked output can seal sibling gaps with an own-colour stroke.

**Architecture:** `internal/gradient/merge.go` moves from `find` closures to explicit root arrays (`r1` first-pass root, `r2` final root per component), so `sample` can report error per first-pass member and the dissolve step can regroup members. Island absorption runs after ramps are decided, before labels are compacted. The stroke change is in `options.go`, `finish.go` and the option surfaces.

**Tech Stack:** Go 1.26 standard library.

**Spec:** `docs/superpowers/specs/2026-11-10-gradients-design.md` §2b

## Global Constraints

- As the v2 plan: every golden byte-identical, flat art byte-identical with gradients on, `float64()`-wrapped products in `internal/gradient`, DCO sign-off, no AI trailer, never `git add -A`, `make lint test web-test wasm-check wasm wasm-test` and `GOARCH=amd64 go test ./ ./internal/gradient/` green.

## Review Focus

1. A dissolved group whose kept members split into two disconnected sub-groups: both re-sampled and both checked (Task 1 `TestMergePartialDissolveSplits`).
2. Label budget after a partial dissolve near 256 labels (Task 1 `TestMergePartialDissolveLabelBudget`: n = 253, lone+lone join that splits into two kept sub-groups must not exceed 256).
3. An island bordering two ramps: the one with more boundary pairs wins, tie by lower root (Task 2 `TestMergeIslandPicksLargestBorder`).
4. An island at the image edge or with no interior pixel at all (1-px sliver): centroid and mean come from all its pixels (Task 2 `TestMergeIslandSliverAbsorbed`).
5. `--cutout-stroke` and `--stroke` given together: the last one on the command line wins, as both set one field (Task 3 `TestFlagStrokeAlias`).

---

### Task 1: Partial dissolve

**Files:**
- Modify: `internal/gradient/merge.go`, `internal/gradient/merge_test.go`

**Interfaces:**
- Produces: `func sample(rgb []uint8, comp []int32, w, h int, root []int32, member []int32, groups []group, tol float64) map[int32]*extent` where `root[i]` is the final root of component i and `member[i]` the first-pass root (nil → no per-member errors); `extent` gains `memberErr map[int32]float64`, `memberCnt map[int32]int`. Merge keeps its signature.

- [ ] **Step 1: Write the failing tests** — `TestMergePartialDissolveKeepsGoodMembers` (256×64: four 64-px bands of a horizontal grey ramp 40..200 over x in 0..255 at y < 48, plus at y ≥ 48 a 256-px band whose grey runs 60..180 along y only; tol 0.03; expect 2 labels, the x ramp with a ramp and ≥ 2 stops, the y band without — or with its own lone ramp — but never one label for all), `TestMergePartialDissolveSplits` (two x ramps separated by a y band that joins both: both x ramps survive as two labels), `TestMergePartialDissolveLabelBudget` (n = 253, N ≤ 256 and no label past N).
- [ ] **Step 2: Run** `go test ./internal/gradient/ -run PartialDissolve` — Expected: FAIL (one label, or N > 256).
- [ ] **Step 3: Implement** per spec: root arrays, per-member error in `sample`, regroup kept members by component edges (union-find over first-pass roots), re-sample, re-check, recount budget.
- [ ] **Step 4: Run** `go test ./internal/gradient/ -race` — Expected: PASS, every v2 test included.
- [ ] **Step 5: Commit** `feat(gradient): a dissolved join keeps the members that fit`

---

### Task 2: Island absorption

**Files:**
- Modify: `internal/gradient/merge.go`, `internal/gradient/merge_test.go`, `gradient_test.go` (root)

**Interfaces:**
- Consumes: Task 1 root arrays and `ext` models.
- Produces: `component` gains `size int`, `sx, sy float64`, `sc [3]float64` over all pixels; `absorbed []int32` per component (−1 or the root whose label it takes).

- [ ] **Step 1: Write the failing tests** — `TestMergeIslandAbsorbed`, `TestMergeIslandOfOtherColourKept`, `TestMergeIslandPicksLargestBorder`, `TestMergeIslandSliverAbsorbed` (1-px-wide 6-px column); root `TestGradientsFlatArtUnchanged` and `TestGradientsBigPosterUnchanged` rerun.
- [ ] **Step 2: Run** — Expected: absorbed tests FAIL (island keeps its label).
- [ ] **Step 3: Implement** per spec after `rampOf` is known: for each component with `m.n < minPixels`, scan its edges, choose the ramp root, compare `colourAt(model, offset)` at the centroid with the island mean, set `absorbed[i]`; `pre[i]` takes the root's fresh label.
- [ ] **Step 4: Run** `go test ./internal/gradient/ -race && go test ./ -run 'Gradient|Golden'` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(gradient): islands inside a ramp take its label`

---

### Task 3: Stroke in both hierarchies

**Files:**
- Modify: `options.go`, `finish.go`, `cmd/rekha/main.go` (+ test), `internal/apiopts/apiopts.go` (+ test), `web/index.html`, `web/app.js`, `web/protocol.js` if keys change, `README.md`, `docs/wasm.md`, `docs/server.md`, `finish_test.go` or `trace_test.go` for `TestStrokeStacked`.

**Interfaces:**
- Produces: `Options.Stroke float64` (0..4); `CutoutStroke` field kept, deprecated, folded into `Stroke` in `Validate` (if `Stroke == 0 && CutoutStroke > 0` → `Stroke = CutoutStroke`); flag `--stroke`; JSON key `stroke`; `cutoutStroke` accepted.

- [ ] **Step 1: Write the failing tests** — `TestStrokeStacked` (gradient-shapes, photo, stacked, Stroke 0.5: output contains `stroke="` on a colour path and not on the page; Stroke 0: none), `TestFlagStrokeAlias`, apiopts `TestParseStrokeAlias`.
- [ ] **Step 2: Run** — Expected: FAIL (no `Stroke` field).
- [ ] **Step 3: Implement**; demo control label "Stroke", id `stroke`, key in COMMON and readOptions and the listener list (finish-only key, not a segment key).
- [ ] **Step 4: Run** `make lint test web-test && GOARCH=amd64 go test ./ ./internal/gradient/` — Expected: PASS.
- [ ] **Step 5: Bird** — `./bin/rekha --preset photo --gradients --stroke 0.5`; render the tip and the tail body at 4×; update `bench/results/2026-11-10-gradients.md` with a v3 paragraph and the numbers.
- [ ] **Step 6: Commit** `feat: --stroke seals seams in stacked output too; v3 baseline`

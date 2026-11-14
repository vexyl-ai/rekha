# Wu colour groups: implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Small shaded objects keep a palette entry at the poster tolerance, with no change on images whose palettes were already right.

**Architecture:** One function in `internal/quant/wu.go` grows a colour-group search used by the far-pile coherence test and the tail/mixture subject test, additive to the single-cell rules, gated by a floor of 1/512 of the image. Goldens that change are re-baselined with the bench as evidence.

**Tech Stack:** Go 1.26, `CGO_ENABLED=0`, stdlib + `golang.org/x/image`.

**Spec:** `docs/superpowers/specs/2026-11-14-wu-colour-groups-design.md`.

## Global Constraints

- No new dependencies; `make lint test wasm-check wasm wasm-test web-test` and `GOARCH=amd64 go test ./ ./internal/gradient/` green before every commit.
- Goldens byte-identical except those the bench justifies; `-update` only in Task 2 with the list named in the commit.
- Commits `git commit -s`, no AI attribution trailer. Never `git add -A`.
- Constants: `groupFloor = 512`; the existing `coherentShare = 0.25`, `coherentMin = 4`, `subjectShare = 64`, `haloShare = 16` unchanged. Distance `colour.Dist`, no chroma weight.
- Anti-goal fixtures: `noisy-disc.png` and the Q ribbon logo palettes unchanged.

## Review Focus

- A gradient ramp box whose far tail spreads over many cells under the floor: no split (Task 1, `TestWuGroupFloorKeepsRamp`).
- Two small shaded objects in one box diluting each other: both promoted (Task 1, `TestWuGroupsTwoShadedSubjects`).
- A noisy flat field: the group rule must not promote noise, so the tail group's halo test still applies (Task 1, `TestWuGroupIgnoresNoise`).
- Determinism: group order from a sort by weight with index tie-break, so the palette is the same run to run (`TestWuDeterministic` kept, Task 1 asserts it on the new fixture).
- The 256-colour cap still holds with promotions from groups (`TestWuCapsAt256` kept).

---

### Task 1: colour groups in `Wu`

**Files:**
- Modify: `internal/quant/wu.go` (`within`, `pile`, constants)
- Test: `internal/quant/quant_test.go`

**Interfaces:**
- Produces: no API change. `Wu(cells []Cell, maxK int, tol float64) []colour.OKLab` as before. Internal: `func groupPile(cells []Cell, members []int, tol float64) (mass, seed int)`; `pile.members []int` collected by `pile.add`; `const groupFloor = 512`.

- [ ] **Step 1: Write the failing tests** (fixtures built as rgb buffers like `TestWuKeepsSmallFeature`; width×height so the shares below hold):

```go
// A 512×512 near-grey field (#cfc6bf with ±2 per-channel texture) holding
// two shaded objects of 0.7 % each: an orange disc ramping #f7752a→#c85a1c
// over 8 levels and a white disc ramping #ffffff→#e8e8e8 over 8 levels.
// Wu at 0.08 returns ≥ 3 colours, one within 0.06 of orange and one within
// 0.06 of white.
func TestWuGroupsTwoShadedSubjects(t *testing.T)
// A 1024×1024 image: a horizontal ramp #000000→#ffffff plus a 150-pixel
// patch of #f7752a (under 1/512 = 2048 px). Wu at 0.08 returns the same
// palette length as the same image without the patch.
func TestWuGroupFloorKeepsRamp(t *testing.T)
// A 256×256 field of #808080 with uniform ±12 noise on every channel: Wu at
// 0.08 returns 1 colour.
func TestWuGroupIgnoresNoise(t *testing.T)
```

- [ ] **Step 2: Run.** `go test ./internal/quant -run 'TestWuGroup'`. Expected: FAIL (two subjects give 1–2 colours; the other two may pass already, that is fine: they pin the anti-goals).

- [ ] **Step 3: Implement.** `pile.add` appends to `members`. `groupPile` sorts a copy of members by weight descending (index ascending on ties), greedily groups within `tol` of each unassigned seed, returns the heaviest group's mass and seed. In `within`: far rule `far.coherent() || groupCoherent(far)` where `groupCoherent` needs `mass >= coherentMin`, `mass >= coherentShare*pile.mass`, `mass*groupFloor >= total`; `subject(p)` becomes `single(p) || groupSubject(p)` where `groupSubject` needs `mass*subjectShare >= total`, `mass*groupFloor >= total`, and the halo test with "in group" = within `tol` of the seed; promotion returns the seed when the group rule fired. Update the comment block above the constants to describe groups.

- [ ] **Step 4: Run.** `go test ./internal/quant`. Expected: PASS, all tests including `TestWuDeterministic`, `TestWuCapsAt256`, `TestWuIgnoresAABand`.

- [ ] **Step 5: Run the goldens without updating.** `go test ./ -run 'Golden'`. Expected: FAIL on a short list of fixtures; write the list down for Task 2. If `noisy-disc` or `gradient-h` is on it, stop: the floor or the halo test is wrong (spec anti-goal).

- [ ] **Step 6: Commit.** `git add internal/quant && git commit -s -m "quant: Wu groups far and tail cells before judging a hidden subject"`.

### Task 2: goldens, bench and changelog

**Files:**
- Modify: `testdata/golden/*.svg` (the Task 1 list only), `CHANGELOG.md` (Unreleased / Changed), `README.md` if the poster row describes palette behaviour
- Create: `bench/results/2026-11-14-wu-groups.md`

- [ ] **Step 1: Re-baseline.** `go test ./ -run 'Golden' -update`, then `git diff --stat testdata/golden` must list exactly the Task 1 fixtures. Render each changed golden before and after with `rsvg-convert` and score against its source (`internal/metrics` PSNR or the bench script); every change must score equal or higher. A lower score stops the task.

- [ ] **Step 2: Bench.** Run the 21-image bench (character + `rekha-cloud/spike-in`) with `-preset poster -color-tolerance 0.06 -gradients -refine` on the committed binary and write `bench/results/2026-11-14-wu-groups.md` in the format of `2026-11-13-auto.md`: table of before/after dB and paths, mean, wins, losses. Expected: no losses; character ≥ 30.0 dB.

- [ ] **Step 3: CHANGELOG.** Under Unreleased, `### Changed`: one entry on the quantiser (small shaded objects keep their colour at the poster tolerance; which goldens changed and why).

- [ ] **Step 4: Gates.** `make lint test wasm-check wasm wasm-test web-test` and `GOARCH=amd64 go test ./ ./internal/gradient/`. Expected: green.

- [ ] **Step 5: Commit.** `git add testdata/golden CHANGELOG.md bench/results/2026-11-14-wu-groups.md && git commit -s -m "quant: re-baseline the goldens the colour groups change, with the bench"`.

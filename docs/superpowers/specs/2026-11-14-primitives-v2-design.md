# Primitives v2 design: arcs and circles that fire on real logos

Status: design, 2026-11-14. Follows `2026-11-10-phase3b-quality-design.md` §5 (primitives), which this replaces in part. The user approved approach B (curvature segmentation) in chat after a measured brainstorm; the output representation question (SVG `<circle>` elements) was closed as paths only. Every other choice below is the author's call.

## 1. Intent

Phase 3b added circle, arc, common-radius and rectangle snapping under `--refine`. On the synthetic shapes it was built against it works (`rounded-rect` is 4 lines + 4 arcs, `rect-rot` is 4 nodes, `circle-aa` is 4 cubics). On real logos it almost never fires:

| image (2048 px, poster 0.06, refine) | cubics | of which circular arcs |
|---|---|---|
| Pokéball (every edge a circle or line) | 93 | 27 |
| G logo (concentric bands) | 1594 | 91 |
| teal abstract | 374 | 37 |

The gap matters to the two buyers primitives exist for: designers who open the SVG and expect a circle to have four nodes, and cutter/CNC users whose DXF (rekha-cloud) turns four-cubic circles into `CIRCLE` and arc cubics into bulge arcs, and everything else into hundreds of polyline vertices.

Instrumenting the Pokéball found four independent blockers, none of them "the fit is wrong":

1. **Whole-loop circles are fit, then reverted.** `FitCircle` accepts the outer ring (r = 880 px, every sample within 0.30 px). `SnapCircle` reverts it because the rendered data energy rises 9.5 % against the 2 % guard (`primRise`). The best circle anywhere, after searching centre and radius against the mask, still rises 6–9 %. In the window that is a PSNR drop of 0.26–0.40 dB at 36–42 dB. The relative-energy guard is the wrong instrument for a near-perfect fit: the free eight-cubic outline absorbs 0.1 px of image imperfection the circle cannot, and the guard calls that a failure.
2. **Short pieces of large arcs count as straight.** `cutJoints` ends a run at any segment `straightCubic` passes at 0.5 px. A cubic along r = 743 px with a chord under ~47 px has handles within 0.5 px of its chord, so it is "straight": it cuts the run, is emitted as a line, and its neighbour's free end is "slid to the tangent point" of a line that is really part of the same arc, which produces a wrong candidate the guard then rightly rejects (`run r=743.0 maxdev=0.113 → guard-reject`).
3. **Runs mix radii.** A fillet that flows smoothly (turn < 5°) into a large arc is one run; one Kåsa circle cannot fit both (`r=743.9 maxdev=7.099 → nofit`). On the G logo 315 of 343 runs die this way.
4. **Only tangent ends exist.** `snapRun` places the circle through fixed ends and tangent to the lines at sliding ends. A half-disc, pie wedge or annular sector meets its lines at a corner, not a tangent; the tangent model gives wrong geometry there and the guard rejects it. Loosening the guard alone made the Pokéball worse (40.4 → 39.0 dB) by admitting those candidates.

Plus a smaller, related defect: sharp corners between two lines come out of fitting as a 1–3 px fillet cubic (origami logo: `L645.93 1188.85 C645.25 1188.76 644.86 1188.68 644.74 1188.21`), so a triangle has six nodes instead of three.

## 2. Goals

With `--refine` (so `Refine.Primitives`), spline mode, any preset and either hierarchy:

- **Whole circles.** A loop that `FitCircle` accepts and that passes the §5 guard becomes four arc cubics. Pokéball under bw: all three ring loops are four-cubic circles.
- **Arcs with any ends.** Runs that lie on one circle become arc cubics whether their ends are tangent to a line, meet a line at a corner, meet another arc, or are fixed junctions. New synthetic `half-disc` comes out as 1 line + 2 arc cubics; new synthetic `annular-sector` (two concentric arcs joined by radial lines) as 2 lines + 4 arc cubics, the two arcs concentric within 0.2 px.
- **Mixed radii.** A large arc flowing smoothly into a fillet becomes two arcs, each at its own radius. New synthetic `fillet-arc` (a D with r = 100 whose straight side meets the arc through r = 12 fillets) comes out as 1 line + 2 fillet arcs + the large arc, radii within 2 % of true.
- **No polygonal arcs.** No segment emitted as a line lies on an arc of radius ≥ 50 px that the neighbouring segments continue (asserted on `annular-sector` and the Pokéball: every curved segment is a circular arc, see §7 for the test).
- **Sharp corners.** A joint of two lines whose fitted fillet is shorter than `cornerFillet` becomes one node at the lines' intersection. New synthetic `triangle` (anti-aliased, 3 sides) comes out as exactly 3 nodes, each within 0.15 px of the true corner.
- **Real logos.** On the nine spike-in logos (rekha-cloud `spike-in/*-logo.png`, user-owned, not in the public corpus) traced with poster 0.06 and `--refine`: the Pokéball (`black-red-circle-symmetric-logo.png`) has at least 90 % of its curved segments as circular arcs and at most 70 nodes; the G logo (`red-yellow-letter-g-minimalistic-circular-logo.png`) at least 40 %. Numbers are recorded in `bench/results/2026-11-14-primitives-v2.md`, not asserted in tests (the images are private).
- **No regressions.** `TestPrimitivesNeverLoseOnGoalImages` holds as written (no goal image loses more than 0.2 dB to the primitive pass, node total does not rise). On the 21-image bench (the 20 files of rekha-cloud `spike-in/` plus the session's `character.png`, the set every result file since 2026-11-13 uses) no image loses more than 0.5 dB against main and the mean does not fall by more than 0.1 dB. `rounded-rect` stays 8 segments with one radius within 0.2 px of 24; `rect-rot` stays 4 nodes; `circle-aa` stays 4 cubics; `star-5` stays 10 lines. `BenchmarkRefineBigPoster` within 1.2× of main.
- Everything else from Phase 3b holds: pixel and polygon goldens byte-identical; spline goldens may change only under the existing PSNR check; determinism across `GOMAXPROCS`; fuzz clean; `CGO_ENABLED=0`, wasm, no new dependencies; `make lint test wasm-check wasm wasm-test web-test` and `GOARCH=amd64 go test ./ ./internal/gradient/` green.

## 3. Non-goals and anti-goals

Non-goals (later work): ellipses, regular polygons, symmetry, snapping lines to the axes or to 45°, SVG `<circle>`/`<rect>` elements (closed: paths only; DXF already converts), primitives without `--refine`, splitting one cubic that holds two radii (pieces are whole refined segments; a merged fillet+arc cubic stays free-form).

Anti-goals:

- A snap that is geometrically right but visibly wrong: every snap is still judged against the mask, only the yardstick changes (§5). A Pokéball ring at 41.9 dB instead of 42.3 dB is the trade this design makes deliberately; a 2 dB drop is not.
- Accepting candidates by loosening the guard: the §4 joint model removes the wrong candidates; the guard is loosened only as far as the whole-circle case needs.
- Slower refinement on busy posters: `big-poster` within 1.2×.

## 4. Design

All in `internal/refine`, under `Refine.Primitives`, after `Refine` and `MergeSplitTracked`, before `b.Segments(lineTol)`. `trace.go:refineConstrained` keeps its order (whole circle → arcs → common radius → rectangle) but calls the new entry points.

### 4.1 Pieces replace runs

A loop is cut into **pieces**, each one or more consecutive refined segments, each classified `line`, `arc` or `free`. Finding them:

- **Hard cuts.** A joint is a hard cut when it is fixed (`Options.Fixed`), when either adjacent segment is frozen, or when its turn (`AnglePrior` at the joint) is at least `arcJointTurn` (5°, unchanged). Nothing else cuts: the `straightCubic` test no longer decides where runs end (blocker 2).
- **Spans.** Segments between consecutive hard cuts form a span. A loop with no hard cut is one span that wraps; it is first offered to the whole-circle snap (§4.3) and, if that declines, cut once at the sample furthest from the loop's Kåsa circle (at the nearest joint) so the span has ends.
- **Split and fit.** A span is fitted by Kåsa on its samples (`circleSamples` = 8 per segment plus the end, as today). A span **fits** when every sample lies within `lim = min(tol, circleRelTol·r)` of the circle, or within `tol` of the total-least-squares line through them (`fit.FitLine`). A span that fits is one piece. Otherwise it is split at the joint nearest the sample of greatest radial deviation and each half recurses; a single segment that fits nothing is `free`.
- **Re-merge.** Adjacent non-free pieces whose union fits (circle or line, same test) are merged, left to right, until nothing merges, so a split that landed inside a true arc heals and a short leftover of a large arc rejoins it instead of becoming a line.
- **Classify.** After re-merging, each piece is a `line` when its samples lie within `tol` of its line **and** the sagitta of its circle fit over its chord is under `tol` (the two explanations agree to within tolerance, so the simpler one wins); otherwise an `arc` when the circle fits; else `free`. The sagitta condition is what stops a leftover piece of a large arc from being called straight; the re-merge is what stops it from being left over at all. (Builder check, author's call.)
- `tol` stays `0.5·FitTolerance`.

### 4.2 Joint model

Each end of an arc piece is typed from the joint it sits on; the type decides how the arc's circle is solved at every trial radius (so tangency survives the radius search, as `snapRun` does today):

| the arc's end sits on | end type | how the circle is solved there |
|---|---|---|
| a fixed joint | point | through the anchor as it is |
| a line piece | tangent, falling back to free | tangent to the fitted line; the anchor becomes the tangent foot (existing `throughAndTangent` / `tangentCircle` when both ends are tangent). When the tangent circle cannot hold the arc's samples within `lim` (a corner), the end is free. The turn at the joint cuts pieces (§4.1) but does not type ends: a refinement can leave a 7° kink at a geometrically tangent joint, and only the samples can tell a kink from a corner (execution ruling, Task 5) |
| an arc piece already snapped | point | through the anchor as it is, which lies on that arc's circle, so the snapped arc is not disturbed (execution ruling, Task 5) |
| an arc piece not yet snapped | free | the anchor does not pin the circle; once the circle is solved the anchor is projected radially onto it and the neighbour's near handle moves with it, so the neighbour keeps its shape (execution ruling, Task 5: a circle pinned through two refined anchors, each a little off, is off by as much all along, and on a clean edge that is worse than the refinement; the Kåsa centre averages the anchors' noise, as it does for whole circles) |
| a free piece | free | as for an unsnapped arc: the anchor is moved onto the circle and the free piece's near handle with it |

Two tangent ends solve jointly with `tangentCircle`; one tangent and one point with `throughAndTangent`; two points with `centreAt`; one tangent or one point with the other end free as the centre nearest the Kåsa centre on the locus the constraint leaves (the offset line, or the circle of radius `r` about the point); two free ends as the Kåsa centre itself, the radius alone searched, as `SnapCircle` does. A tangent solve that cannot keep the arc's samples within `lim` falls back to free: both ends tangent first, then each alone, then neither. Tangency between two arcs of different radii is not enforced (it would need a joint that is neither anchor's); the ≤ `lim` kink is what the refined curve already has.

A **line piece** of two or more segments is rebuilt last, as one line cubic between the anchors its neighbours settled: an arc neighbour's tangent foot or point, a line neighbour's intersection (the two fitted lines' intersection when within `min(tol, 0.1·shorter chord)` of the old anchor, else the old anchor as it is), a free neighbour's old anchor. A line piece of one segment stays as the refinement left it: `Straightened` has already judged its bow, and a chord in its place saves no node (execution ruling, Task 5: on poster-discs such chords lost up to a quarter of the local fit and were admitted by the floor).

**Corner sharpening.** A piece of any kind (a 1–3 px fillet passes the line test at `tol`, so `line` included) of chord length ≤ `cornerFillet` (3 px, author's call) between two `line` pieces of chord > `cornerFillet` whose fitted lines intersect within `cornerReach` (2·`cornerFillet`) of the piece is dropped: the two lines meet at their intersection. Judged by the guard like any snap, over the window of the fillet and the two lines' changed ends.

### 4.3 Rebuilding and guarding

Order: arc pieces first (later pieces first, as today, so earlier indices stay valid), then corner sharpening, then line pieces. Each rebuild is one snap under the guard (§5):

- `arc` piece → circle solved per §4.2 at each trial radius of the existing golden-section search (`radiusSearch`, `radiusSteps`) around the Kåsa radius, pieces of at most `pieceSweep`, handles `4/3·tan(θ/4)·r` (existing `arcPieces`, `replaceRun`); a tangent end rebuilds its line neighbour's end to the foot inside the same candidate, as `snapRun` does. A point unreachable at a trial radius (chord > 2r) fails that trial.
- `line` piece → one line cubic between the anchors its neighbours settled (several collinear refined segments become one segment; node count falls).
- `free` piece → untouched.
- A rejected arc keeps its old control points and its old anchors; a neighbour that later rebuilds against it uses those old anchors (the arc–arc point or the line's end), so the join stays continuous.

**Whole circle** (`SnapCircle`): unchanged fit; the candidate's radius is refitted against the mask by the same golden-section search (centre stays at Kåsa's: the spike's 3-parameter search never moved it); then the guard.

**`Straightened(lineTol)` before the pass and `Segments(lineTol)` after it stay as they are.** Before: a short piece of a large arc reaches the classifier with its handles on the chord but its anchors on the arc, and its samples within 0.19 px of the circle, inside `lim`; the span fit is unaffected. After: a rebuilt arc cubic whose handles sit within 0.25 px of its chord (sweep under about 2° at r = 743) is emitted as a line 0.1 px from the arc, which no test or user can see; `arcShare` counts curved segments only. (Builder check, author's call.)

**Common radius** (`CommonRadius`): unchanged logic over the new arc pieces.

**Rectangle** (`FitRectangle`): unchanged; it now also benefits from line pieces merging collinear segments.

## 5. The guard

`budget(before)` is replaced by a two-sided rule evaluated over the same window (`windowAround` the changed segments, `fineDataEnergy`), for every snap (whole circle, arc, line, corner, common radius):

accept when `E_after − E_before ≤ primRise·E_before` **or** (`E_after − E_before ≤ primRelMax·E_before` **and** `(E_after − E_before)·l0 / L ≤ primAbs`),

where `E` is `DataEnergy` (Σ w·(DataScale·(cov − mask))² / l0, `l0` the loop length), `L` is the polyline length of the segments of `b` the snap changes (the window's share of the boundary), `primRise` stays 2 %, `primAbs` = 1.0 and `primRelMax` = 0.25 (author's calls; the cap was added during execution: on a noise-free mask a quarter ellipse snapped to a circle raises the energy tenfold while costing only 0.13 per boundary pixel, less than the Pokéball ring's 0.26, so the floor alone cannot tell the wrong model from the right one, and a snap that makes the fit more than a quarter worse is the wrong model whatever it costs). The second clause is an absolute fidelity floor: a snap may add at most `primAbs`/`DataScale`² ≈ 0.016 of squared coverage error per pixel of boundary it touches, however good the fit already was. It is not a looser ratio: a drop in dB over the window is the same thing as the ratio (the builder check caught the first draft saying 0.5 dB, which was 12 % everywhere), whereas the absolute floor scales with how little error is left. Calibration from the spike, `l0 = L` for whole loops: Pokéball outer ring +0.26, inner ring +0.13 (accepted); the wrong tangent candidates the current guard rejects rise 4–9 (still rejected, by a margin of 4×). The first clause keeps today's behaviour on rough fits where 2 % of a large `E` exceeds the floor. `TestPrimitivesNeverLoseOnGoalImages` (0.2 dB whole-image, unchanged) is the backstop.

## 6. Synthetic corpus

`internal/corpus` gains `half-disc` (r = 80, flat side vertical), `annular-sector` (radii 60 and 100, 120° sweep), `fillet-arc` (D of r = 100, fillets r = 12), `triangle` (sides 180/150/120 px, no right angle), all 256 px, 4×4 anti-aliased, black on white; regenerated into `testdata/corpus/synthetic` with `SOURCES.md` rows. Goldens for the four under bw spline, with and without refine, added by `-update`.

## 7. Tests

- Unit (`internal/refine`): split-and-fit on a synthetic bezigon of arc(100)+arc(12)+line yields three pieces with the right kinds and radii; re-merge heals an over-split arc; each joint-model row on a hand-built bezigon; corner sharpening drops a 2 px fillet and keeps a 10 px one; the absolute clause accepts a candidate that rises 9 % on a loop whose energy per boundary pixel is small (the Pokéball-ring case, built synthetically) and rejects one that rises 9 % on a rough loop beyond `primAbs`; the relative clause still accepts a 1 % rise on a rough loop; a rejected piece beside an accepted one stays continuous (anchor shared, no gap).
- Integration (root `refine_test.go`): the §2 shape assertions (`half-disc`, `annular-sector`, `fillet-arc`, `triangle`, Pokéball-style ring built synthetically as `ring` if the bw Pokéball cannot be included: a ring with r = 110/80 at 256 px, both loops four cubics); `arcShare(segs)` helper counting cubic segments that are circular (samples at t = ¼, ½, ¾ within 0.02 px of the circle through the ends and midpoint) used by the "no polygonal arcs" assertion; existing primitive tests unchanged.
- Bench: `bench/results/2026-11-14-primitives-v2.md` with per-image PSNR, nodes, arc share for main vs branch on the 21 images and the seven logos.

## 8. Approaches considered

- **A. Patch the four blockers in place.** dB guard, radius-relative straight test, trial split of failed runs, corner-end model bolted onto `snapRun`. Smallest diff; keeps the tangent-slide assumption at the centre and adds a heuristic per blocker. Rejected: blockers 2–4 are one defect (runs cut and ended by the wrong signal), and A would fix each symptom separately.
- **B. Curvature segmentation (chosen).** Replace run finding with split-and-fit pieces, a joint model that chooses tangent or corner per joint, one guard. Medium diff; `Circle`, `kasa`, `arcPieces`, `replaceRun`, `throughAndTangent`, `FitRectangle`, `CommonRadius` reused. Synthetic goldens must be re-earned.
- **C. Snap on the raw contour before refinement.** Fast, but Kåsa on pixel staircases is worse than on the refined outline and loses sub-pixel placement. Rejected.

## 9. Left to the builder

- Sample density for long segments (8 per segment is the floor; more may help fits on 800 px arcs).
- Whether the span's first cut for an uncut non-circle loop is the furthest sample or the sharpest curvature change (either is fine; furthest sample is the default).
- The exact window for a dropped corner fillet (the two lines' changed ends plus the fillet).
- Constants: `cornerFillet` 3 px, `cornerReach` 6 px, `primAbs` 1.0 are starting values; the bench may move them, with the number recorded in the results file.

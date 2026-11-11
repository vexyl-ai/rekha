# Gradient fills design

Status: design, 2026-11-10; §2a (multi-stop ramps, v2) added the same day after the first build left seams between partial ramps on the bird's curved feathers. Follows Phase 4 (open half, merged as 901d4ce) and the web polish (1912e58). The user asked for a gradient option after a bird illustration with smooth feather shading traced into dozens of flat colour bands; design questions were skipped at the user's request, so every choice below is the author's call. Phases 1–4 stay the base: segmentation, cleanup, contour tracing, stacking, the planar map, fitting and refinement are unchanged except where a section below says otherwise.

## Intent

Flat-colour quantisation turns a smooth ramp into a stack of bands: on the user's 4001×4001 bird (`8_1sasa11.jpg`) the photo preset gives 94 colours and 150 KB, and the shading still steps. SVG has `<linearGradient>`. Rekha should notice when adjacent bands are one smooth ramp, merge them into one region, and fill that region with a linear gradient fitted to the pixels, so the SVG renders the shading smoothly with fewer paths. Flat art must be untouched: with the option on, art without ramps gives exactly the output it gives today.

## Goals

- **Bands become ramps.** New option `Gradients bool` (CLI `--gradients`, API/wasm key `gradients`, demo checkbox "Smooth gradients"), with `GradientTolerance float64` (`--gradient-tolerance`, key `gradientTolerance`; 0..1, default 0.03). Under the photo preset with `--gradients`, the `gradient-h` synthetic (a 256-px black-to-white ramp) traces to **at most 3 paths**, at least one filled by a `<linearGradient>`, and a new synthetic `gradient-shapes` (§5) to **at most a third of the paths** of the same options without `--gradients`. On both, PSNR rendered by `rsvg-convert` is **not below** the flat trace's (measured by `make bench` and pinned by a Go test that skips without rsvg).
- **Flat art is untouched.** For every synthetic in `corpus.Small()` other than `gradient-h`, under the poster preset, the SVG with `Gradients` on is **byte-identical** to the SVG with it off. Every existing golden stays byte-identical (the option is off by default and every preset keeps it off).
- **Hard edges survive.** In `gradient-shapes`, a flat red rectangle abutting a ramp keeps its own flat path with the red fill; the ramp regions do not absorb it.
- **Deterministic and portable.** Same SVG for `GOMAXPROCS=1` and `8`; `GOARCH=amd64 go test ./...` green; `make lint test wasm-check wasm wasm-test web-test` green; `CGO_ENABLED=0`; no new dependencies.
- **Reported on the bird.** A test gated by `REKHA_BIRD=<path>` prints paths, bytes and PSNR (when rsvg is present) for the photo preset with and without gradients, and the baseline `bench/results/2026-11-10-gradients.md` records the numbers for the synthetics and the bird.

## Non-goals

- Radial, conic or multi-stop gradients: v1 fits one linear ramp with two stops per region. A ramp that is not linear merges only as far as the tolerance allows and the rest stays banded.
- Turning `Gradients` on in any preset. The baseline says whether `photo` should flip later.
- Gradients in the bw preset (one colour) or with a fixed `--palette`'s colours as stops (a merged region's stops come from its pixels; the palette entry for it is the region's mean, as for quantised colours).
- Changing the quantiser: the bands it makes are the merge candidates; nothing is added upstream.
- Sub-pixel anti-aliasing across a ramp region's edge in refinement: inside a merged region the coverage is full (§3), so `--refine` sees a hard edge there.

## Anti-goals

- A merge that crosses a real edge. The boundary step test (§2) is a hard gate, not a weight.
- Output that changes with thread count, map order or scheduling.
- Any change to the output when the option is off: no shared code path may alter a byte.

## Constraints (carried)

Module `github.com/vexyl-ai/rekha`, Go 1.26, `CGO_ENABLED=0`, only `golang.org/x/image`, Apache-2.0, DCO sign-off, no AI attribution trailer, never `git add -A`, keyed struct literals, `float64()`-wrapped float products in the new package so arm64 and amd64 agree, goldens compared by `svgClose`.

## 1. Where it sits

`Segment` (colour presets only), after `region.Cleanup` and the second lookup pass that sets `Second` and `Blend` against the final palette, and before `meanColours`:

```
labels, final palette ─► gradient.Merge(rgb, labels, w, h, len(final), o.GradientTolerance)
                       ─► relabel, Ramps per new label, Second/Blend remap (§3)
                       ─► meanColours on the new labels
```

`Segmentation` gains `Ramps []*gradient.Ramp` (nil, or one entry per palette index; nil entry = flat). `Finish` is unchanged except that a layer whose palette index has a ramp is filled with `url(#gN)` (§4). Tracing, nesting, stacking, the planar map, fitting and refinement never see the option.

## 2. Package `internal/gradient`

```go
// Stop is one colour of a ramp at an offset in 0..1.
type Stop struct {
    Offset float64
    Colour color.NRGBA
}
// Ramp is a linear gradient in pixel coordinates (SVG userSpaceOnUse):
// colour Stops[0] at (X0,Y0) to Stops[1] at (X1,Y1).
type Ramp struct {
    X0, Y0, X1, Y1 float64
    Stops          []Stop
}
// Result of Merge: labels renumbered compactly by first appearance.
type Result struct {
    Labels   []uint8 // new label per pixel
    N        int     // number of new labels
    Ramps    []*Ramp // per new label; nil for a flat label
    OldToNew []int   // per old label: its new label, or -1 when every pixel of it joined a ramp
}
func Merge(rgb []uint8, labels []uint8, w, h, n int, tol float64) Result
```

All colour arithmetic is in sRGB with channels scaled to 0..1, because that is what an SVG gradient interpolates; `tol` is an RMS distance in those units (0.03 ≈ 8/255). The merge:

1. **Components.** 4-connected components of equal labels (union–find, scan order). For each component: pixel count, and, over its interior pixels only (every in-image 4-neighbour carries the same label, so anti-aliased edge pixels never enter the fit), the moments needed for a least-squares plane per channel on pixel centres (x+0.5, y+0.5): n, Σx, Σy, Σx², Σxy, Σy², and per channel Σc, Σcx, Σcy, Σc². For each adjacent pair of components (4-adjacent pixel pairs with different labels): the number of boundary pairs and Σ|Δc| over them (Euclidean sRGB distance per pair). Moments and boundary sums add under a merge, so no candidate is ever re-scanned.
2. **Direction and 1-D fit.** For a pixel set, fit the plane `c = a + b·x + d·y` per channel from the moments (3×3 normal equations; degenerate when n < 3 or the pixels are collinear: no ramp). The ramp direction `u` is the unit principal eigenvector of `Σ_c g_c g_cᵀ`, `g_c = (b, d)` per channel. Along `t = u·p` fit `c = a' + b'·t` per channel from the same moments (Σt = uₓΣx + u_yΣy, Σt² = uₓ²Σx² + 2uₓu_yΣxy + u_y²Σy², Σct = uₓΣcx + u_yΣcy). `rms` is the root of the total residual over n. `slope` is |b'| summed in quadrature over channels, per pixel.
3. **Candidates.** Every adjacent pair of groups whose two sides both have at least `minPixels = 16` interior pixels, ordered by the sRGB distance between the groups' mean colours ascending, then by group index. A pair merges when, for the union: `rms ≤ tol`; the mean boundary step `Σ|Δc| / pairs ≤ max(3·slope, tol)`; and the sides follow the ramp: every side whose own fit is defined and whose extent along the ramp would change its colour by at least `minSpan = 0.0136` (two counts per channel) under the union's slope must itself carry at least half that slope, and at least one side must be such a voter. Two flats across a soft step, anti-aliased or not, fail the last gate; a thin band of a true ramp carries the ramp's slope. Adjacency is kept incrementally, so a group absorbs any number of neighbours in one pass; passes repeat until one merges nothing. A merge is refused when it would push the number of new labels past 256 (merged groups are counted during the passes; lone ramps, §2 step 4, are labelled afterwards from whatever is left).
4. **Ramp or flat.** After the rounds every group (merged, or a lone band with at least `minPixels` interior pixels, since a shallow ramp can quantise to one band) is fitted; a group whose ramp is shorter than `minLength = 16` px along its direction or changes colour by less than `max(tol, minSpan)` end to end stays flat (a merged one keeps a fresh label with its mean as palette entry); every other such group gets a ramp: the end points are the group's extreme projections on `u` through the centroid, the stops are the 1-D fit at those points clamped to 0..255, offsets 0 and 1. A group that never merged keeps its old label and no ramp.
5. **Relabel.** Unmerged components keep their old label; merged groups take fresh labels; labels are then compacted by first appearance in scan order, which fixes `Ramps`, `N` and `OldToNew`.

## 3. Second and Blend

Inside a merged group every pixel's `Blend` becomes 255 and `Second` becomes its own label: the group's internal band boundaries are gone, and the bands' anti-aliasing against each other is noise to the mask. Elsewhere `Second` maps through `OldToNew`; when the old second label has no unmerged pixels left (`-1`) the pixel gets `Second = Labels`, `Blend = 255`.

## 4. SVG

`internal/svg` gains

```go
type LinearGradient struct {
    ID             string
    X1, Y1, X2, Y2 float64
    Stops          []Stop // Offset 0..1, Colour "#rrggbb"
}
type Document struct {
    Width, Height int
    Gradients     []LinearGradient
    Paths         []Path
}
func WriteDocument(w io.Writer, d Document, prec int) error
```

`Write` becomes `WriteDocument` with no gradients and emits the same bytes as today. With gradients the document is

```
<svg …>
<defs><linearGradient id="g0" gradientUnits="userSpaceOnUse" x1=".." y1=".." x2=".." y2=".."><stop offset="0" stop-color="#…"/><stop offset="1" stop-color="#…"/></linearGradient>…</defs>
<path fill="url(#g0)" d="…"/>
```

`Result` gains `Gradients []svg.LinearGradient`; `Finish` assigns ids `g0, g1, …` in the order palette indices are first emitted; `WriteSVG` writes the document. Stacked paths still merge consecutive subpaths by equal `Fill`, which holds for `url(#gN)`. The white-page rule never omits a layer whose palette index has a ramp.

## 5. Fixture `gradient-shapes` (256×256, in `corpus.All()` and `corpus.Small()`)

White page. A disc (centre 72,128, r 56) with a vertical ramp from blue (30,60,220) at its top to yellow (240,200,30) at its bottom; a rounded rectangle (x 144–240, y 24–120, r 12) with a diagonal ramp from red (220,30,30) at its top-left to white at its bottom-right; a flat red rectangle (x 144–240, y 140–232) whose top edge abuts a grey ramp strip (x 144–240, y 128–140, 40 → 200 grey left to right). All edges anti-aliased by the existing `fill` sampler. The ramps are linear in sRGB per channel.

## 6. Options, CLI, API, demo, docs

- `Options.Gradients bool`, `Options.GradientTolerance float64` (0..1; `Validate` rejects outside); every preset: off, 0.03.
- CLI `--gradients`, `--gradient-tolerance`; `apiopts.Params` `Gradients *bool`, `GradientTolerance *float64`; both in `SegmentKeys()` and `web/protocol.js` `SEGMENT_KEYS`.
- Demo: in the Colours group, a checkbox "Smooth gradients" and a "Gradient tolerance" number; both re-segment.
- README usage line and option paragraph, `docs/wasm.md` and `docs/server.md` option lists, `design/roadmap.md` Phase 5 row mention; `bench/results/2026-11-10-gradients.md`.

## 7. Exit tests (Go, in the root package and `internal/gradient`)

- `TestGradientHCollapses`: photo + gradients on `gradient-h`: ≤ 3 paths, ≥ 1 `<linearGradient`.
- `TestGradientShapesFewerPaths`: photo on `gradient-shapes`: paths with gradients ≤ ⅓ of paths without; a path with `fill="#dc1e1e"` (the red rectangle's hex) exists in both.
- `TestGradientsFlatArtUnchanged`: poster, every `corpus.Small()` synthetic but `gradient-h` and `gradient-shapes`: identical bytes on and off.
- `TestGradientsPSNR` (skips without rsvg): on both fixtures PSNR(gradients) ≥ PSNR(flat) − 0.05 dB.
- `TestGradientsDeterministic`: `gradient-shapes` under GOMAXPROCS 1 and 8: identical bytes (Segment is single-threaded; the test guards the contract).
- `TestBirdReport` (`REKHA_BIRD`): prints the numbers; no assertion.
- Goldens: `gradient-h-photo-gradients-spline.svg`, `gradient-shapes-photo-gradients-spline.svg` under `svgClose`.

## 2a. Multi-stop ramps (v2)

The two-stop build merged the bands of a curved ramp (orange → pink → magenta along a feather) only in chunks that a straight line fits, and each chunk's line disagreed with its neighbour's where they met: a seam worse than the band step it replaced. v2 keeps the direction shared and lets the colour along it bend.

- **Gates (replace §2 step 3's first gate).** For a candidate pair, `u` is the principal direction of the union's plane fit. Each side is fitted in one dimension *along `u`* (the same moments, projected on `u`); each side's `rms ≤ tol` (locally linear along the shared direction). `ρ = max(|slope_A|, |slope_B|)` along `u`. A side is measurable when `ρ · extent_side ≥ minSpan`; every measurable side must carry `|slope| ≥ ½ ρ`, and at least one side must be measurable. The boundary step gate becomes `Σ|Δc| / pairs ≤ max(3ρ, tol)`. Everything else (minPixels, incremental passes, label cap) is unchanged. Signs are ignored, so a ramp that rises then falls along `u` merges into one region.
- **Stops (replace §2 step 4's ramp construction).** For a ramp group: `u` from its plane fit, `tmin`/`tmax` over all its pixels; its interior pixels are binned along `t` into `nb = clamp(⌈(tmax − tmin) / 8⌉, 1, 64)` equal bins; each non-empty bin gives a stop at its centre with the bin's mean colour; stops at offsets 0 and 1 are the linear extrapolation of the two nearest stops, clamped to 0..255; the stop list is then simplified by Douglas-Peucker in colour with tolerance `tol / 2`, so an exactly linear ramp keeps two stops (the gradient-h golden is unchanged). A group with fewer than two non-empty bins, an extent under `minLength`, or a colour span (largest distance of any stop from the first) under `max(tol, minSpan)` stays flat.
- **Exit tests.** `TestMergeCurvedRampJoins`: a quadratic grey ramp (`v = 255·(x/w)²`) quantised into 8 bands merges into one region whose ramp has at least three stops and whose stops lie within `tol` of the true curve. `TestRampStopsLinearIsTwo`: an exact linear ramp gives exactly two stops. `TestMergeRiseAndFall`: a ramp bright in the middle merges into one ramp. All v1 tests still hold; flat-art identity tests still hold; the bird shows no seam inside a feather (visual, recorded in the baseline with the new numbers).

## 2b. Cleanup after v2 (v3)

Three artefacts remain at zoom on the bird after v2: a joined group that one bad member dissolves entirely, tiny islands with labels of their own inside a ramp, and a page-coloured sliver where two stacked siblings meet at a sharp tip.

- **Partial dissolve.** When a joined group's sampled model leaves RMS > tol over its interior pixels, the error is also known per first-pass member (the group it was before the join pass). Members whose own RMS against the model is ≤ tol are kept; the others revert to what they were. Kept members that are still adjacent (a component edge between them) form new joined groups; a kept member left alone reverts too. Each new joined group is sampled again with its own direction and model and must pass the same RMS ≤ tol check, else its members revert. One round, no recursion. Labels: after this the number of merged groups is recounted; while it would exceed the budget, new joined groups revert, last by first pixel first.
- **Island absorption.** A component with fewer than `minPixels` interior pixels (so it never merges) that borders a ramp group, and whose mean colour over all its pixels is within tol of that ramp's model at the island's centroid, takes the ramp's label. The ramp group chosen is the bordering one with the most boundary pixel pairs; a tie goes to the lower root index. Islands that border no ramp, or differ by more than tol (a highlight, an eye), keep their label. Flat art has no ramps, so nothing changes there.
- **Stroke in stacked mode.** `Options.Stroke` (`--stroke`, JSON `stroke`, 0..4 px, default 0) strokes every colour shape in its own fill, in both hierarchies; `CutoutStroke`/`--cutout-stroke`/`cutoutStroke` remain as aliases that set the same value. Pixel mode and the bw preset are unchanged. In stacked mode the stroke of a lower shape runs under the shapes above it and closes the sub-pixel gap between two sibling fits at a sharp junction. The demo labels the control "Stroke".
- **Exit tests.** `TestMergePartialDissolveKeepsGoodMembers` (a chain of four linear bands along x plus one band whose colour varies along y, all pairwise joinable: the four stay one ramp, the fifth reverts). `TestMergeIslandAbsorbed` (a 3×3 speck of the ramp's colour +4 counts inside a ramp band takes the ramp's label) and `TestMergeIslandOfOtherColourKept` (+40 counts stays). `TestStrokeStacked` (stacked output carries `stroke=` on colour shapes when `Stroke` > 0, none at 0; `TestGradientsFlatArtUnchanged` still byte-identical at `Stroke` 0). Bird re-run: tail body seam shortened or gone where the body ramp members fit, no labelled specks in the feathers, no page sliver at the tip with `--stroke 0.5`.

## 2c. Radial ramps (2026-11-11)

Bands that run round a centre (a disc shaded from its middle out, a curved band whose iso-lines are arcs) fit no direction. The field a ramp's colour is a function of is now either `t = u·p` (linear) or `t = |p − c|` (radial); everything downstream of the field (binning, stops, the model, its error, island absorption) is shared.

- **Centre.** The gradient moments gain Σk·∇c with `k = x·∂c/∂y − y·∂c/∂x`, so the point every gradient points through is the least-squares solution of `(p − c) × ∇c = 0` from the moments alone. No centre when the system is singular (parallel gradients: a linear ramp) or the point lies more than 8 RMS radii from the pixels' centroid (the iso-lines are straight to the eye). Gradients are accumulated only at pixels whose four neighbours are interior too, so a difference taken against an edge pixel never enters.
- **Where it is tried.** Only where the linear model fails: a joined group whose sampled linear model leaves RMS > tol is re-measured about its centre before it dissolves, and each pairwise re-join in the replay tries the centre after the direction. Lone bands never get a radial ramp.
- **Gates for rings.** In the bend pass the union's slope is also the RMS colour change per pixel over the gradient samples, whichever way each runs (`gslope`), so the boundary step gate and a side's vote in `sidesBend` see a ring's slope where a fit along `u` sees none. The first pass is unchanged.
- **Output.** `Ramp{Radial, CX, CY, R, Stops}` with offsets as distance over `R`; `svg.RadialGradient` and `Document.Radials`; `<radialGradient gradientUnits="userSpaceOnUse" cx cy r>` with four-decimal offsets. Ids share the `gN` sequence with linear gradients.
- **Exit tests.** `TestCentreOfRadialField`, `TestCentreOfLinearFieldIsNone`, `TestMergeRadialRings` (six concentric bands of a disc shaded 0.25 → 0.75 join into one radial ramp with the centre within a pixel and stops within tol), `TestWriteDocumentRadialDefs`, `TestGradientRadialDisc` (end to end: at most two paths, one `<radialGradient>`). Every existing golden is byte-identical.
- **Not solved here.** A region shaded two ways at once (the Firefox mark's globe: an off-centre highlight over a hue drift, drawn in the original as layered translucent radial gradients) fits no single field: the best 1-D model over its pixels leaves RMS 0.085, a 2-D plane 0.083, against tol 0.03, and its quantised bands themselves sit at 0.045–0.07. Letting a model keep the bands' own error as slack gave two smooth regions with a ragged seam between them and broke the partial-dissolve contract, so it was not kept. §2d takes that class of art.

## 2d. Layered ramps (2026-11-11)

A region shaded more than one way is painted the way an illustrator paints it: a base ramp, then translucent radial layers over it. The model is

    c = (1 − α₂)·((1 − α₁)·B(t) + α₁·H₁) + α₂·H₂

with `B(t)` a multi-stop ramp along the group's linear field, and each overlay a flat colour `Hᵢ` whose opacity `αᵢ` is a profile of the distance from its own centre, sampled in `opacityBins` (32) bins and non-increasing outwards. SVG composites exactly this when the region's path is drawn again over itself with a `<radialGradient>` whose stops fade by `stop-opacity`.

- **Fit.** On a fixed-stride sample of at most `layerSamples` (4096) interior pixels (every 4-neighbour a member). The base is sampled from what the overlays leave of each pixel (`B = (c − G)/F`, weighted by `F²`, only where the base shows at least `showThrough` = 0.1). Each overlay is solved by alternating least squares given the rest: opacity per bin, then made non-increasing by weighted pool-adjacent-violators, then its colour; `layerIters` (8) rounds. Overlays are added one at a time while the sample RMS is above `layerGoal` (0.5) × tol and at most `maxOverlays` (2); a new overlay is kept only when it cuts the RMS to `layerGain` (0.85) of what it was.
- **Seeds.** An overlay's centre starts from the residual: where its gradients point through, the centroid of its peak (the top `peakShare` = 0.3 of the samples by residual), that peak mirrored `mirror` = 2 peak offsets beyond the centroid (a rim light's centre lies outside the region), and `ringSeeds` (8) points on a ring 2 RMS radii out. The colour is seeded `seedReach` = 2 residuals past the pixel colour at the peak. Every seed is primed against a flat base, short-fitted, and the best `refineSeeds` (3) refined: coordinate descent on the centre, `refineTrials` (24) moves starting at R/4 and halving, each judged after a half fit.
- **Where it is tried.** Only where both single fields fail: a joined group whose linear model leaves RMS > tol and whose radial model (when it has a centre) does too, before it dissolves. The layered model is accepted when its RMS over every interior pixel is ≤ tol, the same bar as the single fields; nothing is tried at tol 0. Lone bands and the replay's pairwise re-joins never get it (a replay check would cost a fit each).
- **Output.** `Ramp.Overlays []Overlay{CX, CY, R, Stops}`; an overlay's stops carry the opacity in `Colour.A`, offsets as distance over `R`, the profile simplified like a colour ramp at tol/2. `svg.Stop.Opacity` (1 for opaque; written as `stop-opacity` to three decimals when under 1). `Finish` emits the base path as before and then one more `<path>` with the same subpaths per overlay, filled with its `<radialGradient>`; the stroke stays on the base. A layered base path is never shared with a later shape of its colour (it would slip under the overlays).
- **Exit tests.** `TestFitLayeredSphere` (a disc shaded by a ramp along x under a highlight inside it and a rim light centred outside: RMS ≤ 0.015 with two overlays, one on the highlight within 10 px), `TestFitLayeredPlainRampAddsNothing`, `TestLayeredRamp` (the ramp's base is opaque along x, each overlay's stops fade outwards with the layer's colour), `TestMergeLayeredSphere` (the sphere's bands, the highlight's and the rim's cores labelled apart, join as one layered region holding 95 % of the disc), `TestWriteDocumentStopOpacity`, `TestGradientLayeredSphere` (end to end: `stop-opacity`, one or two `<radialGradient>`s, no more paths than the page, the sphere and its overlays). `bentCanvas`'s wandering band now ripples along y so no smooth model fits the hub and the dissolve contract is still exercised. Every existing golden is byte-identical.
- **Cost.** A fit is about half a second on the sample regardless of the group's size, paid once per joined group that no single field reproduces. The Firefox mark goes from 3.1 to 3.8 s; a 775×800 copy from 0.8 to 2.1 s.


## 2e. A ramp's plateau (2026-11-11)

A shading that reaches its end colour and holds it quantises into a ramp band and a flat band with no step between them. The bend pass refused the pair: a flat side wide enough to tell votes against a join, since a flat colour next to a ramp across a soft step is a shape of its own. The letter G's ring met its own dark end at a hard seam.

- **Continuous boundaries.** Each component edge also counts the pixel pairs stepping by at most `softStep` (2/255). A boundary is continuous when at most one pair in `softShare` (10) steps more. In `sidesBend` a flat side no longer refuses across a continuous boundary: the flat is the ramp's end, and the sampled stops hold its level. A flat of another level behind a blurred edge (pairs stepping by more than two counts) is refused as before; the first pass is unchanged.
- **Exit tests.** `TestMergePlateauContinuesRamp` (a grey ramp 0.3 → 0.6 over 128 columns, then 0.6 held: one ramp whose last stop is the plateau), `TestMergePlateauBehindBlurStaysApart` (the same with a six-pixel blend up to 0.75: two labels). Letter G 18 → 16 paths, 36.65 → 36.97 dB, 21.5 → 13.6 kB; the Firefox mark, bird, Pokéball and green mark are unchanged; every golden byte-identical.
- **Still open at the time.** The Firefox body's remaining ragged seams lay on boundaries that are crisp along most of their length and continuous along the rest (7–29 % of pairs within two counts): overlapping translucent shapes in the original, one edge hard and the other a fade. The whole-boundary gate cannot admit those, and joining them would put the hard part inside a smooth model. Section 2f smooths the label boundary through the soft part instead.

## 2f. Seams between gradient regions (2026-11-11)

Where two gradient regions meet across a soft boundary the exact line between them is quantiser noise: both models paint the seam pixels within tolerance, and the label boundary wanders in a staircase of two-to-four-pixel steps that shows once each side is filled with a smooth gradient. Re-assigning each seam pixel to whichever model predicts it better was tried first and rejected: the two models tie across a wide zone, so the front ran 20 000 pixels into the Firefox body and left a dithered fringe where it stopped.

- **Majority vote.** After `Merge` has compacted its labels, `smoothSeams` (`seam.go`) runs over every pixel of a modelled label that has a 4-neighbour of another modelled label within `seamStep` (4/255). The pixel takes the label that holds the majority of the square window round it when that label's model paints it within tol, or no worse than its current label's model by more than `seamSlack` (2/255: where a fading overlay leaves both models a little off, as along the pink base's edge, the seam is still noise); otherwise it stays. A window only straightens steps shorter than its width, so the vote runs coarse to fine, `seamRadii` {32, 5}: the wide window takes a staircase whose steps run tens of pixels (the Firefox body's lower seams, 30–60 px a step), the narrow one cleans the corners it leaves. The window is clipped symmetrically at the image border: clipped on one side, it would let the larger region eat the smaller one along a straight seam, a pixel a sweep. Moves are applied as they are decided, in queue order (two neighbours voted together swap for ever), and each move queues its neighbourhood for the next sweep, at most `seamSweeps` (16) per radius. Crisp edges are untouched because the other side's model misfits there; flat labels have no model and never gain or lose a pixel to the vote.
- **Strands.** A pixel with fewer than `seamStrand` (2) 4-neighbours of its own label is a strand or a lone island, which would become a path of its own (a 1-px island of a layered region costs three paths). It goes to the label most of its 4-neighbours carry, flat or not, without the fit check; peeling continues from the next pixel in, and a last strands-only pass runs until nothing moves, since a capped sweep can leave one cut loose.
- **Rejected on the way.** Per-pixel best-model relabelling (above). A single radius of 5 (the first version shipped locally): it left every seam whose steps outrun the window. A window clipped on one side at the border: the 64-px test canvas showed the larger label marching 16 px into the smaller one.
- **Exit tests.** `TestSmoothSeamsStraightensSoftBoundary` (one ramp cut by a ±3 px staircase on a 64×128 canvas: the boundary straightens to within a pixel over the rows a window away from the border, and no pixel is left without a neighbour of its own label), `…LeavesCrispBoundary`, `…RefusesMisfit`, `…AbsorbsStrandedPixel`, `…AbsorbsStrandIntoFlat`, `…StrandFollowsItsNeighbours`, `…SkipsFlatLabels`, and `TestMergeSmoothsSoftSeam` (two planes folded along a crease, cut by the staircase, at tol 0.01 so the layered model cannot take both: two ramps with a straight seam, no islands). `…StraightensWhenBothModelsMisfit` (both models 10 counts bright: the seam still straightens). Firefox mark 36.33 → 36.33 dB, 43.0 → 33.9 kB, 35 paths, every body seam a smooth curve (a few-pixel notch remains at the red tail's tip, where the window erodes a wedge thinner than itself); letter G, Pokéball and green mark unchanged in paths and PSNR; every golden byte-identical.
- **Cost.** The votes on seam pixels: the Firefox mark traces in 4.5 s as before.

## 2g. Fades in a layered region, and seam pixels the model misfits (2026-11-11)

The red tail of the Firefox mark ends in a seam against the body where the source has none: the dark band runs on into the body's lower rim. The tail's own ramp paints the tail; the body's layered model, fitted to 1.1 million pixels, missed the rim darkening under the tip by 10–15 counts, so the seam showed as the body's misfit meeting the tail's fit. Two changes, both general.

- **Overlays that fade in, and along a line.** An overlay's opacity profile may rise along its parameter instead of falling (`overlay.outward`: pooled non-decreasing, `nonDecreasing`), which paints a vignette or the darkening of a body towards its rim; and the parameter may be the projection on a unit direction instead of the distance from a centre (`overlay.linear`, `ux, uy`), which paints a shadow on one side. Two seeds are added for each: the residual's gradient centre fading either way, the region's centroid fading outwards, and the direction of the residual's planes (`moments.fit`) fading either way. A linear overlay is refined by turning (an eighth of a turn, halved, down to a degree) in place of moving its centre. Emitted as `Overlay{Linear, X0, Y0, X1, Y1, Stops}` on the line through the region's centroid from the parameter's low end to its high end, as a `<linearGradient>` with `stop-opacity`; the radial form is unchanged.
- **Rescue.** Before the votes, `smoothSeams` moves every soft seam pixel its own model misfits beyond tol to the 4-neighbour label whose model paints it best, when that is within tol, and follows the front through the neighbourhood of each move. Unlike the rejected best-model relabelling, the front runs only through pixels painted wrong already (beyond tol), and stops where its own model comes within tol; the window votes then smooth the edge it leaves. On the Firefox mark it carries the tail some 5 000 pixels down the rim where the tail's ramp still fits, and the notch at the tip goes with the wedge.
- **Tried and not kept.** A continuation fill: paint the smaller region with its host's gradients and one linear overlay fading in from the seam, so the seam vanishes by construction. It fits a synthetic case exactly but never fired on a real image: where the host's model misfits the seam the fade cannot start at zero, and where it fits, the seam is already within tolerance. Three overlays per region: Firefox 37.1 dB, but 42 kB and the tip no better than with two; left at two.
- **Exit tests.** `TestFitLayeredVignette` (a disc darkened towards its rim: RMS ≤ 0.012 with an outward overlay about the centre), `TestFitLayeredFade` (a disc darkened towards one edge along the diagonal: RMS ≤ 0.012 by a linear overlay along it or a radial one centred far down it), `TestLayeredRampLinearOverlay` (the overlay's line and its stops' offsets and alpha), `TestSmoothSeamsRescuesMisfitPixels` (a seam drawn 16 px into the steeper of two ramps that meet smoothly moves back to where the gentle model's error passes tol, no island). `TestFitLayeredSphere` now allows the highlight within 15 px: the new seeds change which of two equivalent decompositions wins. Firefox mark 35 → 33 paths, 36.33 → 36.34 dB, 33.9 → 32.7 kB, the error under the tail's tip halved (mean 6.7 → 3.2 counts over the 220×120 box below it); letter G, Pokéball and green mark unchanged; every golden byte-identical.
- **Still open.** The body's model misfits the whole lower rim (its outer band, 226 000 pixels) by more than tol; the tail's seam is now smooth and faint rather than gone. A per-band fit check on a layered union, with the band that fails re-grouped, is the next step; it needs the layered fit in the replay.

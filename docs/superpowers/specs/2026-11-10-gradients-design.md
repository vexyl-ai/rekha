# Gradient fills design

Status: design, 2026-11-10. Follows Phase 4 (open half, merged as 901d4ce) and the web polish (1912e58). The user asked for a gradient option after a bird illustration with smooth feather shading traced into dozens of flat colour bands; design questions were skipped at the user's request, so every choice below is the author's call. Phases 1–4 stay the base: segmentation, cleanup, contour tracing, stacking, the planar map, fitting and refinement are unchanged except where a section below says otherwise.

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
3. **Candidates.** Every adjacent pair of groups whose two sides both have at least `minPixels = 16` pixels, ordered by the sRGB distance between the groups' mean colours ascending, then by the lower first pixel index. A pair merges when, for the union: `rms ≤ tol` and the mean boundary step `Σ|Δc| / pairs ≤ max(3·slope, tol)`. Rounds repeat until a round merges nothing; within a round a group takes part in at most one merge, and a merge is refused when it would push the number of new labels past 256.
4. **Ramp or flat.** After the rounds every group (merged, or a lone band with at least `minPixels` interior pixels, since a shallow ramp can quantise to one band) is fitted; a group whose ramp is shorter than `minLength = 16` px along its direction or changes colour by less than `tol` end to end stays flat (a merged one keeps a fresh label with its mean as palette entry); every other such group gets a ramp: the end points are the group's extreme projections on `u` through the centroid, the stops are the 1-D fit at those points clamped to 0..255, offsets 0 and 1. A group that never merged keeps its old label and no ramp.
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

# Gradient fills baseline — 2026-11-10

Rekha on branch `gradients`, photo preset with and without `--gradients`, scored by `rekha-bench` (rsvg-convert 2.59.2 rasterisation at 1:1, PSNR against the input PNG) on an Apple M-series laptop (arm64). `./bin/rekha-bench -tools rekha-photo,rekha-photo-gradients` over the 17-image synthetic corpus; the bird is the user's 4001×4001 JPEG illustration that prompted the feature (`REKHA_BIRD=8_1sasa11.jpg go test ./ -run TestBirdReport -v`; not in the repo).

Spec: `docs/superpowers/specs/2026-11-10-gradients-design.md`.

## Ramp fixtures

| Image | Tool | PSNR dB | Paths | Nodes | Bytes | ms |
|---|---|---:|---:|---:|---:|---:|
| gradient-h | rekha-photo | 29.16 | 10 | 50 | 544 | 18 |
| gradient-h | rekha-photo-gradients | ∞ (exact) | 1 | 5 | 341 | 16 |
| gradient-shapes | rekha-photo | 32.21 | 20 | 145 | 2344 | 23 |
| gradient-shapes | rekha-photo-gradients | 34.05 | 4 | 24 | 1050 | 20 |

`gradient-h` (a 256-px black-to-white ramp) becomes one rectangle with one two-stop gradient whose stops are exactly #000000 and #ffffff. `gradient-shapes` keeps the flat red rectangle (`#dc1e1e`) and the hard edge against the grey ramp strip; the disc, the rounded rectangle and the strip each become one gradient-filled path. The missing bottom-right corner of the rounded rectangle is the quantiser's (its lightest band joins the white page under both tools).

## Flat art

The other 15 corpus images (aa-abut, big-poster 2048², checkerboard-2px, circle-aa, circle-hard, logo-holes, noisy-disc, poster-discs, rect-rot, rounded-rect, small-glyphs, star-5, thin-lines, three-layer-stack, two-colour-abut) give the same PSNR, paths, nodes and bytes with `--gradients` as without. `TestGradientsFlatArtUnchanged` (poster preset, every 256² synthetic) and `TestGradientsBigPosterUnchanged` (photo preset, 2048²) pin byte identity. The first build turned a 6-px near-white pocket in big-poster into a ramp; a ramp now needs an extent of at least 16 px along its direction. The review found two flats a soft step apart (100 and 104 grey, or a 10/255 step across an anti-aliased slant) merging into one ramp; a merge now also needs each measurable side to carry the ramp's slope inside itself (`TestMergeNearFlatStaysFlat`, `TestMergeAAEdgeRefused`), and a hub region absorbs all its neighbours in one pass (10 404 islands on a 1020² ramp in under a second, `TestMergeHubAbsorbsIslandsInOnePass`).

## The bird

| Options | Paths | Bytes | ms | PSNR dB |
|---|---:|---:|---:|---:|
| photo | 155 | 86 216 | 5 876 | 36.72 |
| photo --gradients (two-stop, v1) | 203 | 48 396 | 8 073 | 38.72 |
| photo --gradients (multi-stop, v2) | 201 | 47 234 | 10 148 | 39.42 |

v1: 23 gradients. The banded feathers become smooth ramps (see the spec's intent); bytes drop by 44 % and PSNR rises 2.0 dB. The `<path>` count rises because a region with a ramp of its own carries a label of its own, so consecutive stacked shapes that used to share one `<path>` by colour are now separate elements; nodes are what the paths cost, and the byte count shows the net effect. Gradient merging adds about 2 s on 16 megapixels (components, moments and the merge passes are single-threaded).

### Multi-stop ramps (v2, same day)

v1 merged a curved feather (orange → pink → magenta) only in the chunks a straight line fits, and the chunks met at a seam worse than the band step. v2 keeps the linear pass, then joins its groups across bends along a shared direction (the least-squares planes' direction while it fits within tol, else the principal axis of the colour-gradient structure tensor, sign-free so a ramp that rises then falls has a direction; per-pixel gradients of tol or more are left out so outlines do not vote), samples the colour in 8-px bins along it, simplifies the samples by Douglas-Peucker at tol/2 (an exactly linear ramp keeps two stops; both gradient goldens are byte-identical), and dissolves any joined group whose samples leave an RMS error above tol back into its linear groups. On the bird: 21 gradients with 63 stops, PSNR up a further 0.70 dB and bytes down 2 %; the pink wing feather is one smooth ramp with no seam. The torn seam across the tail body remains: that region's colour changes in two directions at once (the body's ramp crossed by a shadow band), which no single `<linearGradient>` holds, and the join that tried it was dissolved. The 15 flat-art images are still byte-identical. Merging now costs about 4 s on 16 megapixels.

## Verdict on the photo default

`--gradients` never lowers PSNR or raises nodes on the corpus, and flat art is byte-identical. It stays opt-in for this release because the corpus has no photographs: a JPEG with blocking noise is the case the merge rule (interior-pixel residual ≤ 0.03, boundary step ≤ max(3·slope, 0.03), both sides carrying the slope) has not been measured on. Flip the `photo` preset once a photo corpus exists and shows no regression.

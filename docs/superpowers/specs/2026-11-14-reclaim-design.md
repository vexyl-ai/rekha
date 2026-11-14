# Reclaimed shading design

Shading the quantiser folded into its surroundings gets a colour of its own after labelling, when it is a solid blob of pixels far from the colour it was given.

## What the measurements said

The boy render (1200×2198, denoised copy), `--preset poster --color-tolerance 0.04 --gradients --refine`: 33.15 dB over the image, 26.98 dB over the chin box (440,380)–(740,620). Under `--preset photo --gradients --refine`: 31.62 / 25.18. The residual in the box is the outline anti-aliasing and one band: the neck under the chin.

OKLab lightness down column x = 630 from y = 515: skin 0.86–0.87, the chin outline 0.05–0.16 over 18 px, then the neck at 0.78–0.81 for 20 px, a 4 px rise, skin 0.87 again. Columns 600 and 660 read the same with the band 24 and 20 px tall. The band's mean is sRGB (225, 157, 120), OKLab L 0.754 a 0.063 b 0.073, against skin (254, 197, 154), L 0.865 a 0.045 b 0.075: 0.113 apart in OKLab, nearly three times the 0.04 tolerance; a few thousand pixels in all.

So the shadow is neither soft nor sub-tolerance, as the flat-lightness bench had it. It is small: `quant.Wu` keeps a far colour group only above `groupFloor`, a 512th of the image (5150 px here), and below that it is noise. `region.Rescue` cannot see it either: Rescue judges whole components of a label, and the band is part of the face component, whose mean is skin.

The upper-cheek strip (grey along the left jaw) is a separate defect: a rim light that is its own component and took a grey entry within tolerance of its mean. Not this design's.

## Intent

A region whose own colour the quantiser never granted should still come out in that colour when it is a solid patch, not grain. The same goes for any small shaded area in a poster or photo trace: a shadow under a collar, a badge's inner disc, a cheek's blush, when the patch is thick enough to be a shape and far enough from its label's colour to be another colour.

## Goals

- Under the colour presets, a blob of pixels inside a labelled component, each pixel farther than the colour tolerance from the label's colour, at least `reclaimMinArea` = 64 px, thick enough to have an interior, and coherent around its mean, gets the nearest existing palette entry within tolerance of its mean, or a new entry at its mean.
- The boy's neck shadow comes back as a band of its own under poster 0.04 and photo; the chin box gains over 1 dB with a handful of paths.
- Nothing changes on the 21-image bench beyond that kind of gain: an image without such a blob traces byte-identically.
- Fixed palettes are never extended; the flat preset's merge rule still applies afterwards.

## Non-goals

- Soft shading below the tolerance (a true few-hundredths ramp): that is `--gradients`' job, and the layered fit's `layerGoal` gate is unchanged.
- The cheek strip's grey (a Rescue entry choice, separate).
- Changing `groupFloor` in the Wu quantiser.

## Anti-goals

- Anti-aliased edges reclaimed as bands. An edge's blend pixels are far from both colours and connect along the whole outline into a long strand; a strand has no interior and must never qualify.
- Grain. A field of noise has far pixels by the thousand, but no blob of them with an interior.
- Palette growth on clean images.

## Constraints (carried)

Go 1.26, `CGO_ENABLED=0`, only `golang.org/x/image`; pixel and polygon goldens byte-identical unless a change is justified in the bench file; refined goldens by quality compare; keyed `geom.Vec2` literals; `float64()` wrapped products where arm64 and amd64 must agree (none here: OKLab distances are compared, not accumulated across platforms in a way the goldens see... the means are, so `meanLab` is reused as is).

## Approaches

**A. Lower the Wu group floor** (a 2048th of the image). Histogram-blind: `2026-11-14-wu-groups.md` set the floor where grain groups stop qualifying, and a lower floor spends entries on grain in every noisy image. Rejected.

**B. Reclaim blobs inside components, in `internal/region`** (chosen). After Rescue, with the same palette rules. Spatial by construction: a blob must be thick, so neither edges nor grain qualify, and a clean image sees no candidate at all. Reuses `meanLab`, `coherent` and the palette-append logic that Rescue already has and that the bench accepted.

**C. Local-residual overlays in the layered gradient fit.** Would only act under `--gradients`, would paint a hard-edged plateau with a radial fall-off, and leaves the plain presets as they are. Rejected.

## Design

`internal/region/reclaim.go`:

```go
const (
    reclaimMinArea   = 64      // pixels a blob needs
    reclaimInterior  = 0.5     // share of a blob's pixels with all four neighbours in it: four pixels thick
    reclaimCoherence = 0.5     // share of a blob within tol of its mean
    reclaimMax       = 32      // entries one Reclaim adds
)

// Reclaim gives a palette entry to a blob the quantiser folded into its
// surroundings: ...
func Reclaim(labels, rgb []uint8, w, h int, palette []colour.OKLab, tol float64) []colour.OKLab
```

1. **Far pixels.** `far[p]` is true when `colour.Dist(pixel, palette[labels[p]]) > tol`. One pass.
2. **Blobs.** 4-connected components of far pixels sharing a label (flood fill over `far`, label equality as the edge test). A blob below `reclaimMinArea` pixels is skipped.
3. **Thickness.** A blob's interior count is its pixels whose four in-image neighbours are all in the blob (same test as `gradient`'s `interior`, restricted to the blob). A blob whose interior is under `reclaimInterior` of its pixels is a strand or a ring of edge pixels and is skipped. A 3-px-wide strip has a third, a 4-px one a half, the neck band nine tenths.
4. **Colour.** `m = meanLab(blob)`. The blob is skipped unless `coherent` holds at `reclaimCoherence`: half the blob within tol of `m`. A plateau passes; a blur from one colour to another far away spans a ramp of which only `2·tol / span` fits.
5. **Entry.** As Rescue: the nearest existing entry within tol of `m` takes the blob; otherwise the blob is a candidate with gain `pixels × d²`, candidates by descending gain while gain ≥ `reclaimMinArea × (2 tol)²`, at most `reclaimMax` new entries and 256 in all, a candidate within tol of an entry added in this pass shares it. `compact` afterwards.
6. **Placement.** `segmentColour`, directly after `region.Rescue`, inside the same `len(o.Palette) == 0` guard, before Flatten. Rescue first, so a whole component that is far takes its entry as one piece and Reclaim sees it as no longer far.

Flatten afterwards may fold the reclaimed shade back when it is within `FlatLightness` of its parent: the neck shadow (ΔL 0.11) stays under the default 0.12 only just, and the design accepts whatever Flatten rules. Gradients afterwards see two bands and join them only under their own ramp gates; a plateau behind a 4 px rise is a hard step there and stays a band.

## Exit tests

`internal/region/reclaim_test.go`:

- `TestReclaimBlobInsideComponent`: a 64×64 field of A with a 20×12 patch of B at 2.5 tol from A, labelled A throughout, palette {A}: the patch is labelled 1, palette {A, ≈B}.
- `TestReclaimTakesExistingEntry`: the same with C within tol of B already in the palette and unused: the patch takes C's index, palette unchanged in length.
- `TestReclaimIgnoresStrand`: a 2-px-wide line of B, 200 px long: nothing changes.
- `TestReclaimIgnoresRamp`: a 20×12 patch running linearly from A to a colour 6 tol away: untouched (half of it is not within tol of the mean).
- `TestReclaimIgnoresSmall`: an 8×6 patch of B: untouched.
- `TestReclaimCapsEntries`: 40 patches of distinct colours: at most 32 new entries.

Root package: `TestSegmentReclaimsShadow` (a synthetic 128×128: a skin field, an ink line, a 100×20 band at 2.5 tol under it, labelled by the poster preset: the band is its own palette entry and its pixels carry it). Goldens: run the full suite; any change is reported in the bench file with its PSNR.

Bench `bench/results/2026-11-14-reclaim.md`: the 21 images, plain and refined, before and after, plus the boy's chin box PSNR under poster 0.04 and photo.

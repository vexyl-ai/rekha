# Palette rescue, and the soft-band pass that was not built

Date: 2026-11-14. Status: rescue built; the soft-band pass investigated
and dropped (see "What the measurements said").

## Intent

Two label artefacts seen on an AI-rendered cartoon (the "boy" bench
image, cleaned at 1×, traced by `--auto`: photo preset, tolerance 0.04,
154 colours) that Vectorizer.ai does not show:

1. A 3–4 px band along the lower edge of an eyebrow carries a mid-brown
   palette entry of its own. The source edge is soft (the render and the
   clean-up blur it over several pixels), so the pixels between brow and
   skin are a mixture of the two, nearest to a third palette colour that
   the hair uses legitimately. The sliver pass in `internal/region` only
   absorbs bands thinner than 1.5 px, the width of a rasteriser's
   anti-aliasing; this one is too thick for it.
2. The light-blue interior of a small badge is labelled a light grey
   (palette (174,169,166), source mean (127,149,166) over 198 px). No
   palette entry is within tolerance of the badge's colour: at a few
   hundred pixels in 2.6 MP it is below every rule in the Wu quantiser.

The second causes a wrong colour, the first a wrong shape. Both are
fixed in the label map, after `region.Cleanup`, with no new options.

## Goals

- The eyebrow band joins brow and skin pixel by pixel (each to the
  nearer colour); the badge interior gets its own palette entry at its
  mean colour.
- A real thin stripe between two colours whose colour lies between them
  (a grey line on a black-and-white page) is kept.
- No change to flat art: the 21-image bench moves by less than 0.1 dB
  on images without soft edges; pixel goldens stay byte-identical where
  the image has no soft band or missing colour.
- Fixed palettes (`Options.Palette`) are never extended.

## What the measurements said

Written after the design, before the build; it changed what was built.

- The eyebrow band is not there. At 8× the brow renders within the
  edge anti-aliasing of the source, with and without gradients; the
  mid-brown "band" was the brow's own colour (64,37,22) seen beside its
  darker outline. The visible face defects are the badge, a light strip
  down the left cheek (a rim light from cream to white, 118 and 174 px,
  labelled the eye-white (241,240,239)) and a grey-brown patch in the
  hair whose label is 0.017 from its pixels, within the tolerance asked
  for. The first two are rescue cases.
- Soft bands are mostly not artefacts. Counting components 1.5–4 px
  thick whose mean lies within tolerance of their two longest
  neighbours' mixture: 1494 on the boy at 1× (224 k px), 2576 at 2×
  (1.17 M px), 1634 on tom-jerry at 2×. The largest are the black
  outline strokes (near-black entries a few levels apart, which OKLab
  holds far apart) and 2.6 px shading bands inside fur and cloth
  gradients, where the band is the quantiser's step of the ramp and
  absorbing it coarsens the ramp. Telling an anti-aliased edge between
  two flat regions from a step of a ramp needs to know whether the
  neighbours are flat, which is the gradient merge's job, not the
  cleanup's. No pass was built; the sliver limit stays at 1.5 px.

## Non-goals

- Changing the Wu quantiser or its tolerance rules.
- A posterised preset or new primitives (items 3 and 4, separate work).

## Approaches considered

- **Raise the sliver thickness limit** to 4 px with the existing colour
  test. Simplest; loses every 2–4 px stripe whose colour happens to lie
  between its neighbours, which is exactly a grey rule between black and
  white. Rejected.
- **Soft-band test: colour follows position** (chosen). A soft edge is
  a mixture whose share changes across the band: pixels next to colour
  A are mostly A, pixels next to B mostly B. A flat stripe has the same
  colour on both sides. Regressing each pixel's mixture share on its
  position across the band separates the two with one number.
- **Rescue in the quantiser** (a pass over the image for pixel groups
  far from every palette entry). More general but blind to connectivity:
  noise pixels far from the palette are as eligible as the badge. The
  component pass has the area and the shape for free. Chosen for item 2.

## Design

### Soft bands

Not built; see above. The design was: a second branch of the sliver
pass for 1.5–4 px bands whose mean lies within `tol` of the neighbours'
sRGB mixture, admitted when the mixture share rises across the band
(the interior pixels nearer a and nearer b differ by at least `tol/2`
in the right direction), so a flat stripe is kept.

### Palette rescue (`internal/region/rescue.go`)

`Rescue(labels, rgb []uint8, w, h int, palette []colour.OKLab, minArea
int, tol float64) []colour.OKLab` runs once after `Cleanup`, over the
components of the cleaned map:

- candidates: area ≥ max(minArea, 16) and the mean of the pixels'
  OKLab colours `m` (what a k-means centroid is; the OKLab of the sRGB
  mean lands a dark band nearer the next entry than its own) farther
  than `tol` from the component's own palette colour, and either `m` or
  no entry fits the component: an entry fits when a third of the pixels
  lie within `tol` of it. Anti-aliased glyph strokes are black pixels
  with greys among them, whose mean is a dark grey none of them has
  while black fits most, and keep black; the cheek rim runs from cream
  to white, fits neither its label nor its mean, and takes the mean,
  the colour nearest all of its pixels;
- if some existing palette entry is within `tol` of `m`, the component
  is relabelled to the nearest such entry;
- otherwise it is ranked by gain `area × Dist(m, own)²`; candidates
  are taken in descending gain while the gain is at least
  `16 × (2·tol)²` (sixteen pixels at twice the tolerance), at most
  `rescueMax = 32` new entries and 256 palette entries in all. A
  candidate within `tol` of an entry added earlier in this pass shares
  it; otherwise `m` is appended and the component relabelled.

`segmentColour` calls `Rescue` after `Cleanup` when no fixed palette
was given, before the second `Map`, so `Second` and `Blend` see the
new colours and the ramp merge works on the rescued labels.

### Tests

Region tests on synthetic maps (`rescue_test.go`): a 10×10 blue square
inside a black outline on a grey label with no blue in the palette gets
a new entry; a 3×3 one does not; a square within tolerance of an
existing entry moves to it; two squares share one entry; a
checkerboard square (mean a colour no pixel has, label fitting half)
keeps its label; a graded square (no colour fits a third) takes its
mean; nil pixels leave the palette alone. `segment_test.go`:
`Options.Palette` keeps exactly its entries.

### Bench

The boy image (badge, cheek) and the 21-image bench, plain and
refined, against `main`; results in
`bench/results/2026-11-14-rescue.md`.

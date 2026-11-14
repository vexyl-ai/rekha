# The flat preset

Date: 2026-11-14. Status: approved (item 3 of the quantisation list; the
user asked for the work without further questions).

## Intent

A posterised, flat-colour trace in the style of Vectorizer.ai's default:
every drawn region one colour of the right hue, shading collapsed into
it, around twenty colours on a cartoon, clean shapes. The existing
presets do not give it: `poster` at 0.08 keeps shading bands, and
raising the tolerance makes the Wu quantiser merge hues before it
merges shades (at 0.10 the boy's skin goes grey and his hair blue-grey;
at 0.06 his brown iris is already a dark grey and the brown strap part
of the olive hoodie), because OKLab's Euclidean distance weighs a
lightness step as much as a hue step and most of a cartoon's shading
is lightness.

## Goals

- `--preset flat` (`PresetFlat`, API and wasm `"flat"`): on the boy
  render, the iris brown, the strap brown, the badge blue, shading
  merged into each region's colour, about twenty fills.
- The behaviour is an option, `Options.Flatten` (`--flatten`, API
  `flatten`), so any colour preset can ask for it; the preset turns it
  on.
- A fixed palette is never flattened.
- Other presets and every golden are unchanged.

## Non-goals

- Chroma weighting in the quantiser itself.
- Edge or shape changes; `--refine` applies as before.

## Approaches considered

- **Higher tolerance.** Merges hues first. Rejected (measured above).
- **Chroma-weighted distance** throughout `colour`, `quant`, `region`,
  `gradient`. Touches every golden and the gradient fits for one look.
  Rejected for now.
- **Quantise fine, then merge same-hue lightness steps** (chosen). The
  0.04 segmentation keeps every hue; a pass over the label adjacency
  then joins neighbouring colours that differ in lightness only. The
  merge is by hue, which is what the quantiser's distance under-weighs,
  so the two passes cover each other's blind spot. Prototype on the
  boy: 20 colours, 331 paths, iris and strap right, at 28.6 dB (the
  poster at 0.06 scores 32.4 with 26 colours, keeping the shading the
  flat look drops).

## Design

`region.Flatten(labels, rgb []uint8, w, h int, palette []colour.OKLab)
[]colour.OKLab`, after `Rescue`, when `Options.Flatten` is set and no
fixed palette is given:

- per label, the pixel count and the sum of the pixels' OKLab colours;
  per pair of labels, the length of their shared 4-connected border;
- the adjacent pairs are visited in ascending OKLab distance between
  their palette colours (ties by label index), with union–find over
  labels. Two groups merge when their colours (each group's colour is
  its largest member's, below) are compatible: lightness within
  `flatLight = 0.12`, and either both neutral (chroma `hypot(A, B)`
  under `flatNeutral = 0.02`) or both chromatic with hue angles within
  `flatHue = 0.4` rad (23°). A neutral and a chromatic colour never
  merge (outlines stay off the hair, eye whites off the skin), and a
  chain of steps is judged against the group's colour, so it stops when
  the next step is too far from the band that anchors the group;
- each label takes its group's colour, that of the group's largest
  member (the mean of that label's pixels): the smaller is the shading
  of the larger, and a logo's brand red must not drift when its
  shade joins it (the first cut used the pixel mean, which put the
  Pokéball's red between its two reds; the dB cost there is the same
  either way, since it is the merge itself that costs, but the brand
  colour is now exact); the palette is compacted.

`PresetFlat`: the poster settings with `ColorTolerance 0.04`,
`FilterSpeckle 16`, `Flatten true`, `Hierarchical stacked`. `Validate`
admits it. `apiopts.Params.Flatten`, `--flatten`, the demo page's
preset list and table, `docs/wasm.md`, the README preset list and the
CLI help name it. One golden, `gradient-shapes-flat-spline`.

### Tests

Region: two adjacent bands of one hue a lightness step apart merge to
the larger band's colour; two adjacent colours a hue apart at one
lightness stay; a neutral next to a chromatic colour stays; three
same-hue steps of 0.1 L each merge the first two and leave the third
(the chain guard); two same-hue labels that do not touch stay apart.
Root: `PresetOptions(PresetFlat)` fields and `Validate`; `Segment` of
a two-band image under the flat preset has one colour; a fixed palette
under `Flatten` keeps its entries. apiopts and CLI: `flatten` parses.

### Bench

Boy and the 21 images: `flat --refine` against `poster 0.06 --refine`,
PSNR, paths and distinct fills, in `bench/results/2026-11-14-flat.md`.
A flat look costs PSNR by design; the table shows what it buys in
fills and paths.

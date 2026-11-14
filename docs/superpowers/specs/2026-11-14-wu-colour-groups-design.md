# Wu quantiser: colour groups for hidden subjects

## Intent

The poster preset lost whole objects on a cartoon character: the skin
merged into the wall, the orange shoes and the white trims vanished.
`photo` kept them at twice the paths and `--auto` rescued the image at
three times. Buyers judge a tracer on exactly this: right colours with
few paths. Fix the quantiser so small shaded objects survive at the
poster tolerance.

## Cause

`quant.Wu` stops splitting a box when its RMS radius is under the
tolerance, then looks for a hidden subject among the far cells. Two rules
decide, both on single cells: the far pile is coherent when one cell
holds 25 % of it; a tail or mixture cell is promoted when it alone holds
1/64 of the image and stands apart from its neighbours. A shaded object
spreads over many cells and two small objects in the same box dilute
each other, so neither rule fires. On the character the wall box held
the white trims (0.7 % of the image, heaviest cell 21 % of them) and
the shoes, declared itself done, and the skin never came up for a split.

## Change

In `within`, before the single-cell rules, group a pile's cells: sort by
weight, seed a group at the heaviest unassigned cell, take every
unassigned cell within the tolerance of the seed, repeat; the pile's
group is the heaviest one. Then:

- far pile: split when the single-cell rule fires OR the heaviest group
  holds `coherentShare` of the pile with at least `coherentMin` weight;
- tail and mixture piles: promote when the single-cell rule fires OR the
  heaviest group holds 1/`subjectShare` of the image and the halo test
  (the box's cells near the seed that are not in the group hold under
  1/`haloShare` of the group) passes; promote the group's seed cell;
- a group counts only when it holds at least 1/`groupFloor` of the image,
  `groupFloor = 512`: gradient tails of a few hundred pixels on a
  16-megapixel logo must not force splits.

Distance is plain OKLab. No chroma weighting: it amplifies noise
(noisy-disc 29.7 → 26.2 dB, mr-bean, unicorn and the Q ribbon all lost).
Tolerances, presets, k-means and cleanup are untouched.

## Evidence (spike, 2026-11-14)

Poster 0.06 with gradients and refine, 21 images, PSNR over white:
character 25.34 → 30.09 dB at fewer paths (567 → 527), cat-dog 21.99 →
26.76 (59 → 30 paths), tom-jerry 25.78 → 27.18, boy-render 27.72 →
28.57; 15 images unchanged to the byte; no losses. Plain poster 0.08:
mean +1.2 dB, no losses.

## Goals, non-goals, anti-goals

- Goal: small shaded objects at the poster tolerance keep a palette entry.
- Goal: images whose palettes were right stay byte-identical.
- Non-goal: skin-versus-wall separation by a perceptual distance (the
  grouping already frees the box to split).
- Anti-goal: more palette entries on noise or on gradient ramps. The
  noisy disc and the Q ribbon must not change.

## Approaches considered

- Lower the poster tolerance: fixes skin, not the shoes (needs 0.04),
  costs paths everywhere. No.
- Chroma weighting in the box radius: noise amplifier. No.
- Palette-first `--auto` candidate: leaves `preset=poster` broken. No.
- Colour groups with a size floor (this): no losses on the corpus. Yes.

## Left to the builder

Group search is O(n²) over a pile's cells; piles are small in practice
(hundreds of cells), and a far pile over a few thousand cells may be
capped at the heaviest 4096. Goldens whose palettes change are
re-baselined with the bench numbers as justification.

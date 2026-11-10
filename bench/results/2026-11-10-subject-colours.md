# Flat subject colours inside a within-tolerance box (2026-11-10)

Found on a 2048×2048 Pokéball logo (poster preset; not in the repository):
the soft grey shadow crescent under the ball, 2.2 % of the canvas at
`#dfdfdf`, came out white. Two changes.

**Wu: a heavy, isolated cell is a palette entry of its own.** The grey lay
in the white box, which was within tolerance by RMS (the page dwarfs it)
and explained the grey as a black–white mixture cell. `within` now piles
the box's explained cells (mixture per other centroid, and the noise tail)
and takes the heaviest cell of a pile as a *subject* when it holds at
least 1/64 of the image and its halo (the box's other cells within two
cells of it in sRGB) holds under 1/16 of its weight. A subject is
promoted straight to the palette and its box marked done; splitting the
box by variance instead cut the edge's mixture cells into boxes of their
own first (junk anti-aliasing entries), which is what the first attempt
did.

The halo test is what keeps a ramp whole. A letter-G logo has two flat
cyans 0.048 apart joined by a ramp; the plateau cell carries 16.5 % of
its mass in the neighbouring cells, so it stays in its box and the
gradient fit covers both (cutting it out ran a region boundary through
the ramp and cost 1.1 dB on the refined output). The Pokéball grey's halo
is 0.2 %.

**Cleanup: a sliver's pixels go to the nearer neighbour each.** With the
anti-aliasing entries gone, the edge between two flat colours is one
sliver component; absorbing it whole into the longest-border neighbour
moved the edge by a pixel. Each pixel now goes to whichever of the two
neighbours it is nearer in OKLab. Specks keep the wholesale rule: a
first version split specks too, and the orphans it left became specks of
their own (66 → 79 paths on a photo).

Poster preset unless noted; PSNR over white; "before" is the previous
commit.

| image | paths before → after | PSNR before → after |
|---|---|---|
| Pokéball | 4 → 5 | 31.82 → 37.79 dB |
| Pokéball `--refine` | 4 → 5 | 32.91 → 41.29 dB |
| letter G | 25 → 25 | 34.59 → 35.26 dB |
| letter G `photo --gradients` | 21 → 21 | 36.10 → 36.17 dB |
| letter G `photo --gradients --refine` | 21 → 21 | 40.43 → 40.92 dB |
| geometric | 12 → 12 | 35.91 → 36.05 dB |
| Firefox `photo --gradients` | 34 → 34 | 34.42 → 34.48 dB |
| bird photo `photo --gradients` | 66 → 66 | 39.67 → 39.70 dB |

Goldens that changed, all measured against their fixture:
`poster-discs-poster-{pixel,polygon,spline}` 31.4/31.3/32.3 → 31.9/31.8/32.8 dB,
`poster-discs-refine-spline` 40.77 → 42.03 dB,
`gradient-shapes-poster-{pixel,polygon,spline}` +0.02 dB each, and
`aa-abut-refine-spline`, which the refine border fix had changed
(52.42 → 52.44 dB) and which is compared loosely, so it had not been
regenerated then.

**Still open on the Pokéball:** the right horizontal seam has a one-pixel
notch and the inner button ring's thickness is uneven; both are fit
questions, not colour ones.

# Lone bands get the slack and the retries (2026-11-19)

`sample` in `internal/gradient`: a lone band (a region no neighbour
joined) is judged like a joined group: its linear fit within its slack
(`slackTol`), else about its centre, else as a layered ramp. Spec §2i.

Image: the white wolf of `2026-11-19-slack.md`, 1024², JPEG. Scored
against the source with `rsvg-convert` at 1:1, PSNR over white.

## The problem

After §2h the cheek and the thin fur strands still rendered as flat
patches with hard edges. A label dump of the photo+gradients trace:
each was a lone band whose shading ran two ways inside it, with a
linear residual of 0.032–0.046 (interior pixels), between `tol` 0.03
and the band's slack. A lone band became a ramp only within `tol`.

| lone band (label/component) | pixels | flat RMS | linear RMS | slack |
|---|---:|---:|---:|---:|
| cheek strand 4/136 | 4 299 | 0.047 | 0.045 | 0.060 |
| brow 1/64 | 2 001 | 0.055 | 0.038 | 0.060 |
| strand 16/61 | 1 684 | 0.038 | 0.032 | 0.060 |
| strand 5/76 | 1 504 | 0.046 | 0.045 | 0.060 |
| mid-grey mass 1/38 | 81 303 | 0.041 | 0.041 | 0.038 |

A geodesic field (colour by distance along the band from its far end,
the "skeleton field" §2h left open) explained no more than the linear
fit on any of them: 0.041 → 0.037 on the mass, 0.045 → 0.032 on the
cheek strand. The shading is two-dimensional, not along the strand.

## The fix

photo + gradients, speckle 2, no refine:

| image | before dB | after dB | gradients | bytes |
|---|---:|---:|---|---|
| wolf | 29.10 | 29.26 | 44+2 → 107+4 | 221 258 → 243 952 |
| nine other logos | — | — | byte-identical | byte-identical |

`--auto` (what the app runs): 31.29 → 31.59 dB, 46 → 127 gradients,
486 → 365 kB (a ramp a band costs fewer paths than the steps it
replaces). No stop off the wolf's red-and-grey hull (every stop and
fill checked by hue). Photo trace 2.6 → 3.0 s; the 16-megapixel logo
12.4 s unchanged. Every golden byte-identical.

Steps on the way: slack on the lone band's `fitWithin` alone did
nothing, since above `tol` `fitWithin` takes the tensor's direction,
which fit the test patch at 0.13 against the planes' 0.041; the planes'
direction is what a lone band is judged on. The lone retry (centre,
then layered) added 3 ramps on the wolf and 0.03 dB; it is what gives
the sphere of `layer_test.go` its overlays as one label.

## Still open

The mid-grey mass across the face (81 000 pixels) misses its slack by
0.003 and the layered fit too, and stays flat with a hard edge against
the white it blends into. Its error is a feathered rim, a function of
the distance to its boundary. White flecks along the strands' edges
are islands of the face's label inside the grey.

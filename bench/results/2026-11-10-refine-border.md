# Refinement at the image border (2026-11-10)

Two fixes found on a 1984×2048 transparent-background logo with wide,
smooth radial gradients (a Firefox mark; not in the repository).

**Handles clamped to the image.** `refine.Refine` clamped every free
control point to within a pixel of the image before optimising, and each
piece's trust box did the same. The plain fit of the mark's bottom arc had
two handles 76 px below the canvas (a long arc hugging the border puts its
handles along the tangents, well outside), so clamping bent the arc away
from the edge before any optimisation ran, and the 2 px trust region could
not bring it back. The refined edge sat 30 px too high. Now only anchors
are clamped; a moved anchor carries its handles.

| `--preset photo` | paths | PSNR before | PSNR after |
|---|---|---|---|
| `--gradients` | 35 | 34.42 dB | 34.42 dB |
| `--gradients --refine` | 35 | 27.04 dB | 35.56 dB |
| flat | 47 | 31.26 dB | 31.26 dB |
| flat `--refine` | 47 | 26.75 dB | 31.78 dB |

The three other logos in the same batch (opaque, flat colour, poster
preset, `--refine`) are unchanged at 32.91, 36.71 and 35.65 dB, as are all
goldens and the bench corpus.

**Transparent page emitted.** The mark covers 68 % of the canvas and
touches the left edge, so the white (transparent) background was a
minority at depth 0 and came out as a white path. Segment now counts the
input's fully transparent pixels per colour; a near-white colour that was
mostly transparent is the page whatever its share. One path fewer, PSNR
unchanged (measured over white).

**Still open on this mark:** its shading is radial and two-directional,
so the linear ramp merge leaves visible bands with ragged seams in the
body and the globe. That needs a radial-gradient fit (roadmap phase 6).

# Primitives v2, 2026-11-14

Twenty-one images (the character, nine logos, synthetics from the corpus, six cartoons, renders and typography), PSNR over white by rsvg-convert, with the path count of each SVG. Both columns are `--preset poster --color-tolerance 0.06 --gradients --refine --no-mark`; before is e0b6ecf (main), after is the primitives v2 branch. The primitive pass now cuts each refined outline into line, arc and free pieces by split-and-fit, types each arc's ends by the joint it meets (tangent to a line, a point on a corner, free beside a curve), sharpens the fillet the refinement leaves at a sharp corner, and admits a snap under an absolute energy floor (`primAbs` per pixel of boundary touched) capped by a relative rise (`primRelMax`), where before only a 2 % relative rise passed. The plain trace places a corner between a straight edge and a curve where the line meets the circle.

| image | before dB / paths | after dB / paths |
|---|---|---|
| character | 30.09 / 527 | 30.11 / 527 |
| black-green-angular-geometric-logo | 37.20 / 12 | 36.99 / 12 |
| black-red-circle-symmetric-logo | 42.61 / 5 | 41.83 / 5 |
| blue-green-letter-v-dots-stylized-logo | 40.90 / 20 | 40.93 / 20 |
| blue-letter-u-shield-floral-logo | 38.24 / 2 | 38.34 / 2 |
| blue-purple-letter-m-abstract-floral-logo | 32.50 / 159 | 32.56 / 159 |
| blue-purple-letter-q-ribbon-gradient-logo | 47.35 / 7 | 47.44 / 7 |
| boy-render | 28.57 / 107 | 28.56 / 107 |
| cat-dog | 26.76 / 30 | 26.82 / 30 |
| fusion-logo | 29.31 / 40 | 29.27 / 40 |
| gradient-shapes | 42.72 / 5 | 42.72 / 5 |
| mr-bean | 24.76 / 120 | 24.73 / 120 |
| noisy-disc | 29.72 / 22 | 29.64 / 22 |
| overheat-type | 28.20 / 23 | 28.21 / 23 |
| red-yellow-letter-g-minimalistic-circular-logo | 40.76 / 24 | 40.71 / 24 |
| small-glyphs | 42.08 / 2 | 42.06 / 2 |
| teal-blue-abstract-modern-logo | 40.07 / 25 | 40.19 / 25 |
| thin-lines | 33.37 / 1 | 33.37 / 1 |
| tom-jerry | 26.00 / 565 | 26.02 / 565 |
| unicorn | 24.28 / 666 | 24.27 / 666 |
| 8_1sasa11 | 47.85 / 69 | 47.64 / 69 |

Mean −0.04 dB, 9 wins, 10 losses, 2 unchanged; every path count is the same. Apart from the Pokéball (black-red-circle-symmetric-logo, −0.78 dB) no image moves by more than 0.21 dB either way; without it the mean is −0.01 dB.

## Nodes and arcs on the logos

Nodes are the anchors of the SVG; an arc is a cubic whose four control points lie within max(0.02, 3·10⁻⁴·r) of one circle, the same tolerance `isArc` uses in the tests.

| logo | before nodes / arcs | after nodes / arcs |
|---|---|---|
| black-green-angular-geometric-logo | 119 / 14 (18 % of 76 cubics) | 110 / 41 (59 % of 69 cubics) |
| black-red-circle-symmetric-logo | 105 / 44 (47 % of 93 cubics) | 53 / 33 (73 % of 45 cubics) |
| blue-green-letter-v-dots-stylized-logo | 116 / 11 (10 % of 110 cubics) | 122 / 24 (21 % of 113 cubics) |
| blue-letter-u-shield-floral-logo | 1246 / 80 (7 % of 1166 cubics) | 1130 / 165 (16 % of 1058 cubics) |
| blue-purple-letter-m-abstract-floral-logo | 3292 / 108 (3 % of 3133 cubics) | 3014 / 328 (12 % of 2824 cubics) |
| blue-purple-letter-q-ribbon-gradient-logo | 183 / 74 (45 % of 165 cubics) | 122 / 70 (58 % of 121 cubics) |
| fusion-logo | 636 / 22 (4 % of 606 cubics) | 651 / 59 (10 % of 585 cubics) |
| red-yellow-letter-g-minimalistic-circular-logo | 470 / 178 (42 % of 421 cubics) | 322 / 153 (52 % of 294 cubics) |
| teal-blue-abstract-modern-logo | 418 / 79 (21 % of 374 cubics) | 387 / 99 (28 % of 348 cubics) |

Every logo but two has fewer nodes and every logo has more of its curves as exact arcs. The Pokéball halves its nodes (105 to 53, the spec asked for at most 70) and the G logo goes from 470 to 322 at 52 % arcs (the spec asked for 40 %).

## The synthetic shapes

Refined output on the corpus fixtures the pass was built for, against the source image: half-disc 8 nodes at 44.14 dB before, 3 nodes at 51.13 after (one line, two arc cubics meeting it at the corners); annular-sector 11 nodes at 49.64 before, 6 at 51.94 (two radial lines, two arcs each of two cubics, one shared centre); poster-discs 76 nodes at 43.82 before, 53 at 44.24; fillet-arc 12 nodes at 37.52 before, 6 at 40.01 (the fixture's own geometry was corrected on the way, see the commit). Plain traces move with the corner placement alone: half-disc 31.97 to 43.95 dB, annular-sector 33.01 to 36.78, poster-discs at the poster preset 32.94 to 34.27 with 84 to 74 nodes.

## Speed

`go test ./ -run xxx -bench RefineBigPoster -count 5`, ns per op as seconds: main 3.58, 3.56, 3.72, 3.54, 3.54; branch 4.13, 4.10, 4.11, 4.17, 5.06. Median 3.56 against 4.11, 1.15×, under the 1.2× the plan allowed.

## Goals met and missed

The spec set four marks for this bench. The G logo at 40 % arcs is met (52 %). The Pokéball at 70 nodes is met (53). The mean not falling by more than 0.1 dB is met (−0.04). Two are missed, both on the Pokéball: it was to reach 90 % arcs and reaches 73 %, and no image was to lose more than 0.5 dB and it loses 0.78.

The loss was traced piece by piece. With every stage of the primitive pass switched off the branch renders the Pokéball at 42.55 dB (main 42.61), so the corner placement and the split-and-fit cost nothing. The whole-circle snap of the inner white disc (r = 170) costs 0.42 dB and one arc snap on the lower white half's rim (r = 742, five cubics to two) costs 0.40; the other accepted snaps, including two on the same rim that lower the energy, cost nothing measurable, and skipping the rim snap alone puts the branch above main at 42.65. Both snaps are correct circles: the disc's centre is right to 0.3 px (moving it either way raises the energy) and the energy over radius is a clean parabola with its minimum at 169.88. The source is what is not circular. Measured along 720 rays with a half-maximum crossing, the inner disc's edge has a 0.19 px sin 2θ term and a 0.14 px rms residual about its best circle, and the lower rim a 0.14 px rms; the free cubics follow those tenths of a pixel and the exact circle cannot. At 42 dB the image's whole error budget is about 0.1 px of edge, so a 0.1 px systematic offset round a 1000 px ring is 0.4 dB. The energy guard measured both snaps at +5 % and +1 %, within what the spec set the floor to admit (the +1 % snap passes the old 2 % clause as well); the guard sees the colour mask, and the mask puts the exact circle a tenth of a pixel inside the luminance edge where it puts the free cubics a little less so. This is the trade the primitive pass exists to make, an exact circle in place of cubics that wobble to follow a resampled logo, and the drop is invisible at this level; the goal of 0.5 dB was set from the 0.3 dB the spec measured in one ring's window and did not allow for two rings on an image with no other error.

The 90 % arc goal assumed every curve on the Pokéball is a rim. Of the twelve free cubics left, the boundary between the two shades of red (a shading curve, not a circle) accounts for the snaps the guard rightly refused at +22 to +35 %, and the rest are short joins at the band's corners.

## Constants moved

The spec gave `primAbs` = 1.0 and `primRelMax` = 0.25 as starting values and left the bench to move them. At those values the bench read Pokéball 41.83 (the same), 8_1sasa11 46.90 (−0.95), gradient-shapes 42.41 (−0.31), black-green 36.95 (−0.25) and a mean of −0.12 dB. At `primAbs` = 0.5 and `primRelMax` = 0.15, the values committed, 8_1sasa11 reads 47.64 (−0.21), gradient-shapes is unchanged, black-green 36.99 and the mean −0.04; the Pokéball's two costly snaps (0.16 and 0.04 per pixel) are admitted either way. `cornerFillet` moved from 3 to 4 px during Task 7 (the refined corner anchor drifts a pixel along its edge, so a sliver the sharpening should remove measures 3.2 px). The two constants' comments in `internal/refine/guard.go` record the move.

## Not pursued

A refit of the exact circle against the luminance rather than the colour mask would recover most of the Pokéball's 0.8 dB and would equally move every refined edge, since the mask is what the refinement itself optimises; that is a change to the refinement's data term, not to the primitive pass. The outer black ring (r = 880, an eight-cubic loop) is refused as a whole circle at +22 %: its refined outline is 0.3 px off round, beyond what the floor admits at that radius, and it keeps its eight cubics.

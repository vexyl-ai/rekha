# Flat lightness, 2026-11-14

The boy render (1200×2198, cleaned copy), `--preset flat --refine`, PSNR over white, path count and distinct fills, at four values of `--flat-lightness` (the colour tolerance at the preset's 0.04):

| flat-lightness | dB | paths | fills |
|---|---|---|---|
| 0.03 | 31.45 | 714 | 43 |
| 0.05 | 31.65 | 515 | 39 |
| 0.08 | 30.07 | 389 | 26 |
| 0.12 | 28.58 | 331 | 20 |

The knob keeps the quantiser's shade steps (the hair's tones, the hoodie's folds): fills double between 0.12 and 0.05 and PSNR gains 3 dB. The soft shadow under the chin, the case that prompted the knob, does not come back at any value, and not with the colour tolerance lowered either:

| colour tolerance, flat-lightness | dB | paths | fills |
|---|---|---|---|
| 0.02, 0.03 | 30.61 | 1437 | 67 |
| 0.02, 0.05 | 30.55 | 1003 | 43 |
| 0.03, 0.05 | 31.13 | 621 | 43 |

That shadow is a few hundredths of lightness over a soft ramp: the quantiser folds it into the skin before Flatten runs, and a lower tolerance spends its colours on grain elsewhere (1437 paths) without resolving it. Keeping soft shading is what `--gradients` under the photo or poster preset does; the flat preset is for the look that drops it.

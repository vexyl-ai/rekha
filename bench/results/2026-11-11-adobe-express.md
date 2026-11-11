# Rekha against Adobe Express (2026-11-11)

Three logos, each a 2048-pixel PNG with a transparent background,
traced by Rekha at `a4afdc0` with `--preset photo --gradients` (defaults
otherwise) and converted by Adobe Express's image-to-SVG tool, exported
on 2026-11-11 with its default settings. The source PNGs and the Adobe
Express exports are not in the repository (third-party marks); the
strips under `docs/images/compare/` are crops of the three renders.

## Method

Both SVGs are rendered back at the source size with librsvg
(`rsvg-convert -b white`) and scored against the source composited on
white, over the source's opaque pixels only (`bench/opaque_psnr.py`),
so neither tool is charged for how it treats the transparent
background: Adobe Express paints it black, Rekha leaves it clear.

    python3 bench/opaque_psnr.py source.png output.svg render.png

Paths are `<path` elements; bytes are the file as exported.

## Results

| logo | tool | paths | bytes | gradients | PSNR (opaque) |
|---|---|---|---|---|---|
| browser mark, 1984×2048 | Rekha | 33 | 32 725 | 13 | 34.82 dB |
| | Adobe Express | 197 | 324 626 | 0 | 28.67 dB |
| letter G, 2048×2048 | Rekha | 16 | 13 563 | 4 | 32.60 dB |
| | Adobe Express | 49 | 77 521 | 0 | 24.01 dB |
| ball, 2048×2045 | Rekha | 5 | 3 845 | 0 | 33.34 dB |
| | Adobe Express | 5 | 12 492 | 0 | 26.58 dB |

Rekha's whole-image PSNR on white (the figure the other bench notes
quote) is higher, 36.3, 37.0 and 35.5 dB, because the clear background
scores perfectly; the opaque-pixel figure is the fair one across tools.

## What the strips show

- Browser mark, globe: Adobe Express cuts the radial shading into flat
  cells with visible edges; Rekha paints it as one region, a base ramp
  under two translucent radial layers.
- Browser mark, tail: both follow the shapes; Rekha's seams between
  gradient regions are smooth curves, with one faint seam left at the
  tail's tip (`2026-11-11-fades.md`).
- Letter G: Adobe Express bands each ring; Rekha fits one ramp per
  ring, the plateau at the ring's end included.
- Ball: the shapes agree; Rekha's curves are a third of the bytes.

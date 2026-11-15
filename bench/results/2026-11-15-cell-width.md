# The quantiser's cell width, 2026-11-15

Twenty-one images as in `2026-11-14-rescue.md`, `--preset poster --color-tolerance 0.06 --gradients`, plain and with `--refine`, main (1bb8cbd) against the cell-width fix (`quant.Wu` takes the colour precision and derives the cell width from it instead of inferring it from the spacing of the occupied cells). Every one of the 42 SVGs is byte-identical: real images occupy neighbouring cells, so the inferred width was already the true one there. The fix shows on an image of a few flat colours and no anti-aliasing, where the inferred width was the gap between two real colours and a band near the mixture line to a black outline counted its parent as its halo:

| image | main | fix |
|---|---|---|
| skin field, ink line, 100×20 band 0.10 darker (128², 12 % of the image, 2.5 tolerances) | 2 colours, band lost | 3 colours |

Exit tests `TestWuKeepsShadeBesideInkWithoutAA` (quant) and `TestSegmentKeepsShadeBesideInk` (root). Every golden byte-identical.

## Why this bench exists

It started as "gradients for sub-tolerance shadows" on the boy render. Measured, the neck shadow is a 20 px plateau 0.11 OKLab below the skin, held by the quantiser at poster 0.04 and photo (entry L 0.788) and painted by the plain render; only the flat preset folds it, under its lightness rule. A spatial pass that gave far blobs inside a component their own colour was built and dropped: on the boy, 47 more paths for 0.02 dB and no change at the chin. The synthetic that was meant to pin that pass exposed the cell-width inference instead. Details in `docs/superpowers/specs/2026-11-14-reclaim-design.md`.

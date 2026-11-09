# Rekha

Rekha traces raster images (PNG, JPEG, GIF) into SVG. Pure Go, no cgo, one static binary, same library behind a CLI, an HTTP server and a WebAssembly build.

**Status:** Phase 3: bilevel and colour tracing (`bw`, `poster`, `photo` presets) with optional curve refinement (`--refine`). Server and WASM builds follow; see `design/roadmap.md`.

## Usage

```
go build -o bin/rekha ./cmd/rekha
bin/rekha image.png                      # writes image.svg
bin/rekha -o out.svg --threshold auto image.png
bin/rekha --mode polygon - < in.png > out.svg
bin/rekha --preset poster logo.png       # flat colours, stacked layers
bin/rekha --preset photo --color-tolerance 0.03 photo.jpg
bin/rekha --preset poster --palette "#000000,#ff0000,#ffffff" logo.png
bin/rekha --refine logo.png                # optimise the curves against the anti-aliasing
```

`--preset bw|poster|photo` seeds every default; flags given on the command line override it. `bw` thresholds to black on white (`--threshold 0..1|auto`, `--turn-policy minority|majority|black|white`). `poster` and `photo` quantise colours in OKLab: `--color-tolerance 0..1` is the distance at which colours merge (0.08 and 0.04), `--max-colors N` caps the palette (0 = as many as the tolerance needs), `--palette "#rrggbb,…"` fixes it, `--color-precision` bits per channel. `--hierarchical stacked|cutout` chooses between layered shapes (no seams between abutting colours; the white page — near-white shapes at depth 0, when white holds at least half the depth-0 area — is omitted so transparent input stays transparent) and shapes with holes cut out. Common: `--mode pixel|polygon|spline` (default spline), `--filter-speckle N` pixels, `--simplify` px, `--corner-threshold` degrees, `--fit-tolerance` px, `--path-precision` decimals. `--refine` optimises the fitted curves against the image's anti-aliasing (spline mode; after Yang et al. 2016): on the anti-aliased synthetics it adds 5–20 dB PSNR at about the same node count, and costs roughly 0.1–0.3 s per 256×256 image or 15 s for a 3.2-megapixel logo, against milliseconds for the plain fit; `--refine-passes N` sets the sweeps (1–10, default 3). `--cutout-stroke px` strokes each cutout shape in its own colour to hide the hairline seams between abutting fits (0–4; 0.5 for `poster` and `photo`, 0 for `bw`); at 1:1 scale the stroke thickens edges and costs 2–7 dB, so pass 0 when measuring. Exit codes: 0 ok, 1 usage, 2 a trace failed (remaining inputs are still processed).

## Independent implementation

Rekha is written from published algorithms, not from existing tracer code:

- Pipeline design after VTracer's published linear pipeline (MIT / Apache-2.0).
- Curve fitting after Schneider, *An Algorithm for Automatically Fitting Digitized Curves*, Graphics Gems, 1990.
- Refinement after Yang et al., *Effective Clipart Image Vectorization Through Direct Optimization of Bezigons*, IEEE TVCG 2016.

It contains no code derived from Potrace or any other GPL-licensed tracer. Potrace is used only as a black-box binary in the benchmark harness. See `CONTRIBUTING.md` for the clean-room rule.

## Documents

- `design/roadmap.md`: phases 0 to 6, open-core product split, legal checklist.
- `design/refine-bezigon-optimisation.md`: design notes for the refinement stage.
- `docs/benchmark.md`: how output quality is measured against reference tools (`make bench`).
- `bench/results/`: recorded baselines.

## Licence

Apache License 2.0. See `LICENSE` and `NOTICE`.

<p align="center"><img src="BrandLogo/Rekha-logo.svg" width="260" alt="Rekha: fast vector engine, raster to SVG"></p>

# Rekha

**Raster to clean SVG. Pure Go. Your image never leaves your machine.**

Rekha traces raster images (PNG, JPEG, GIF) into SVG: a library, a CLI and a WebAssembly build from one codebase, with no cgo and no dependencies beyond `golang.org/x/image`. Clean-room, Apache-2.0.

**Status:** Phase 4 (open half): the tracer runs in the browser as WebAssembly behind a worker-backed demo page (`make serve`, `docs/wasm.md`) and behind a capped HTTP API (`cmd/rekha-serve`, `docs/server.md`). Tracing itself is at Phase 3b plus gradient fills: bilevel and colour (`bw`, `poster`, `photo` presets), optional curve refinement (`--refine`) that recognises circles, arcs, shared corner radii and rectangles and shares every boundary between cutout shapes, and `--gradients`, which turns smooth shading into multi-stop `<linearGradient>` fills. Accounts, billing and the polished GUI are the private half; see `design/roadmap.md`. Baselines: `bench/results/2026-11-10-phase3b.md` (quality), `bench/results/2026-11-10-gradients.md` (gradient fills), `bench/results/2026-11-10-slivers.md` (anti-aliasing cleanup), `bench/results/2026-11-10-phase4-wasm.md` (wasm latency).

## New in November 2026

- **Gradient fills.** `--gradients` merges adjacent colour bands whose pixels follow one ramp into a single region filled by a `<linearGradient>` with as many stops as the colour needs (bends included), joins ramps across curved feathers, and lets specks inside a ramp take the ramp's colour. A 16-megapixel illustration goes from 155 flat paths at 36.7 dB to 82 paths at 39.8 dB, 60 % fewer bytes. Flat art is byte-identical with the option on.
- **Clean edges between colours.** Anti-aliased pixels between two colours used to quantise to whatever third colour was nearest and survive as dashed slivers along every edge; the cleanup now judges a thin component by its own pixels against the mixture of its neighbours. A fifth to a third fewer paths on busy posters at the same PSNR.
- **Seams sealed in every layer mode.** `--stroke px` strokes each shape in its own fill, in stacked output too (it was cutout-only as `--cutout-stroke`, still accepted).
- **Progress you can watch.** `Options.Progress` reports each stage (quantise, label, ramps, outlines, nest, edges, refine, curves) with a count where there is one; the wasm API takes a callback and the demo shows a bar.
- **A demo you can inspect with.** The page keeps to the viewport, and the canvas zooms and pans (ctrl/⌘ + wheel or pinch, drag, double-click, a toolbar and the keys `+` `-` `0` `1`), rasterising the SVG at the zoom shown so detail is real. Source and SVG are compared with a wipe; the trace runs in a worker with a cancel button and never leaves the machine.

## Why Rekha

- **Curves, not polygon soup.** `--refine` optimises every Bézier against the image's anti-aliasing, so edges land where the artist drew them, to a fraction of a pixel. On the anti-aliased synthetics that is 5–20 dB PSNR over the plain fit at about the same node count.
- **Primitives stay primitives.** A circle comes out as four exact arcs, a rounded rectangle as four lines and four arcs with one shared radius, a rotated rectangle as four nodes. Editable in Illustrator, Figma or Inkscape without untangling hundreds of handles.
- **Small text survives.** On the small-glyphs fixture Rekha renders at 32.26 dB with 86 nodes against Potrace's 32.22 dB with 107.
- **Few paths.** A 3.4-megapixel colour logo traces to 14 paths with `--preset poster`; VTracer matches the PSNR (28.95 vs 28.98 dB) but tints the white counters grey.
- **Fast enough to sit behind a slider.** 0.1–0.25 s per 256×256 image with `--refine`, 1.1 s plain or 4.6 s refined for that 3.4-megapixel logo, on a laptop.
- **Deterministic.** The same input gives the same SVG on one core or eight, arm64 or amd64, so a diff in CI is a real change.
- **One static binary, anywhere Go runs.** No Python, no libpotrace, no native libraries. The same code runs in the browser as WebAssembly (`make serve` starts the demo), so a logo is traced without uploading it; `rekha-serve` puts the same tracer behind HTTP with byte, pixel, time and concurrency caps.
- **Safe to embed.** Written from the published papers, never from Potrace or any GPL tracer (see below). Apache-2.0 with DCO sign-offs from the first commit.

Where it is not there yet: photographs posterise into colour bands (an AI pre-pass is the planned answer, roadmap Phase 6); cutout mode shares boundaries correctly but most viewers still bleed the page faintly through shared edges at 1:1, so stacked layers stay the default; and side-by-side numbers against Vector Magic and Vectorizer.AI are only produced once their outputs are placed under `bench/reference/`, see `docs/benchmark.md`. All quality claims above are measured by `make test` and `make bench`; the commands and fixtures are in the repo.

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
bin/rekha --preset photo --gradients art.png   # smooth shading as <linearGradient> fills instead of bands
bin/rekha --preset poster --stroke 0.5 logo.png # seal hairline seams between shapes when viewed enlarged
make serve                                 # build the wasm and open the browser demo on :8080
go run ./cmd/rekha-serve                   # HTTP API only; curl -F image=@logo.png -F preset=poster localhost:8080/v1/trace
```

`--preset bw|poster|photo` seeds every default; flags given on the command line override it. `bw` thresholds to black on white (`--threshold 0..1|auto`, `--turn-policy minority|majority|black|white`). `poster` and `photo` quantise colours in OKLab: `--color-tolerance 0..1` is the distance at which colours merge (0.08 and 0.04), `--max-colors N` caps the palette (0 = as many as the tolerance needs), `--palette "#rrggbb,…"` fixes it, `--color-precision` bits per channel. `--hierarchical stacked|cutout` chooses between layered shapes (no seams between abutting colours; the white page — near-white shapes at depth 0, when white holds at least half the depth-0 area — is omitted so transparent input stays transparent) and shapes with holes cut out (every boundary between two cutout shapes is one curve, fitted and refined once, so both shapes carry the same coordinates; a viewer that anti-aliases each shape on its own still lets the page bleed faintly through shared edges at 1:1, which is why stacked is the default). Common: `--mode pixel|polygon|spline` (default spline), `--filter-speckle N` pixels, `--simplify` px, `--corner-threshold` degrees, `--fit-tolerance` px, `--path-precision` decimals. `--refine` optimises the fitted curves against the image's anti-aliasing (spline mode; after Yang et al. 2016): on the anti-aliased synthetics it adds 5–20 dB PSNR at about the same node count, and costs roughly 0.1–0.25 s per 256×256 image or 5 s for a 3.2-megapixel logo, against milliseconds for the plain fit; `--refine-passes N` sets the sweeps (1–10, default 3). It also replaces what it recognises, each only when the rendered error rises by no more than 2 %: a near-circle becomes four exact arcs, a run of curved segments on one circle becomes arcs of that circle, corners whose radii agree within 10 % get one radius, and a four-sided outline whose angles are right within 2° becomes a rectangle (snapped to the axes within 1°). Self-intersections are kept out by a constraint on every optimiser step rather than a weighted prior (`refine.Weights` has no `Spt` since Phase 3b), and the pieces of a loop are optimised in parallel. `--stroke px` (formerly `--cutout-stroke`, still accepted) strokes each colour shape in its own fill to hide hairline seams when the SVG is viewed enlarged: every shared edge in cutout, and the sub-pixel gap where two stacked siblings meet at a sharp tip (0–4, default 0: opt-in, because at 1:1 scale 0.1–0.2 px is neutral while 0.5 px thickens every edge and costs up to 8 dB PSNR). `--gradients` (colour presets) merges adjacent colour bands whose pixels fit one linear ramp and meet along a soft boundary into one region filled by a `<linearGradient>` whose stops follow the colour along the ramp (two for a linear ramp, more where it bends), so smooth shading renders smoothly with fewer paths; `--gradient-tolerance 0..1` (default 0.03, an RMS distance in sRGB units, about 8/255) is the error a ramp may leave. Flat art is untouched: with the option on, art without ramps gives the same bytes. Exit codes: 0 ok, 1 usage, 2 a trace failed (remaining inputs are still processed).

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
- `docs/wasm.md`: the WebAssembly API, the worker protocol, progress callbacks and the demo page.
- `docs/server.md`: the HTTP API and its caps.
- `docs/superpowers/specs/2026-11-10-gradients-design.md`: how gradient fills are found, joined and cleaned.
- `bench/results/`: recorded baselines.

## Licence

Apache License 2.0. See `LICENSE` and `NOTICE`.

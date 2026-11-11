# Rekha roadmap and product split

Status: draft, 2026-11-10. Phases 0–3 are delivered (Phase 3b baseline: `bench/results/2026-11-10-phase3b.md`); the open half of Phase 4 (wasm, worker demo, capped HTTP API) is delivered 2026-11-10 (`bench/results/2026-11-10-phase4-wasm.md`). Phases 0–5 summarise `Go SVG image tracing tool.md`; Phase 3 refinement is detailed in `refine-bezigon-optimisation.md`; Phase 6 is new.

## Product split: open core

Two products from one codebase.

| | Open-source CLI | Subscription online |
|---|---|---|
| Repo | `rekha` (public, AGPL-3.0 + commercial) | `rekha-cloud` (private) |
| Contains | Library, CLI, WASM build, all classical tracing including Phase 3 refinement | Web GUI, HTTP API, accounts, billing, batch, storage, Phase 6 AI services |
| Depends on | Standard library, `golang.org/x/image` | Imports public `rekha` module as a dependency, never forks it |
| Runs where | User's machine, CI, browser (WASM) | Our servers |

Rules that keep the split clean:

1. **All tracing quality lives in the open repo.** Refinement, presets, cutout, primitive detection: all open. Quality is the marketing for the paid tier, and a crippled CLI would get forked. The paid tier sells convenience, scale and AI, not better curves.
2. **Private repo only adds, never patches.** If the cloud product needs a change in the tracer, the change goes upstream into the public module. No private fork of the core.
3. **AI layer is private and separate** (Phase 6). It never enters the public module's import graph, so the public build stays pure Go, cgo-free and WASM-capable, and model licences never touch the open repo.
4. **Dual licence, decided 2026-11-11.** The public repo started under Apache-2.0 (the moat was to be the AI layer, the GUI and the brand). To fund development it moved to AGPL-3.0 plus a paid commercial licence (`COMMERCIAL.md`) before any tagged release and while the sole author held every copyright; DCO sign-offs from the first commit made that clean. Versions up to `fe2e104` stay Apache-2.0. Contributions now come with a grant to relicense commercially (`CONTRIBUTING.md`).
5. **Trademark the product name** before launch. Open code, protected name. Same approach Potrace's author uses.

## Phases

| Phase | Deliverable | Contents | Exit test | Ships in |
|---|---|---|---|---|
| 0. Foundations | Repo, licence, harness | MIT or Apache-2.0 decision; module layout; DCO; test corpus by category (line art, logos, pixel art, posterised, photos, pathological); round-trip benchmark with Potrace, VTracer and Vector Magic previews as black-box references; lawyer consult (Potrace clean-room, WASM distribution, Yang 2016 patents, trademark) | Benchmark runs on reference tools and reports PSNR, path count, node count, bytes, time | Open |
| 1. Bilevel MVP | Library + CLI, black and white | Fixed and adaptive threshold; pixel-edge contour tracing with holes and turn policy; despeckle; staircase removal and polygon simplification; corner detection plus Schneider fitting; compact SVG writer; `pixel`/`polygon`/`spline` modes; golden tests, fuzzing | Logos and scans trace cleanly; PSNR within a set margin of Potrace on bilevel corpus | Open |
| 2. Colour | Colour tracing with presets | OKLab; Wu + k-means quantisation; fixed palette and max-colours; small-region merging; soft per-region coverage masks from anti-aliasing (needed by Phase 3); stacked layers; `bw`/`poster`/`photo` presets; per-layer parallelism, deterministic ordering | Fidelity and size comparable to VTracer on corpus | Open |
| 3. Refinement and quality | Vector Magic grade on flat art | Direct bezigon optimisation (`internal/refine`, see design notes); segment merge/split; primitive detection (lines, arcs, circles, rectangles); post-fit simplification; output optimisation levels; cutout mode (stroke stopgap, then shared-boundary planar map) | Measured PSNR gain over unrefined output at equal or lower node count; side-by-side with Vector Magic on corpus (hand-downloaded outputs scored by `make bench`, see `docs/benchmark.md`) | Open (delivered 2026-11-10) |
| 4. Online | Browser and API | WASM build in Web Worker, browser-side decoding, before/after preview, cached segment step with cheap re-finish on slider moves; then HTTP API with dimension caps, timeouts, concurrency limits, scale-to-zero host; accounts and billing scaffold | WASM latency measured; API survives fuzzed uploads | WASM, demo page and capped API open (delivered 2026-11-10); accounts, billing and GUI private |
| 5. Optional extensions | By demand | Gradient fills (`--gradients`: adjacent bands that form one ramp, straight or bent, become a multi-stop `<linearGradient>`, bands round a centre a `<radialGradient>`, a region shaded more than one way a base gradient under translucent radial layers; delivered 2026-11-10, `bench/results/2026-11-10-gradients.md`, `2026-11-11-radial.md`, `2026-11-11-layered.md`, `2026-11-11-plateau.md`, `2026-11-11-seams.md` and `2026-11-11-fades.md`); clean-room Potrace-style optimal-polygon fitter from the paper (after legal sign-off); centerline tracing; PDF/EPS/DXF export; build-tagged SIMD | Each justified by benchmark or user demand | Open |
| 6. AI layer | Final stage, paid tier | See below | Measured improvement on photo and noisy-input corpus categories over classical pipeline alone | Private |

Phases 0–3 are sequential. Phase 4 can start once Phase 2 is stable. Phases 5 and 6 are independent of each other and gated on benchmark evidence.

## Phase 6: AI layer

### Principle

AI never touches geometry. Classical pipeline plus Phase 3 refinement already exceeds commercial quality on clipart, logos and line art, and ML vectorisers are slower and worse there (report §ML). AI runs **before** the tracer, on pixels, and produces pixels or masks. The tracer never knows it happened.

### Architecture

- Separate service, private repo, Python + ONNX Runtime (or PyTorch), CPU first, GPU only if latency demands.
- Contract: image in, image or mask stack out. Nothing else crosses the boundary. No model code, model weights or Python in the Go module.
- Server calls AI service, then calls the public tracer library with the processed image and optional external mask stack. The library gains one new input: `Options.ExternalMasks []Mask` so a caller can supply segmentation instead of the built-in quantiser. That hook is public and documented so the open CLI can also consume masks from any external tool, which keeps the open product honest and the hook tested.
- Open CLI gets `--masks dir/` to read a mask stack from disk. No network calls from the CLI, ever.
- WASM build never includes any of this.

### Components, in priority order

| # | Component | Problem it solves | Candidate models | Licence check | Latency target |
|---|---|---|---|---|---|
| 1 | Semantic segmentation | Photos and complex illustrations: colour quantisation gives bands, not objects. Output regions follow objects (face, shirt, sky) instead | SAM family and successors; small promptable segmenters | SAM is Apache-2.0; verify each successor | < 2 s CPU at 1024 px |
| 2 | Denoise and upscale pre-pass | JPEG ringing and low resolution: one review saw 720 paths from a noisy JPEG vs 81 from a clean PNG of the same content; Yang 2016 lists noisy input as its own failure case | Lightweight denoisers, 2x super-resolution models tuned for graphics not photos | Many SR models are non-commercial; filter hard | < 1 s CPU |
| 3 | Background removal | Users expect it on upload | Matting models | Most popular ones are non-commercial; need a commercially licensed or own-trained one | < 1 s |
| 4 | Input classifier and auto preset | Zero-config mode: pick logo / line art / pixel art / photo preset and colour count | Tiny CNN, or hand heuristics first (colour histogram entropy, edge density) | Trivial | < 50 ms |
| 5 | Text detection | Keep text as `<text>` or trace with tighter corner priors; sliver-free type | OCR detection models | Check | < 1 s |
| 6 | Layer ordering and grouping | Which shape is above which, named groups for editing in Illustrator or Figma | Derived from segmentation hierarchy; no separate model needed at first | None | Free |

Build 1 and 2 first; they are the only items that change output quality. 3 and 4 are product polish. 5 and 6 are differentiation later.

### What stays classical even in the paid tier

Contour tracing, simplification, curve fitting, refinement, primitive detection, SVG writing. If a future model beats the classical fitter on the benchmark at acceptable latency, it can be slotted in as another pre-processor that emits a better mask, never as a replacement for the fitter.

### Exit test

On the corpus's photo and noisy-input categories: PSNR, path count and designer preference with AI pre-processing vs without, at matched latency budget. On logo and line-art categories: AI pre-processing must not degrade anything; if it does, the classifier routes those images around it.

### Risks

- Model licences. Non-commercial weights are common. Every model needs a licence read before integration; budget for training or buying a matting model.
- Latency and cost. CPU inference adds seconds; GPU adds fixed monthly cost that conflicts with scale-to-zero hosting. Decide per component once usage is known.
- Scope creep into generative features. Out of scope. Rekha traces what is there; it does not invent content.

## Legal checklist (Phase 0 lawyer consult)

1. Clean-room status of a Potrace-style fitter written from the paper only.
2. WASM delivery as GPL "distribution" (moot if no GPL code, but confirm the no-GPL position).
3. Patents around Yang et al. 2016 (Microsoft Research Asia co-authors) and Manson-Schaefer wavelet rasterisation.
4. Trademark search and registration for the product name.
5. CLA vs DCO for the open repo, preserving relicensing freedom.
6. Model licence review process for Phase 6.

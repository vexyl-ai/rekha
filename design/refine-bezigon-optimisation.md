# Design notes: direct bezigon refinement stage

Status: draft, 2026-11-09. Source paper read in full (16 pages incl. appendix).

Source: Yang, Chao, Zhang, Guo, Yuan, Sun. *Effective Clipart Image Vectorization Through Direct Optimization of Bezigons.* IEEE TVCG 22(2), 2016. arXiv 1602.01913. Local copy: `design/refs/yang2016-bezigon-optimization-arxiv1602.01913.pdf`.

These notes describe the algorithm as published and how it fits into our pipeline. Equation numbers refer to the paper. Everything under "Our design" is our own choice, not the paper's.

---

## 1. Why this stage exists

Every classical tracer (Potrace, VTracer, Inkscape, our Phase 1–2 pipeline) fits curves to an intermediate polygon derived from pixel edges. The raster is never consulted again after the polygon is built. Three consequences (paper §1):

1. Errors in the polygon cannot be corrected by the curve fitter.
2. Even a perfect polygon leaves ambiguity: many bezigons fit one polygon.
3. No curve-level priors exist to break the ambiguity.

The paper's fix: treat the Phase 1–2 output as an **initial guess** and then minimise an energy that compares a differentiable rasterisation of the bezigon directly against the input pixels, plus four priors that encode what a professional tracer would draw.

Reported results against commercial tools (paper §5.2, clipart corpus, equal bezigon counts):

- PSNR +0 to +5 dB vs Vector Magic (itself based on Diebel's polygon-level direct optimisation, ref [1]).
- PSNR +10 to +20 dB vs Adobe Illustrator Image Trace.
- User study (120 pairs, designers): ~80% preferred over Vector Magic, ~25% "much better".

Specific wins (paper §5.3): smooth high-curvature boundaries get fewer false corners; obtuse corners preserved; slightly bent edges kept instead of straightened.

This is the stage that gets us from "VTracer parity" to "Vector Magic grade" on clipart, logos and flat art. It does nothing for photos.

---

## 2. The algorithm as published

### 2.1 Representation (paper §3.1)

A **bezigon** is a closed chain of N cubic Béziers, eq. (1)–(3). Segment j has control points (x_{j,1..4}, y_{j,1..4}); x_{j,4} = x_{j+1,1}, so there are 3N distinct points and 6N geometric parameters. Each bezigon also has colour parameters C. The prototype uses a uniform colour C_0 ∈ RGB; the framework allows a quadratic colour model (18 params) but it is never used.

Each bezigon is optimised **independently**. The paper does not handle overlapping or stacked shapes.

### 2.2 Energy (eq. 5–6)

    E(W; I) = E_data(W; I) + E_prior(W)
    W* = argmin E

### 2.3 Data energy (eq. 7–8, 10)

    E_data = (1 / l_0) · Σ_{(x,y) ∈ Λ} ‖ R(W; x, y) − I(x, y) ‖²

- Λ is the pixel lattice of the input.
- l_0 is the arc length of the **initial** bezigon, fixed for the whole run. It normalises the energy so prior weights mean the same thing for small and large shapes.
- R(W; x, y) is the rasteriser. The paper's central claim is that R must be **continuous and differentiable in the control points**. Sample-based rasterisers (Cairo, AGG, Batik, Illustrator, scanline + Lanczos) are piecewise flat with jumps (Fig. 2b), and any gradient or quasi-Newton solver fails on them.
- They adopt **wavelet rasterisation** (Manson & Schaefer 2011, ref [35]): the anti-aliased image is expressed in a 2D Haar basis whose coefficients are closed-form line integrals along the Bézier (eq. 9, 27–28). Appendix A proves continuity (Thm 1) and differentiability in the sense of generalised functions (Thm 2), and derives analytic gradients (eq. 31–49). The derivatives contain Dirac terms at the real roots of cubic equations 2^s·Y_j(t) − k_y = 0, i.e. at the parameters where the curve crosses Haar cell boundaries.
- Fig. 5 shows that pixels produced by R_MS differ from Cairo/AGG/Batik/Illustrator output by zero-mean, small-variance noise on boundary pixels only, so using R_MS as the model for an input that was rendered by some other tool is safe.

### 2.4 Prior energy (eq. 11)

    E_prior = λ_spt·E_spt + λ_apt·E_apt + λ_hpt·E_hpt + λ_lpt·E_lpt

Default weights (paper §5.1): **λ_spt = 1.0, λ_apt = 0.08, λ_hpt = 0.1, λ_lpt = 0.1**. Authors report low sensitivity, except λ_apt may need raising on very low-resolution input.

Each prior targets one of four failure modes seen when data energy alone is used (Fig. 3):

**(a) Self-intersection prior, SPT (eq. 12–13).** Find all parameter pairs (t_1, t_2) where S(t_1) = S(t_2). Each splits the outline into two arcs; sum the **shorter arc length** over all intersections:

    E_spt = Σ_{(t_1,t_2)} min( L(t_1,t_2), L(0,N) − L(t_1,t_2) )

Zero when no self-intersection; grows as the loop grows; gives gradient guidance to shrink a loop out of existence rather than a hard penalty.

**(b) Angle-variation prior, APT (eq. 14).** At every joint j, a_j = incoming handle direction (x_{j,1} − x_{j−1,3}), b_j = outgoing handle direction (x_{j,2} − x_{j,1}):

    E_apt = Σ_j arccos( a_j·b_j / (‖a_j‖‖b_j‖) )

A sum of absolute angles is an L1-type sparsity term: most joints go to exactly zero angle (G1 smooth), a few stay sharp. This **is** the corner detector; no threshold is needed.

**(c) Short-handle prior, HPT (eq. 15).** Inverse barrier on both handle lengths of each segment:

    E_hpt = Σ_j ( 1/‖x_{j,2} − x_{j,1}‖ + 1/‖x_{j+1,1} − x_{j,3}‖ )

Prevents degenerate handles that create kinks the raster cannot see. Side effect: handles come out slightly longer than ideal (Fig. 9).

**(d) Curve-length prior, LPT (eq. 16).** Total arc length. Penalises twisted sections that have no evidence in the raster (Fig. 10).

### 2.5 Piecewise optimisation (paper §4.3, Alg. 1)

Full joint optimisation over 6N params is slow and converges badly. Bézier curves have local control, so:

- A **piece** P_j = two consecutive segments j and j+1. Fix the outer endpoints x_{j,1} and x_{j+1,4}. Free the five points in eq. (17): x_{j,2}, x_{j,3}, x_{j+1,1} (the shared middle endpoint), x_{j+1,2}, x_{j+1,3}. That is a 10-dimensional problem.
- Solve eq. (18): argmin over P_j of E_data(P_j; rest fixed) + E_prior(P_j; rest fixed). Only pixels near the piece change, and all priors except SPT are per-segment sums, so only two segments' terms need recomputing.
- Sweep j = 1..N. Pieces overlap, so every handle is optimised twice per sweep.
- Repeat sweeps until converged; **2–3 sweeps** suffice in practice.
- Optional final global pass over all 6N params (Alg. 1 line 7), now fast because the start point is good.
- Solvers used: NEWUOA, L-BFGS, conjugate gradient. All work because the energy is continuous.

### 2.6 Initialisation (paper §3.2)

Segmentation (ref [30]) + boundary curve fit (ref [31]). Initial bezigons "are not required to be highly accurate" and "most initial bezigons in our experiments are far from perfect". Only gross drift toward random bezigons causes local-minimum failure. **N is fixed by the initialisation; the paper never adds or removes segments.**

### 2.7 Performance (paper §5.1)

Python prototype, interpreted, single-threaded, Intel i5-2410M 2.53 GHz: **10 s to 10 min per image**. Authors state it should be much faster in a compiled language and is highly parallel (wavelet rasterisation and separate pieces/bezigons are independent).

### 2.8 Limitations stated by the authors (§6, Fig. 17)

- Noisy input and low-resolution input with complex content are too ambiguous for any automatic method including theirs. Their answer is an interactive mode: user drags a control point, optimisation resumes from there.
- Uniform colour per bezigon only in the prototype.
- No segment insertion/removal.
- No treatment of overlapping shapes.

---

## 3. Our design

Package: `internal/refine`. Runs inside the cheap "finish" step, after Schneider fitting, before the SVG writer. Off by default in Phase 1–2, on by `--refine` in Phase 3, promoted to preset default once the benchmark justifies it.

### 3.1 Data model

```go
// Bezigon is a closed chain of cubic Béziers. Pts has 3*N entries:
// for segment j (0-based), Pts[3j] is its start anchor, Pts[3j+1] and
// Pts[3j+2] are its two handles, and Pts[3(j+1) mod 3N] is its end anchor.
type Bezigon struct {
    Pts []Vec2
}

type Piece struct {
    J int // optimises segments J and J+1 (mod N)
}
```

Ten free scalars per piece: Pts[3J+1], Pts[3J+2], Pts[3J+3], Pts[3J+4], Pts[3J+5] (indices mod 3N). Pts[3J] and Pts[3J+6] are fixed.

### 3.2 Target image: per-region coverage mask, not RGB

**Deviation from the paper, and the key integration decision.**

The paper compares an RGB rendering of one isolated bezigon with the whole image. That only works when shapes do not overlap. Our output is stacked layers (report §"Layering"), so neighbouring regions share boundaries and lower layers extend under upper ones.

Instead, for each region r the segmentation stage emits a **soft coverage mask** M_r ∈ [0,1] per pixel (1 inside, 0 outside, fractional on anti-aliased boundary pixels, derived from how far each boundary pixel's colour lies between the two adjacent region colours). The refine stage then minimises

    E_data = (1 / l_0) · Σ_{(x,y) ∈ window} ( cov(W; x, y) − M_r(x, y) )²

where cov is scalar coverage in [0,1]. Benefits:

- Decouples geometry from colour. Colour is solved once per region by the quantiser; refinement moves only control points.
- Rasteriser is scalar coverage, which is exactly what analytic coverage rasterisers compute.
- Stacking is handled: for a region partly hidden by a layer above, the hidden boundary is simply absent from M_r (mask is 1 there if the region extends underneath), so the optimiser is free to run the curve wherever the lower layer plausibly continues, and the curve-length prior keeps it short.
- Anti-aliasing is used as sub-pixel evidence for free, which is the thing the report lists as a commercial-tool advantage.

Trade-off: boundaries between two abutting regions are refined twice, once per side, and the two curves may disagree by a fraction of a pixel. In stacked mode this is invisible (lower layer is covered). In cutout mode it matters, and the shared-boundary planar map (report Phase 3) must refine each shared edge once and both regions reference it. Defer to that work.

Optional later variant (Phase 3b): render the full stacked composite in RGB and compare to the real image, optimising all bezigons jointly. Faithful to the paper's spirit but couples everything. Only if mask-based refinement leaves measurable error.

### 3.3 Rasteriser

Requirement from the paper: continuous and differentiable in control points. Two candidates.

**A. Exact-area coverage accumulation (recommended first).** The signed-area accumulation technique used by font rasterisers: flatten each Bézier to line segments, for each line add its exact signed area and cover contributions to the cells it crosses, prefix-sum per scanline. Coverage per pixel is the exact area of the polygon inside the pixel, so it is continuous and piecewise-smooth in every vertex. Roughly 150–250 lines of Go. No external dependency.

Caveats to control:

- Flattening count must be **fixed per segment during one piece's optimisation** (e.g. 16 or 32 subdivisions by uniform t). Adaptive flattening changes the polygon topology between evaluations and reintroduces tiny discontinuities.
- Gradient by **central finite differences** over the 10 free scalars: 20 extra rasterisations of a small local window per gradient. Window ≈ bounding box of the two segments ± 2 px. For typical clipart pieces that is a 20×20 to 60×60 px window, cheap.
- Step h = 0.01 px in control-point space. Validate against a numerically differentiated reference on synthetic shapes.

**B. Wavelet rasterisation with analytic gradient (paper's choice).** Exact per the paper, analytic derivatives (Appendix A), fully parallel. Cost: more mathematics to implement and test (Haar coefficient integrals of cubic polynomials, root-finding for crossing parameters per Haar level), and the Dirac terms in eq. 43–49 need careful handling. Implement only if A is too slow or finite differences prove noisy on the benchmark.

Decision gate: implement A, run the synthetic recovery test in §3.8. If recovery is within 0.05 px and runtime fits the budget in §3.7, A stays.

### 3.4 Priors in Go

Weights as the paper: λ_spt 1.0, λ_apt 0.08, λ_hpt 0.1, λ_lpt 0.1, exposed as `Options.Refine.Weights` but not surfaced as CLI flags until someone needs them.

- **APT:** direct from eq. 14. Clamp the dot/norm ratio to [−1, 1] before `math.Acos`. Only joints J, J+1, J+2 change for a piece.
- **HPT:** direct from eq. 15. Guard division by zero with a floor of 1e-6. Only segments J, J+1 change.
- **LPT:** arc length of segments J, J+1 by Gauss-Legendre quadrature on the flattened polyline (same flattening as the rasteriser, so it is free).
- **SPT:** needs self-intersection detection. Use the flattened polyline. Prune by bounding box: only test the piece's polyline against the rest of the bezigon when boxes overlap, which is rare for sane shapes. For each intersection compute the shorter arc length along the polyline. Complexity O(k·n) worst case per evaluation, k = piece segments' lines, n = total lines; acceptable with bbox pruning. If profiling shows SPT dominating, test SPT only every k-th evaluation or only at sweep end as a reject/accept check.

Normalisation: E_data carries 1/l_0 with l_0 = arc length of the initial bezigon, computed once. Priors are unnormalised as in the paper. Keep exactly this so the published weights transfer.

### 3.5 Optimiser

Per piece: **L-BFGS** (m = 5) on 10 scalars with finite-difference gradient, Armijo backtracking line search, hard cap **30 iterations** per piece per sweep, stop when relative energy decrease < 1e-4 or max control-point move < 0.005 px. Write our own small L-BFGS (about 100 lines) rather than pull a dependency; keeps WASM/TinyGo clean.

Sweep schedule as Alg. 1: for j in 0..N−1 optimise piece j; repeat up to **3 sweeps**; stop early if a full sweep moved no point more than 0.01 px. Skip the paper's optional global pass in v1.

Determinism: fixed sweep order, no data-dependent parallelism inside a bezigon. Parallelise **across bezigons** with a worker pool and collect results by region index, so output is byte-identical regardless of goroutine scheduling.

### 3.6 Segment insertion and removal (our extension)

The paper keeps N fixed. Schneider's fitter in our pipeline already picks N by error threshold, but it decides before refinement and tends to over-segment at anti-aliased edges. After the final sweep:

- **Merge:** for each joint with APT angle < 0.5° and both adjacent segments' handles collinear within tolerance, tentatively replace the two segments by one cubic fitted to the flattened points, re-run a single piece optimisation, accept if E_data of the window does not rise by more than 1%.
- **Split:** for each segment whose local data residual exceeds 3× the bezigon mean, split at t = 0.5 by de Casteljau, re-optimise the two new pieces.

One merge/split pass, then one more sweep. Measure node-count reduction on the corpus. If gains are small, drop this to keep the code simple.

### 3.7 Performance budget and parallelism

Rough cost per piece evaluation: window W×W px × (two segments × 16 flatten lines). Per gradient ≈ 21 evaluations. Per piece ≈ 30 iterations × (1 + line-search) ≈ 50 gradients. Per sweep N pieces, 3 sweeps.

Example: logo with 40 regions, 30 segments each, 40×40 px windows: 40 × 30 × 3 × 50 × 21 ≈ 3.8 M window rasterisations of 1600 px each ≈ 6 G pixel-ops. At an optimistic 1 G/s in Go that is seconds, not the paper's minutes, but it is the dominant cost of the pipeline. Hence:

- Off by default until measured.
- Per-bezigon parallelism on CLI and server.
- In WASM (single-threaded) expose a `--refine-passes` of 1 and let the UI run it as a second, explicit "Refine" action rather than on every slider move.
- Early-exit: skip pieces whose initial window residual is already below a floor (e.g. mean squared coverage error < 1e-4); on clean raster input most pieces will be skipped after sweep 1.

Measure before trusting any of these numbers.

### 3.8 Tests

1. **Synthetic recovery.** Build a random smooth bezigon, rasterise it with our coverage rasteriser to a mask, perturb control points by up to 1.5 px, refine, assert every point returns within 0.05 px. Repeat with the mask rendered by an independent renderer (e.g. the test-only rasteriser from the report's testing section, or a one-off reference via an external tool) to check robustness to rasteriser mismatch, mirroring Fig. 5.
2. **Prior unit tests.** Figure-eight bezigon gives E_spt > 0 and refinement removes the crossing; a square with four sharp joints keeps APT ≈ 4·π/2 and does not round the corners; a near-zero handle triggers large E_hpt and refinement lengthens it.
3. **Determinism golden test.** Same input, GOMAXPROCS=1 vs 8, byte-identical SVG.
4. **Benchmark gate.** On the Phase 0 corpus, refine must raise mean PSNR of rasterised output vs input by a threshold to be set after the first run (paper's +0–5 dB over Vector Magic suggests +3 dB over our own unrefined output is a sane first target) at equal or lower node count. Record per-image time.
5. **Fuzz.** Degenerate bezigons (N = 1, N = 2, all points coincident, zero-area) must not panic or NaN; guard arccos and divisions.

### 3.9 Options surface

```go
type RefineOptions struct {
    Enabled    bool
    Sweeps     int     // default 3
    MaxIter    int     // default 30, per piece
    Weights    PriorWeights // spt 1.0, apt 0.08, hpt 0.1, lpt 0.1
    MergeSplit bool    // §3.6, default false until measured
}
```

CLI: `--refine`, `--refine-passes N`. Server and WASM read the same struct via the shared validator.

---

## 4. Open questions and risks

- **Patents [LAWYER].** Co-authors include Microsoft Research Asia staff (Lu Yuan, Jian Sun). No patent search done. Implementing a published algorithm is not a copyright issue; patents are a separate question. Add to the Phase 0 lawyer list alongside the Potrace clean-room items.
- **Coverage rasteriser smoothness.** Exact-area accumulation is continuous but has gradient discontinuities when a flattened vertex crosses a pixel edge. L-BFGS tolerates this in practice but may stall near such points. If the synthetic test shows stalls, options are: smooth the coverage with a 1-px tent filter before comparing, or switch to the wavelet rasteriser (§3.3 B).
- **Mask quality.** §3.2 moves the burden onto the segmentation stage producing good soft masks from anti-aliasing. For JPEG input with ringing noise the mask is noisy and refinement will chase noise; the paper's Fig. 17a shows the same failure. Mitigation: optional pre-blur or median filter on masks when JPEG artefacts are detected, or lower λ weight on data energy for low-quality input.
- **Low-resolution icons.** Paper says raise λ_apt. We can scale λ_apt by 1/image-scale automatically; measure.
- **Cutout mode seams.** Mask-based per-region refinement refines shared edges twice. Must be revisited with the planar-map work.

## 5. What this does not change

Report pipeline stays the initialiser. Segmentation, contour tracing, staircase removal and Schneider fitting are unchanged. This stage is purely additive, lives in one package, and can be disabled with one flag.

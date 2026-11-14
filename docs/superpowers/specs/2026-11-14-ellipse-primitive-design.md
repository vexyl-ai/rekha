# Ellipse primitive design: a whole loop that is an ellipse becomes four cubics

Status: design, 2026-11-14. The fourth item of the quantisation-quality run ("more shape primitives: ellipses, rounded rects, symmetry"); follows `2026-11-14-primitives-v2-design.md`, whose non-goals listed ellipses. The user asked for the work without questions; every choice below is the author's call.

## 1. Intent

`--refine` snaps whole circles (`SnapCircle`), arcs with typed ends, shared radii and rectangles. A loop that is an ellipse gets none of that: `FitCircle` refuses it, the pieces pass cuts it where its curvature changes and emits free cubics or, on a mild ellipse, circular arcs of two or three radii that do not meet smoothly. Ellipses are common in logos and icons (eyes, discs seen at an angle, the shadow under an object, speech bubbles) and a designer who opens the SVG expects one, four nodes and symmetric handles. A whole-loop ellipse is the piece that is missing; its cost is one fit and one guard per free loop that the circle declined.

## 2. Scope of "more primitives"

- **Ellipses** (this design): whole loops only, as the circle is.
- **Rounded rectangles**: already covered. `CommonRadius` makes four lines and four quarter arcs of one radius (`rounded-rect` golden); nothing to add.
- **Symmetry** (deferred, with reasons): mirroring a loop onto itself needs a correspondence between its two halves and a joint refinement of both under one axis, and symmetry between loops needs the same across the whole trace. Neither has a local guard the way a snap does, the node count does not fall (a mirrored loop has as many nodes), and on the 21-image bench no image is a candidate where PSNR would move. Not worth its complexity now.
- **Regular polygons, axis-snapped lines**: not asked for; lines are already exact.

## 3. Goals

With `--refine`, spline mode, any preset, either hierarchy:

- New synthetics `ellipse-aa` (centre (128,128), semi-axes 100 and 60, axis-aligned) and `ellipse-rot` (90 and 50 at 30°) come out as exactly four cubics, every point of them within 0.2 px of the true ellipse.
- A circle is still a circle: `circle-aa` stays four arc cubics, the Pokéball's rings stay circles (the ellipse runs only after the circle declined, and refuses an axis ratio under 1 %).
- A rounded rectangle is not an ellipse: `rounded-rect` stays eight segments.
- On the 21-image refined bench no image loses more than 0.2 dB and node totals do not rise; `bench/results/2026-11-14-ellipse.md` records it with the two synthetics before and after.
- Everything from the earlier primitive designs holds: pixel and polygon goldens byte-identical, spline goldens only with the PSNR check, `CGO_ENABLED=0`, wasm, no new dependencies, both gates green.

## 4. Design

### 4.1 The fit (`internal/fit/ellipse.go`)

`type Ellipse struct { Centre geom.Vec2; A, B, Theta float64 }`, `A ≥ B > 0`, `Theta` the direction of the A axis in (−π/2, π/2].

`FitEllipse(pts []geom.Vec2) (Ellipse, bool)` (the type takes the bare name): the algebraic least-squares conic `a·x² + b·xy + c·y² + d·x + e·y = 1` on data centred on its mean (the mean of a closed outline lies inside it, so the constant term is safe to fix at 1 and the system is linear), solved by the 5×5 normal equations with Gaussian elimination (`solve5`, as `solve3` with a pivot threshold relative to the matrix's scale). ok is false with fewer than six points, a singular system, a discriminant `b² − 4ac ≥ 0` (a parabola or hyperbola), or axes that are not finite and positive. The algebraic fit is biased on noisy short arcs; here the points are a refined smooth outline a fraction of a pixel off the curve all the way round, where the bias is far below the tolerance.

`(Ellipse) At(phi float64) geom.Vec2` is the parametric point; `(Ellipse) Dist(p geom.Vec2) float64` is the geometric distance, by Newton on the parametric angle from the angle of p in the ellipse's frame (`atan2(y/B, x/A)`), eight steps, which converges for points within a few per cent of the curve, the only points asked about.

### 4.2 The snap (`internal/refine/ellipse.go`)

`FitEllipse(b Bezigon, tol float64) (fit.Ellipse, bool)`: at least four segments; `circleSamples` points per segment; `fit.Ellipse`; refused when `A/B < ellipseMinRatio = 1.01` (a circle's job, which has already declined), when `B < ellipseMinAxis = 4` px, or when any sample is farther than `min(tol, circleRelTol·B)` from the ellipse, the same relative cap the circle uses so a small rounded box is not an ellipse at any scale.

`SnapEllipse(b, m, img, tol) (Bezigon, bool)`: as `SnapCircle`. Both axes are corrected by `chordFactor` for the refinement's polygon sag, then one scale factor on both axes is searched by golden section within `radiusSearch` against `fineDataEnergy` over the window of the loop and the largest candidate, and the result stands under `withinBudget`. The centre and the angle are not searched: the fit places them to a hundredth of a pixel on an outline this smooth, and the sag is a radial inflation.

`EllipseBezigon(e fit.Ellipse, start geom.Vec2, ccw bool) Bezigon`: the four-cubic circle mapped by the affine transform of the unit circle onto e, its first anchor at the ellipse point whose parametric angle is nearest start. Bézier curves are affine-invariant, so the circle approximation's relative error (2.7e-4 of the radius) carries over, bounded by 2.7e-4·A; an ellipse of A = 500 is within 0.14 px of true.

### 4.3 The hook (`trace.go:refineConstrained`)

For a free loop (no fixed anchor, no frozen segment), after `SnapCircle` declines, `SnapEllipse` runs; when it fires the loop is final and returns at once: the pieces pass, `CommonRadius` and `FitRectangle` would only cut its quadrants into arcs. A constrained loop is never an ellipse, as it is never a circle.

### 4.4 Synthetics and tests

`internal/corpus/synthetic.go` gains `ellipse(cx, cy, a, b, theta)` and the two shapes, with `EllipseAAGeometry()` and `EllipseRotGeometry()` returning centre and axes for the tests; `go run ./internal/corpus/gen` writes the PNGs; the goldens are new files. Tests: the fit on exact points (parameters within 1e-6, rejections for collinear and hyperbolic points, `Dist` on and off the curve), the snap on a nudged eight-cubic ellipse against the exact ellipse's mask (four cubics, axes within 0.15 px), refusals for a circle and a rounded rectangle, the two synthetics end to end, and the existing primitive tests unchanged.

## 5. Outcome (execution, 2026-11-14)

Both synthetics come out as four cubics within 0.2 px of the true ellipse (`ellipse-aa` 40.38 → 45.43 dB, `ellipse-rot` 43.37 → 43.87). The 21 real images are byte-identical under the pass: none has an ellipse loop, and the ring circles stay circles. Two refined goldens (`annular-sector`, `poster-discs`) differ in their last bits when regenerated on arm64 with or without this change, which is the known arch drift the quality comparison allows for; they were left as committed. Results: `bench/results/2026-11-14-ellipse.md`.

## 6. What is left to the builder

Names of the constants, the Newton step count, the exact window of the energy guard (follow the circle), and the choice of the golden-section bounds beyond following `radiusSearch`.

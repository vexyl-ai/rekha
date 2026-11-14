# Ellipse primitive implementation plan

**Goal:** a free loop that is an ellipse comes out of `--refine` as four cubics.
**Spec:** `docs/superpowers/specs/2026-11-14-ellipse-primitive-design.md`.
**Constraints:** Go 1.26, `CGO_ENABLED=0`, only `golang.org/x/image`; keyed `geom.Vec2` literals and `float64()`-wrapped products in `internal/fit` and `internal/refine`; pixel and polygon goldens byte-identical; gates `make lint test wasm-check wasm wasm-test web-test` and `GOARCH=amd64 go test ./ ./internal/gradient/`; DCO, no AI trailer.

### Task 1: `fit.Ellipse`
- Test: `internal/fit/ellipse_test.go` — `TestEllipseRecoversParameters` (72 points on centre (40,30), A 25, B 10, θ 0.6: centre within 1e-6, A and B within 1e-6, θ within 1e-6), `TestEllipseRejectsCollinear`, `TestEllipseRejectsHyperbola` (points on xy = 100), `TestEllipseDist` (a point on the curve 0 within 1e-9, centre + (A+1)·axis gives 1 within 1e-6).
- Code: `Ellipse`, `At`, `Dist`, `solveN` in `internal/fit/ellipse.go`.
- Run `go test ./internal/fit/`. Commit.

### Task 2: `refine.FitEllipse`, `EllipseBezigon`, `SnapEllipse`
- Test: `internal/refine/ellipse_test.go` — `TestEllipseBezigonStartAndOrientation` (N = 4, start nearest, signed area sign, every fine point within 0.03 of the ellipse at A = 17), `TestFitEllipseOnIdealEllipse`, `TestFitEllipseRejectsCircle` (ratio), `TestFitEllipseRejectsRoundedRect`, `TestSnapEllipseAcceptsNudgedEllipse` (mask `supersampled(EllipseBezigon)` A 100 B 60 θ 0.5 in a 256 window, nudged 8-cubic outline ±0.15 px → N 4, A and B within 0.15), `TestSnapEllipseRefusesOutsideTheWindow`.
- Code in `internal/refine/ellipse.go` with constants `ellipseMinRatio = 1.01`, `ellipseMinAxis = 4`.
- Run `go test ./internal/refine/`. Commit.

### Task 3: synthetics
- Test: `internal/corpus/synthetic_test.go` — `TestNewPrimitiveShapesRender` lists `ellipse-aa`, `ellipse-rot`; geometry helpers return (128,128), 100, 60, 0 and (128,128), 90, 50, π/6.
- Code: `ellipse(...)`, the two shapes in `All()`, `EllipseAAGeometry`, `EllipseRotGeometry`; `go run ./internal/corpus/gen -out testdata/corpus/synthetic`.
- Commit (code + PNGs).

### Task 4: the hook and the end-to-end tests
- Test: `refine_test.go` — `TestEllipsesAreFourCubics` over both synthetics (4 segments, none a line, every fine point within 0.2 px by `Dist` of the geometry helper's ellipse); `colour_test.go` `TestTraceGoldenRefine` gains both at `PresetBW`.
- Code: `trace.go:refineConstrained` as §4.3.
- `go test ./ -run 'TestTraceGolden' -update` for the new goldens only (no existing golden may change; check with `git status`).
- Commit.

### Task 5: bench, docs, gates
- `bench/results/2026-11-14-ellipse.md`: 21 images refined, main vs branch, plus the two synthetics' nodes and PSNR before and after.
- CHANGELOG Unreleased/Added, README primitives bullet, design doc outcome line.
- Both gates; commit; ff-merge to main; push.

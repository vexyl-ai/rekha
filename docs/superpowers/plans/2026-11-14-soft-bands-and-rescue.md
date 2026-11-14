# Soft bands and palette rescue — plan

Spec: `docs/superpowers/specs/2026-11-14-soft-bands-and-rescue-design.md`.
Inline execution, TDD, one commit per task, branch `region-soft-bands`.

Global constraints: Go 1.26, `CGO_ENABLED=0`, only `golang.org/x/image`;
pixel goldens byte-identical unless the image has a soft band or a
missing colour, with the PSNR change recorded; `make lint test
wasm-check wasm wasm-test web-test` and `GOARCH=amd64 go test ./
./internal/gradient/` before the merge.

## Task 1: soft-band test in the sliver pass

Files: `internal/region/cleanup.go`, `internal/region/softband.go`
(new: `softEdge(c *component, labels, rgb []uint8, w, h int, a, b
colour.OKLab) bool`), `internal/region/cleanup_test.go`.

1. Tests `TestCleanupAbsorbsSoftEdgeBand` (12×6 map: columns 0–3 label
   0 cyan, 4–7 label 2 with pixels mixed cyan→navy at t = 0.125, 0.375,
   0.625, 0.875, 8–11 label 1 navy; expect palette {cyan, navy} and
   columns 4–5 → 0, 6–7 → 1), `TestCleanupKeepsFlatStripe` (same map,
   the four columns all at the sRGB midpoint; expect three colours),
   `TestCleanupKeepsSoftBandAgainstThirdLabel` (band whose top three
   rows border a fourth label instead of navy; expect kept). Run: FAIL.
2. Implement `softEdge` and the branch; run: PASS; `go test ./internal/region/`.
3. Commit.

## Task 2: palette rescue

Files: `internal/region/rescue.go`, `internal/region/rescue_test.go`,
`segment.go`, `segment_test.go` or `rekha_test.go`.

1. Tests `TestRescueAddsMissingColour` (20×20 grey rgb with a 10×10
   blue square, labels all 0, palette {grey}; expect 2 entries, square
   labelled 1, entry within 0.01 of blue), `TestRescueIgnoresSmallBlob`
   (3×3 square; expect 1 entry), `TestRescueRelabelsToExistingColour`
   (palette {grey, blue-ish within tol}; expect 2 entries, square → 1),
   `TestSegmentFixedPaletteNotExtended` (Segment with Options.Palette
   of two colours on an image with a third; palette length ≤ 2). FAIL.
2. Implement; PASS; `go test ./internal/region/ ./`.
3. Wire into `segmentColour`; goldens: run `go test ./ -run Golden`,
   record any change with PSNR; commit.

## Task 3: bench and docs

Bench boy image and the 21 images (plain + refine) against main; write
`bench/results/2026-11-14-soft-bands.md`; CHANGELOG entry; README
pipeline sentence if it names the sliver pass; gates; merge; push.

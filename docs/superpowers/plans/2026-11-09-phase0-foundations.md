# Phase 0 Foundations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up the Rekha repository (Apache-2.0, pure Go, DCO) with the root `Options` API skeleton, a CLI skeleton, a synthetic test corpus, and a round-trip benchmark harness that scores any tracer (Potrace, VTracer, later Rekha) by PSNR, path count, node count, bytes and time.

**Architecture:** One Go module. Root package `rekha` exposes `Options` and `Validate`; `cmd/rekha` is the CLI; `internal/metrics`, `internal/corpus`, `internal/bench` are the benchmark pieces; `cmd/rekha-bench` drives them. External tracers are invoked as black-box subprocesses, their SVG is rasterised with `rsvg-convert`, and the result is compared pixel-wise to the input. No tracing code is written in this phase.

**Tech Stack:** Go 1.26, standard library, `golang.org/x/image/bmp` (BSD-3) for Potrace input, `rsvg-convert` (external, test-time only), GitHub Actions.

**Spec:** `design/roadmap.md` (Phase 0 row, product split, legal checklist), `Go SVG image tracing tool.md` (§"One Options struct", §"Testing", §"A five-phase plan").

## Global Constraints

- Module path `github.com/<OWNER>/rekha`; `<OWNER>` is `gh api user -q .login` at Task 1 time.
- `go.mod` says `go 1.26`. No toolchain directive.
- `CGO_ENABLED=0` everywhere. CI fails if any package needs cgo.
- Only third-party dependency allowed in this phase: `golang.org/x/image`.
- Everything under the module must build with `GOOS=js GOARCH=wasm` except `cmd/rekha-bench` and `internal/bench` (they shell out). Mark those files `//go:build !js`.
- Licence Apache-2.0. `LICENSE` is the verbatim Apache-2.0 text. `NOTICE` names the project and copyright year 2026.
- Every commit carries a DCO `Signed-off-by:` line (`git commit -s`).
- CLI exit codes: 0 success, 1 usage error, 2 any other error. `-` means stdin or stdout.
- Flag names follow VTracer vocabulary where meaning matches: `--preset`, `--mode`, `--filter-speckle`, `--color-precision`, `--path-precision`, `--max-colors`.
- All benchmark output ordering is deterministic: sort by image path, then tool name.
- No Potrace or gotrace source is read, copied or paraphrased. Potrace is used only as a binary.

## Review Focus

1. Corpus directory with no images, or with non-PNG files mixed in: harness must report "0 images" and skip non-PNG files by name, not crash. Test in Task 6.
2. Reference tool binary missing on PATH: harness must mark that tool `skipped: not installed` for every image and still produce the table. Test in Task 6.
3. `rsvg-convert` missing: harness must fail fast at startup with a message naming the binary and `brew install librsvg`, exit 2. Test in Task 6.
4. Rasterised SVG dimensions differ from input (tool emitted a viewBox with margins): PSNR must return an error, not index out of range, and the result row must show the error. Test in Task 4 (metrics) and Task 6 (row rendering).
5. Identical images (tool reproduced input exactly): PSNR is infinite; table must print `inf`, CSV must print `inf`, nothing must divide by zero. Test in Task 4.

---

### Task 1: Repository, licence, DCO, CI

**Files:**
- Create: `go.mod`, `LICENSE`, `NOTICE`, `README.md`, `CONTRIBUTING.md`, `DCO`, `.gitignore`, `Makefile`, `.github/workflows/ci.yml`
- Create: `doc.go` (package `rekha` doc comment only, so the module has a root package from the first commit)

**Interfaces:**
- Produces: module path used by every later import; `make test`, `make lint`, `make wasm-check` targets used by CI and by later tasks' verification steps.

- [ ] **Step 1: Resolve owner and initialise**

Run: `OWNER=$(gh api user -q .login); cd /Users/anilmathew/Documents/AICoding/Rekha && git init -b main && go mod init github.com/$OWNER/rekha`
Expected: `go.mod` exists with `module github.com/<OWNER>/rekha` and `go 1.26`.

- [ ] **Step 2: Write `LICENSE` (verbatim Apache-2.0 from https://www.apache.org/licenses/LICENSE-2.0.txt) and `NOTICE`**

`NOTICE` content, exactly:

```
Rekha
Copyright 2026 The Rekha Authors

This product includes software developed as an independent implementation.
It contains no code derived from Potrace or any GPL-licensed tracer.
```

- [ ] **Step 3: Write `DCO`** (verbatim Developer Certificate of Origin 1.1 from https://developercertificate.org/) **and `CONTRIBUTING.md`**

`CONTRIBUTING.md` must state: sign-off required (`git commit -s`); Apache-2.0 inbound = outbound; the clean-room rule "do not read, port or paraphrase Potrace, gotrace or any GPL tracer source; black-box binary comparison only"; link to `design/roadmap.md`.

- [ ] **Step 4: Write `README.md`**

Sections: one-paragraph description; status "Phase 0, not yet usable"; "Independent implementation" statement (algorithms from VTracer's published design, Schneider 1990, Yang et al. 2016; no Potrace-derived code); licence line; link to `design/` docs.

- [ ] **Step 5: Write `doc.go`**

```go
// Package rekha traces raster images into SVG.
package rekha
```

- [ ] **Step 6: Write `.gitignore`, `Makefile`, CI**

`.gitignore`: `/bin/`, `/bench/out/`, `*.wasm`, `.DS_Store`.

`Makefile` targets:
- `test`: `CGO_ENABLED=0 go test ./...`
- `lint`: `test -z "$$(gofmt -l .)" && go vet ./...`
- `wasm-check`: `CGO_ENABLED=0 GOOS=js GOARCH=wasm go build ./...`
- `build`: `CGO_ENABLED=0 go build -o bin/ ./cmd/...`

`.github/workflows/ci.yml`: on push and pull_request; ubuntu-latest; `actions/setup-go` with `go-version-file: go.mod`; runs `make lint test wasm-check`; env `CGO_ENABLED=0`.

- [ ] **Step 7: Verify**

Run: `make lint test wasm-check`
Expected: all three succeed; `go test` prints `ok` or `no test files` for `github.com/<OWNER>/rekha`.

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -s -m "chore: initialise Rekha under Apache-2.0 with DCO and CI"
```

---

### Task 2: Root `Options` and `Validate`

**Files:**
- Create: `options.go`, `options_test.go`

**Interfaces:**
- Produces:
  ```go
  type Mode string          // ModePixel = "pixel", ModePolygon = "polygon", ModeSpline = "spline"
  type Preset string        // PresetBW = "bw", PresetPoster = "poster", PresetPhoto = "photo"
  type Options struct {
      Preset         Preset
      Mode           Mode
      FilterSpeckle  int   // pixels; 0..1024
      ColorPrecision int   // bits; 1..8
      PathPrecision  int   // decimals; 0..8
      MaxColors      int   // 0 = unlimited (bw forces 2); 0..256
  }
  func DefaultOptions() Options            // Preset bw, Mode spline, FilterSpeckle 4, ColorPrecision 6, PathPrecision 2, MaxColors 0
  func (o Options) Validate() error        // returns *ValidationError
  type ValidationError struct{ Field, Msg string }
  func (e *ValidationError) Error() string // "<Field>: <Msg>"
  ```
- Consumed by: Task 3 CLI, later HTTP and WASM entry points.

- [ ] **Step 1: Write failing tests in `options_test.go`**

```go
func TestDefaultOptionsValid(t *testing.T) { if err := DefaultOptions().Validate(); err != nil { t.Fatal(err) } }

func TestValidateRejects(t *testing.T) {
    cases := []struct{ name string; mut func(*Options); field string }{
        {"preset", func(o *Options){ o.Preset = "neon" }, "Preset"},
        {"mode", func(o *Options){ o.Mode = "curvy" }, "Mode"},
        {"speckle-neg", func(o *Options){ o.FilterSpeckle = -1 }, "FilterSpeckle"},
        {"speckle-big", func(o *Options){ o.FilterSpeckle = 1025 }, "FilterSpeckle"},
        {"colorprec-0", func(o *Options){ o.ColorPrecision = 0 }, "ColorPrecision"},
        {"colorprec-9", func(o *Options){ o.ColorPrecision = 9 }, "ColorPrecision"},
        {"pathprec-9", func(o *Options){ o.PathPrecision = 9 }, "PathPrecision"},
        {"maxcolors-257", func(o *Options){ o.MaxColors = 257 }, "MaxColors"},
    }
    // each: o := DefaultOptions(); c.mut(&o); err := o.Validate(); require *ValidationError with Field == c.field
}

func TestValidationErrorMessage(t *testing.T) {
    e := &ValidationError{Field: "Mode", Msg: "unknown value \"curvy\""}
    if e.Error() != "Mode: unknown value \"curvy\"" { t.Fatal(e.Error()) }
}
```

- [ ] **Step 2: Run tests, expect compile failure**

Run: `go test ./ -run 'TestDefault|TestValidat' -v`
Expected: FAIL, undefined: DefaultOptions.

- [ ] **Step 3: Implement `options.go` per the Interfaces block**

Validate checks fields in struct order and returns the first failure. Use the exact range bounds from the struct comments.

- [ ] **Step 4: Run tests, expect pass**

Run: `go test ./ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add options.go options_test.go
git commit -s -m "feat: add Options struct with validation"
```

---

### Task 3: CLI skeleton `cmd/rekha`

**Files:**
- Create: `internal/cli/paths.go`, `internal/cli/paths_test.go`, `internal/cli/exit.go`, `cmd/rekha/main.go`

**Interfaces:**
- Consumes: `rekha.Options`, `rekha.DefaultOptions`, `Validate` from Task 2.
- Produces:
  ```go
  package cli
  const ( ExitOK = 0; ExitUsage = 1; ExitError = 2 )
  // OutputPath maps an input path to its output path. "-" maps to "-".
  // Otherwise the extension is replaced by ".svg"; a file with no extension gets ".svg" appended.
  // If explicit != "" it is returned unchanged (single-input case).
  func OutputPath(input, explicit string) string
  ```
  `cmd/rekha` flags: `--preset`, `--mode`, `--filter-speckle`, `--color-precision`, `--path-precision`, `--max-colors`, `-o/--output`, `--version`. Positional inputs. In Phase 0 the command validates options, computes output paths, prints `rekha: tracing not implemented yet (phase 0)` to stderr and exits 2. `--version` prints `rekha dev` and exits 0. No inputs exits 1 with usage.

- [ ] **Step 1: Write failing test `internal/cli/paths_test.go`**

```go
func TestOutputPath(t *testing.T) {
    cases := []struct{ in, explicit, want string }{
        {"-", "", "-"},
        {"logo.png", "", "logo.svg"},
        {"dir/photo.jpeg", "", "dir/photo.svg"},
        {"noext", "", "noext.svg"},
        {"a.png", "out/b.svg", "out/b.svg"},
        {"-", "x.svg", "x.svg"},
    }
    // assert OutputPath(c.in, c.explicit) == c.want
}
```

- [ ] **Step 2: Run, expect compile failure**

Run: `go test ./internal/cli/ -v`
Expected: FAIL, undefined: OutputPath.

- [ ] **Step 3: Implement `paths.go`, `exit.go`, `cmd/rekha/main.go`**

Use standard `flag`. Register each long flag; also register `-o` as alias of `--output`. Validation failure → print `rekha: <err>` to stderr, exit 1.

- [ ] **Step 4: Run tests and manual checks**

Run: `go test ./internal/cli/ -v && go run ./cmd/rekha --version; echo "exit=$?"; go run ./cmd/rekha; echo "exit=$?"; go run ./cmd/rekha --mode curvy x.png; echo "exit=$?"; go run ./cmd/rekha x.png; echo "exit=$?"`
Expected: PASS; `rekha dev` exit=0; usage exit=1; `rekha: Mode: unknown value "curvy"` exit=1; `rekha: tracing not implemented yet (phase 0)` exit=2.

- [ ] **Step 5: Commit**

```bash
git add internal/cli cmd/rekha
git commit -s -m "feat: add rekha CLI skeleton with exit codes and output path mapping"
```

---

### Task 4: Metrics package

**Files:**
- Create: `internal/metrics/psnr.go`, `internal/metrics/psnr_test.go`, `internal/metrics/svgstats.go`, `internal/metrics/svgstats_test.go`

**Interfaces:**
- Produces:
  ```go
  package metrics
  // PSNR compares a and b over RGB (alpha composited onto white), 8-bit, returns dB.
  // Returns +Inf when images are identical. Returns ErrDimensionMismatch when bounds differ.
  func PSNR(a, b image.Image) (float64, error)
  var ErrDimensionMismatch = errors.New("metrics: image dimensions differ")
  type SVGStats struct { Paths, Nodes, Bytes int }
  // Stats counts <path elements, and nodes = number of path-data command letters
  // (MmLlHhVvCcSsQqTtAaZz) across all d attributes. Bytes = len(svg).
  func Stats(svg []byte) SVGStats
  ```
- Consumed by: Task 6.

- [ ] **Step 1: Write failing tests**

`psnr_test.go`:
```go
func TestPSNRIdentical(t *testing.T)   // 8x8 solid red vs same → IsInf(+1)
func TestPSNRKnownValue(t *testing.T)  // 2x2 images, every channel differs by 16 → MSE 256 → 20*log10(255)-10*log10(256) ≈ 24.07; assert within 0.01
func TestPSNRDimensionMismatch(t *testing.T) // 8x8 vs 8x9 → errors.Is(err, ErrDimensionMismatch)
func TestPSNRAlphaOnWhite(t *testing.T) // fully transparent pixel vs opaque white → identical → +Inf
```

`svgstats_test.go`:
```go
func TestStatsCountsPathsAndNodes(t *testing.T) {
    svg := []byte(`<svg><path d="M0 0L1 1 2 2Z"/><path d="m1,1c1 1 2 2 3 3z"/></svg>`)
    // Paths 2; Nodes 6 (M L Z m c z; the implicit second L point is not a letter); Bytes len(svg)
}
func TestStatsEmpty(t *testing.T) // Stats([]byte("<svg/>")) == {0,0,6}
```

- [ ] **Step 2: Run, expect compile failure**

Run: `go test ./internal/metrics/ -v`

- [ ] **Step 3: Implement**

PSNR: convert both to `*image.NRGBA` via `draw.Draw` onto white background, iterate bytes, skip alpha channel, MSE over 3·W·H samples. Stats: `bytes.Count(svg, []byte("<path"))`; for nodes, find each `d="` … `"` (and `d='`) and count bytes in the command-letter set. Use `bytes` and `strings` only, no XML parser.

- [ ] **Step 4: Run, expect pass**

Run: `go test ./internal/metrics/ -v`

- [ ] **Step 5: Commit**

```bash
git add internal/metrics
git commit -s -m "feat: add PSNR and SVG stats metrics"
```

---

### Task 5: Synthetic corpus generator and corpus layout

**Files:**
- Create: `internal/corpus/synthetic.go`, `internal/corpus/synthetic_test.go`, `internal/corpus/gen/main.go`, `testdata/corpus/README.md`, `testdata/corpus/synthetic/.gitkeep`, and empty category dirs each with `SOURCES.md`: `line-art`, `logos`, `pixel-art`, `posterised`, `photos`, `pathological`

**Interfaces:**
- Produces:
  ```go
  package corpus
  type Synthetic struct { Name string; Draw func(*image.NRGBA) }
  // All returns the fixed list below, in this order.
  func All() []Synthetic
  // Render draws s onto a 256x256 white NRGBA.
  func Render(s Synthetic) *image.NRGBA
  ```
  Fixed list (names are file stems): `circle-aa` (anti-aliased filled disc r=96, black), `circle-hard` (same, no AA), `rounded-rect` (r=24, black), `star-5` (5-point star, black), `two-colour-abut` (left half red, right half blue, hard edge), `three-layer-stack` (blue square, red disc on it, white small square on that), `checkerboard-2px` (pathological 2x2 alternation), `thin-lines` (1px black lines, horizontal, vertical, diagonal), `gradient-h` (horizontal black→white, out of scope for tracing but tests graceful handling), `noisy-disc` (disc with deterministic ±24 pseudo-random noise, seed 1).
  AA via 4x4 supersampling implemented in the package; no external dependency.
- `go run ./internal/corpus/gen -out testdata/corpus/synthetic` writes `<name>.png` for each.

- [ ] **Step 1: Write failing tests**

```go
func TestAllNamesUnique(t *testing.T)
func TestRenderDeterministic(t *testing.T) // Render each twice, compare pixel bytes equal
func TestRenderSize(t *testing.T)          // bounds 256x256
func TestCircleAAHasFractionalEdge(t *testing.T) // some pixel in circle-aa has R strictly between 0 and 255; circle-hard has none
```

- [ ] **Step 2: Run, expect compile failure**

Run: `go test ./internal/corpus/ -v`

- [ ] **Step 3: Implement `synthetic.go` and `gen/main.go`**

Noise: `math/rand/v2` `rand.New(rand.NewPCG(1, 1))` so bytes never change across Go versions (PCG is specified).

- [ ] **Step 4: Write `testdata/corpus/README.md`**

Content: category list with one-line definition each; provenance rule "only CC0, public-domain or author-owned images; every category has `SOURCES.md` listing file, origin URL, licence"; note that `synthetic/` is regenerated by `go run ./internal/corpus/gen -out testdata/corpus/synthetic` and that PNG bytes are not golden (compare pixels, never files).

- [ ] **Step 5: Run tests, generate corpus, verify**

Run: `go test ./internal/corpus/ -v && go run ./internal/corpus/gen -out testdata/corpus/synthetic && ls testdata/corpus/synthetic | wc -l`
Expected: PASS; `10`.

- [ ] **Step 6: Commit**

```bash
git add internal/corpus testdata/corpus
git commit -s -m "feat: add synthetic corpus generator and corpus layout"
```

---

### Task 6: Benchmark runner

**Files:**
- Create: `internal/bench/tool.go`, `internal/bench/raster.go`, `internal/bench/run.go`, `internal/bench/report.go`, `internal/bench/run_test.go`, `internal/bench/testdata/fixture.svg`
- All `.go` files in `internal/bench` carry `//go:build !js`.

**Interfaces:**
- Consumes: `metrics.PSNR`, `metrics.Stats`, `metrics.ErrDimensionMismatch` (Task 4).
- Produces:
  ```go
  package bench
  type Tool struct {
      Name   string
      Bin    string            // looked up with exec.LookPath
      Args   []string          // tokens; "{in}" and "{out}" are substituted
      Input  string            // "png" or "bmp"; bmp inputs are converted with golang.org/x/image/bmp
  }
  func BuiltinTools() []Tool   // potrace: Bin "potrace", Args ["--svg","-o","{out}","{in}"], Input "bmp"
                               // vtracer: Bin "vtracer", Args ["--input","{in}","--output","{out}"], Input "png"
  type Result struct {
      Image, Tool string
      PSNR        float64      // NaN when Err != nil
      Stats       metrics.SVGStats
      Millis      int64
      Skipped     string       // non-empty: "not installed"
      Err         string       // non-empty on failure
  }
  type Runner struct {
      Rasterize func(ctx context.Context, svgPath string, w, h int) (image.Image, error) // default: RSVGConvert
      Timeout   time.Duration // default 60s per tool invocation
      WorkDir   string        // scratch; default os.MkdirTemp
  }
  func RSVGConvert(ctx context.Context, svgPath string, w, h int) (image.Image, error) // runs rsvg-convert -w w -h h svgPath -o tmp.png
  func CheckRasterizer() error // exec.LookPath("rsvg-convert"); error text includes "brew install librsvg"
  // Run walks corpusDir recursively for *.png (case-insensitive), sorted; for each tool in order.
  func (r *Runner) Run(ctx context.Context, corpusDir string, tools []Tool) ([]Result, error)
  func WriteMarkdown(w io.Writer, rs []Result) error  // table: Image | Tool | PSNR dB | Paths | Nodes | Bytes | ms | Note
  func WriteCSV(w io.Writer, rs []Result) error       // header image,tool,psnr,paths,nodes,bytes,ms,skipped,err
  ```
  Formatting: PSNR `%.2f`, `+Inf` printed as `inf`, NaN printed as empty cell. Results sorted by Image then Tool.

- [ ] **Step 1: Write `testdata/fixture.svg`**: a 256x256 SVG with one black `<path d="M0 0H256V256H0Z"/>` (full black square).

- [ ] **Step 2: Write failing tests `run_test.go`**

```go
func TestRunSkipsMissingTool(t *testing.T)     // Tool{Bin:"definitely-not-a-binary-xyz"} → every Result.Skipped == "not installed", no error
func TestRunIgnoresNonPNG(t *testing.T)        // dir with a.png + b.txt + c.jpg → results only for a.png
func TestRunEmptyDir(t *testing.T)             // → len(results)==0, err==nil
func TestRunFakeToolEndToEnd(t *testing.T)     // Tool{Bin:"cp", Args:["testdata/fixture.svg","{out}"]} (test cwd is the package dir); Rasterize stubbed to return a 256x256 black image; input a 256x256 black PNG → PSNR +Inf, Paths 1, Nodes 5, Skipped=="" , Err==""
func TestRunDimensionMismatchIsRowError(t *testing.T) // Rasterize stub returns 100x100 → Err contains "dimensions differ", PSNR NaN
func TestRunToolTimeout(t *testing.T)          // Tool{Bin:"sleep", Args:["5"]}, Timeout 100ms → Err non-empty, Millis < 2000
func TestWriteMarkdownFormats(t *testing.T)    // one +Inf row, one NaN-with-Err row → contains "| inf |" and the err text
func TestWriteCSVHeader(t *testing.T)
func TestCheckRasterizerMessage(t *testing.T)  // temporarily set PATH="" → error contains "rsvg-convert" and "brew install librsvg"
```

- [ ] **Step 3: Run, expect compile failure**

Run: `go test ./internal/bench/ -v`

- [ ] **Step 4: Implement the four files per Interfaces**

Add `golang.org/x/image` with `go get golang.org/x/image@latest` (note the resolved version in the commit message). BMP conversion: decode PNG, `draw` onto white `*image.RGBA`, `bmp.Encode`. Tool invocation: `exec.CommandContext` with per-call `context.WithTimeout(ctx, r.Timeout)`; capture stderr into `Err` on non-zero exit. Measure `Millis` around the tool process only, not the rasteriser.

- [ ] **Step 5: Run, expect pass**

Run: `go test ./internal/bench/ -v && make wasm-check`
Expected: PASS; wasm-check still succeeds because of build tags.

- [ ] **Step 6: Commit**

```bash
git add internal/bench go.mod go.sum
git commit -s -m "feat: add black-box benchmark runner with PSNR and SVG stats"
```

---

### Task 7: `cmd/rekha-bench`, reference tools, first baseline

**Files:**
- Create: `cmd/rekha-bench/main.go` (`//go:build !js`), `docs/benchmark.md`, `bench/results/2026-11-09-baseline.md`
- Modify: `Makefile` (add `bench` target), `README.md` (link docs/benchmark.md)

**Interfaces:**
- Consumes: everything in Task 6.
- CLI: `rekha-bench -corpus testdata/corpus -tools potrace,vtracer -out bench/out` writes `bench/out/results.md` and `bench/out/results.csv` and prints the markdown table to stdout. `-tools` defaults to all builtin. Exit 2 if `CheckRasterizer` fails, 1 on bad flags, 0 otherwise even when some rows have errors.

- [ ] **Step 1: Install reference tools**

Run: `brew install potrace && (brew install vtracer || cargo install vtracer)`
Expected: `potrace --version` prints 1.16; `vtracer --version` prints a version. If `vtracer` cannot be installed, note it in `docs/benchmark.md` and continue; the harness skips it.

- [ ] **Step 2: Write `cmd/rekha-bench/main.go`** per the CLI contract above.

- [ ] **Step 3: Write `docs/benchmark.md`**

Sections: what is measured and how (PSNR against input after rasterising with rsvg-convert at input size; path, node, byte counts; wall time of tool process only); install commands (`brew install librsvg potrace`, vtracer install line that worked); how to run; how to add a tool (`Tool` struct fields); Vector Magic manual procedure (upload to vectormagic.com, download preview SVG when available, drop into `bench/manual/<image>/vectormagic.svg`, run with `-tools manual` — **not implemented in this phase; list as follow-up**); caveat that PSNR rewards fidelity not editability.

- [ ] **Step 4: Add `bench` Makefile target**

`bench: build` → `./bin/rekha-bench -corpus testdata/corpus -out bench/out`

- [ ] **Step 5: Run the baseline**

Run: `make bench && cp bench/out/results.md bench/results/2026-11-09-baseline.md && cat bench/results/2026-11-09-baseline.md`
Expected: table with 10 synthetic images × installed tools; potrace rows have numeric PSNR on bilevel images; vtracer rows numeric or `not installed`.

- [ ] **Step 6: Verify full suite**

Run: `make lint test wasm-check`
Expected: all pass.

- [ ] **Step 7: Commit**

```bash
git add cmd/rekha-bench docs/benchmark.md bench/results Makefile README.md
git commit -s -m "feat: add rekha-bench command and record baseline against reference tools"
```

---

### Task 8: GitHub remote

**Files:** none new.

- [ ] **Step 1: Create the public repository and push**

Run: `gh repo create rekha --public --source=. --description "Pure-Go raster to SVG tracer" --push`
Expected: remote `origin` set, `main` pushed, CI run starts.

- [ ] **Step 2: Confirm CI green**

Run: `gh run watch --exit-status`
Expected: exit 0.

---

## Deferred to Phase 0 follow-ups (not in this plan)

- Manual-result tool (`-tools manual`) for Vector Magic previews.
- Populating `line-art`, `logos`, `pixel-art`, `posterised`, `photos`, `pathological` with CC0 images and `SOURCES.md` entries.
- Lawyer consult: items are listed in `design/roadmap.md` §Legal checklist; no code.
- Trademark search for the name.

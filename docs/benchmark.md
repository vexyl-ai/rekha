# Benchmark harness

`rekha-bench` runs tracers as black-box subprocesses over `testdata/corpus`, rasterises each SVG back to pixels, and scores it against the input. It is how every quality claim in this project is checked.

## What is measured

| Column | Meaning |
|---|---|
| PSNR dB | Peak signal-to-noise ratio between the input PNG and the SVG rasterised at the input's size with `rsvg-convert`. Both are composited onto white; alpha is ignored after that. `inf` = pixel-identical. |
| Paths | Number of `<path` elements. |
| Nodes | Number of path segments across all `d` attributes, counting implicit repeats (`l1 1 2 2` is two segments, `M0 0 1 1` is a move plus one line) and `Z` as one each. Comparable across tools regardless of how compactly they write path data. A proxy for editability. |
| Bytes | SVG file size. |
| ms | Wall time of the tracer process only. Rasterisation and scoring are not included. |
| Note | `not installed` when the tool binary is missing, `no reference` when a reference directory has no file for the image, otherwise the tool's error. |

PSNR rewards fidelity to the raster, not editability or aesthetics. A tracer that emits one path per pixel scores perfectly. Read PSNR together with Paths and Nodes, and look at the output.

## Install

```
brew install librsvg potrace
```

`rsvg-convert` (from librsvg) is required; the harness exits with code 2 without it. Potrace is a reference tool; missing tools are skipped per row.

Pin the reference versions when recording a baseline: `potrace --version`, `vtracer --version`.

**VTracer** has no Homebrew formula and needs either a Rust toolchain (`brew install rust && cargo install vtracer`) or the prebuilt binary from the [GitHub releases page](https://github.com/visioncortex/vtracer/releases) (`vtracer-aarch64-apple-darwin.tar.gz` on Apple Silicon). Put the `vtracer` binary on `PATH`. The 2026-11-09 baseline was recorded without it.

Potrace and VTracer are used only as binaries. Do not read their source; see `CONTRIBUTING.md`.

## Run

```
make bench                                  # all builtin tools plus bin/rekha, writes bench/out/
./bin/rekha-bench -tools potrace            # one tool
./bin/rekha-bench -tools rekha,potrace      # rekha against one reference
./bin/rekha-bench -tools rekha-poster,vtracer  # colour preset against VTracer
./bin/rekha-bench -tools rekha,rekha-refine    # what refinement adds
./bin/rekha-bench -rekha /path/to/rekha     # benchmark another rekha build
./bin/rekha-bench -corpus path/to/images    # another corpus
```

Rekha itself is always included (`-rekha` names the binary, default `bin/rekha`, which `make bench` builds first) as six tools: `rekha` (`--preset bw`), `rekha-poster` and `rekha-photo`, each run as `rekha --preset <p> -o {out} {in}`, plus `rekha-refine` (`--preset bw --refine`), `rekha-poster-refine` (`--preset poster --refine`) and `rekha-photo-gradients` (`--preset photo --gradients`), the same black-box treatment the reference tools get. Compare `rekha` with Potrace on bilevel images, `rekha-poster`/`rekha-photo` with VTracer on colour images, each `-refine` tool with its plain counterpart for what the refinement stage adds, and `rekha-photo-gradients` with `rekha-photo` for what gradient fills change (`gradient-h`, `gradient-shapes`).

Outputs `bench/out/results.md` and `bench/out/results.csv` and prints the markdown table. Rows are sorted by image path then tool name, and error notes never contain scratch paths, so two runs differ only in the `ms` column.

Recorded baselines live in `bench/results/<date>-<label>.md`. Copy `bench/out/results.md` there when a run is worth keeping.

## Add a tool

Edit `BuiltinTools` in `internal/bench/tool.go`:

```go
{Name: "mytool", Bin: "mytool", Args: []string{"{in}", "-o", "{out}"}, Input: "png"}
```

- `Bin` is looked up on `PATH`.
- `{in}` and `{out}` are replaced with the prepared input path and the SVG output path.
- `Input` is `png` or `bmp`. BMP inputs are composited onto white because BMP has no alpha.

Rekha is not in `BuiltinTools`; it comes from `RekhaTools(bin)` so the binary path can be configured.

A tool with `Dir` instead of `Bin` runs nothing and scores files already in that directory (next section).

## Reference tools without a CLI (Vector Magic, Vectorizer.AI)

Vector Magic and Vectorizer.AI are web services. The harness scores their outputs from files you download by hand into `bench/reference/vectormagic/` and `bench/reference/vectorizer-ai/`; only the READMEs there are committed, the outputs are git-ignored.

Procedure, once per corpus image:

1. Upload `testdata/corpus/<category>/<name>.png` at [vectormagic.com](https://vectormagic.com) or [vectorizer.ai](https://vectorizer.ai) with the service's default settings. Note the settings and the date in the directory's README.
2. Download the SVG and save it as `bench/reference/<tool>/<name>.svg` (the image's base name, no category). With only a preview available, save the preview as `<name>.png` at the input's exact size instead.
3. `make bench` (or `./bin/rekha-bench -tools rekha-refine,vectormagic,vectorizer-ai`).

An SVG is rasterised and scored like any tool's output: PSNR, Paths, Nodes, Bytes, with `ms` 0 because nothing ran. A PNG gives PSNR only, and a PNG of the wrong size is a row error. An image with neither file gets the note `no reference`, so a partial download still produces a table.

Downloads are for measurement only. Do not commit them, and do not trace them: the outputs are the services' work, not the corpus.

## Latency in the browser

Speed of the WebAssembly build against native, per preset and image, is a separate table: `REKHA_LATENCY=1 go test -run TestReport -v ./internal/latency` natively and `REKHA_LATENCY=1 make wasm-test ARGS="-run TestWasmLatency -v"` under Node, recorded in `bench/results/2026-11-10-phase4-wasm.md`.

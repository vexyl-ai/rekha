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
| Note | `not installed` when the tool binary is missing, otherwise the tool's error. |

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

Rekha itself is always included (`-rekha` names the binary, default `bin/rekha`, which `make bench` builds first) as five tools: `rekha` (`--preset bw`), `rekha-poster` and `rekha-photo`, each run as `rekha --preset <p> -o {out} {in}`, plus `rekha-refine` (`--preset bw --refine`) and `rekha-poster-refine` (`--preset poster --refine`), the same black-box treatment the reference tools get. Compare `rekha` with Potrace on bilevel images, `rekha-poster`/`rekha-photo` with VTracer on colour images, and each `-refine` tool with its plain counterpart for what the refinement stage adds.

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

## Vector Magic (manual, follow-up)

Vector Magic has no CLI. Planned procedure, **not implemented yet**: upload each corpus image at vectormagic.com, download the SVG, save as `bench/manual/<image-stem>/vectormagic.svg`, then run with `-tools manual` to score the saved files alongside the automated tools.

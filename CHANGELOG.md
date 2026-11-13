# Changelog

Notable changes to Rekha. Versions are tags on `main`; the release
pipeline builds every artefact from the tag.

## Unreleased

### Added

- `--scale` (library `Options.Scale`, API and wasm key `scale`): the
  image is this many times the size the output is for. The SVG's width
  and height are the image size divided by it with the viewBox at image
  size; PDF and EPS pages follow. Convert keeps it. For an image
  upscaled before tracing to keep thin lines.

## v0.2.0

### Added

- `--auto` (library `rekha.Auto`, `Options.Auto`; API and wasm key
  `auto`; a checkbox in the playground): traces the image under four
  candidate settings, poster at tolerance 0.06, poster at 0.04 with
  gradients, photo with gradients, and bw with the automatic threshold,
  all refined, renders each with a rasteriser built in and keeps the
  closest to the image. A `--palette` adds a fifth candidate. Candidates
  run at once as far as memory allows. On twenty mixed images: 1.7 dB
  above the fixed poster setting, fifteen wins, one loss of half a
  decibel (`bench/results/2026-11-13-auto.md`).
- PDF and EPS output (`--format`, or the output extension) from the same
  paths and gradients, and conversion of an SVG Rekha wrote, edited or
  not, into either; the playground downloads both.
- `Options.ExternalMasks`: regions decided by the caller (a segmentation
  model's objects, a matte, a selection) that replace the quantiser's
  verdict where they cover the image, with soft edges fed to the
  refinement and a cut-out option.
- Playground edits before download: select shapes, delete, recolour,
  remove the background, undo and redo.
- A gallery of traced logos on the site, with wipes and the SVGs.
- Signed apt and dnf repositories on GitHub Pages, and deb, rpm and apk
  packages on Cloudsmith.

### Changed

- JPEG chroma is interpolated (libjpeg's triangle filter) rather than
  replicated per block: 4:2:0 files trace with 20 to 30 % fewer paths.
- A speck on the border between two regions goes to the one its colour
  leans to, not to the one earlier in the palette. Eight goldens changed,
  seven for the better; identical images decoded a level apart now trace
  alike (they could differ by 2.8 dB).
- Export formats draw the open-core line: SVG, PDF and EPS are open;
  DXF, HPGL, G-code, AI and layered PDF are commercial.

## v0.1.2

Release pipeline: Homebrew cask, Scoop bucket, container image, Linux
packages; documentation fixes. The tracer is as in v0.1.0.

## v0.1.0

First release: bilevel and colour tracing with the bw, poster and photo
presets, curve refinement against the anti-aliasing, primitives,
gradient fills (linear, radial, layered), the CLI, the HTTP API, the
WebAssembly build and the playground.

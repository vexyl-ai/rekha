# Changelog

Notable changes to Rekha. Versions are tags on `main`; the release
pipeline builds every artefact from the tag.

## Unreleased

### Fixed

- PDF: a translucent gradient overlay (a layered gradient's soft-masked
  layer) painted far too strong, since the soft mask's ExtGState was
  set after the page matrix and the mask's form went through that
  matrix twice. The state is now set first, inside each path's own
  `q … Q`. On a 1200×2198 render with 14 overlays the PDF now matches
  the SVG at 42 dB (poppler) where it was 37 dB, with the hair no
  longer darkened.

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
- `--scale` (library `Options.Scale`, API and wasm key `scale`): the
  image is this many times the size the output is for. The SVG's width
  and height are the image size divided by it with the viewBox at image
  size; PDF and EPS pages follow. Convert keeps it. For an image
  upscaled before tracing to keep thin lines.
- `server` package: the HTTP handler behind `rekha-serve`, importable
  with `server.New`, a `WithTrace` hook that sees the decoded image,
  options and extra request fields and decides what is written, and
  `WithFields` naming those fields. `rekha-serve` behaves as before.
- `apiopts` package: the option parser behind the API and the
  playground (`Parse` for JSON, `FromStrings` for form fields), public so
  an embedder reads options as text the way the service does.
- `server.WithFormat`: a wrapper registers an output format by name,
  media type and writer; the server traces the request and the wrapper
  writes the body. The open build registers none.

### Changed

- JPEG chroma is interpolated (libjpeg's triangle filter) rather than
  replicated per block: 4:2:0 files trace with 20 to 30 % fewer paths.
- A speck on the border between two regions goes to the one its colour
  leans to, not to the one earlier in the palette. Eight goldens changed,
  seven for the better; identical images decoded a level apart now trace
  alike (they could differ by 2.8 dB).
- Export formats draw the open-core line: SVG, PDF and EPS are open;
  DXF, HPGL, G-code, AI and layered PDF are commercial.
- Geometric primitives, second pass. `--refine` now cuts each outline
  into line, arc and free pieces by split-and-fit before it snaps, types
  each arc's ends by the joint they meet (tangent to a line, a point on a
  corner, free), and sharpens the fillet the refinement leaves at a sharp
  corner into one node. Half-discs, pie wedges and concentric bands come
  out as exact arcs ending on their straight edges; a quarter circle
  beside a line is one arc cubic tangent to it; a corner between a
  straight edge and a curve is placed where the line meets the circle at
  trace time too. The guard that admits a snap gains an absolute floor
  under a relative cap, so a near-perfect circle may be snapped although
  the free cubics fitted the image's 0.1 px imperfections a little
  better. Half-disc refined goes from 8 nodes at 44.1 dB to 3 at 51.1,
  annular-sector 11 to 6 (49.6 to 51.9 dB), the Pokéball logo 105 to 53
  nodes with 73 % of its curves exact arcs; the 21-image bench is in
  `bench/results/2026-11-14-primitives-v2.md` (mean −0.04 dB; the
  Pokéball loses 0.78 dB at 42 dB because its rings are 0.15 px off
  circular in the source and an exact circle cannot follow that).
  Refined goldens are now compared as quality (same paths and fills,
  node count within a fifth, rendering within 1 dB of the source), since
  the primitive pass's inputs differ in the last bits between arm64 and
  amd64.
- The Wu quantiser judges a colour group, not only a single cell, when
  it asks whether a box hides a subject, so a small object shaded over
  several cells keeps a palette entry at the poster tolerance (a
  cartoon's skin and orange shoes were each merged into a grey). A
  group counts only above a 512th of the image, a subject group's halo
  is taken around the whole group, and a far group with a matching cap
  opposite it is read as noise, so grainy fields and plain ramps trace
  as before. Three goldens change, `gradient-shapes-poster-*`, each
  about 1.9 dB closer to the image with one more fill (the bright end
  of one ramp crossing into the white box); the 21-image bench is in
  `bench/results/2026-11-14-wu-groups.md` (mean +0.51 dB, no losses).

## v0.1.2

Release pipeline: Homebrew cask, Scoop bucket, container image, Linux
packages; documentation fixes. The tracer is as in v0.1.0.

## v0.1.0

First release: bilevel and colour tracing with the bw, poster and photo
presets, curve refinement against the anti-aliasing, primitives,
gradient fills (linear, radial, layered), the CLI, the HTTP API, the
WebAssembly build and the playground.

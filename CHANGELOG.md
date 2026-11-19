# Changelog

Notable changes to Rekha. Versions are tags on `main`; the release
pipeline builds every artefact from the tag.

## Unreleased

### Changed

- A lone band (a region no neighbour joined) gets what a joined group
  already had: the slack of `slackTol` on its linear fit, and when that
  misses, a try about its centre and as a layered ramp, with the planes'
  direction rather than the tensor's (a lone band is judged on its
  linear fit, not on a bend). A band that no model reproduces stays
  flat as before. On the white wolf the soft greys of the cheek and the
  thin strands of the fur had each quantised into one band whose shading
  ran two ways inside it; they are ramps now. Photo + gradients 29.10 →
  29.26 dB, 46 → 111 gradients; auto 31.29 → 31.59 dB, 486 → 365 kB (a
  ramp a band costs fewer paths than the flat steps it replaces); nine
  other logos byte-identical; every golden byte-identical.
  `bench/results/2026-11-19-lone-bands.md`.

## v0.2.3

### Changed

- Gradient merging gives a small region slack: a region of under an
  eighth of the image may miss its ramp by up to twice
  `--gradient-tolerance`, growing as the square root of the inverse
  share (`slackTol` in `internal/gradient`), since its error is a small
  share of the image's while the seam between two flat bands is a
  visible step whatever its size. The page and a subject's body are
  judged at the tolerance as before. On a white wolf logo whose fur was
  shaded in soft greys, the shading under the chin and down the neck
  had quantised into flat bands with hard steps that no one model
  reproduced within 0.03; it is now one ramp a step. Auto 31.51 →
  31.29 dB, 517 → 486 kB; nine other logos byte-identical; every golden
  byte-identical. `bench/results/2026-11-19-slack.md`.

### Fixed

- The quantiser took the width of a colour cell from the spacing of the
  cells an image occupies. On an image of a few flat colours and no
  anti-aliasing that spacing is the gap between two real colours, so a
  band lying near the mixture line from its colour to a black outline
  (every darker shade of a colour does) counted its own parent as its
  halo and was never promoted: a band of 12 % of the image, two and a
  half tolerances from the skin around it, was lost. The width now comes
  from the colour precision. Every golden is byte-identical.

## v0.2.2

### Added

- `--flat-lightness` (`Options.FlatLightness`, API and wasm `flatLightness`,
  a field in the playground): how far apart in OKLab lightness two
  adjacent shades of one hue may be and still merge under `--flatten`,
  default the 0.12 the flat preset used. On the boy render, refined:

  | flat-lightness | dB | paths | fills |
  |---|---|---|---|
  | 0.03 | 31.45 | 714 | 43 |
  | 0.05 | 31.65 | 515 | 39 |
  | 0.08 | 30.07 | 389 | 26 |
  | 0.12 | 28.58 | 331 | 20 |

  At 0.05 the hair's and the hoodie's shade steps stay as their own flat
  tones at twice the fills; a shadow softer than the colour tolerance
  (the neck's) is merged by the quantiser before Flatten runs and is a
  gradients matter, not this knob's (`bench/results/2026-11-14-flat-lightness.md`).

## v0.2.1

### Added

- Ellipses. Under `--refine` a free loop that is an ellipse (every
  sample of its outline within tolerance of the least-squares ellipse,
  the axes at least 1 % apart so a circle stays a circle, the short
  semi-axis at least 4 px) becomes four cubics, the four-cubic circle
  under the affine map onto the ellipse, after the circle snap declines
  and under the same energy guard with the axes refitted against the
  image; an ellipse loop is final, so the pieces pass does not cut its
  quadrants into arcs. Two new synthetics, `ellipse-aa` (semi-axes 100
  and 60) and `ellipse-rot` (90 and 50 at 30°), trace as exactly four
  cubics within 0.2 px of the true ellipse where the free refinement
  left them 0.3 px off; `bench/results/2026-11-14-ellipse.md` has the
  21-image refined bench, on which nothing changes: no logo there has an
  ellipse, and the Pokéball's rings stay circles. Rounded rectangles were
  already covered (`CommonRadius`); symmetry is deferred, with the
  reasons in the design doc.
- `--preset flat` (`PresetFlat`, API and wasm `"flat"`), the posterised
  look: the photo preset's colours (tolerance 0.04, so every hue
  survives the quantiser), then adjacent shades of one hue merged into
  one flat colour (`Options.Flatten`, `--flatten`, API `flatten`, which
  any colour preset may ask for): lightness within 0.12, hue within 23°,
  a neutral never merged with a chromatic colour, a merged group taking
  its largest member's colour, not a pixel mean (a logo's brand red
  stays its own red when its shade joins it), chains judged against
  that colour. The boy render comes out in 20 fills and 331 paths with the iris and the strap brown; raising the poster tolerance
  instead merges hues before shades (at 0.10 the skin is grey). A flat
  look costs PSNR by design; `bench/results/2026-11-14-flat.md` has the
  21 images against the poster at 0.06. One new golden,
  `gradient-shapes-flat-spline`.

### Changed

- A region whose pixels the quantiser gave no colour of their own gets
  one after cleanup (`region.Rescue`): a component of at least sixteen
  pixels whose mean is farther than the tolerance from its own palette
  entry moves to an existing entry within tolerance, or adds its mean as
  a new one, when the mean fits a third of its pixels or no entry does;
  anti-aliased glyph strokes, whose mean is a grey no pixel has while
  black fits most of them, keep black. The badge on a 2.6-megapixel
  cartoon render goes from grey to its light blue, and a cream rim light
  on the cheek stops being white; the image gains 0.29 dB with 258 fewer
  paths. Fixed palettes are never extended. One golden changes,
  `gradient-shapes-poster-*`, +0.18 dB at the same path count (a grey
  box labelled pink gets its grey); the 21-image bench is in
  `bench/results/2026-11-14-rescue.md`.

### Fixed

- `rekha.Auto` called `Options.Progress` from several candidate
  goroutines at once; it now serialises the calls.
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

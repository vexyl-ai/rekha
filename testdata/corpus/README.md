# Benchmark corpus

Images used by `rekha-bench` (see `docs/benchmark.md`). One directory per category.

| Category | Definition |
|---|---|
| `synthetic/` | Generated shapes with known geometry. Regenerate with `go run ./internal/corpus/gen -out testdata/corpus/synthetic`. |
| `line-art/` | Black-and-white drawings, scans, signatures. Bilevel input. |
| `logos/` | Flat-colour logos and icons with few colours and sharp corners. |
| `pixel-art/` | Small images where every pixel is intentional; nearest-neighbour semantics. |
| `posterised/` | Illustrations with a limited palette and anti-aliased edges. |
| `photos/` | Photographs and continuous-tone images. Out of scope for quality targets; must not crash. |
| `pathological/` | Inputs designed to break tracers: checkerboards, 1 px lines, huge dimensions, extreme aspect ratios. |

## Provenance rule

Only CC0, public-domain, or author-owned images may be added. Every category directory has a `SOURCES.md` listing, for each file: filename, origin URL, licence. A file without a `SOURCES.md` entry is removed.

## PNG bytes are not golden

Go's `compress/flate` output changes between Go versions, so PNG files produced by the generator may differ byte-for-byte across toolchains. Compare decoded pixels, never files. The generator's determinism test (`internal/corpus`) checks pixels.

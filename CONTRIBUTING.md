# Contributing to Rekha

## Sign-off required

Every commit must carry a `Signed-off-by:` line certifying the [Developer Certificate of Origin](DCO). Use `git commit -s`. Commits without sign-off are not merged.

## Licence

Inbound = outbound. By contributing you agree your contribution is licensed under the [Apache License 2.0](LICENSE), the same licence as the project.

## Clean-room rule

Rekha is an independent implementation. **Do not read, port, translate or paraphrase the source code of Potrace, gotrace, or any other GPL-licensed tracer** when working on this project. Those tools may be used only as black-box binaries for benchmark comparison (see `docs/benchmark.md`).

Algorithms are taken from published papers and permissively licensed designs: VTracer's published pipeline (MIT/Apache-2.0), Schneider's curve fitting (Graphics Gems, 1990), Yang et al. direct bezigon optimisation (TVCG 2016). Cite the paper section in a code comment or design note when implementing from one.

Do not prompt an AI assistant with GPL tracer source either. Review generated code for resemblance to Potrace structure or identifiers before committing.

## Where things are decided

- `design/roadmap.md`: phases, product split, legal checklist.
- `design/refine-bezigon-optimisation.md`: refinement stage design.
- `docs/superpowers/plans/`: implementation plans per phase.

## Development

```
make lint test wasm-check wasm wasm-test web-test
```

All of them must pass (the last three need Node 20 for the wasm and worker tests). `CGO_ENABLED=0` always; no cgo dependencies are accepted.

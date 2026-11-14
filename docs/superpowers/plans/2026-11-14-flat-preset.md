# Flat preset — plan

Spec: `docs/superpowers/specs/2026-11-14-flat-preset-design.md`. Inline,
TDD, branch `flat-preset`, same global constraints as the rescue plan.

1. `internal/region/flatten.go` + `flatten_test.go`: the five region
   tests RED, `Flatten` GREEN.
2. `options.go`: `PresetFlat`, `Options.Flatten`, `PresetOptions`,
   `Validate`; `options_test.go` RED→GREEN. `segment.go` wiring;
   `segment_test.go` two-band flat test and fixed-palette guard.
3. `apiopts` (`flatten`), `cmd/rekha` (`--flatten`, help text), web
   demo (option + preset table), `docs/wasm.md`, README, CHANGELOG,
   golden `gradient-shapes-flat-spline` (new file, `-update`).
4. Bench, results file, gates, commit, merge, push, rekha-cloud bump.

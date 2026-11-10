# Phase 4 (open half): WebAssembly, Web Worker demo, HTTP API

Status: design, 2026-11-10. Implements the open-source part of roadmap Phase 4 ("Online"). Accounts, billing, the polished GUI and hosting stay in the private `rekha-cloud` repo and are out of scope here.

## Intent

Rekha's pitch is "your image never leaves your machine". That needs the tracer to run in the browser. The roadmap also wants an HTTP API with caps so the same library can sit behind a server without being a denial-of-service magnet. Both are thin shells over the existing `Segment` / `Finish` split; neither changes tracing.

The user chose to skip the brainstorming questions. Every "how" below is the author's call unless marked otherwise.

## Goals

1. **`cmd/rekha-wasm`**: a `GOOS=js GOARCH=wasm` program that installs a global `rekha` object with `segment`, `finish`, `free`, `trace` and `version`. Input is raw RGBA (the browser decodes), options are a plain JS object with the CLI's flag names in camelCase, output is the SVG text plus timings. Tested under Node with `go test` (`GOOS=js GOARCH=wasm`).
2. **`web/`**: a bare demo page (vanilla HTML/JS, no bundler, no npm) that runs the wasm in a Web Worker: drop an image, see source and SVG side by side, move controls, download. Controls that change the segmentation re-run `segment`; everything else re-runs only `finish` on the cached handle. A cancel button terminates the worker and starts a fresh one.
3. **`cmd/rekha-serve`**: an HTTP server with `POST /v1/trace` (multipart or raw image body, options as form/query fields), `GET /healthz`, and `-static dir` to serve the demo. Caps: request bytes, pixels, wall time per trace, concurrent traces, queue length. Fuzzed uploads never panic or hang past the timeout.
4. **Shared option parsing**: one package turns a JSON object (wasm) or string fields (HTTP) into `rekha.Options`, seeded by the preset exactly as the CLI does, rejecting unknown keys.
5. **Measured**: wasm latency vs native on the corpus and the 3.4-megapixel logo, recorded in `bench/results/2026-11-10-phase4-wasm.md`; wasm size gzipped. These are the roadmap's exit test together with the fuzzed-upload test.
6. **Documented**: `docs/wasm.md` (JS API, worker protocol, embedding), `docs/server.md` (endpoints, caps, errors), README usage and status, roadmap row.

## Non-goals

- Accounts, billing, API keys, per-IP rate limiting, TLS, CORS headers (demo is same-origin when served by `rekha-serve`; a reverse proxy adds the rest).
- Progress reporting or cancellation inside the library. The worker is terminated to cancel; a timed-out server trace runs to completion in the background, bounded by the pixel cap.
- Multi-threaded wasm. Go's wasm target has one thread; `par` already degrades to inline and output is identical to the CLI (determinism across GOMAXPROCS is proven in Phase 3b).
- A polished UI, image editing, zoom/pan sync. The page exists to prove the pipeline and to measure.
- Decoding inside wasm. The browser's `createImageBitmap` handles PNG/JPEG/GIF/WebP/AVIF/HEIC faster and keeps the binary small; the server keeps Go's PNG/JPEG/GIF decoders.
- Deployment artefacts (Dockerfile, scale-to-zero config). The private repo owns hosting.

## Anti-goals

- Any change to tracing output. Every golden stays byte-identical; `go test ./...` on the existing packages must be unaffected.
- New dependencies. Still only `golang.org/x/image`; no JS packages; `wasm_exec.js` is copied from `$(go env GOROOT)/lib/wasm` at build time, never vendored.
- A server that can be driven out of memory or CPU by one client: every input is capped before it is decoded.

## Constraints

- Go 1.26, `CGO_ENABLED=0`, `make lint test wasm-check` green, plus new `make wasm wasm-test` green in CI (ubuntu-latest has Node 20).
- DCO sign-off on every commit; no AI attribution trailers.
- Keyed struct literals, `float64()`-wrapped products in numeric code (none expected here).
- Build artefacts (`web/rekha.wasm`, `web/wasm_exec.js`) are git-ignored.

## Design

### Option parsing: `internal/apiopts`

```go
// Params is the JSON shape shared by the wasm and HTTP entry points.
// Every field is optional; the preset seeds the rest exactly as the CLI.
type Params struct {
    Preset          *string    `json:"preset"`
    Mode            *string    `json:"mode"`
    FilterSpeckle   *int       `json:"filterSpeckle"`
    ColorPrecision  *int       `json:"colorPrecision"`
    PathPrecision   *int       `json:"pathPrecision"`
    MaxColors       *int       `json:"maxColors"`
    Threshold       *Threshold `json:"threshold"`      // number or "auto"
    TurnPolicy      *string    `json:"turnPolicy"`
    Simplify        *float64   `json:"simplify"`
    CornerThreshold *float64   `json:"cornerThreshold"`
    FitTolerance    *float64   `json:"fitTolerance"`
    ColorTolerance  *float64   `json:"colorTolerance"`
    Palette         *Palette   `json:"palette"`        // "#rrggbb,#rrggbb" or ["#rrggbb", …]
    Hierarchical    *string    `json:"hierarchical"`
    Refine          *bool      `json:"refine"`
    RefinePasses    *int       `json:"refinePasses"`
    CutoutStroke    *float64   `json:"cutoutStroke"`
}

func Parse(jsonText []byte) (rekha.Options, error)            // DisallowUnknownFields; preset seeds, fields override; Validate
func FromStrings(kv map[string]string) (rekha.Options, error) // HTTP form/query: each value is a JSON literal if it parses as number/bool, else a string
func SegmentKeys() []string                                   // preset, threshold, colorPrecision, colorTolerance, maxColors, palette, filterSpeckle
```

Errors wrap `rekha.ValidationError` or name the offending JSON key. `palette` moves `#rrggbb` parsing out of `cmd/rekha/main.go` into this package (`apiopts.ParsePalette`), and the CLI calls it; behaviour unchanged.

### `cmd/rekha-wasm`

`main()` sets `js.Global().Set("rekha", obj)` and blocks on a channel. The object:

| Method | Arguments | Returns |
|---|---|---|
| `version` | – | string |
| `segment(image, options)` | `image = {width, height, data: Uint8ClampedArray}` (RGBA, premultiplied not expected: `ImageData` is straight alpha), `options` = object or JSON string | `{handle: number, width, height, palette: ["#rrggbb", …], millis}` |
| `finish(handle, options)` | handle from `segment` | `{svg: string, paths: number, bytes: number, millis}` |
| `trace(image, options)` | convenience: segment + finish + free | `{svg, paths, bytes, width, height, palette, millis}` |
| `free(handle)` | – | undefined |

Rules: options are serialised with `JSON.stringify` on the Go side (`js.Global().Get("JSON")`) if not already a string, then `apiopts.Parse`. Pixel data is copied in with `js.CopyBytesToGo` into an `image.NRGBA`. Handles are integers from a counter; a bad handle throws. All errors are thrown as JS `Error` with the Go message (`rekha: …`). `finish` validates `options` fully (the library does), so the worker always sends the full current option set. Methods are synchronous; the caller runs them in a worker.

Tests (`cmd/rekha-wasm`, run under Node via `go_js_wasm_exec`): the API traces a 64×64 synthetic and the SVG equals `rekha.Trace` on the same `image.NRGBA`; bad options throw with the validation message; bad handle throws; `free` then `finish` throws; an options object (not string) is accepted; `segment` twice with the same bytes gives identical results. A non-wasm build of the package compiles to nothing (`//go:build js && wasm` on every file, plus a `doc.go` so `go vet ./...` on darwin is happy).

### `web/`

Files: `index.html`, `app.js`, `worker.js`, `rekha.css` (small), plus git-ignored `rekha.wasm`, `wasm_exec.js`.

Worker protocol (one in-flight request at a time per worker; the page serialises):

```
page → worker: {id, op: "segment", width, height, data: ArrayBuffer (transferred), options}
page → worker: {id, op: "finish", handle, options}
page → worker: {id, op: "free", handle}
worker → page: {id, ok: true, result}            // result as returned by the wasm method
worker → page: {id, ok: false, error: string}
worker → page: {op: "ready", version}            // once, after instantiation
```

Page behaviour:

- File input and drag-drop. `createImageBitmap` → `OffscreenCanvas` (fallback `<canvas>`) → `getImageData`. If `width*height` exceeds the page's pixel cap (default 4,000,000, editable in a number field) the image is downscaled to fit with aspect kept, and the page says so.
- Controls: preset, mode, hierarchical, max colours, colour tolerance, threshold (number + auto checkbox), filter speckle, refine (checkbox) and passes, cutout stroke, path precision. Changing a key in `SegmentKeys()` re-segments; otherwise finish only. A pending request is superseded: the page keeps a generation counter and drops stale results.
- Output: source image left, SVG right (`<img>` from a Blob URL, so the browser's own SVG renderer is the judge), a status line with segment ms, finish ms, paths, bytes, and a Download button. Errors shown in the status line.
- Cancel: `worker.terminate()`, new worker, waits for `ready`, status says "cancelled". The cached handle is gone, so the next change re-segments.
- No framework, no build step. ES modules; `importScripts('wasm_exec.js')` in the worker (classic worker, for Safari).

### `cmd/rekha-serve`

Flags: `-addr :8080`, `-max-bytes 16MiB`, `-max-pixels 16000000`, `-timeout 60s`, `-concurrency GOMAXPROCS`, `-queue 2×concurrency`, `-static ""` (directory served at `/`), `-version`.

Routes:

- `GET /healthz` → `200 ok`.
- `POST /v1/trace`: `multipart/form-data` with file field `image` and option fields; or any `image/*` body with options as query parameters. Response `200 image/svg+xml; charset=utf-8` with `X-Rekha-Paths`, `X-Rekha-Millis` headers. Errors are JSON `{"error": "…"}`:

| Status | When |
|---|---|
| 400 | unreadable multipart, unknown option, invalid option value, undecodable image |
| 413 | body over `-max-bytes` (`http.MaxBytesReader`) or image over `-max-pixels` (checked with `image.DecodeConfig` before decoding pixels) |
| 415 | not multipart and not `image/*` |
| 503 | queue full (`Retry-After: 1`) |
| 504 | trace exceeded `-timeout` |
| 405 | wrong method |

Concurrency: a buffered channel of `concurrency` tokens; a request waits at most while the queue has room (`queue` counter, atomic); the trace runs in a goroutine; the handler selects on done, request context, and the timeout. On timeout or client disconnect the handler returns and the goroutine's result is discarded (the library cannot be cancelled; the pixel cap bounds the waste). Server has `ReadHeaderTimeout`, `ReadTimeout = timeout + 10s`, `WriteTimeout`, graceful `Shutdown` on SIGINT/SIGTERM. Decoders: PNG, JPEG, GIF (same as the CLI).

Tests (`httptest`): happy path multipart and raw; every error row above; concurrency = 1 with queue 0 gives 503 on the second request; timeout fires at a small `-timeout` on a refine of a large synthetic; `FuzzTraceHandler` feeds arbitrary bodies and content types and asserts no panic and a status in the table; options parsed via `apiopts.FromStrings` with `preset=poster&refine=true&threshold=auto`.

### Build and CI

Makefile:

```
wasm:       GOOS=js GOARCH=wasm go build -o web/rekha.wasm ./cmd/rekha-wasm && cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" web/
wasm-test:  GOOS=js GOARCH=wasm go test -exec "$(go env GOROOT)/lib/wasm/go_js_wasm_exec" ./cmd/rekha-wasm
serve:      go run ./cmd/rekha-serve -static web
```

CI runs `make lint test wasm-check wasm wasm-test`. `build` keeps building every `cmd/` for the host (the wasm command is excluded by its build tags).

### Measurement

`bench/results/2026-11-10-phase4-wasm.md`: for each corpus image plus `Logo.jpg` (user file, not in repo; row named "3.4 MP logo"), native `rekha` ms vs wasm-under-Node ms for `bw`, `poster`, `poster --refine`; wasm binary size raw and gzipped; peak Node RSS on the logo. Driven by a Go test-benchmark in `cmd/rekha-wasm` that reads PNGs from `testdata/corpus` through Node's `fs` (`js.Global().Get("require")`) and a small script in `bench/`; recorded by hand into the markdown as other baselines are.

## What is left to the builder

- Exact HTML layout and CSS (keep it to one screen, two panes, controls in a bar).
- Whether `OffscreenCanvas` is available is detected at runtime; the fallback is a hidden canvas.
- Error message wording, as long as it carries the library's `rekha: …` text.
- Multipart option field order and the raw-body query names are the `Params` JSON keys.
- Benchmark script shape, as long as the markdown table is reproducible from it.

## Acceptance

- `make lint test wasm-check wasm wasm-test` green on arm64 and under `GOARCH=amd64`.
- `rekha-serve -static web` serves the demo; dropping a PNG shows source and SVG; toggling refine re-runs finish only (status shows segment ms unchanged); cancel works mid-refine.
- `FuzzTraceHandler` runs 30 s without a crash.
- `bench/results/2026-11-10-phase4-wasm.md` has native vs wasm timings for every corpus image and the logo.
- README, docs, roadmap updated; existing goldens untouched (`git status` shows no change under `testdata/`).

# Phase 4 Online (open half) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Run Rekha in the browser (wasm in a Web Worker behind a bare demo page) and behind a capped HTTP API, with wasm latency measured and fuzzed uploads survived.

**Architecture:** One option-parsing package feeds both entry points. `cmd/rekha-wasm` exposes `segment`/`finish`/`free`/`trace` on a global via `syscall/js`; `web/` is vanilla HTML/JS with a classic worker; `cmd/rekha-serve` wraps `rekha.TraceReader` in `net/http` with byte, pixel, time, concurrency and queue caps. No tracing code changes.

**Tech Stack:** Go 1.26 (`syscall/js`, `net/http`, `httptest`, `testing.F`), Node 20 for wasm tests and the protocol unit test, `wasm_exec.js` from GOROOT.

**Spec:** `docs/superpowers/specs/2026-11-10-phase4-online-design.md`

## Global Constraints

- Go 1.26, `CGO_ENABLED=0`, only `golang.org/x/image`; no npm, no vendored `wasm_exec.js`.
- `make lint test wasm-check wasm wasm-test web-test` green; `GOARCH=amd64 go test ./...` green.
- Every golden under `testdata/` byte-identical; no change to tracing packages.
- DCO `git commit -s`; never a Claude attribution trailer; never `git add -A` (user files in repo root).
- Keyed struct literals. Error text from the library passes through unchanged (`rekha: …`).
- Build artefacts `web/rekha.wasm`, `web/wasm_exec.js` git-ignored.

## Review Focus

1. Transparent PNG through wasm: `ImageData` is straight alpha; the SVG must equal `rekha.Trace` on the same `image.NRGBA` (Task 2 `TestWasmAlphaMatchesTrace`).
2. Malformed pixel payload: `width*height*4 != data.length`, or 0×0: a thrown JS Error, never a Go panic that kills the worker (Task 2 `TestWasmBadImageThrows`).
3. A tiny PNG whose header declares 100000×100000: 413 from `DecodeConfig` before any pixel allocation (Task 4 `TestServeHugeHeader413`).
4. Client disconnects mid-trace: the concurrency token is returned, the next request runs (Task 4 `TestServeDisconnectReleasesToken`).
5. A stale worker result arriving after a newer request: dropped by the page's generation counter (Task 3 `protocol.test.js` "stale results are ignored").

---

### Task 1: Shared option parsing `internal/apiopts`

**Files:**
- Create: `internal/apiopts/apiopts.go`, `internal/apiopts/apiopts_test.go`
- Modify: `cmd/rekha/main.go` (replace `parsePalette` with `apiopts.ParsePalette`), `cmd/rekha/main_test.go` if it names `parsePalette`

**Interfaces:**
- Produces:
  - `type Params struct{…}` with the JSON tags from the spec (`preset, mode, filterSpeckle, colorPrecision, pathPrecision, maxColors, threshold, turnPolicy, simplify, cornerThreshold, fitTolerance, colorTolerance, palette, hierarchical, refine, refinePasses, cutoutStroke`), every field a pointer.
  - `type Threshold struct{ Auto bool; Value float64 }` with `UnmarshalJSON` accepting a JSON number or the string `"auto"`.
  - `type Palette []color.NRGBA` with `UnmarshalJSON` accepting `"#rrggbb,#rrggbb"` or `["#rrggbb", …]`.
  - `func ParsePalette(s string) ([]color.NRGBA, error)` (moved from the CLI, same messages).
  - `func Parse(jsonText []byte) (rekha.Options, error)`: `DisallowUnknownFields`; `rekha.PresetOptions(preset or bw)` seeded, set fields override, then `Validate`.
  - `func FromStrings(kv map[string]string) (rekha.Options, error)`: each value becomes a JSON number if `strconv.ParseFloat` accepts it, `true`/`false` a bool, else a JSON string; then `Parse`.
  - `func SegmentKeys() []string` = `preset, threshold, colorPrecision, colorTolerance, maxColors, palette, filterSpeckle`.

- [ ] **Step 1: Write failing tests** in `apiopts_test.go`:

```go
func TestParsePresetSeedsDefaults(t *testing.T)   // {"preset":"poster"} == rekha.PresetOptions(PresetPoster)
func TestParseOverrides(t *testing.T)              // {"preset":"poster","refine":true,"refinePasses":5,"threshold":"auto","palette":"#000000,#ffffff","maxColors":3} sets Refine.Enabled, Sweeps 5, AutoThreshold, Palette len 2, MaxColors 3
func TestParseThresholdNumber(t *testing.T)        // {"threshold":0.3} → Threshold 0.3, AutoThreshold false
func TestParsePaletteArray(t *testing.T)           // {"palette":["#ff0000","#00ff00"]} → 2 entries
func TestParseUnknownKey(t *testing.T)             // {"colour":1} → error containing "colour"
func TestParseInvalidValue(t *testing.T)           // {"fitTolerance":99} → error containing "FitTolerance"
func TestParseEmpty(t *testing.T)                  // "{}" and "" → bw defaults
func TestFromStrings(t *testing.T)                 // {"preset":"poster","refine":"true","threshold":"auto","maxColors":"4"} → as above
func TestFromStringsBadNumber(t *testing.T)        // {"maxColors":"four"} → error containing "maxColors"
func TestSegmentKeys(t *testing.T)                 // exact list
func TestParsePaletteErrors(t *testing.T)          // "fff" → error `"fff" is not a #rrggbb colour`
```

- [ ] **Step 2: Run** `go test ./internal/apiopts/` — Expected: FAIL, package does not exist.
- [ ] **Step 3: Implement** `apiopts.go` per Interfaces. `Parse` with empty input treats it as `{}`.
- [ ] **Step 4: Run** `go test ./internal/apiopts/` — Expected: PASS.
- [ ] **Step 5: Switch the CLI** to `apiopts.ParsePalette`; delete `parsePalette` from `cmd/rekha/main.go`. Run `go test ./cmd/rekha/` — Expected: PASS (same messages).
- [ ] **Step 6: Commit** `feat(apiopts): JSON and string option parsing shared by wasm and HTTP`.

---

### Task 2: `cmd/rekha-wasm` and the wasm test target

**Files:**
- Create: `cmd/rekha-wasm/doc.go` (no build tag, package comment only), `cmd/rekha-wasm/main.go` (`//go:build js && wasm`), `cmd/rekha-wasm/api.go` (same tag; the JS-facing functions), `cmd/rekha-wasm/api_test.go` (same tag)
- Modify: `Makefile` (targets `wasm`, `wasm-test`; `build` unchanged), `.gitignore` (`/web/rekha.wasm`, `/web/wasm_exec.js`), `.github/workflows/ci.yml` (`make lint test wasm-check wasm wasm-test web-test`; `web-test` lands in Task 3, add the target name now and let Task 3 define it — until then CI would fail, so add a placeholder `web-test:` target echoing "no web tests yet" here).

**Interfaces:**
- Consumes: `apiopts.Parse`.
- Produces (JS global `rekha`): `version()`, `segment(image, options)`, `finish(handle, options)`, `trace(image, options)`, `free(handle)` as in the spec table. Go side:
  - `func optionsJSON(v js.Value) ([]byte, error)`: string as-is, otherwise `JSON.stringify`.
  - `func imageFromJS(v js.Value) (*image.NRGBA, error)`: reads `width`, `height`, `data`; error if non-positive or `len != w*h*4`.
  - `func segment(img *image.NRGBA, o rekha.Options) (handle int, palette []string, millis float64, err error)` and `func finish(handle int, o rekha.Options) (svg string, paths int, millis float64, err error)` kept pure for tests; the `js.FuncOf` wrappers convert errors into `js.Global().Get("Error").New(msg)` thrown via panic recovery (`throw` helper: return a JS Error object and let the JS wrapper in `api.go` `throw` it — use the pattern of returning `{error: msg}` objects? No: throw. Implement by `js.Global().Get("Reflect")`? Simplest: the wrapper returns a JS object `{ok:false,error}` on failure and `{ok:true, …}` on success; the worker converts to a throw/reject. Decision: **return `{ok, error, …}` objects**, never throw, since `syscall/js` cannot throw natively without a JS trampoline.)
- Tests run under Node: `go test` with `-exec go_js_wasm_exec`.

- [ ] **Step 1: Write failing tests** in `api_test.go` (build tag `js && wasm`):

```go
func TestWasmTraceMatchesNative(t *testing.T)   // 64×64 disc NRGBA via corpus synthetic or a loop; call the JS global's trace() through js.Global().Get("rekha") with a JS ImageData-like object built from js.CopyBytesToJS; result.ok true; svg == rekha.Trace(img, o).WriteSVG
func TestWasmAlphaMatchesTrace(t *testing.T)    // same with a transparent background (alpha 0 outside the disc)
func TestWasmSegmentFinishFree(t *testing.T)    // segment → finish twice (refine false then true: svgs differ) → free → finish returns ok:false with "handle"
func TestWasmBadOptions(t *testing.T)           // options {"fitTolerance":99} → ok:false, error contains "FitTolerance"
func TestWasmBadImageThrows(t *testing.T)       // data length w*h*4-1 → ok:false containing "rekha: image"; width 0 → ok:false
func TestWasmOptionsObject(t *testing.T)        // options passed as a JS object {preset:"poster"} works
func TestWasmVersion(t *testing.T)              // version() is a non-empty string
```

- [ ] **Step 2: Add Makefile targets**:

```
wasm:
	GOOS=js GOARCH=wasm go build -o web/rekha.wasm ./cmd/rekha-wasm
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" web/
wasm-test:
	GOOS=js GOARCH=wasm go test -exec "$$(go env GOROOT)/lib/wasm/go_js_wasm_exec" ./cmd/rekha-wasm
```

- [ ] **Step 3: Run** `make wasm-test` — Expected: FAIL (no package / undefined API).
- [ ] **Step 4: Implement** `main.go` (`install()` then `select {}`), `api.go` per Interfaces. Handles: `map[int]*rekha.Segmentation`, counter from 1. Timings with `time.Since` in ms as float64.
- [ ] **Step 5: Run** `make wasm-test` — Expected: PASS. Run `make wasm && ls -la web/rekha.wasm && gzip -c web/rekha.wasm | wc -c` and note sizes. Run `make lint wasm-check build` — Expected: green; `bin/` has no rekha-wasm.
- [ ] **Step 6: Commit** `feat(wasm): rekha-wasm JS API with segment/finish/free/trace, tested under Node`.

---

### Task 3: Demo page and worker `web/`

**Files:**
- Create: `web/index.html`, `web/rekha.css`, `web/app.js` (module), `web/worker.js` (classic), `web/protocol.js` (plain script usable by both: exposes `RekhaProtocol` global via `self.RekhaProtocol = …` and `module.exports` when present), `web/protocol.test.js` (`node --test`)
- Modify: `Makefile` (`web-test: node --test web/`), `.github/workflows/ci.yml` already calls it

**Interfaces:**
- Consumes: wasm global `rekha` (Task 2), `apiopts.SegmentKeys` list copied as `SEGMENT_KEYS` in `protocol.js` (Task 1 test and `protocol.test.js` pin the same 7 names; the Go test in Task 1 is the authority).
- Produces (`protocol.js`):
  - `SEGMENT_KEYS` array.
  - `needsResegment(prev, next)`: true when any `SEGMENT_KEYS` value differs (`JSON.stringify` of each key's value) or `prev` is null.
  - `class Requests { next() → id; accept(id) → boolean (true only for the latest id); }` the generation counter.
  - Worker messages exactly as the spec's protocol block.

- [ ] **Step 1: Write failing tests** `web/protocol.test.js`:

```js
test('segment keys', …)                       // deepEqual SEGMENT_KEYS, ['preset','threshold','colorPrecision','colorTolerance','maxColors','palette','filterSpeckle']
test('needsResegment on preset change', …)    // {preset:'bw'} → {preset:'poster'} true
test('finish-only change', …)                 // refine false→true false
test('null prev resegments', …)
test('stale results are ignored', …)          // r.next()=1, r.next()=2, accept(1) false, accept(2) true
```

- [ ] **Step 2: Run** `node --test web/` — Expected: FAIL, cannot find module.
- [ ] **Step 3: Implement** `protocol.js`; add `web-test` Makefile target. Run `make web-test` — Expected: PASS.
- [ ] **Step 4: Write** `worker.js`: `importScripts('wasm_exec.js','protocol.js')`, instantiate `rekha.wasm` (`instantiateStreaming` with `arrayBuffer` fallback), `go.run`, post `{op:'ready', version}`; `onmessage` dispatches `segment|finish|free` to the global, converts `{ok:false,error}` into `{id, ok:false, error}`.
- [ ] **Step 5: Write** `index.html`, `rekha.css`, `app.js`: layout = control bar on top, two panes (source `<img>`, result `<img>` from `URL.createObjectURL(new Blob([svg], {type:'image/svg+xml'}))`), status line, Download and Cancel buttons; pixel cap number input default 4000000 with downscale; controls from the spec list; `createImageBitmap` → `OffscreenCanvas` or hidden canvas → `getImageData`; transfer `data.buffer`; `needsResegment` decides; `Requests` drops stale replies; Cancel terminates and respawns the worker.
- [ ] **Step 6: Verify in a browser**: `go run ./cmd/rekha-serve -static web` is Task 4, so for now `python3 -m http.server -d web 8000` (or any static server); open `http://localhost:8000`, drop `testdata/corpus/synthetic/rounded-rect.png`: SVG appears; toggle refine: status shows finish ms changes and segment ms unchanged; change preset to poster: segment re-runs; Cancel during a refine of `big-poster.png`: status "cancelled", next change works. Record what was seen in the ledger.
- [ ] **Step 7: Commit** `feat(web): worker-backed demo page with cached segmentation and cheap re-finish`.

---

### Task 4: `cmd/rekha-serve`

**Files:**
- Create: `cmd/rekha-serve/main.go` (flags, server lifecycle), `cmd/rekha-serve/handler.go` (`Handler` type), `cmd/rekha-serve/handler_test.go`, `cmd/rekha-serve/main_test.go` (flag parsing only)
- Modify: `Makefile` (`serve: go run ./cmd/rekha-serve -static web`), `.gitignore` nothing

**Interfaces:**
- Consumes: `apiopts.FromStrings`, `rekha.TraceReader`, image decoders.
- Produces:
  - `type Limits struct{ MaxBytes int64; MaxPixels int; Timeout time.Duration; Concurrency, Queue int }`
  - `func NewHandler(l Limits, static string) http.Handler`: routes `/healthz`, `/v1/trace`, and static files at `/` when `static != ""`.
  - Response headers `X-Rekha-Paths`, `X-Rekha-Millis`; JSON errors `{"error":"…"}` with the spec's status table; `Retry-After: 1` on 503.

- [ ] **Step 1: Write failing tests** `handler_test.go` with `httptest.NewServer(NewHandler(limits, ""))`:

```go
func TestServeHealthz(t *testing.T)                      // 200 "ok"
func TestServeMultipartTrace(t *testing.T)               // PNG of a 32×32 disc as field "image", fields preset=bw mode=polygon → 200, Content-Type image/svg+xml, body starts "<svg", X-Rekha-Paths "1"
func TestServeRawBodyTrace(t *testing.T)                 // Content-Type image/png body, ?preset=poster&refine=true → 200
func TestServeBadOption(t *testing.T)                    // fitTolerance=99 → 400 JSON error containing "FitTolerance"
func TestServeUnknownOption(t *testing.T)                // colour=1 → 400 containing "colour"
func TestServeUndecodable(t *testing.T)                  // body "hello" as image/png → 400
func TestServeUnsupportedMedia(t *testing.T)             // text/plain → 415
func TestServeTooManyBytes(t *testing.T)                 // MaxBytes 100, 200-byte body → 413
func TestServeHugeHeader413(t *testing.T)                // valid PNG header declaring 100000×100000 (hand-built IHDR) with MaxPixels 16e6 → 413 before decode
func TestServeMethodNotAllowed(t *testing.T)             // GET /v1/trace → 405
func TestServeQueueFull503(t *testing.T)                 // Concurrency 1, Queue 0; first request blocks (slow trace: big image with refine, or a Limits.traceFn hook for tests set to a func that waits on a channel); second → 503 with Retry-After
func TestServeTimeout504(t *testing.T)                   // Timeout 50ms with the same blocking hook → 504
func TestServeDisconnectReleasesToken(t *testing.T)      // Concurrency 1: request 1 with a cancelled context on the blocking hook; then release hook; request 2 → 200
func TestServeStatic(t *testing.T)                       // static dir with index.html → GET / 200
func FuzzTraceHandler(f *testing.F)                      // seeds: valid PNG, truncated PNG, "", random bytes, each with content types image/png, multipart (with/without field), text/plain; assert status ∈ {200,400,405,413,415,503,504} and no panic
```

The blocking hook: unexported `traceFn func(io.Reader, rekha.Options) (*rekha.Result, error)` field on the handler, defaulting to `rekha.TraceReader`; tests override it.

- [ ] **Step 2: Run** `go test ./cmd/rekha-serve/` — Expected: FAIL (undefined).
- [ ] **Step 3: Implement** `handler.go` and `main.go` per spec: `http.MaxBytesReader`; multipart via `r.MultipartReader()` streaming the `image` part into memory up to the cap, other parts as option strings; raw body when `Content-Type` has prefix `image/`; `image.DecodeConfig` on a `bytes.Reader` then reset; pixel cap `w*h > MaxPixels` → 413; semaphore `chan struct{}` of `Concurrency`; `atomic.Int64` waiting counter vs `Queue`; goroutine + `select` on result, `r.Context().Done()`, `time.After(Timeout)`; `main.go` flags with the spec's defaults, `http.Server` timeouts, `signal.NotifyContext` shutdown.
- [ ] **Step 4: Run** `go test ./cmd/rekha-serve/` — Expected: PASS. Run `go test -run XXX -fuzz FuzzTraceHandler -fuzztime 30s ./cmd/rekha-serve/` — Expected: no crash.
- [ ] **Step 5: Manual**: `make wasm && go run ./cmd/rekha-serve -static web` then `curl -s -F image=@testdata/corpus/synthetic/rounded-rect.png -F preset=bw localhost:8080/v1/trace | head -c 200` shows `<svg`; browser at `localhost:8080` shows the demo (repeat Task 3 Step 6 checks once).
- [ ] **Step 6: Commit** `feat(serve): HTTP trace API with byte, pixel, time, concurrency and queue caps`.

---

### Task 5: Measurement and documentation

**Files:**
- Create: `bench/wasm/bench_test.go` (build tag `js && wasm`, in package `cmd/rekha-wasm`? No: put `bench_wasm_test.go` in `cmd/rekha-wasm` with tag `js && wasm` and a `-run TestWasmLatency -v` test gated on env `REKHA_WASM_BENCH=1` that reads every `testdata/corpus/**/*.png` via Node `fs` and prints a markdown table of ms for bw, poster, poster+refine), `bench/results/2026-11-10-phase4-wasm.md`, `docs/wasm.md`, `docs/server.md`
- Modify: `README.md` (status, usage for wasm/serve), `design/roadmap.md` (Phase 4 row: open half delivered 2026-11-10), `docs/benchmark.md` (one line pointing at the wasm table), `CONTRIBUTING.md` (`make lint test wasm-check wasm wasm-test web-test`)

**Interfaces:**
- Consumes: `cmd/rekha-wasm` API (Task 2), native CLI timings via `bin/rekha`.

- [ ] **Step 1: Write** the gated latency test. Run `REKHA_WASM_BENCH=1 make wasm-test ARGS="-run TestWasmLatency -v"` (add `ARGS` pass-through to the Makefile target) — Expected: a table with one row per corpus image.
- [ ] **Step 2: Native timings**: `make build` then a shell loop over the same images with `bin/rekha --preset … [--refine]` timed via `python3 -c time` as in Phase 3b; plus `Logo.jpg` through both (wasm: decode with Go `image/jpeg` inside the test for this row only, since Node has no decoder; note it).
- [ ] **Step 3: Write** `bench/results/2026-11-10-phase4-wasm.md`: environment (Go, Node, machine), wasm size raw/gz, table native ms vs wasm ms and ratio per image and preset, peak Node RSS on the logo (`/usr/bin/time -l` on macOS), pass/fail against "wasm ≤ 5× native" (author's bar) and a note on what the demo's 4 MP default cap means for the logo.
- [ ] **Step 4: Write** `docs/wasm.md` (JS API table, worker protocol, embedding snippet, size, latency pointer), `docs/server.md` (flags table, routes, status table, curl examples, note on uncancellable background trace), README (status line → Phase 4 open half; usage block gains `make wasm`, `rekha-serve`, demo URL; Why Rekha bullet on browser now true), roadmap row, CONTRIBUTING, benchmark.md pointer.
- [ ] **Step 5: Run** `make lint test wasm-check wasm wasm-test web-test` and `GOARCH=amd64 go test ./...` — Expected: all green; `git status` shows nothing under `testdata/`.
- [ ] **Step 6: Commit** `docs(phase4): wasm latency baseline, wasm and server docs, README and roadmap`.

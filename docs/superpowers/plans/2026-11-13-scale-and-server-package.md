# Scale option and importable server package: implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Two public additions the clean-up pre-pass in rekha-cloud needs: `Options.Scale`, so a trace of an upscaled image is written at the size it is for, and a `server` package another module can wrap.

**Architecture:** `Scale` travels on `svg.Document` and is honoured by the SVG writer, the PDF and EPS exporters and the SVG parser (so Convert keeps it). The HTTP handler of `cmd/rekha-serve` moves to `github.com/vexyl-ai/rekha/server` unchanged in behaviour, gaining a trace hook shaped like `rekha.Auto` and a list of extra field names it passes through to that hook.

**Tech Stack:** Go 1.26, `CGO_ENABLED=0`, standard library and `golang.org/x/image` only.

**Spec:** `/Users/anilmathew/Documents/AICoding/rekha-cloud/docs/superpowers/specs/2026-11-13-denoise-sidecar-design.md`, section "Prerequisites in the public repo".

## Global Constraints

- Go 1.26, `CGO_ENABLED=0`, no dependency beyond `golang.org/x/image` (and its indirect `golang.org/x/sys`).
- Gates before every commit: `make lint test wasm-check wasm wasm-test web-test` and `GOARCH=amd64 go test ./ ./internal/gradient/`.
- Goldens (`testdata/golden/*.svg`) stay byte-identical: a document with Scale 1 (or 0) is written exactly as today.
- Commits `git commit -s`, no AI attribution trailer, one task per commit, messages in the repo's style (`area: what, in a sentence`).
- No cloud knowledge in the public code: no mention of clean-up, denoising or sidecars.
- `docs/server.md`, `docs/wasm.md`, README option tables and `CHANGELOG.md` (an "Unreleased" section above v0.2.0) are updated in the task that adds the behaviour.

## Review Focus

- A Scale that leaves a fractional display size (1000 px at Scale 3): the SVG `width` is written with two decimals, never rounded to an integer, and Parse reads it back to the same Scale within 1e-6. Pinned in Task 1 (`TestScaleFractionalSizeRoundTrips`).
- Convert of a Rekha SVG with Scale through PDF: the page is the display size in points, not the trace size. Pinned in Task 1 (`TestConvertKeepsScale`).
- An SVG with `width` but no `viewBox` (Rekha before this change wrote both; a hand-edited file may drop one): Parse treats the width as the trace size, Scale 1. Pinned in Task 1 (`TestParseWithoutViewBox`).
- A request naming an extra field the server was not told about: still `400` with the field named, exactly as today. Pinned in Task 2 (`TestServeUnknownOption` kept).
- The trace hook returning options whose Format differs from the request (a wrapper must not be able to change the content type by accident): the handler writes with the hook's options but sets the content type from them too, so they agree. Pinned in Task 2 (`TestServeHookOptionsDriveTheWrite`).

---

### Task 1: `Options.Scale`

**Files:**
- Modify: `options.go` (field, doc, Validate range)
- Modify: `internal/svg/writer.go` (`Document.Scale`, width/height attributes)
- Modify: `internal/svg/parse.go` (read `viewBox` and a float `width`, derive Scale)
- Modify: `internal/export/export.go`, `pdf.go`, `eps.go` (page size and transform divide by Scale)
- Modify: `format.go` (`document()` copies `o.Scale` into the Document)
- Modify: `cmd/rekha/main.go` (`--scale`), `internal/apiopts/apiopts.go` (`scale` key)
- Modify: `README.md` (Output table row), `docs/server.md`, `docs/wasm.md`, `CHANGELOG.md`
- Test: `format_test.go`, `internal/svg/writer_test.go`, `internal/svg/parse_test.go`, `internal/export/export_test.go`, `cmd/rekha/main_test.go`, `internal/apiopts/apiopts_test.go`

**Interfaces:**
- Produces: `Options.Scale float64` ("the image is Scale times the size the output is for; 0 means 1; Validate accepts 0 and 0.25..8"); `svg.Document.Scale float64` with the same meaning; `svg.Parse` returns a Document whose Width/Height are the trace size (the viewBox) and whose Scale is viewBox width ÷ width attribute.

- [ ] **Step 1: Write the failing tests.** In `internal/svg/writer_test.go`:

```go
func TestWriteDocumentScaleSetsDisplaySize(t *testing.T) {
	d := Document{Width: 200, Height: 100, Scale: 2, Paths: onePath()} // onePath: any filled rect helper already in the file, or add one
	var b strings.Builder
	WriteDocument(&b, d, 2)
	if !strings.Contains(b.String(), `width="100" height="50" viewBox="0 0 200 100"`) { t.Fatal(b.String()) }
	d.Scale = 0
	b.Reset()
	WriteDocument(&b, d, 2)
	if !strings.Contains(b.String(), `width="200" height="100" viewBox="0 0 200 100"`) { t.Fatal(b.String()) }
}
func TestScaleFractionalSizeRoundTrips(t *testing.T) {
	d := Document{Width: 1000, Height: 10, Scale: 3, Paths: onePath()}
	var b strings.Builder
	WriteDocument(&b, d, 2)
	if !strings.Contains(b.String(), `width="333.33"`) { t.Fatal(b.String()) }
	got, _, err := Parse(strings.NewReader(b.String()))
	if err != nil || got.Width != 1000 || math.Abs(got.Scale-3) > 1e-6 { t.Fatalf("%+v %v", got, err) }
}
```

In `internal/svg/parse_test.go`: `TestParseWithoutViewBox` parses `<svg xmlns="http://www.w3.org/2000/svg" width="20" height="10"><path fill="#000000" d="M0 0H20V10H0Z"/></svg>` and expects Width 20, Height 10, Scale 1.

In `internal/export/export_test.go`: `TestPDFPageIsDisplaySize` writes a Document{Width: 200, Height: 100, Scale: 2} and expects `/MediaBox [0 0 75 37.5]` in the PDF and `%%BoundingBox: 0 0 75 38` in the EPS (today's values for Width 100, Height 50), and the content transform `0.375 0 0 -0.375 0 37.5 cm` in the PDF (export.Scale ÷ d.Scale).

In `format_test.go`: `TestConvertKeepsScale` traces `star-5` with `Options{Scale: 2, NoMark: true}` (synthetic traced as is; the size is what matters), writes SVG, runs `Convert` to PDF and expects the MediaBox of the display size.

In `options_test.go`: `TestValidateScale` accepts 0, 0.25, 8, rejects 0.2, 9 and -1 with a ValidationError on field "Scale".

In `cmd/rekha/main_test.go`: `TestRunScale` runs `--scale 2 --no-mark -` on `starPNG` and expects `width="32"` for the 64-px star. In `internal/apiopts/apiopts_test.go`: `TestParseScale` parses `{"scale":2}` into `o.Scale == 2`.

- [ ] **Step 2: Run them.** `go test ./internal/svg ./internal/export ./ ./cmd/rekha ./internal/apiopts -run 'Scale|WithoutViewBox|PageIsDisplay'`. Expected: build failures on the missing fields, then FAIL.

- [ ] **Step 3: Implement.** `Options.Scale float64` after `Format` in `options.go`, doc as in Interfaces; `Validate`: `if o.Scale != 0 { inRangeF("Scale", o.Scale, 0.25, 8) }`. `svg.Document.Scale float64`; in `WriteDocument` compute `s := d.Scale; if s == 0 { s = 1 }`; when `s == 1` keep today's `%d` attributes verbatim (goldens), else `width="%s" height="%s"` with `FormatNum(float64(d.Width)/s, 2)`. In `parse.go` read `viewBox` (four numbers; the last two are Width and Height when present) and `width`/`height` as floats; Scale = viewBoxW ÷ width when both present and width > 0, else 1 with Width/Height from the attributes (the existing integer path). In `export.go` the transform and in `pdf.go`/`eps.go` the page size use `Scale/d.Scale` (treat 0 as 1); `BoundingBox` rounds up as today. `format.go` `document()` sets `d.Scale = o.Scale`. CLI `fs.Float64Var(&o.Scale, "scale", 0, "the image is this many times the size the output is for (0.25-8); the SVG is written at image size ÷ scale")`; apiopts `Scale *float64 \`json:"scale"\`` applied with `setFloat`. Docs: README Output table row `| --scale | 1 | … |`, one clause in `docs/server.md` and `docs/wasm.md` option lists, CHANGELOG "Unreleased: `--scale` …".

- [ ] **Step 4: Run the gates.** `make lint test wasm-check wasm wasm-test web-test && GOARCH=amd64 go test ./ ./internal/gradient/`. Expected: all ok; `git status` shows no golden changed.

- [ ] **Step 5: Commit.** `git add -p`-level precision on the files above; `git commit -s -m "rekha: Options.Scale writes the output at the size the image is for"`.

### Task 2: `server` package with a trace hook

**Files:**
- Create: `server/server.go` (from `cmd/rekha-serve/handler.go`), `server/server_test.go` (from `handler_test.go`), `server/doc.go`
- Modify: `cmd/rekha-serve/main.go` (imports `server`, calls `server.New`), `cmd/rekha-serve/main_test.go` (keeps the flag and shutdown tests)
- Delete: `cmd/rekha-serve/handler.go`, `cmd/rekha-serve/handler_test.go`
- Modify: `docs/server.md` (a "Library" paragraph), `CHANGELOG.md`

**Interfaces:**
- Consumes: `rekha.Trace`, `apiopts.FromStrings`, `Options.Scale` from Task 1 (only through `Result.Write`).
- Produces, in package `server`:

```go
type Limits struct { MaxBytes int64; MaxPixels int; Timeout time.Duration; Concurrency, Queue int } // moved as is
// TraceFunc traces a decoded image. extra holds the request fields named by
// WithFields, already removed from the options. The options returned are the
// ones the result is written with; the header's entries are added to the
// response.
type TraceFunc func(ctx context.Context, img image.Image, o rekha.Options, extra map[string]string) (*rekha.Result, rekha.Options, http.Header, error)
type Option func(*handler)
func WithTrace(f TraceFunc) Option
func WithFields(names ...string) Option
func New(l Limits, static string, opts ...Option) http.Handler
const WriteGrace = 10 * time.Second // exported so main can size the shutdown grace
```

The default TraceFunc decodes nothing (the handler already decoded) and returns `rekha.Trace(img, o)`, `o`, `nil`.

- [ ] **Step 1: Move the files and rename.** `git mv cmd/rekha-serve/handler.go server/server.go`, `git mv cmd/rekha-serve/handler_test.go server/server_test.go`; package `server`; `NewHandler` becomes `New` with the `opts ...Option` parameter; `writeGrace` becomes `WriteGrace`. `main.go` calls `server.New(c.limits, c.static)` and `server.WriteGrace`. Tests that set `h.traceFn` directly (`TestServeTimeout504`, `TestServeDisconnectReleasesToken`, `TestServePanicIsolated`, `TestServeQueueWaitTimesOut`, any other) use `WithTrace` instead.

- [ ] **Step 2: Run the moved tests.** `go test ./server ./cmd/rekha-serve`. Expected: PASS, unchanged behaviour.

- [ ] **Step 3: Write the failing hook tests** in `server/server_test.go`:

```go
func TestServeHookSeesExtraFields(t *testing.T) {
	var got map[string]string
	h := New(limits(), "", WithFields("cleanup"), WithTrace(func(_ context.Context, img image.Image, o rekha.Options, extra map[string]string) (*rekha.Result, rekha.Options, http.Header, error) {
		got = extra
		r, err := rekha.Trace(img, o)
		return r, o, http.Header{"X-Test": {"yes"}}, err
	}))
	rec := post(t, h, starPNG(), map[string]string{"preset": "bw", "cleanup": "auto"}) // post: the file's multipart helper
	if rec.Code != 200 || got["cleanup"] != "auto" || rec.Header().Get("X-Test") != "yes" { t.Fatalf("%d %v %v", rec.Code, got, rec.Header()) }
}
func TestServeHookOptionsDriveTheWrite(t *testing.T) {
	// The hook asks for PDF although the request did not: body and content type follow the hook.
	h := New(limits(), "", WithTrace(func(_ context.Context, img image.Image, o rekha.Options, _ map[string]string) (*rekha.Result, rekha.Options, http.Header, error) {
		o.Format = rekha.FormatPDF
		r, err := rekha.Trace(img, o)
		return r, o, nil, err
	}))
	rec := post(t, h, starPNG(), map[string]string{"preset": "bw"})
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/pdf" || !bytes.HasPrefix(rec.Body.Bytes(), []byte("%PDF")) { t.Fatal(rec.Code, rec.Header()) }
}
```

`TestServeUnknownOption` stays as it is: `cleanup=auto` without `WithFields` is still `400` naming `cleanup`.

- [ ] **Step 4: Run them.** `go test ./server -run 'Hook'`. Expected: FAIL (no WithFields/WithTrace, or extra not delivered).

- [ ] **Step 5: Implement.** In `trace()`: after `readRequest`, move the named fields out of `fields` into `extra`; `apiopts.FromStrings(fields)` as before; decode the image once (`image.Decode(bytes.NewReader(img))`) after the DecodeConfig cap check; call `h.traceFn(ctx, decoded, o, extra)` on the trace goroutine; write with the returned options (`res.Write(&buf, o2)`, `contentType(o2.Format)`) and copy the returned header entries onto the response before the body. Keep every status code and message as it is.

- [ ] **Step 6: Run the gates.** `make lint test wasm-check wasm wasm-test web-test && GOARCH=amd64 go test ./ ./internal/gradient/`. Expected: ok.

- [ ] **Step 7: Docs and commit.** `docs/server.md`: a "Library" paragraph with the three exported names and the hook's shape; CHANGELOG "Unreleased: the HTTP handler is importable as `server`". `git commit -s -m "server: the HTTP handler as a package, with a trace hook"`.

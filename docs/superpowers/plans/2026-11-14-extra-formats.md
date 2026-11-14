# Extra Output Formats Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a wrapper of the public server register an output format it writes itself, and make option parsing public.

**Architecture:** `server.WithFormat` adds a registry of name → (media type, writer) consulted around the trace; `internal/apiopts` becomes `apiopts`.

**Tech Stack:** Go 1.26, CGO_ENABLED=0, stdlib only.

**Spec:** docs/superpowers/specs/2026-11-14-extra-formats-design.md

## Global Constraints

- Go 1.26, `CGO_ENABLED=0`, dependencies only `golang.org/x/image`.
- Gates before every commit: `make lint test wasm-check wasm wasm-test web-test` and `GOARCH=amd64 go test ./ ./internal/gradient/`.
- Goldens byte-identical. DCO `git commit -s`, no AI trailer.

## Review Focus

- A registered format name that is also a built-in (`svg`): the registry wins for the registered name only when the wrapper registered it; `TestServeFormatRegistryOverridesBuiltin` pins that a wrapper may override.
- `format` given with a registered name but the TraceFunc returning another Format: the returned options decide (`TestServeHookOptionsDriveTheWrite` already pins the returned options drive the write).
- Writer error: 500, not a half-written body (`TestServeExtraFormatWriterError`).
- Unregistered paid name: still 400 naming the field (`TestServeUnknownOption` style, `TestServeUnregisteredFormatRefused`).
- Case: `format=DXF` matches `dxf` the way `ParseFormat` is case-insensitive (`TestServeExtraFormat` uses mixed case).

---

### Task 1: apiopts public

**Files:**
- Move: `internal/apiopts/` → `apiopts/`
- Modify: `cmd/rekha/main.go`, `cmd/rekha-wasm/api.go`, `server/server.go` (import path), `README.md` (library section: one paragraph), `CHANGELOG.md` (Unreleased / Added)

- [ ] **Step 1:** `git mv internal/apiopts apiopts`; fix the three imports; package doc says it is the option parser behind the API, the playground and any embedder.
- [ ] **Step 2:** `go build ./... && go test ./apiopts ./server ./cmd/...` Expected: PASS.
- [ ] **Step 3:** README library section: `apiopts.Parse` / `apiopts.FromStrings` in two sentences. CHANGELOG Added entry.
- [ ] **Step 4:** Gates. Commit `apiopts: the option parser as a public package`.

### Task 2: server.WithFormat

**Files:**
- Modify: `server/server.go`, `server/server_test.go`, `README.md` (server paragraph), `CHANGELOG.md`

**Interfaces:**
- Produces: `type FormatWriter func(w io.Writer, r *rekha.Result, o rekha.Options, extra map[string]string) error`; `func WithFormat(name, contentType string, write FormatWriter) Option`.

- [ ] **Step 1: Failing tests.** `TestServeExtraFormat` (register `dxf` with a writer that writes `"DXF " + fill of first path`; POST `format=DXF`; expect 200, Content-Type `application/dxf`, body prefix `DXF `, and the hook saw `o.Format == "dxf"`); `TestServeUnregisteredFormatRefused` (no registration, `format=dxf` → 400 mentioning `format`); `TestServeExtraFormatWriterError` (writer returns error → 500, body is the error); `TestServeFormatRegistryOverridesBuiltin` (register `svg` with a writer that writes `"mine"` → body `mine`).
- [ ] **Step 2:** Run `go test ./server -run 'ExtraFormat|Unregistered|Registry'`. Expected: FAIL (WithFormat undefined).
- [ ] **Step 3:** Implement: `h.formats map[string]format{ct string; write FormatWriter}`; in `trace`, after `readRequest`, `if f, ok := h.formats[strings.ToLower(fields["format"])]; ok { delete(fields, "format"); forced = name }`; after `FromStrings`, `o.Format = rekha.Format(name)`; after the hook, `if f, ok := h.formats[strings.ToLower(string(o.Format))]; ok { err = f.write(&buf, out.res, o, extra); ct = f.ct } else { err = out.res.Write(&buf, o); ct = contentType(o.Format) }`.
- [ ] **Step 4:** `go test ./server`. Expected: PASS. Gates. README + CHANGELOG. Commit `server: WithFormat registers an output format the wrapper writes`.

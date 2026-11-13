# rekha-serve: HTTP API

`cmd/rekha-serve` puts the tracer behind HTTP with caps so one client cannot exhaust the host. It is the open half of the roadmap's online phase; accounts, keys and rate limits per client belong to a reverse proxy or the private service.

```
go run ./cmd/rekha-serve -static web          # API on :8080 and the demo page at /
docker run --rm -p 8080:8080 ghcr.io/vexyl-ai/rekha   # the same API from the release image (no demo page)
curl -F image=@logo.png -F preset=poster -F refine=true localhost:8080/v1/trace > logo.svg
curl -H 'Content-Type: image/png' --data-binary @logo.png 'localhost:8080/v1/trace?preset=bw&threshold=auto' > logo.svg
```

## Flags

| Flag | Default | Meaning |
|---|---|---|
| `-addr` | `:8080` | listen address |
| `-static` | (none) | directory served at `/`, normally `web/` after `make wasm` |
| `-max-bytes` | 16 MiB | largest request body; over it → 413 |
| `-max-pixels` | 16,000,000 | largest decoded image, checked from the header before decoding; over it → 413 |
| `-timeout` | 60s | wall time for one trace; over it → 504 |
| `-concurrency` | GOMAXPROCS | traces running at once |
| `-queue` | 2 × GOMAXPROCS | requests waiting for a slot; more → 503 with `Retry-After: 1` |

## Routes

`GET /healthz` → `200 ok`.

`POST /v1/trace`, either `multipart/form-data` with a file field `image` and one text field per option, or a raw body with `Content-Type: image/*` and options as query parameters. Option names are the CLI flags in camelCase (`auto` = race four settings and keep the closest, as `--auto`; `preset`, `mode`, `hierarchical`, `maxColors`, `colorTolerance`, `threshold` = number or `auto`, `refine`, `refinePasses`, `gradients`, `gradientTolerance`, `noMark`, `format` = `svg`, `pdf` or `eps`, `palette` = `#rrggbb,#rrggbb`, …; the full list is `internal/apiopts.Params`). PNG, JPEG and GIF decode.

Success: `200`, `Content-Type: image/svg+xml; charset=utf-8` (`application/pdf` or `application/postscript` for `format=pdf` or `eps`), headers `X-Rekha-Paths` and `X-Rekha-Millis`. Errors are JSON `{"error": "…"}`:

| Status | When |
|---|---|
| 400 | unreadable multipart, no `image` field, unknown or out-of-range option (the field is named), undecodable image |
| 405 | not POST |
| 413 | body over `-max-bytes`, or image over `-max-pixels` |
| 415 | neither multipart nor `image/*` |
| 500 | the tracer panicked on this input (the server keeps running) |
| 503 | queue full, or no slot freed within `-timeout`; retry after a second |
| 504 | trace exceeded `-timeout` |

## Behaviour under load

At most `-concurrency` + `-queue` requests are admitted at once, and admission happens before the body is read, so waiting uploads hold at most that many bodies in memory. Each trace holds one of `-concurrency` slots for its whole run. A request that finds no slot waits while fewer than `-queue` are already waiting and for at most `-timeout`, otherwise it is refused with 503. A client that disconnects while waiting leaves the queue; one that disconnects (or times out) while its trace runs gets no answer, and the trace finishes in the background before its slot is freed, because the library has no cancellation. The pixel cap bounds that waste; set `-timeout` with the slowest preset (`poster --refine` on a `-max-pixels` image) in mind. On SIGINT or SIGTERM the server stops accepting and waits up to `-timeout` + 10 s for in-flight requests. `FuzzTraceHandler` feeds arbitrary bodies and content types through the handler; `make test` runs its seeds and `go test -fuzz FuzzTraceHandler ./cmd/rekha-serve` runs it for as long as you like.

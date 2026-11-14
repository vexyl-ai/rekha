# Extra output formats through the server, and public option parsing

Short design, 2026-11-14. Approved in chat ("dive in").

**Intent.** The commercial build adds output formats (DXF first) on top
of the public module. Rule 2 of the roadmap says the private repo adds
and never patches, so the public server has to let a wrapper register a
format it does not know, and a private CLI has to parse the public
options without re-declaring every flag.

**Goals.** `server.WithFormat` registers a name, a media type and a
writer; a request with that `format` is traced by the public server and
written by the wrapper's writer. `apiopts` becomes a public package, so
any embedder parses options from JSON or strings the way the API and
the playground do. The open CLI still refuses `--format dxf`.

**Non-goals.** Arc metadata on fitted segments (the exporter recovers
arcs from the cubics at its tolerance: the fitter's arcs are exact
circles, so recovery is exact). New option fields.

**Anti-goals.** Any change to SVG, PDF or EPS output; goldens stay
byte-identical. A paid format name leaking into the open CLI's help.

**Approaches.** (a) Tag arcs upstream and teach the server a writer hook:
touches the fitter and the SVG model for a property only one exporter
reads. (b) Writer hook only, arcs recovered downstream: the exporter
owns its tolerance and nothing public moves but the server. (c) Private
fork of the server: forbidden by rule 1. Picked (b).

**Design.** `type FormatWriter func(w io.Writer, r *rekha.Result, o rekha.Options, extra map[string]string) error`.
`WithFormat(name, contentType string, write FormatWriter) Option`. In
the handler, a `format` field naming a registered format is taken out
before `apiopts.FromStrings` (which would refuse it) and passed to the
TraceFunc in `extra["format"]`, the options' Format staying SVG so the
tracer's own validation passes (ruled during execution: `rekha.Trace`
validates Format); after the trace, the request's registered name, or
the returned options' Format when it names a registered format, picks
the writer that writes the body and the media type; otherwise
`Result.Write` as today. `internal/apiopts` moves to `apiopts` unchanged, three importers
follow.

**Left to the builder.** Test names; CHANGELOG wording.

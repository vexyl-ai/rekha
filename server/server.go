// Package server puts the tracer behind HTTP with caps so one client
// cannot exhaust the host: POST /v1/trace returns an SVG, PDF or EPS.
// Command rekha-serve is a thin main over it; another module wraps it
// with WithTrace to run its own work round the trace and WithFields to
// accept request fields of its own. See docs/server.md.
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/vexyl-ai/rekha"
	"github.com/vexyl-ai/rekha/apiopts"

	// Decoders for the formats the API accepts, the same as the CLI.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// Limits caps what one request may cost.
type Limits struct {
	MaxBytes    int64         // request body, bytes
	MaxPixels   int           // width×height of the decoded image
	Timeout     time.Duration // wall time for one trace
	Concurrency int           // traces running at once
	Queue       int           // requests waiting for a slot before 503
}

// maxFieldBytes bounds one multipart option field.
const maxFieldBytes = 4096

// TraceFunc traces a decoded image. extra holds the request fields named
// by WithFields, already taken out of the options. The options returned
// are the ones the result is written with (a wrapper may set Scale or
// Format); the header's entries are added to the response.
type TraceFunc func(ctx context.Context, img image.Image, o rekha.Options, extra map[string]string) (*rekha.Result, rekha.Options, http.Header, error)

// Option configures New.
type Option func(*handler)

// WithTrace replaces the trace: by default rekha.Trace on the decoded
// image, the options unchanged, no extra headers.
func WithTrace(f TraceFunc) Option { return func(h *handler) { h.traceFn = f } }

// WithFields names request fields that are not tracer options: they are
// passed to the TraceFunc in extra instead of being rejected as unknown.
func WithFields(names ...string) Option {
	return func(h *handler) {
		for _, n := range names {
			h.fields[n] = true
		}
	}
}

func defaultTrace(_ context.Context, img image.Image, o rekha.Options, _ map[string]string) (*rekha.Result, rekha.Options, http.Header, error) {
	r, err := rekha.Trace(img, o)
	return r, o, nil, err
}

type handler struct {
	l        Limits
	sem      chan struct{}
	waiting  atomic.Int64 // requests waiting for a slot
	inflight atomic.Int64 // requests admitted: reading, waiting or tracing
	traceFn  TraceFunc
	fields   map[string]bool // extra request fields passed to traceFn
	mux      *http.ServeMux
}

// panicError is a tracer panic caught on the trace goroutine.
type panicError struct{ v any }

func (e *panicError) Error() string { return fmt.Sprintf("rekha: internal error: %v", e.v) }

// WriteGrace is how long the response write may take after the trace;
// a server's shutdown grace should allow it on top of Limits.Timeout.
const WriteGrace = 10 * time.Second

// New routes /healthz, POST /v1/trace and, when static is a directory,
// files under / for the demo page.
func New(l Limits, static string, opts ...Option) http.Handler {
	h := &handler{l: l, sem: make(chan struct{}, max(l.Concurrency, 1)), traceFn: defaultTrace, fields: map[string]bool{}, mux: http.NewServeMux()}
	for _, opt := range opts {
		opt(h)
	}
	h.mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		io.WriteString(w, "ok\n")
	})
	h.mux.HandleFunc("/v1/trace", h.trace)
	if static != "" {
		files := http.FileServer(http.Dir(static))
		h.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			// Revalidate on every load (cheap: the browser sends
			// If-Modified-Since), so a rebuilt page, worker or wasm is
			// picked up without a cache-busting query.
			w.Header().Set("Cache-Control", "no-cache")
			files.ServeHTTP(w, r)
		})
	}
	return h
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.mux.ServeHTTP(w, r) }

type httpError struct {
	status int
	msg    string
}

func (e *httpError) Error() string { return e.msg }

func fail(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if status == http.StatusServiceUnavailable {
		w.Header().Set("Retry-After", "1")
	}
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func (h *handler) trace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		fail(w, http.StatusMethodNotAllowed, "POST an image to /v1/trace")
		return
	}
	// Admission before the body is read: at most Concurrency+Queue
	// requests hold a body at once, so uploads cannot pile up memory.
	if h.inflight.Add(1) > int64(h.l.Concurrency+h.l.Queue) {
		h.inflight.Add(-1)
		fail(w, http.StatusServiceUnavailable, "busy, try again")
		return
	}
	defer h.inflight.Add(-1)
	r.Body = http.MaxBytesReader(w, r.Body, h.l.MaxBytes)
	img, fields, err := readRequest(r)
	if err != nil {
		var he *httpError
		if errors.As(err, &he) {
			fail(w, he.status, he.msg)
			return
		}
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			fail(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("request body over %d bytes", h.l.MaxBytes))
			return
		}
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	extra := map[string]string{}
	for k := range h.fields {
		if v, ok := fields[k]; ok {
			extra[k] = v
			delete(fields, k)
		}
	}
	o, err := apiopts.FromStrings(fields)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(img))
	if err != nil {
		fail(w, http.StatusBadRequest, "rekha: decode: "+err.Error())
		return
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > h.l.MaxPixels || cfg.Height > h.l.MaxPixels || cfg.Width*cfg.Height > h.l.MaxPixels {
		fail(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("image is %d×%d, over %d pixels", cfg.Width, cfg.Height, h.l.MaxPixels))
		return
	}

	// One slot per trace; at most Queue requests wait for one.
	ctx := r.Context()
	select {
	case h.sem <- struct{}{}:
	default:
		if int(h.waiting.Add(1)) > h.l.Queue {
			h.waiting.Add(-1)
			fail(w, http.StatusServiceUnavailable, "busy, try again")
			return
		}
		select {
		case h.sem <- struct{}{}:
			h.waiting.Add(-1)
		case <-time.After(h.l.Timeout):
			h.waiting.Add(-1)
			fail(w, http.StatusServiceUnavailable, fmt.Sprintf("no free slot within %s, try again", h.l.Timeout))
			return
		case <-ctx.Done():
			h.waiting.Add(-1)
			return
		}
	}

	type outcome struct {
		res *rekha.Result
		o   rekha.Options
		hdr http.Header
		err error
	}
	done := make(chan outcome, 1)
	start := time.Now()
	go func() {
		defer func() { <-h.sem }()
		defer func() {
			// net/http recovers panics on its own goroutine only; one here
			// would end the process for every client.
			if v := recover(); v != nil {
				done <- outcome{err: &panicError{v}}
			}
		}()
		decoded, _, err := image.Decode(bytes.NewReader(img))
		if err != nil {
			done <- outcome{err: fmt.Errorf("rekha: decode: %w", err)}
			return
		}
		res, o2, hdr, err := h.traceFn(ctx, decoded, o, extra)
		done <- outcome{res, o2, hdr, err}
	}()
	select {
	case out := <-done:
		if out.err != nil {
			var pe *panicError
			if errors.As(out.err, &pe) {
				fail(w, http.StatusInternalServerError, out.err.Error())
				return
			}
			fail(w, http.StatusBadRequest, out.err.Error())
			return
		}
		o = out.o
		var buf bytes.Buffer
		if err := out.res.Write(&buf, o); err != nil {
			fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		// The server's write deadline started at the request header; the
		// upload, the queue wait and the trace may have used it up.
		http.NewResponseController(w).SetWriteDeadline(time.Now().Add(WriteGrace))
		for k, vs := range out.hdr {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.Header().Set("Content-Type", contentType(o.Format))
		w.Header().Set("X-Rekha-Paths", strconv.Itoa(len(out.res.Paths)))
		w.Header().Set("X-Rekha-Millis", strconv.FormatInt(time.Since(start).Milliseconds(), 10))
		w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
		w.Write(buf.Bytes())
	case <-time.After(h.l.Timeout):
		fail(w, http.StatusGatewayTimeout, fmt.Sprintf("trace exceeded %s", h.l.Timeout))
	case <-ctx.Done():
		// Client gone; the trace finishes in the background and frees its slot.
	}
}

// readRequest returns the image bytes and the option fields: a multipart
// form with an "image" file, or a raw image/* body with query options.
func readRequest(r *http.Request) (img []byte, fields map[string]string, err error) {
	ct := r.Header.Get("Content-Type")
	fields = map[string]string{}
	switch {
	case strings.HasPrefix(ct, "multipart/"):
		mr, err := r.MultipartReader()
		if err != nil {
			return nil, nil, &httpError{http.StatusBadRequest, "multipart: " + err.Error()}
		}
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, nil, fmt.Errorf("multipart: %w", err)
			}
			if part.FormName() == "image" {
				img, err = io.ReadAll(part)
				if err != nil {
					return nil, nil, err
				}
				continue
			}
			v, err := io.ReadAll(io.LimitReader(part, maxFieldBytes+1))
			if err != nil {
				return nil, nil, err
			}
			if len(v) > maxFieldBytes {
				return nil, nil, &httpError{http.StatusBadRequest, fmt.Sprintf("field %q over %d bytes", part.FormName(), maxFieldBytes)}
			}
			fields[part.FormName()] = string(v)
		}
		if img == nil {
			return nil, nil, &httpError{http.StatusBadRequest, `multipart form has no "image" file`}
		}
	case strings.HasPrefix(ct, "image/"):
		img, err = io.ReadAll(r.Body)
		if err != nil {
			return nil, nil, err
		}
		for k, v := range r.URL.Query() {
			fields[k] = v[0]
		}
	default:
		return nil, nil, &httpError{http.StatusUnsupportedMediaType, "send multipart/form-data with an \"image\" file, or an image/* body"}
	}
	return img, fields, nil
}

// contentType is the media type of each output format.
func contentType(f rekha.Format) string {
	switch f {
	case rekha.FormatPDF:
		return "application/pdf"
	case rekha.FormatEPS:
		return "application/postscript"
	default:
		return "image/svg+xml; charset=utf-8"
	}
}

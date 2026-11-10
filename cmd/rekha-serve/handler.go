package main

import (
	"bytes"
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
	"github.com/vexyl-ai/rekha/internal/apiopts"

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

type handler struct {
	l       Limits
	sem     chan struct{}
	waiting atomic.Int64
	traceFn func(io.Reader, rekha.Options) (*rekha.Result, error)
	mux     *http.ServeMux
}

// NewHandler routes /healthz, POST /v1/trace and, when static is a
// directory, files under / for the demo page.
func NewHandler(l Limits, static string) http.Handler {
	h := &handler{l: l, sem: make(chan struct{}, max(l.Concurrency, 1)), traceFn: rekha.TraceReader, mux: http.NewServeMux()}
	h.mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		io.WriteString(w, "ok\n")
	})
	h.mux.HandleFunc("/v1/trace", h.trace)
	if static != "" {
		h.mux.Handle("/", http.FileServer(http.Dir(static)))
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
		case <-ctx.Done():
			h.waiting.Add(-1)
			return
		}
	}

	type outcome struct {
		res *rekha.Result
		err error
	}
	done := make(chan outcome, 1)
	start := time.Now()
	go func() {
		defer func() { <-h.sem }()
		res, err := h.traceFn(bytes.NewReader(img), o)
		done <- outcome{res, err}
	}()
	select {
	case out := <-done:
		if out.err != nil {
			fail(w, http.StatusBadRequest, out.err.Error())
			return
		}
		var buf bytes.Buffer
		if err := out.res.WriteSVG(&buf, o); err != nil {
			fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
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

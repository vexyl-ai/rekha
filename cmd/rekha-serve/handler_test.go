package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vexyl-ai/rekha"
)

func limits() Limits {
	return Limits{MaxBytes: 1 << 20, MaxPixels: 1 << 20, Timeout: 10 * time.Second, Concurrency: 2, Queue: 4}
}

// discPNG is a 32×32 black disc on white.
func discPNG(t testing.TB) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			dx, dy := float64(x)+0.5-16, float64(y)+0.5-16
			c := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
			if dx*dx+dy*dy < 100 {
				c = color.NRGBA{A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// hugeHeaderPNG is a valid PNG signature and IHDR declaring w×h, nothing else.
func hugeHeaderPNG(w, h uint32) []byte {
	var b bytes.Buffer
	b.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], w)
	binary.BigEndian.PutUint32(ihdr[4:], h)
	ihdr[8], ihdr[9] = 8, 2 // 8-bit RGB
	chunk := append([]byte("IHDR"), ihdr...)
	binary.Write(&b, binary.BigEndian, uint32(13))
	b.Write(chunk)
	binary.Write(&b, binary.BigEndian, crc32.ChecksumIEEE(chunk))
	return b.Bytes()
}

func multipartBody(t testing.TB, imageField string, img []byte, fields map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if imageField != "" {
		fw, err := mw.CreateFormFile(imageField, "in.png")
		if err != nil {
			t.Fatal(err)
		}
		fw.Write(img)
	}
	for k, v := range fields {
		mw.WriteField(k, v)
	}
	mw.Close()
	return &buf, mw.FormDataContentType()
}

func post(t testing.TB, h http.Handler, url, contentType string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, url, body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestServeHealthz(t *testing.T) {
	h := NewHandler(limits(), "")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != "ok" {
		t.Fatalf("%d %q", rec.Code, rec.Body.String())
	}
}

func TestServeMultipartTrace(t *testing.T) {
	body, ct := multipartBody(t, "image", discPNG(t), map[string]string{"preset": "bw", "mode": "polygon"})
	rec := post(t, NewHandler(limits(), ""), "/v1/trace", ct, body)
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "image/svg+xml") {
		t.Fatalf("content-type %q", ct)
	}
	if !strings.HasPrefix(rec.Body.String(), "<svg") || rec.Header().Get("X-Rekha-Paths") != "1" || rec.Header().Get("X-Rekha-Millis") == "" {
		t.Fatalf("body %q headers %v", rec.Body.String()[:40], rec.Header())
	}
}

func TestServeRawBodyTrace(t *testing.T) {
	rec := post(t, NewHandler(limits(), ""), "/v1/trace?preset=poster&refine=true", "image/png", bytes.NewReader(discPNG(t)))
	if rec.Code != 200 || !strings.HasPrefix(rec.Body.String(), "<svg") {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestServeBadOption(t *testing.T) {
	rec := post(t, NewHandler(limits(), ""), "/v1/trace?fitTolerance=99", "image/png", bytes.NewReader(discPNG(t)))
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "FitTolerance") || !strings.Contains(rec.Body.String(), `"error"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestServeUnknownOption(t *testing.T) {
	rec := post(t, NewHandler(limits(), ""), "/v1/trace?colour=1", "image/png", bytes.NewReader(discPNG(t)))
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "colour") {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestServeUndecodable(t *testing.T) {
	rec := post(t, NewHandler(limits(), ""), "/v1/trace", "image/png", strings.NewReader("hello"))
	if rec.Code != 400 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestServeUnsupportedMedia(t *testing.T) {
	rec := post(t, NewHandler(limits(), ""), "/v1/trace", "text/plain", strings.NewReader("hello"))
	if rec.Code != 415 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestServeTooManyBytes(t *testing.T) {
	l := limits()
	l.MaxBytes = 100
	rec := post(t, NewHandler(l, ""), "/v1/trace", "image/png", bytes.NewReader(make([]byte, 200)))
	if rec.Code != 413 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestServeHugeHeader413(t *testing.T) {
	l := limits()
	l.MaxPixels = 16_000_000
	rec := post(t, NewHandler(l, ""), "/v1/trace", "image/png", bytes.NewReader(hugeHeaderPNG(100000, 100000)))
	if rec.Code != 413 || !strings.Contains(rec.Body.String(), "pixels") {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestServeMethodNotAllowed(t *testing.T) {
	rec := httptest.NewRecorder()
	NewHandler(limits(), "").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/trace", nil))
	if rec.Code != 405 {
		t.Fatalf("%d", rec.Code)
	}
}

// blocking installs a trace hook that blocks until release is closed.
func blocking(h http.Handler) (release chan struct{}) {
	release = make(chan struct{})
	h.(*handler).traceFn = func(r io.Reader, o rekha.Options) (*rekha.Result, error) {
		<-release
		return rekha.TraceReader(r, o)
	}
	return release
}

func TestServeQueueFull503(t *testing.T) {
	l := limits()
	l.Concurrency, l.Queue = 1, 0
	h := NewHandler(l, "")
	release := blocking(h)
	first := make(chan *httptest.ResponseRecorder)
	go func() { first <- post(t, h, "/v1/trace", "image/png", bytes.NewReader(discPNG(t))) }()
	time.Sleep(50 * time.Millisecond)
	rec := post(t, h, "/v1/trace", "image/png", bytes.NewReader(discPNG(t)))
	if rec.Code != 503 || rec.Header().Get("Retry-After") != "1" {
		t.Fatalf("%d %v %s", rec.Code, rec.Header(), rec.Body.String())
	}
	close(release)
	if r := <-first; r.Code != 200 {
		t.Fatalf("first request %d %s", r.Code, r.Body.String())
	}
}

func TestServeTimeout504(t *testing.T) {
	l := limits()
	l.Timeout = 50 * time.Millisecond
	h := NewHandler(l, "")
	release := blocking(h)
	defer close(release)
	rec := post(t, h, "/v1/trace", "image/png", bytes.NewReader(discPNG(t)))
	if rec.Code != 504 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestServeDisconnectReleasesToken(t *testing.T) {
	l := limits()
	l.Concurrency, l.Queue = 1, 0
	h := NewHandler(l, "")
	release := blocking(h)
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/v1/trace", bytes.NewReader(discPNG(t))).WithContext(ctx)
	req.Header.Set("Content-Type", "image/png")
	done := make(chan struct{})
	go func() { h.ServeHTTP(httptest.NewRecorder(), req); close(done) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not return after the client disconnected")
	}
	close(release) // the abandoned trace finishes and returns its token
	for i := 0; len(h.(*handler).sem) > 0 && i < 200; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	rec := post(t, h, "/v1/trace", "image/png", bytes.NewReader(discPNG(t)))
	if rec.Code != 200 {
		t.Fatalf("second request %d %s", rec.Code, rec.Body.String())
	}
}

func TestServeStatic(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>demo</h1>"), 0o644)
	rec := httptest.NewRecorder()
	NewHandler(limits(), dir).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "demo") {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("Cache-Control %q, want no-cache so a rebuilt page or wasm is picked up on reload", cc)
	}
	rec = httptest.NewRecorder()
	NewHandler(limits(), "").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != 404 {
		t.Fatalf("no static dir: %d", rec.Code)
	}
}

func FuzzTraceHandler(f *testing.F) {
	valid := discPNG(f)
	for _, body := range [][]byte{valid, valid[:len(valid)/2], nil, []byte("random bytes"), hugeHeaderPNG(100000, 100000)} {
		for _, ct := range []string{"image/png", "multipart", "multipart-nofield", "text/plain", ""} {
			f.Add(body, ct)
		}
	}
	l := Limits{MaxBytes: 1 << 16, MaxPixels: 1 << 14, Timeout: 2 * time.Second, Concurrency: 2, Queue: 2}
	h := NewHandler(l, "")
	f.Fuzz(func(t *testing.T, body []byte, ct string) {
		var rec *httptest.ResponseRecorder
		switch ct {
		case "multipart":
			b, mct := multipartBody(t, "image", body, map[string]string{"preset": "bw"})
			rec = post(t, h, "/v1/trace", mct, b)
		case "multipart-nofield":
			b, mct := multipartBody(t, "", body, map[string]string{"other": "x"})
			rec = post(t, h, "/v1/trace", mct, b)
		default:
			rec = post(t, h, "/v1/trace", ct, bytes.NewReader(body))
		}
		switch rec.Code {
		case 200, 400, 405, 413, 415, 503, 504:
		default:
			t.Fatalf("status %d for content type %q", rec.Code, ct)
		}
	})
}

// readFails is a body that fails the test if anything reads it.
type readFails struct{ t *testing.T }

func (r readFails) Read([]byte) (int, error) {
	r.t.Error("body was read before admission")
	return 0, io.EOF
}

// A request that cannot be admitted (slot busy, queue full) is refused
// before its body is read, so waiting uploads cannot pile up memory.
func TestServeAdmissionBeforeBodyRead(t *testing.T) {
	l := limits()
	l.Concurrency, l.Queue = 1, 0
	h := NewHandler(l, "")
	release := blocking(h)
	defer close(release)
	go post(t, h, "/v1/trace", "image/png", bytes.NewReader(discPNG(t)))
	time.Sleep(50 * time.Millisecond)
	req := httptest.NewRequest(http.MethodPost, "/v1/trace", readFails{t})
	req.Header.Set("Content-Type", "image/png")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 503 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

// Waiting for a slot is bounded by the trace timeout: 503, not a hang.
func TestServeQueueWaitTimesOut(t *testing.T) {
	l := limits()
	l.Concurrency, l.Queue, l.Timeout = 1, 1, 50*time.Millisecond
	h := NewHandler(l, "")
	release := blocking(h)
	defer close(release)
	go post(t, h, "/v1/trace", "image/png", bytes.NewReader(discPNG(t)))
	time.Sleep(20 * time.Millisecond)
	start := time.Now()
	rec := post(t, h, "/v1/trace", "image/png", bytes.NewReader(discPNG(t)))
	if rec.Code != 503 || time.Since(start) > 2*time.Second {
		t.Fatalf("%d after %s: %s", rec.Code, time.Since(start), rec.Body.String())
	}
}

// A panic inside the tracer answers 500 and leaves the server running.
func TestServePanicIsolated(t *testing.T) {
	l := limits()
	l.Concurrency = 1
	h := NewHandler(l, "")
	h.(*handler).traceFn = func(io.Reader, rekha.Options) (*rekha.Result, error) { panic("boom") }
	rec := post(t, h, "/v1/trace", "image/png", bytes.NewReader(discPNG(t)))
	if rec.Code != 500 || !strings.Contains(rec.Body.String(), "boom") {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	h.(*handler).traceFn = rekha.TraceReader
	if rec := post(t, h, "/v1/trace", "image/png", bytes.NewReader(discPNG(t))); rec.Code != 200 {
		t.Fatalf("after panic: %d %s", rec.Code, rec.Body.String())
	}
}

func TestServeFormats(t *testing.T) {
	for format, want := range map[string][2]string{"pdf": {"application/pdf", "%PDF-1.4"}, "eps": {"application/postscript", "%!PS-Adobe-3.0 EPSF-3.0"}} {
		rec := post(t, NewHandler(limits(), ""), "/v1/trace?preset=bw&format="+format, "image/png", bytes.NewReader(discPNG(t)))
		if rec.Code != 200 {
			t.Fatalf("%s: %d %s", format, rec.Code, rec.Body.String())
		}
		if ct := rec.Header().Get("Content-Type"); ct != want[0] {
			t.Errorf("%s: content-type %q", format, ct)
		}
		if !strings.HasPrefix(rec.Body.String(), want[1]) {
			t.Errorf("%s: body starts %q", format, rec.Body.String()[:20])
		}
	}
}

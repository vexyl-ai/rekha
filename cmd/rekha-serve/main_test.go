package main

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/vexyl-ai/rekha"
	"github.com/vexyl-ai/rekha/server"
)

// discPNG is a 32×32 black disc on white.
func discPNG() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			dx, dy := float64(x)+0.5-16, float64(y)+0.5-16
			c := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
			if dx*dx+dy*dy <= 12*12 {
				c = color.NRGBA{A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func TestParseFlagsDefaults(t *testing.T) {
	c, err := parseFlags(nil, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if c.addr != ":8080" || c.limits.MaxBytes != 16<<20 || c.limits.MaxPixels != 16_000_000 || c.limits.Timeout != 60*time.Second || c.limits.Concurrency < 1 || c.limits.Queue != 2*c.limits.Concurrency {
		t.Fatalf("%+v", c)
	}
}

func TestParseFlagsRejectsZeroTimeout(t *testing.T) {
	if _, err := parseFlags([]string{"-timeout", "0s"}, io.Discard); err == nil {
		t.Fatal("zero timeout accepted")
	}
}

// serve must let an in-flight request finish after the context is
// cancelled: the shutdown is graceful.
func TestServeGracefulShutdown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	l := server.Limits{MaxBytes: 1 << 20, MaxPixels: 1 << 20, Timeout: 10 * time.Second, Concurrency: 2, Queue: 4}
	release := make(chan struct{})
	h := server.New(l, "", server.WithTrace(func(ctx context.Context, img image.Image, o rekha.Options, extra map[string]string) (*rekha.Result, rekha.Options, http.Header, error) {
		<-release
		r, err := rekha.Trace(img, o)
		return r, o, nil, err
	}))
	ctx, cancel := context.WithCancel(context.Background())
	srvDone := make(chan error, 1)
	go func() { srvDone <- serve(ctx, &http.Server{Handler: h}, ln, l.Timeout) }()
	resp := make(chan *http.Response, 1)
	go func() {
		r, err := http.Post("http://"+ln.Addr().String()+"/v1/trace", "image/png", bytes.NewReader(discPNG()))
		if err != nil {
			t.Error(err)
			resp <- nil
			return
		}
		resp <- r
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)
	select {
	case <-srvDone:
		t.Fatal("serve returned while a request was in flight")
	default:
	}
	close(release)
	r := <-resp
	if r == nil || r.StatusCode != 200 {
		t.Fatalf("in-flight request: %v", r)
	}
	r.Body.Close()
	if err := <-srvDone; err != nil {
		t.Fatalf("serve: %v", err)
	}
}

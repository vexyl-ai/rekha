package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

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
	h := NewHandler(limits(), "")
	release := blocking(h)
	ctx, cancel := context.WithCancel(context.Background())
	srvDone := make(chan error, 1)
	go func() { srvDone <- serve(ctx, &http.Server{Handler: h}, ln, limits().Timeout) }()
	resp := make(chan *http.Response, 1)
	go func() {
		r, err := http.Post("http://"+ln.Addr().String()+"/v1/trace", "image/png", bytes.NewReader(discPNG(t)))
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

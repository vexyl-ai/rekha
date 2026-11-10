package main

import (
	"io"
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

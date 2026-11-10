// Command rekha-serve is an HTTP front for Rekha: POST /v1/trace returns
// an SVG; caps on bytes, pixels, time, concurrency and queue keep one
// client from exhausting the host. See docs/server.md.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"
)

const version = "dev"

type config struct {
	addr    string
	static  string
	limits  Limits
	version bool
}

func parseFlags(args []string, stderr io.Writer) (config, error) {
	fs := flag.NewFlagSet("rekha-serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var c config
	fs.StringVar(&c.addr, "addr", ":8080", "listen address")
	fs.StringVar(&c.static, "static", "", "directory served at / (the demo page: web/)")
	fs.Int64Var(&c.limits.MaxBytes, "max-bytes", 16<<20, "largest request body in bytes")
	fs.IntVar(&c.limits.MaxPixels, "max-pixels", 16_000_000, "largest image in pixels")
	fs.DurationVar(&c.limits.Timeout, "timeout", 60*time.Second, "wall time allowed for one trace")
	fs.IntVar(&c.limits.Concurrency, "concurrency", runtime.GOMAXPROCS(0), "traces running at once")
	fs.IntVar(&c.limits.Queue, "queue", 2*runtime.GOMAXPROCS(0), "requests waiting for a slot before 503")
	fs.BoolVar(&c.version, "version", false, "print version and exit")
	if err := fs.Parse(args); err != nil {
		return c, err
	}
	if c.limits.MaxBytes <= 0 || c.limits.MaxPixels <= 0 || c.limits.Timeout <= 0 || c.limits.Concurrency <= 0 || c.limits.Queue < 0 {
		return c, errors.New("limits must be positive (queue may be 0)")
	}
	return c, nil
}

func main() {
	c, err := parseFlags(os.Args[1:], os.Stderr)
	if err != nil {
		os.Exit(2)
	}
	if c.version {
		fmt.Println("rekha-serve " + version)
		return
	}
	srv := &http.Server{
		Addr:              c.addr,
		Handler:           NewHandler(c.limits, c.static),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       c.limits.Timeout + 10*time.Second,
		WriteTimeout:      c.limits.Timeout + 10*time.Second,
		IdleTimeout:       60 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), c.limits.Timeout)
		defer cancel()
		srv.Shutdown(shutdown)
	}()
	log.Printf("rekha-serve %s listening on %s (max %d bytes, %d pixels, %s, %d concurrent, queue %d)", version, c.addr, c.limits.MaxBytes, c.limits.MaxPixels, c.limits.Timeout, c.limits.Concurrency, c.limits.Queue)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

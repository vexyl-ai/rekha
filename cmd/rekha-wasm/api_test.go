//go:build js && wasm

package main

import (
	"bytes"
	"image"
	"image/color"
	"strings"
	"syscall/js"
	"testing"

	"github.com/vexyl-ai/rekha"
)

func init() { install() }

// disc is a 64×64 anti-aliased dark disc on white (or on transparent).
func disc(transparent bool) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			dx, dy := float64(x)+0.5-32, float64(y)+0.5-32
			cov := 24.5 - (dx*dx+dy*dy)/24
			if cov < 0 {
				cov = 0
			}
			if cov > 1 {
				cov = 1
			}
			a := uint8(cov*255 + 0.5)
			if transparent {
				img.SetNRGBA(x, y, color.NRGBA{R: 20, G: 40, B: 200, A: a})
			} else {
				v := 255 - a
				img.SetNRGBA(x, y, color.NRGBA{R: v, G: v, B: v, A: 255})
			}
		}
	}
	return img
}

func jsImage(img *image.NRGBA) js.Value {
	data := js.Global().Get("Uint8ClampedArray").New(len(img.Pix))
	js.CopyBytesToJS(data, img.Pix)
	obj := js.Global().Get("Object").New()
	obj.Set("width", img.Rect.Dx())
	obj.Set("height", img.Rect.Dy())
	obj.Set("data", data)
	return obj
}

func api() js.Value { return js.Global().Get("rekha") }

func nativeSVG(t *testing.T, img image.Image, opts string) string {
	t.Helper()
	o, err := rekha.PresetOptions(rekha.PresetBW), error(nil)
	if opts != "" {
		o, err = parseOptions(js.ValueOf(opts))
		if err != nil {
			t.Fatal(err)
		}
	}
	res, err := rekha.Trace(img, o)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := res.WriteSVG(&buf, o); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestWasmTraceMatchesNative(t *testing.T) {
	img := disc(false)
	r := api().Call("trace", jsImage(img), `{"preset":"bw","refine":true}`)
	if !r.Get("ok").Bool() {
		t.Fatalf("trace failed: %s", r.Get("error").String())
	}
	if got, want := r.Get("svg").String(), nativeSVG(t, img, `{"preset":"bw","refine":true}`); got != want {
		t.Fatalf("svg differs from native:\n%s\n---\n%s", got, want)
	}
	if r.Get("width").Int() != 64 || r.Get("paths").Int() < 1 || r.Get("bytes").Int() == 0 || !(r.Get("millis").Float() >= 0) {
		t.Fatalf("result fields: %v", js.Global().Get("JSON").Call("stringify", r).String())
	}
}

func TestWasmAlphaMatchesTrace(t *testing.T) {
	img := disc(true)
	r := api().Call("trace", jsImage(img), `{"preset":"poster"}`)
	if !r.Get("ok").Bool() {
		t.Fatalf("trace failed: %s", r.Get("error").String())
	}
	if got, want := r.Get("svg").String(), nativeSVG(t, img, `{"preset":"poster"}`); got != want {
		t.Fatalf("svg differs from native:\n%s\n---\n%s", got, want)
	}
}

func TestWasmSegmentFinishFree(t *testing.T) {
	img := disc(false)
	s := api().Call("segment", jsImage(img), `{"preset":"bw"}`)
	if !s.Get("ok").Bool() {
		t.Fatalf("segment failed: %s", s.Get("error").String())
	}
	h := s.Get("handle")
	if s.Get("palette").Length() != 2 || s.Get("width").Int() != 64 {
		t.Fatalf("segment result: %s", js.Global().Get("JSON").Call("stringify", s).String())
	}
	plain := api().Call("finish", h, `{"preset":"bw"}`)
	refined := api().Call("finish", h, `{"preset":"bw","refine":true}`)
	if !plain.Get("ok").Bool() || !refined.Get("ok").Bool() {
		t.Fatalf("finish failed: %s / %s", plain.Get("error").String(), refined.Get("error").String())
	}
	if plain.Get("svg").String() == refined.Get("svg").String() {
		t.Fatal("refine had no effect")
	}
	api().Call("free", h)
	after := api().Call("finish", h, `{"preset":"bw"}`)
	if after.Get("ok").Bool() || !strings.Contains(after.Get("error").String(), "handle") {
		t.Fatalf("finish after free: %s", js.Global().Get("JSON").Call("stringify", after).String())
	}
}

func TestWasmBadOptions(t *testing.T) {
	r := api().Call("trace", jsImage(disc(false)), `{"fitTolerance":99}`)
	if r.Get("ok").Bool() || !strings.Contains(r.Get("error").String(), "FitTolerance") {
		t.Fatalf("%s", js.Global().Get("JSON").Call("stringify", r).String())
	}
}

func TestWasmBadImageThrows(t *testing.T) {
	short := jsImage(disc(false))
	short.Set("data", short.Get("data").Call("subarray", 0, 64*64*4-1))
	r := api().Call("trace", short, `{}`)
	if r.Get("ok").Bool() || !strings.Contains(r.Get("error").String(), "rekha: image") {
		t.Fatalf("short data: %s", js.Global().Get("JSON").Call("stringify", r).String())
	}
	zero := jsImage(disc(false))
	zero.Set("width", 0)
	r = api().Call("trace", zero, `{}`)
	if r.Get("ok").Bool() {
		t.Fatal("width 0 accepted")
	}
}

func TestWasmOptionsObject(t *testing.T) {
	obj := js.Global().Get("Object").New()
	obj.Set("preset", "poster")
	r := api().Call("trace", jsImage(disc(true)), obj)
	if !r.Get("ok").Bool() {
		t.Fatalf("object options: %s", r.Get("error").String())
	}
	if got, want := r.Get("svg").String(), nativeSVG(t, disc(true), `{"preset":"poster"}`); got != want {
		t.Fatal("object options gave a different svg than the JSON string")
	}
}

func TestWasmVersion(t *testing.T) {
	if v := api().Call("version"); v.Type() != js.TypeString || v.String() == "" {
		t.Fatalf("version %v", v)
	}
}

// A third argument to segment, finish and trace is a progress callback
// (stage, done, total); it is optional.
func TestProgressCallback(t *testing.T) {
	var calls []string
	cb := js.FuncOf(func(_ js.Value, args []js.Value) any {
		calls = append(calls, args[0].String())
		if args[1].Int() > args[2].Int() {
			t.Errorf("done %d > total %d", args[1].Int(), args[2].Int())
		}
		return nil
	})
	defer cb.Release()
	r := api().Call("segment", jsImage(disc(false)), `{"preset":"poster"}`, cb)
	if !r.Get("ok").Bool() {
		t.Fatalf("segment: %s", r.Get("error").String())
	}
	if len(calls) == 0 || calls[0] != "quantise" {
		t.Fatalf("segment progress calls %v, want quantise first", calls)
	}
	n := len(calls)
	r = api().Call("finish", r.Get("handle"), `{"preset":"poster"}`, cb)
	if !r.Get("ok").Bool() {
		t.Fatalf("finish: %s", r.Get("error").String())
	}
	if len(calls) == n || calls[len(calls)-1] != "curves" {
		t.Fatalf("finish progress calls %v, want curves last", calls[n:])
	}
	if r = api().Call("trace", jsImage(disc(false)), `{"preset":"poster"}`); !r.Get("ok").Bool() {
		t.Fatalf("trace without a callback: %s", r.Get("error").String())
	}
}

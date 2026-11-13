//go:build js && wasm

package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"strings"
	"sync"
	"syscall/js"
	"time"

	"github.com/vexyl-ai/rekha"
	"github.com/vexyl-ai/rekha/internal/apiopts"
)

const version = "dev"

// Every method returns a plain object {ok: true, …} or {ok: false, error};
// syscall/js cannot throw, so the worker turns a failure into a rejection.

var (
	mu      sync.Mutex
	handles = map[int]*rekha.Segmentation{}
	nextID  = 1
)

// install sets the global "rekha" object.
func install() {
	obj := js.Global().Get("Object").New()
	obj.Set("version", js.FuncOf(func(js.Value, []js.Value) any { return version }))
	obj.Set("segment", js.FuncOf(func(_ js.Value, args []js.Value) any {
		return guard(func() (js.Value, error) {
			img, o, err := imageAndOptions(args)
			if err != nil {
				return js.Null(), err
			}
			o.Progress = progressOf(args, 2)
			return segment(img, o)
		})
	}))
	obj.Set("finish", js.FuncOf(func(_ js.Value, args []js.Value) any {
		return guard(func() (js.Value, error) {
			if len(args) < 2 {
				return js.Null(), errors.New("rekha: finish(handle, options)")
			}
			o, err := parseOptions(args[1])
			if err != nil {
				return js.Null(), err
			}
			o.Progress = progressOf(args, 2)
			return finish(args[0], o)
		})
	}))
	obj.Set("convert", js.FuncOf(func(_ js.Value, args []js.Value) any {
		return guard(func() (js.Value, error) {
			if len(args) < 2 || args[0].Type() != js.TypeString {
				return js.Null(), errors.New("rekha: convert(svgText, options)")
			}
			o, err := parseOptions(args[1])
			if err != nil {
				return js.Null(), err
			}
			return convert(args[0].String(), o)
		})
	}))
	obj.Set("trace", js.FuncOf(func(_ js.Value, args []js.Value) any {
		return guard(func() (js.Value, error) {
			img, o, err := imageAndOptions(args)
			if err != nil {
				return js.Null(), err
			}
			o.Progress = progressOf(args, 2)
			start := time.Now()
			s, err := rekha.Segment(img, o)
			if err != nil {
				return js.Null(), err
			}
			res, err := s.Finish(o)
			if err != nil {
				return js.Null(), err
			}
			out, err := svgResult(res, o)
			if err != nil {
				return js.Null(), err
			}
			out.Set("width", res.Width)
			out.Set("height", res.Height)
			out.Set("palette", paletteJS(s))
			out.Set("millis", ms(start))
			return out, nil
		})
	}))
	obj.Set("free", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) == 1 && args[0].Type() == js.TypeNumber {
			mu.Lock()
			delete(handles, args[0].Int())
			mu.Unlock()
		}
		return js.Undefined()
	}))
	js.Global().Set("rekha", obj)
}

// progressOf wraps args[i], when it is a function, as a Progress that
// calls it with (stage, done, total); otherwise nil.
func progressOf(args []js.Value, i int) rekha.Progress {
	if len(args) <= i || args[i].Type() != js.TypeFunction {
		return nil
	}
	cb := args[i]
	return func(stage string, done, total int) { cb.Invoke(stage, done, total) }
}

// guard runs fn and shapes its outcome as {ok, …} or {ok: false, error},
// turning a panic into an error so the worker survives.
func guard(fn func() (js.Value, error)) (out js.Value) {
	defer func() {
		if r := recover(); r != nil {
			out = failure(fmt.Sprintf("rekha: internal error: %v", r))
		}
	}()
	v, err := fn()
	if err != nil {
		return failure(err.Error())
	}
	v.Set("ok", true)
	return v
}

func failure(msg string) js.Value {
	obj := js.Global().Get("Object").New()
	obj.Set("ok", false)
	obj.Set("error", msg)
	return obj
}

func imageAndOptions(args []js.Value) (*image.NRGBA, rekha.Options, error) {
	if len(args) < 1 {
		return nil, rekha.Options{}, errors.New("rekha: image argument missing")
	}
	img, err := imageFromJS(args[0])
	if err != nil {
		return nil, rekha.Options{}, err
	}
	var opts js.Value = js.Undefined()
	if len(args) > 1 {
		opts = args[1]
	}
	o, err := parseOptions(opts)
	if err != nil {
		return nil, rekha.Options{}, err
	}
	return img, o, nil
}

// parseOptions accepts a JSON string, an object (stringified here), or
// undefined/null (defaults).
func parseOptions(v js.Value) (rekha.Options, error) {
	b, err := optionsJSON(v)
	if err != nil {
		return rekha.Options{}, err
	}
	return apiopts.Parse(b)
}

func optionsJSON(v js.Value) ([]byte, error) {
	switch v.Type() {
	case js.TypeUndefined, js.TypeNull:
		return nil, nil
	case js.TypeString:
		return []byte(v.String()), nil
	case js.TypeObject:
		return []byte(js.Global().Get("JSON").Call("stringify", v).String()), nil
	default:
		return nil, fmt.Errorf("rekha: options must be an object or a JSON string, not %s", v.Type())
	}
}

// imageFromJS reads {width, height, data} with data a Uint8ClampedArray
// (or Uint8Array) of straight-alpha RGBA, as ImageData provides.
func imageFromJS(v js.Value) (*image.NRGBA, error) {
	if v.Type() != js.TypeObject {
		return nil, errors.New("rekha: image must be {width, height, data}")
	}
	w, h, data := v.Get("width"), v.Get("height"), v.Get("data")
	if w.Type() != js.TypeNumber || h.Type() != js.TypeNumber || data.Type() != js.TypeObject {
		return nil, errors.New("rekha: image must be {width, height, data}")
	}
	wi, hi := w.Int(), h.Int()
	if wi <= 0 || hi <= 0 {
		return nil, fmt.Errorf("rekha: image size %d×%d", wi, hi)
	}
	n := data.Get("length").Int()
	if n != wi*hi*4 {
		return nil, fmt.Errorf("rekha: image data has %d bytes, %d×%d RGBA needs %d", n, wi, hi, wi*hi*4)
	}
	img := image.NewNRGBA(image.Rect(0, 0, wi, hi))
	js.CopyBytesToGo(img.Pix, data)
	return img, nil
}

func segment(img *image.NRGBA, o rekha.Options) (js.Value, error) {
	start := time.Now()
	s, err := rekha.Segment(img, o)
	if err != nil {
		return js.Null(), err
	}
	mu.Lock()
	id := nextID
	nextID++
	handles[id] = s
	mu.Unlock()
	out := js.Global().Get("Object").New()
	out.Set("handle", id)
	out.Set("width", s.Width)
	out.Set("height", s.Height)
	out.Set("palette", paletteJS(s))
	out.Set("millis", ms(start))
	return out, nil
}

func finish(h js.Value, o rekha.Options) (js.Value, error) {
	if h.Type() != js.TypeNumber {
		return js.Null(), errors.New("rekha: handle must be a number")
	}
	mu.Lock()
	s, ok := handles[h.Int()]
	mu.Unlock()
	if !ok {
		return js.Null(), fmt.Errorf("rekha: unknown handle %d (freed or never returned by segment)", h.Int())
	}
	start := time.Now()
	res, err := s.Finish(o)
	if err != nil {
		return js.Null(), err
	}
	out, err := svgResult(res, o)
	if err != nil {
		return js.Null(), err
	}
	out.Set("millis", ms(start))
	return out, nil
}

// convert rewrites an SVG Rekha wrote (edited in the page or not) in
// o.Format and returns {bytes: Uint8Array, format}.
func convert(svgText string, o rekha.Options) (js.Value, error) {
	var buf bytes.Buffer
	if err := rekha.Convert(strings.NewReader(svgText), &buf, o); err != nil {
		return js.Null(), err
	}
	f, _ := rekha.ParseFormat(string(o.Format))
	data := js.Global().Get("Uint8Array").New(buf.Len())
	js.CopyBytesToJS(data, buf.Bytes())
	out := js.Global().Get("Object").New()
	out.Set("bytes", data)
	out.Set("format", string(f))
	return out, nil
}

func svgResult(res *rekha.Result, o rekha.Options) (js.Value, error) {
	if o.Format != "" && o.Format != rekha.FormatSVG {
		return js.Null(), fmt.Errorf("rekha: the wasm build writes SVG only, not %s", o.Format)
	}
	var buf bytes.Buffer
	if err := res.WriteSVG(&buf, o); err != nil {
		return js.Null(), err
	}
	out := js.Global().Get("Object").New()
	out.Set("svg", buf.String())
	out.Set("paths", len(res.Paths))
	out.Set("bytes", buf.Len())
	return out, nil
}

func paletteJS(s *rekha.Segmentation) js.Value {
	arr := js.Global().Get("Array").New(len(s.Palette))
	for i, c := range s.Palette {
		arr.SetIndex(i, strings.ToLower(fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)))
	}
	return arr
}

func ms(start time.Time) float64 { return float64(time.Since(start).Microseconds()) / 1000 }

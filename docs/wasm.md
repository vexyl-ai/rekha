# Rekha in the browser (WebAssembly)

`cmd/rekha-wasm` is the Go tracer compiled with `GOOS=js GOARCH=wasm`. It installs a global `rekha` object; the browser decodes the image, the wasm traces raw RGBA, and the SVG comes back as text. Nothing is uploaded anywhere.

```
make wasm            # web/demo/rekha.wasm (4.4 MB, 1.2 MB gzipped) + web/demo/wasm_exec.js from GOROOT
make wasm-test       # go test under Node through go_js_wasm_exec
make serve           # rekha-serve -static web → http://localhost:8080/ (site) and /demo/ (playground)
```

`web/` is the site: `index.html` is the landing page (product page with a pixels-versus-SVG wipe over the Rekha logo, the comparison against Adobe Express and VTracer, and the licence terms) and `web/demo/` is the playground: drop, paste or choose an image, wipe between source and SVG with a draggable divider, zoom and pan the canvas (ctrl or ⌘ + wheel or a pinch zooms about the cursor, wheel or drag pans, double-click zooms in, the toolbar and the keys `+` `-` `0` fit `1` actual pixels do the same; the SVG is rasterised at the zoom shown, so detail is real), move the controls, tidy the result and download. Edits run on the inline SVG, in `web/demo/edit.js` (pure decisions, tested with `node --test`) and `app.js` (the DOM): a shape is every path with the same data (a layered gradient is a base path under translucent overlays), click selects, shift-click adds, double-click selects a colour, Delete removes, the colour input repaints (a gradient keeps each stop's offset in hue, saturation and lightness from the first), Remove background drops shapes whose box touches three borders and covers at least 40% of the image plus border shapes of the same fill, and a history of serialised states gives undo and redo; the download is the inline SVG with the writer's size restored and unused gradients pruned. A new trace starts a fresh history. The page keeps to the viewport height; the control rail scrolls on its own. It needs no build step beyond `make wasm` and any static file server. CI builds and tests the wasm on every push.

## JS API

`rekha.wasm` must run under Go's `wasm_exec.js` (`new Go()`, `go.run(instance)`). Every method is synchronous and returns a plain object with `ok`; on failure `error` carries the Go message (`rekha: …`). Run it in a Web Worker: a trace takes 0.1–30 s and would freeze the page.

| Method | Arguments | Returns |
|---|---|---|
| `version()` | – | string |
| `segment(image, options, onProgress?)` | `image = {width, height, data}` with `data` a `Uint8ClampedArray` of straight-alpha RGBA (what `ImageData` gives); `options` an object or JSON string | `{ok, handle, width, height, palette: ["#rrggbb", …], millis}` |
| `finish(handle, options, onProgress?)` | a handle from `segment`; the full option set (it is validated whole) | `{ok, svg, paths, bytes, millis}` |
| `trace(image, options, onProgress?)` | segment + finish without keeping a handle | `{ok, svg, paths, bytes, width, height, palette, millis}` |
| `free(handle)` | – | undefined |

Options use the CLI flag names in camelCase: `preset` (`bw`, `poster`, `photo`; seeds every other default exactly as the CLI), `mode`, `hierarchical`, `maxColors`, `colorTolerance`, `colorPrecision`, `threshold` (number or `"auto"`), `turnPolicy`, `filterSpeckle`, `simplify`, `cornerThreshold`, `fitTolerance`, `palette` (`"#rrggbb,#rrggbb"` or an array), `refine`, `refinePasses`, `stroke` (`cutoutStroke` still accepted), `gradients`, `gradientTolerance`, `pathPrecision`, `noMark`. `format` on `trace` and `finish` must be `svg`; PDF and EPS come from `rekha.convert(svgText, options)`, which reads an SVG Rekha wrote (edited in the page or not) back and returns `{bytes: Uint8Array, format}` in the `format` the options name. The worker exposes it as the `convert` op and the page's download uses it on the edited inline SVG. Unknown keys and out-of-range values are errors with the field named. The authoritative list is `internal/apiopts.Params`.

`onProgress(stage, done, total)`, when given, is called as the work advances, synchronously from inside the call: stages `quantise`, `label`, `ramps` (segment, no count: `total` 0) and `outlines`, `nest`, `edges`, `refine`, `curves` (finish; `outlines`, `edges`, `refine` and `curves` count `done` of `total`, at most one call per whole percent; `nest` has no count). Counts may arrive out of order; keep the maximum.

`segment` is the expensive, option-light half (quantisation); `finish` is the fit. Keep the handle and call `finish` again when the user moves a control that is not one of `preset`, `threshold`, `colorPrecision`, `colorTolerance`, `maxColors`, `palette`, `filterSpeckle`; re-segment when one of those changes. Handles hold about 3 bytes per pixel until freed.

## Worker protocol (web/demo/worker.js)

```
page → worker  {id, op: "segment", width, height, data: ArrayBuffer (transferred), options}
page → worker  {id, op: "finish", handle, options}
page → worker  {id, op: "free", handle}
worker → page  {op: "ready", version}             once, after instantiation
worker → page  {id, op: "progress", stage, done, total}   while a segment or finish runs
worker → page  {id, ok: true, result}             result as the wasm method returned it, minus ok
worker → page  {id, ok: false, error}
```

`web/demo/protocol.js` holds the segment-key list and a generation counter so a reply to a superseded request is dropped. Cancel by `worker.terminate()` and starting a new worker; the handle is gone, so the next change re-segments.

## Embedding

```html
<script src="wasm_exec.js"></script>
<script>
const go = new Go();
WebAssembly.instantiateStreaming(fetch("rekha.wasm"), go.importObject).then(({instance}) => {
  go.run(instance);                       // installs window.rekha, never returns
  const ctx = canvas.getContext("2d");
  const img = ctx.getImageData(0, 0, canvas.width, canvas.height);
  const r = rekha.trace(img, {preset: "poster", refine: true});
  if (!r.ok) throw new Error(r.error);
  console.log(r.svg);
});
</script>
```

`ImageData` already has `width`, `height` and `data`, so it can be passed as is. Serve `rekha.wasm` as `application/wasm` (or `instantiateStreaming` falls back to `arrayBuffer`).

## Limits

- One thread. Go's wasm target has no threads; the parallel pieces of refinement run inline. Output is identical to the CLI (determinism across GOMAXPROCS is tested).
- Memory grows and never shrinks within a page. The demo caps input at 4 megapixels and downscales beyond it; raise the cap in the page if the device can take it.
- No decoders inside: the browser's `createImageBitmap` handles PNG, JPEG, GIF, WebP, AVIF and HEIC faster and keeps the binary small.
- No cancellation inside a call; terminate the worker.

Latency against native: `bench/results/2026-11-10-phase4-wasm.md`.

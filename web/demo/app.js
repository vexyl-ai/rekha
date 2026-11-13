// Demo page: decodes the image in the browser (file, drop or paste), runs
// rekha.wasm in a worker, re-segments only when a segmentation option
// changed, drops stale replies, frees replaced handles, and cancels by
// terminating the worker. The result is shown as a wipe over the source,
// inline, so a shape can be picked and deleted or recoloured before the
// download.
const { needsResegment, Requests, Handles, imageFileFromItems, progressText, Viewport } = self.RekhaProtocol;
const { groupShapes, backgroundShapes, pruneDefs, History, tintStops, selectionText } = self.RekhaEdit;

const $ = (id) => document.getElementById(id);
const status = $('status');
const requests = new Requests();

// Preset defaults mirror rekha.PresetOptions so the form shows what runs;
// the photo preset, the page's starting one, also turns gradients on.
const PRESETS = {
  bw:     { hierarchical: 'cutout',  maxColors: 2, colorTolerance: 0,    filterSpeckle: 4,  gradients: false },
  poster: { hierarchical: 'stacked', maxColors: 0, colorTolerance: 0.08, filterSpeckle: 4,  gradients: false },
  photo:  { hierarchical: 'stacked', maxColors: 0, colorTolerance: 0.04, filterSpeckle: 16, gradients: true },
};
const COMMON = { mode: 'spline', threshold: 0.5, thresholdAuto: false, refine: false, refinePasses: 3, stroke: 0, pathPrecision: 2, gradientTolerance: 0.03 };

let worker = null;
let pending = new Map();         // id → {resolve, reject}
let image = null;                // {width, height, data, note}
let handles = new Handles();     // the live wasm segmentation handle
let cachedOptions = null;        // options the handle was segmented with
let segmentMs = null;
let svgURL = '';
let sourceURL = '';
let svgEl = null;                // the inline result
let svgSize = ['', ''];          // its width and height attributes, for the download
let history = null;              // History of serialised states
let selected = new Set();        // path data of the selected shapes
let tintBase = null;             // gradient id → original stop colours, while the colour input drags

function applyPreset(name) {
  const p = { ...COMMON, ...PRESETS[name] };
  for (const [k, v] of Object.entries(p)) {
    const el = $(k);
    if (!el) continue;
    if (el.type === 'checkbox') el.checked = v; else el.value = v;
  }
}

function readOptions() {
  return {
    preset: $('preset').value,
    mode: $('mode').value,
    hierarchical: $('hierarchical').value,
    maxColors: +$('maxColors').value,
    colorTolerance: +$('colorTolerance').value,
    threshold: $('thresholdAuto').checked ? 'auto' : +$('threshold').value,
    filterSpeckle: +$('filterSpeckle').value,
    gradients: $('gradients').checked,
    gradientTolerance: +$('gradientTolerance').value,
    refine: $('refine').checked,
    refinePasses: +$('refinePasses').value,
    stroke: +$('stroke').value,
    pathPrecision: +$('pathPrecision').value,
  };
}

function setStatus(text, kind) {
  status.textContent = text;
  status.className = kind || '';
  // While working, the same text sits in an overlay in the middle of the
  // page: on a big image the bottom bar is off screen or overlooked.
  const busy = $('busy');
  busy.hidden = kind !== 'working';
  if (!busy.hidden) $('busyText').textContent = text;
  setProgress(null, 0, 0);
  document.body.setAttribute('aria-busy', busy.hidden ? 'false' : 'true');
}

// setProgress shows the tracer's stage under the overlay text and fills
// the bar when the stage has a count; a stage without one keeps the bar
// sweeping. Null clears it for the next request.
let progressMax = 0;
function setProgress(stage, done, total) {
  const bar = $('busyBar');
  const fill = $('busyFill');
  if (stage === null) {
    progressMax = 0;
    $('busyStage').textContent = '';
    bar.classList.add('sweep');
    bar.removeAttribute('aria-valuenow');
    fill.style.width = '0%';
    return;
  }
  // Parallel work reports out of order: never step backwards.
  if (done < progressMax) done = progressMax;
  progressMax = done;
  $('busyStage').textContent = progressText(stage, done, total);
  if (total > 0) {
    const pct = Math.round(100 * done / total);
    bar.classList.remove('sweep');
    bar.setAttribute('aria-valuenow', String(pct));
    fill.style.width = pct + '%';
  } else {
    bar.classList.add('sweep');
    bar.removeAttribute('aria-valuenow');
    fill.style.width = '0%';
  }
}

function startWorker() {
  if (worker) worker.terminate();
  for (const p of pending.values()) p.reject(new Error('cancelled'));
  pending = new Map();
  handles = new Handles(); // the old worker took its handles with it
  cachedOptions = null; segmentMs = null;
  worker = new Worker('worker.js');
  worker.onmessage = (ev) => {
    const m = ev.data;
    if (m.op === 'ready') { if (!image) setStatus('Ready. Drop an image, paste one, or choose a file.'); return; }
    if (m.op === 'progress') { if (requests.accept(m.id)) setProgress(m.stage, m.done, m.total); return; }
    const p = pending.get(m.id);
    if (!p) return;
    pending.delete(m.id);
    if (m.ok) p.resolve(m.result); else p.reject(new Error(m.error));
  };
  worker.onerror = (e) => setStatus('The tracer stopped: ' + e.message + '. Reload the page.', 'error');
}

function free(handle) {
  if (handle !== null && worker) worker.postMessage({ id: 0, op: 'free', handle });
}

function call(msg, transfer) {
  const id = requests.next();
  return new Promise((resolve, reject) => {
    pending.set(id, { resolve, reject });
    worker.postMessage({ id, ...msg }, transfer || []);
  }).then((r) => {
    if (!requests.accept(id)) {
      if (msg.op === 'segment') free(r.handle); // nobody will adopt it
      throw new Error('superseded');
    }
    return r;
  });
}

async function decode(file) {
  const bitmap = await createImageBitmap(file);
  const cap = Math.max(10000, +$('pixelCap').value || 4300000);
  let { width, height } = bitmap;
  let note = '';
  if (width * height > cap) {
    const s = Math.sqrt(cap / (width * height));
    width = Math.max(1, Math.floor(width * s));
    height = Math.max(1, Math.floor(height * s));
    note = `, downscaled from ${bitmap.width}×${bitmap.height}`;
  }
  const canvas = typeof OffscreenCanvas !== 'undefined' ? new OffscreenCanvas(width, height) : Object.assign(document.createElement('canvas'), { width, height });
  const ctx = canvas.getContext('2d');
  ctx.drawImage(bitmap, 0, 0, width, height);
  const data = ctx.getImageData(0, 0, width, height).data;
  bitmap.close();
  return { width, height, data, note };
}

// The stage is a viewport over the compare box: the box is laid out at
// the image's natural size times the zoom (so the SVG is rasterised at
// what is shown, not scaled up from a thumbnail) and placed by the
// viewport's offset. The view keeps a 20 px margin when fitted.
const vp = new Viewport();
const MARGIN = 20;
function viewSize() {
  const r = stage.getBoundingClientRect();
  return [Math.max(1, r.width - 2 * MARGIN), Math.max(1, r.height - 2 * MARGIN)];
}
function render() {
  const w = vp.image.w * vp.z, h = vp.image.h * vp.z;
  compare.style.width = w + 'px';
  compare.style.height = h + 'px';
  compare.style.left = (vp.x + MARGIN) + 'px';
  compare.style.top = (vp.y + MARGIN) + 'px';
  $('zoomLevel').textContent = Math.round(vp.z * 100) + '%';
  $('zoomFit').disabled = vp.fitted;
}
function fitCompare() {
  const img = $('sourceImg');
  if (!img.naturalWidth) return;
  vp.setView(...viewSize());
  vp.setImage(img.naturalWidth, img.naturalHeight);
  render();
}
function resized() {
  if (!vp.image.w) return;
  vp.setView(...viewSize());
  render();
}
// stagePoint maps a pointer event to view coordinates (margin removed).
function stagePoint(e) {
  const r = stage.getBoundingClientRect();
  return [e.clientX - r.left - MARGIN, e.clientY - r.top - MARGIN];
}
function zoomBy(factor, at) {
  if (!vp.image.w) return;
  const [w, h] = viewSize();
  vp.zoomAt(factor, ...(at || [w / 2, h / 2]));
  render();
}

function showSource(file) {
  if (sourceURL) URL.revokeObjectURL(sourceURL);
  sourceURL = URL.createObjectURL(file);
  $('sourceImg').src = sourceURL;
  clip.replaceChildren();
  svgEl = null; history = null; selected = new Set();
  $('compare').hidden = false;
  $('zoom').hidden = false;
  $('tools').hidden = true;
  $('drop').hidden = true;
  $('download').disabled = true;
  $('format').disabled = true;
}

// showSVG puts a fresh trace on the stage and starts its edit history.
function showSVG(svg) {
  history = new History(svg);
  mount(svg);
  $('tools').hidden = false;
}

// mount replaces the inline SVG with the given state; the download is
// rebuilt from it with unused gradients dropped.
function mount(svg) {
  const doc = new DOMParser().parseFromString(svg, 'image/svg+xml');
  svgEl = doc.documentElement;
  svgSize = [svgEl.getAttribute('width'), svgEl.getAttribute('height')];
  svgEl.setAttribute('width', '100%');
  svgEl.setAttribute('height', '100%');
  clip.replaceChildren(svgEl);
  selected = new Set();
  tintBase = null;
  refreshSelection();
  if (svgURL) URL.revokeObjectURL(svgURL);
  svgURL = URL.createObjectURL(new Blob([serialise()], { type: 'image/svg+xml' }));
  $('download').disabled = false;
  $('format').disabled = false;
  $('undo').disabled = !history.canUndo;
  $('redo').disabled = !history.canRedo;
}

// serialise returns the drawing as the file to download: the writer's
// size restored, the selection marks dropped, orphaned gradients pruned.
function serialise() {
  const copy = svgEl.cloneNode(true);
  copy.setAttribute('width', svgSize[0]);
  copy.setAttribute('height', svgSize[1]);
  for (const el of copy.querySelectorAll('[class]')) el.removeAttribute('class');
  return pruneDefs(new XMLSerializer().serializeToString(copy));
}

// commit records the current drawing as a new state after an edit and
// reports it.
function commit(note) {
  const svg = serialise();
  history.push(svg);
  mount(svg);
  report({ paths: svgEl.querySelectorAll('path').length, bytes: new Blob([svg]).size }, note);
}

function report(fin, note) {
  const kb = (fin.bytes / 1024).toFixed(1);
  const timing = fin.millis != null ? ` Segmented in ${segmentMs.toFixed(0)} ms, fitted in ${fin.millis.toFixed(0)} ms.` : '';
  setStatus(`${note ? note + ' ' : ''}${image.width}×${image.height}${image.note}.${timing} ${fin.paths} paths, ${kb} KB.`);
}

// Shapes: every path with the same data is one shape (a layered gradient
// is a base path under translucent overlays). Selection is a set of path
// data; the classes on the paths follow it.
function paths() { return svgEl ? [...svgEl.querySelectorAll('path')] : []; }
function shapes() {
  const ps = paths();
  return groupShapes(ps.map((p) => ({ d: p.getAttribute('d') }))).map((idx) => ({ d: ps[idx[0]].getAttribute('d'), paths: idx.map((i) => ps[i]) }));
}
function refreshSelection() {
  const ps = paths();
  for (const p of ps) p.classList.toggle('sel', selected.has(p.getAttribute('d')));
  const any = selected.size > 0;
  $('deleteSel').disabled = !any;
  $('colour').disabled = !any;
  if (any) {
    const first = ps.find((p) => selected.has(p.getAttribute('d')));
    $('colour').value = fillColour(first);
    let area = 0;
    for (const s of shapes()) if (selected.has(s.d)) { const b = s.paths[0].getBBox(); area += b.width * b.height; }
    const vb = svgEl.viewBox.baseVal;
    setStatus(selectionText(selected.size, Math.min(1, area / (vb.width * vb.height))));
  }
}
function select(d, extend) {
  if (!extend) selected = new Set();
  if (selected.has(d)) selected.delete(d); else selected.add(d);
  tintBase = null;
  refreshSelection();
}
function clearSelection() {
  if (!selected.size) return;
  selected = new Set();
  tintBase = null;
  refreshSelection();
}
// fillColour is the hex colour of a path for the colour input: its fill,
// or the first stop of its gradient.
function fillColour(p) {
  const fill = p.getAttribute('fill') || '#000000';
  const m = /^url\(#([^)]+)\)$/.exec(fill);
  if (!m) return fill.length === 4 ? '#' + [...fill.slice(1)].map((c) => c + c).join('') : fill;
  const stop = svgEl.querySelector(`#${CSS.escape(m[1])} stop`);
  return stop ? stop.getAttribute('stop-color') : '#000000';
}

function deleteSelected() {
  if (!selected.size) return;
  const n = selected.size;
  for (const p of paths()) if (selected.has(p.getAttribute('d'))) p.remove();
  commit(`Deleted ${n} shape${n === 1 ? '' : 's'}.`);
}

function removeBackground() {
  if (!svgEl) return;
  const all = shapes();
  const vb = svgEl.viewBox.baseVal;
  const idx = backgroundShapes(all.map((s) => ({ bbox: s.paths[0].getBBox(), fill: s.paths[0].getAttribute('fill') })), vb.width, vb.height);
  if (!idx.length) {
    setStatus('No background found. Click the background and press Delete.', 'error');
    return;
  }
  for (const i of idx) for (const p of all[i].paths) p.remove();
  commit(`Removed ${idx.length} background shape${idx.length === 1 ? '' : 's'}.`);
}

// recolour paints the selected shapes with the colour input's value: a
// solid fill is replaced, a gradient is tinted from the stops it had
// when the drag began, so dragging the picker does not compound.
function recolour(to) {
  if (!tintBase) {
    tintBase = new Map();
    for (const p of paths()) {
      if (!selected.has(p.getAttribute('d'))) continue;
      const m = /^url\(#([^)]+)\)$/.exec(p.getAttribute('fill') || '');
      if (m && !tintBase.has(m[1])) {
        tintBase.set(m[1], [...svgEl.querySelectorAll(`#${CSS.escape(m[1])} stop`)].map((st) => st.getAttribute('stop-color')));
      }
    }
  }
  const tinted = new Set();
  for (const p of paths()) {
    if (!selected.has(p.getAttribute('d'))) continue;
    const m = /^url\(#([^)]+)\)$/.exec(p.getAttribute('fill') || '');
    if (!m) { p.setAttribute('fill', to); continue; }
    if (tinted.has(m[1])) continue;
    tinted.add(m[1]);
    const stops = svgEl.querySelectorAll(`#${CSS.escape(m[1])} stop`);
    tintStops(tintBase.get(m[1]), to).forEach((c, i) => stops[i].setAttribute('stop-color', c));
  }
}

async function run() {
  if (!image || !worker) return;
  const options = readOptions();
  $('cancel').disabled = false;
  try {
    if (needsResegment(cachedOptions, options) || handles.current === null) {
      setStatus('Segmenting colours…', 'working');
      const copy = image.data.slice(); // keep ours; the worker gets a transferred copy
      const seg = await call({ op: 'segment', width: image.width, height: image.height, data: copy.buffer, options }, [copy.buffer]);
      free(handles.adopt(seg.handle));
      cachedOptions = options; segmentMs = seg.millis;
    }
    setStatus(`Segmented in ${segmentMs.toFixed(0)} ms. Fitting curves…`, 'working');
    const fin = await call({ op: 'finish', handle: handles.current, options });
    showSVG(fin.svg);
    report(fin);
  } catch (e) {
    if (e.message === 'superseded') return;
    if (e.message === 'cancelled') { setStatus('Cancelled. Change a setting to trace again.'); return; }
    setStatus(String(e.message || e), 'error');
  } finally {
    $('cancel').disabled = true;
  }
}

async function load(file) {
  if (!file) return;
  showSource(file);
  setStatus('Decoding ' + (file.name || 'image') + '…', 'working');
  try {
    image = await decode(file);
  } catch (e) {
    setStatus('Could not decode ' + (file.name || 'the image') + ': ' + e.message, 'error');
    return;
  }
  free(handles.release());
  cachedOptions = null;
  run();
}

// Input: file picker, drop anywhere on the stage, paste anywhere.
const stage = document.querySelector('.stage');
const compare = $('compare'), divider = $('divider'), clip = $('svgClip');
$('sourceImg').addEventListener('load', fitCompare);
window.addEventListener('resize', resized);
$('file').addEventListener('change', (e) => load(e.target.files[0]));
stage.addEventListener('dragover', (e) => { e.preventDefault(); stage.classList.add('over'); });
stage.addEventListener('dragleave', () => stage.classList.remove('over'));
stage.addEventListener('drop', (e) => {
  e.preventDefault(); stage.classList.remove('over');
  load(imageFileFromItems(e.dataTransfer.items) || e.dataTransfer.files[0]);
});
document.addEventListener('paste', (e) => {
  const f = imageFileFromItems(e.clipboardData && e.clipboardData.items);
  if (f) { e.preventDefault(); load(f); }
});

// Controls.
$('preset').addEventListener('change', () => { applyPreset($('preset').value); run(); });
for (const id of ['mode', 'hierarchical', 'maxColors', 'colorTolerance', 'threshold', 'thresholdAuto', 'filterSpeckle', 'gradients', 'gradientTolerance', 'refine', 'refinePasses', 'stroke', 'pathPrecision']) {
  $(id).addEventListener('change', () => run());
}
function cancel() { startWorker(); setStatus('Cancelled. Change a setting to trace again.'); }
$('cancel').addEventListener('click', cancel);
$('busyCancel').addEventListener('click', cancel);
// Download: the SVG as edited; PDF and EPS by handing that SVG to the
// wasm, which reads its own dialect back and writes the other format.
// The conversion has its own ids so it never supersedes a trace.
const MIME = { svg: 'image/svg+xml', pdf: 'application/pdf', eps: 'application/postscript' };
let convertID = 0;
function convert(svg, format) {
  const id = --convertID;
  return new Promise((resolve, reject) => {
    pending.set(id, { resolve, reject });
    worker.postMessage({ id, op: 'convert', svg, options: { ...readOptions(), format } });
  });
}
function save(blob, name) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url; a.download = name; a.click();
  setTimeout(() => URL.revokeObjectURL(url), 10000);
}
$('download').addEventListener('click', async () => {
  const format = $('format').value;
  if (format === 'svg') { save(new Blob([serialise()], { type: MIME.svg }), 'rekha.svg'); return; }
  $('download').disabled = true;
  try {
    const svg = serialise();
    const r = await convert(svg, format);
    save(new Blob([r.bytes], { type: MIME[format] }), 'rekha.' + format);
    const note = format === 'eps' && svg.includes('stop-opacity') ? ' EPS has no transparency: translucent overlay layers are left out.' : '';
    setStatus(`${format.toUpperCase()} written, ${(r.bytes.length / 1024).toFixed(1)} KB.${note}`);
  } catch (e) {
    setStatus('Could not write the ' + format.toUpperCase() + ': ' + e.message, 'error');
  } finally {
    $('download').disabled = false;
  }
});

// Edits: click a shape on the SVG side of the wipe to select it (shift
// adds), double-click to select every shape of its colour, Delete
// removes the selection, the colour input repaints it, and undo and
// redo walk the history. A click that moved the pointer was a pan.
let pressAt = null;
stage.addEventListener('pointerdown', (e) => { pressAt = [e.clientX, e.clientY, e.target]; });
stage.addEventListener('pointerup', (e) => {
  if (!pressAt) return;
  const [x, y, target] = pressAt;
  pressAt = null;
  if (Math.hypot(e.clientX - x, e.clientY - y) > 4 || e.button !== 0) return;
  const path = target.closest && target.closest('#svgClip path');
  if (path) select(path.getAttribute('d'), e.shiftKey || e.metaKey || e.ctrlKey);
  else if (!target.closest('.divider, .zoom, .tools')) clearSelection();
});
stage.addEventListener('dblclick', (e) => {
  const path = e.target.closest && e.target.closest('#svgClip path');
  if (!path) return;
  e.stopImmediatePropagation();
  const fill = path.getAttribute('fill');
  selected = new Set(paths().filter((p) => p.getAttribute('fill') === fill).map((p) => p.getAttribute('d')));
  tintBase = null;
  refreshSelection();
}, true);
$('deleteSel').addEventListener('click', deleteSelected);
$('removeBg').addEventListener('click', removeBackground);
$('colour').addEventListener('input', (e) => recolour(e.target.value));
$('colour').addEventListener('change', () => { if (tintBase || selected.size) { const keep = selected; commit('Recoloured.'); selected = keep; refreshSelection(); } });
function undo() { if (history && history.canUndo) { mount(history.undo()); report({ paths: paths().length, bytes: new Blob([history.current]).size }, 'Undone.'); } }
function redo() { if (history && history.canRedo) { mount(history.redo()); report({ paths: paths().length, bytes: new Blob([history.current]).size }, 'Redone.'); } }
$('undo').addEventListener('click', undo);
$('redo').addEventListener('click', redo);
document.addEventListener('keydown', (e) => {
  if (!svgEl || /^(INPUT|SELECT|TEXTAREA)$/.test(e.target.tagName)) return;
  const mod = e.metaKey || e.ctrlKey;
  if (mod && e.key.toLowerCase() === 'z') { if (e.shiftKey) redo(); else undo(); }
  else if (mod && e.key.toLowerCase() === 'y') redo();
  else if ((e.key === 'Delete' || e.key === 'Backspace') && selected.size) deleteSelected();
  else if (e.key === 'Escape') clearSelection();
  else return;
  e.preventDefault();
});

// Zoom and pan: ctrl/cmd + wheel or a pinch zooms about the cursor,
// a plain wheel pans, dragging the image pans, double-click zooms in;
// the toolbar and the keys + - 0 1 do the same from the keyboard.
stage.addEventListener('wheel', (e) => {
  if (!vp.image.w) return;
  e.preventDefault();
  if (e.ctrlKey || e.metaKey) {
    zoomBy(Math.exp(-e.deltaY * 0.01), stagePoint(e));
  } else {
    vp.pan(-e.deltaX, -e.deltaY);
    render();
  }
}, { passive: false });
let panFrom = null;
stage.addEventListener('pointerdown', (e) => {
  if (!vp.image.w || e.button !== 0 || e.target.closest('.divider, .zoom, .tools')) return;
  panFrom = [e.clientX, e.clientY];
  stage.setPointerCapture(e.pointerId);
  stage.classList.add('panning');
});
stage.addEventListener('pointermove', (e) => {
  if (!panFrom) return;
  vp.pan(e.clientX - panFrom[0], e.clientY - panFrom[1]);
  panFrom = [e.clientX, e.clientY];
  render();
});
const endPan = () => { panFrom = null; stage.classList.remove('panning'); };
stage.addEventListener('pointerup', endPan);
stage.addEventListener('pointercancel', endPan);
stage.addEventListener('dblclick', (e) => { if (!e.target.closest('.divider, .zoom, .tools')) zoomBy(2, stagePoint(e)); });
stage.addEventListener('keydown', (e) => {
  if (e.target !== stage) return;
  if (e.key === '+' || e.key === '=') zoomBy(1.25);
  else if (e.key === '-' || e.key === '_') zoomBy(0.8);
  else if (e.key === '0') { if (vp.image.w) { vp.fit(); render(); } }
  else if (e.key === '1') { if (vp.image.w) { const [w, h] = viewSize(); vp.zoomTo(1, w / 2, h / 2); render(); } }
  else return;
  e.preventDefault();
});
$('zoomIn').addEventListener('click', () => zoomBy(1.25));
$('zoomOut').addEventListener('click', () => zoomBy(0.8));
$('zoomFit').addEventListener('click', () => { if (vp.image.w) { vp.fit(); render(); } });
$('zoomLevel').addEventListener('click', () => { if (vp.image.w) { const [w, h] = viewSize(); vp.zoomTo(1, w / 2, h / 2); render(); } });

// Wipe divider: drag, or arrow keys when focused.
function setWipe(frac) {
  frac = Math.min(1, Math.max(0, frac));
  divider.style.left = (frac * 100) + '%';
  clip.style.clipPath = `inset(0 0 0 ${frac * 100}%)`;
  divider.setAttribute('aria-valuenow', Math.round(frac * 100));
}
setWipe(0.5);
let dragging = false;
divider.addEventListener('pointerdown', (e) => { dragging = true; divider.setPointerCapture(e.pointerId); });
divider.addEventListener('pointerup', () => { dragging = false; });
divider.addEventListener('pointermove', (e) => {
  if (!dragging) return;
  const r = compare.getBoundingClientRect();
  setWipe((e.clientX - r.left) / r.width);
});
divider.addEventListener('keydown', (e) => {
  const cur = +divider.getAttribute('aria-valuenow') / 100;
  if (e.key === 'ArrowLeft') setWipe(cur - 0.05);
  if (e.key === 'ArrowRight') setWipe(cur + 0.05);
});

applyPreset('photo');
startWorker();

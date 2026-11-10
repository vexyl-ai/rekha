// Demo page: decodes the image in the browser (file, drop or paste), runs
// rekha.wasm in a worker, re-segments only when a segmentation option
// changed, drops stale replies, frees replaced handles, and cancels by
// terminating the worker. The result is shown as a wipe over the source.
const { needsResegment, Requests, Handles, imageFileFromItems } = self.RekhaProtocol;

const $ = (id) => document.getElementById(id);
const status = $('status');
const requests = new Requests();

// Preset defaults mirror rekha.PresetOptions so the form shows what runs.
const PRESETS = {
  bw:     { hierarchical: 'cutout',  maxColors: 2, colorTolerance: 0,    filterSpeckle: 4 },
  poster: { hierarchical: 'stacked', maxColors: 0, colorTolerance: 0.08, filterSpeckle: 4 },
  photo:  { hierarchical: 'stacked', maxColors: 0, colorTolerance: 0.04, filterSpeckle: 16 },
};
const COMMON = { mode: 'spline', threshold: 0.5, thresholdAuto: false, refine: false, refinePasses: 3, cutoutStroke: 0, pathPrecision: 2 };

let worker = null;
let pending = new Map();         // id → {resolve, reject}
let image = null;                // {width, height, data, note}
let handles = new Handles();     // the live wasm segmentation handle
let cachedOptions = null;        // options the handle was segmented with
let segmentMs = null;
let svgURL = '';
let sourceURL = '';

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
    refine: $('refine').checked,
    refinePasses: +$('refinePasses').value,
    cutoutStroke: +$('cutoutStroke').value,
    pathPrecision: +$('pathPrecision').value,
  };
}

function setStatus(text, kind) {
  status.textContent = text;
  status.className = kind || '';
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
  const cap = Math.max(10000, +$('pixelCap').value || 4000000);
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

// fitCompare sizes the compare box to the stage, keeping the image's aspect.
function fitCompare() {
  const img = $('sourceImg');
  if (!img.naturalWidth) return;
  const r = stage.getBoundingClientRect();
  const w = r.width - 40, h = r.height - 40;
  const s = Math.min(w / img.naturalWidth, h / img.naturalHeight);
  compare.style.width = Math.max(1, Math.floor(img.naturalWidth * s)) + 'px';
  compare.style.height = Math.max(1, Math.floor(img.naturalHeight * s)) + 'px';
}

function showSource(file) {
  if (sourceURL) URL.revokeObjectURL(sourceURL);
  sourceURL = URL.createObjectURL(file);
  $('sourceImg').src = sourceURL;
  $('svgImg').removeAttribute('src');
  $('compare').hidden = false;
  $('drop').hidden = true;
  $('download').disabled = true;
}

function showSVG(svg) {
  if (svgURL) URL.revokeObjectURL(svgURL);
  svgURL = URL.createObjectURL(new Blob([svg], { type: 'image/svg+xml' }));
  $('svgImg').src = svgURL;
  $('download').disabled = false;
}

function report(fin) {
  const kb = (fin.bytes / 1024).toFixed(1);
  setStatus(`${image.width}×${image.height}${image.note}. Segmented in ${segmentMs.toFixed(0)} ms, fitted in ${fin.millis.toFixed(0)} ms. ${fin.paths} paths, ${kb} KB.`);
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
window.addEventListener('resize', fitCompare);
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
for (const id of ['mode', 'hierarchical', 'maxColors', 'colorTolerance', 'threshold', 'thresholdAuto', 'filterSpeckle', 'refine', 'refinePasses', 'cutoutStroke', 'pathPrecision']) {
  $(id).addEventListener('change', () => run());
}
$('cancel').addEventListener('click', () => { startWorker(); setStatus('Cancelled. Change a setting to trace again.'); });
$('download').addEventListener('click', () => {
  const a = document.createElement('a');
  a.href = svgURL; a.download = 'rekha.svg'; a.click();
});

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

applyPreset('bw');
startWorker();

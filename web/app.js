// Demo page: decodes the image in the browser, runs rekha.wasm in a
// worker, re-segments only when a segmentation option changed, drops
// stale replies, and cancels by terminating the worker.
const { needsResegment, Requests } = self.RekhaProtocol;

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
let pending = new Map(); // id → {resolve, reject}
let image = null;        // {width, height, data: Uint8ClampedArray}
let handle = null;       // wasm segmentation handle
let cachedOptions = null; // options the handle was segmented with
let segmentMs = null;
let svgText = '';
let svgURL = '';
let busy = false;

function applyPreset(name) {
  const p = { ...COMMON, ...PRESETS[name] };
  for (const [k, v] of Object.entries(p)) {
    const el = $(k);
    if (!el) continue;
    if (el.type === 'checkbox') el.checked = v; else el.value = v;
  }
}

function readOptions() {
  const o = {
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
  return o;
}

function setStatus(text, isError) {
  status.textContent = text;
  status.className = isError ? 'error' : '';
}

function startWorker() {
  if (worker) worker.terminate();
  for (const p of pending.values()) p.reject(new Error('cancelled'));
  pending = new Map();
  handle = null; cachedOptions = null; segmentMs = null;
  worker = new Worker('worker.js');
  worker.onmessage = (ev) => {
    const m = ev.data;
    if (m.op === 'ready') { if (!image) setStatus('rekha.wasm ' + m.version + ' ready. Drop an image.'); return; }
    const p = pending.get(m.id);
    if (!p) return;
    pending.delete(m.id);
    if (m.ok) p.resolve(m.result); else p.reject(new Error(m.error));
  };
  worker.onerror = (e) => setStatus('worker error: ' + e.message, true);
}

function call(msg, transfer) {
  const id = requests.next();
  return new Promise((resolve, reject) => {
    pending.set(id, { resolve, reject });
    worker.postMessage({ id, ...msg }, transfer || []);
  }).then((r) => {
    if (!requests.accept(id)) throw new Error('superseded');
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
    note = ` (downscaled from ${bitmap.width}×${bitmap.height} to fit the pixel cap)`;
  }
  const canvas = typeof OffscreenCanvas !== 'undefined' ? new OffscreenCanvas(width, height) : Object.assign(document.createElement('canvas'), { width, height });
  const ctx = canvas.getContext('2d');
  ctx.drawImage(bitmap, 0, 0, width, height);
  const data = ctx.getImageData(0, 0, width, height).data;
  bitmap.close();
  return { width, height, data, note };
}

function showSource(file) {
  const pane = $('sourcePane');
  pane.innerHTML = '';
  const img = document.createElement('img');
  img.src = URL.createObjectURL(file);
  pane.appendChild(img);
}

function showSVG(svg) {
  svgText = svg;
  if (svgURL) URL.revokeObjectURL(svgURL);
  svgURL = URL.createObjectURL(new Blob([svg], { type: 'image/svg+xml' }));
  const pane = $('resultPane');
  pane.innerHTML = '';
  const img = document.createElement('img');
  img.src = svgURL;
  pane.appendChild(img);
  $('download').disabled = false;
}

async function run() {
  if (!image || !worker) return;
  const options = readOptions();
  busy = true;
  $('cancel').disabled = false;
  try {
    if (needsResegment(cachedOptions, options) || handle === null) {
      setStatus('segmenting…');
      const copy = image.data.slice(); // keep ours; the worker gets a transferred copy
      const seg = await call({ op: 'segment', width: image.width, height: image.height, data: copy.buffer, options }, [copy.buffer]);
      if (handle !== null) worker.postMessage({ id: 0, op: 'free', handle });
      handle = seg.handle; cachedOptions = options; segmentMs = seg.millis;
    }
    setStatus(`segment ${segmentMs.toFixed(0)} ms · finishing…`);
    const fin = await call({ op: 'finish', handle, options });
    showSVG(fin.svg);
    setStatus(`segment ${segmentMs.toFixed(0)} ms · finish ${fin.millis.toFixed(0)} ms · ${fin.paths} paths · ${(fin.bytes / 1024).toFixed(1)} KB · ${image.width}×${image.height}${image.note}`);
  } catch (e) {
    if (e.message === 'superseded') return;
    if (e.message === 'cancelled') { setStatus('cancelled'); return; }
    setStatus(String(e.message || e), true);
  } finally {
    busy = false;
    $('cancel').disabled = true;
  }
}

async function load(file) {
  if (!file) return;
  showSource(file);
  setStatus('decoding…');
  try {
    image = await decode(file);
  } catch (e) {
    setStatus('cannot decode ' + file.name + ': ' + e.message, true);
    return;
  }
  handle = null; cachedOptions = null;
  run();
}

$('file').addEventListener('change', (e) => load(e.target.files[0]));
const drop = $('drop');
for (const el of [drop, document.body]) {
  el.addEventListener('dragover', (e) => { e.preventDefault(); drop.classList.add('over'); });
  el.addEventListener('dragleave', () => drop.classList.remove('over'));
  el.addEventListener('drop', (e) => { e.preventDefault(); drop.classList.remove('over'); load(e.dataTransfer.files[0]); });
}
$('preset').addEventListener('change', () => { applyPreset($('preset').value); run(); });
for (const id of ['mode', 'hierarchical', 'maxColors', 'colorTolerance', 'threshold', 'thresholdAuto', 'filterSpeckle', 'refine', 'refinePasses', 'cutoutStroke', 'pathPrecision']) {
  $(id).addEventListener('change', () => run());
}
$('cancel').addEventListener('click', () => { startWorker(); setStatus('cancelled'); });
$('download').addEventListener('click', () => {
  const a = document.createElement('a');
  a.href = svgURL; a.download = 'rekha.svg'; a.click();
});

applyPreset('bw');
startWorker();

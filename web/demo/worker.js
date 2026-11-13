// Classic worker: loads the Go wasm runtime and rekha.wasm, then serves
// trace / segment / finish / free / convert requests from the page one at a time.
'use strict';
importScripts('wasm_exec.js', 'protocol.js');

const go = new Go();
const ready = (async () => {
  let result;
  try {
    result = await WebAssembly.instantiateStreaming(fetch('rekha.wasm', { cache: 'no-cache' }), go.importObject);
  } catch (e) {
    const bytes = await (await fetch('rekha.wasm', { cache: 'no-cache' })).arrayBuffer();
    result = await WebAssembly.instantiate(bytes, go.importObject);
  }
  go.run(result.instance); // returns when main() exits; it never does
  // main() installs the global synchronously before blocking.
  for (let i = 0; i < 1000 && typeof self.rekha === 'undefined'; i++) {
    await new Promise((r) => setTimeout(r, 1));
  }
  if (typeof self.rekha === 'undefined') throw new Error('rekha.wasm did not install its API');
  self.postMessage({ op: 'ready', version: self.rekha.version() });
})();

// progress returns the callback the wasm calls as a stage advances; each
// call becomes a message the page matches to request id.
function progress(id) {
  return (stage, done, total) => self.postMessage({ id, op: 'progress', stage, done, total });
}

self.onmessage = async (ev) => {
  const msg = ev.data;
  try {
    await ready;
    let r;
    switch (msg.op) {
      case 'trace':
        r = self.rekha.trace({ width: msg.width, height: msg.height, data: new Uint8ClampedArray(msg.data) }, msg.options, progress(msg.id));
        break;
      case 'segment':
        r = self.rekha.segment({ width: msg.width, height: msg.height, data: new Uint8ClampedArray(msg.data) }, msg.options, progress(msg.id));
        break;
      case 'finish':
        r = self.rekha.finish(msg.handle, msg.options, progress(msg.id));
        break;
      case 'free':
        self.rekha.free(msg.handle);
        r = { ok: true };
        break;
      case 'convert':
        r = self.rekha.convert(msg.svg, msg.options);
        break;
      default:
        r = { ok: false, error: 'unknown op ' + msg.op };
    }
    if (r.ok) {
      const { ok, ...result } = r;
      self.postMessage({ id: msg.id, ok: true, result });
    } else {
      self.postMessage({ id: msg.id, ok: false, error: r.error });
    }
  } catch (e) {
    self.postMessage({ id: msg.id, ok: false, error: String(e && e.message || e) });
  }
};

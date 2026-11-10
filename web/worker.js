// Classic worker: loads the Go wasm runtime and rekha.wasm, then serves
// segment / finish / free requests from the page one at a time.
'use strict';
importScripts('wasm_exec.js', 'protocol.js');

const go = new Go();
const ready = (async () => {
  let result;
  try {
    result = await WebAssembly.instantiateStreaming(fetch('rekha.wasm'), go.importObject);
  } catch (e) {
    const bytes = await (await fetch('rekha.wasm')).arrayBuffer();
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

self.onmessage = async (ev) => {
  const msg = ev.data;
  try {
    await ready;
    let r;
    switch (msg.op) {
      case 'segment':
        r = self.rekha.segment({ width: msg.width, height: msg.height, data: new Uint8ClampedArray(msg.data) }, msg.options);
        break;
      case 'finish':
        r = self.rekha.finish(msg.handle, msg.options);
        break;
      case 'free':
        self.rekha.free(msg.handle);
        r = { ok: true };
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

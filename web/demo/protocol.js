// Shared between the page and the worker: which options change the
// segmentation, and the generation counter that drops stale replies.
// Plain script (no modules) so the classic worker can importScripts it;
// also a CommonJS module for `node --test`.
(function (root) {
  'use strict';

  // Keys of the options object that change Segment; anything else only
  // changes Finish. Pinned by internal/apiopts.SegmentKeys in Go.
  const SEGMENT_KEYS = ['preset', 'threshold', 'colorPrecision', 'colorTolerance', 'maxColors', 'palette', 'filterSpeckle', 'gradients', 'gradientTolerance'];

  // needsResegment reports whether moving from options prev to next
  // requires a new segmentation (prev null: nothing cached yet).
  function needsResegment(prev, next) {
    if (prev == null) return true;
    return SEGMENT_KEYS.some((k) => JSON.stringify(prev[k]) !== JSON.stringify(next[k]));
  }

  // Requests hands out increasing ids; only the latest one is accepted,
  // so a reply to a superseded request is dropped.
  class Requests {
    constructor() { this.latest = 0; }
    next() { return ++this.latest; }
    accept(id) { return id === this.latest; }
  }

  // Handles tracks the one live segmentation handle so every handle it
  // replaces or drops is freed in the worker; the wasm heap never shrinks.
  class Handles {
    constructor() { this.current = null; }
    // adopt makes h current and returns the handle to free, if any.
    adopt(h) { const prev = this.current; this.current = h; return prev; }
    // release clears the current handle and returns it to free, once.
    release() { const prev = this.current; this.current = null; return prev; }
  }

  // imageFileFromItems returns the first image file among clipboard or
  // drag items (DataTransferItemList-like), or null.
  function imageFileFromItems(items) {
    if (!items) return null;
    for (const it of items) {
      if (it.kind === 'file' && /^image\//.test(it.type)) {
        const f = it.getAsFile();
        if (f) return f;
      }
    }
    return null;
  }

  // Stage names the tracer reports (rekha.Options.Progress) and what
  // the page says for each.
  const STAGES = {
    quantise: 'Quantising colours…',
    label: 'Labelling pixels…',
    ramps: 'Merging ramps…',
    outlines: 'Tracing outlines…',
    nest: 'Nesting shapes…',
    edges: 'Fitting edges…',
    refine: 'Refining…',
    curves: 'Fitting curves…',
  };

  // progressText is the line shown for a progress report: the stage,
  // then "done of total" when the stage has a count.
  function progressText(stage, done, total) {
    const text = STAGES[stage] || 'Working…';
    if (!total) return text;
    const fmt = (n) => n.toLocaleString('en-US');
    return `${text} ${fmt(done)} of ${fmt(total)}`;
  }

  // Viewport is the pan and zoom state of the stage: an image of w×h
  // natural pixels shown at scale z with its top-left corner at (x, y)
  // in view pixels. The zoom stays between a quarter of the fit scale
  // and 32; an image smaller than the view sits centred, a larger one
  // never leaves a gap at an edge. fitted is true until the viewer
  // zooms or pans, so a resize refits.
  class Viewport {
    constructor() {
      this.z = 1; this.x = 0; this.y = 0;
      this.image = { w: 0, h: 0 }; this.view = { w: 0, h: 0 };
      this.fitted = true;
    }
    setImage(w, h) { this.image = { w, h }; this.fit(); }
    setView(w, h) { this.view = { w, h }; if (this.fitted) this.fit(); else this.clamp(); }
    fitScale() {
      if (!this.image.w || !this.image.h) return 1;
      return Math.min(this.view.w / this.image.w, this.view.h / this.image.h);
    }
    fit() { this.z = this.fitScale(); this.fitted = true; this.clamp(); }
    // zoomTo sets the scale keeping the image point under (px, py) still.
    zoomTo(z, px, py) {
      z = Math.min(32, Math.max(this.fitScale() / 4, z));
      const k = z / this.z;
      this.x = px - (px - this.x) * k;
      this.y = py - (py - this.y) * k;
      this.z = z; this.fitted = false;
      this.clamp();
    }
    zoomAt(factor, px, py) { this.zoomTo(this.z * factor, px, py); }
    pan(dx, dy) { this.x += dx; this.y += dy; this.fitted = false; this.clamp(); }
    clamp() {
      const w = this.image.w * this.z, h = this.image.h * this.z;
      this.x = w <= this.view.w ? (this.view.w - w) / 2 : Math.min(0, Math.max(this.view.w - w, this.x));
      this.y = h <= this.view.h ? (this.view.h - h) / 2 : Math.min(0, Math.max(this.view.h - h, this.y));
    }
  }

  const api = { SEGMENT_KEYS, needsResegment, Requests, Handles, imageFileFromItems, progressText, Viewport };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  root.RekhaProtocol = api;
})(typeof self !== 'undefined' ? self : globalThis);

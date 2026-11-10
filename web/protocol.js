// Shared between the page and the worker: which options change the
// segmentation, and the generation counter that drops stale replies.
// Plain script (no modules) so the classic worker can importScripts it;
// also a CommonJS module for `node --test`.
(function (root) {
  'use strict';

  // Keys of the options object that change Segment; anything else only
  // changes Finish. Pinned by internal/apiopts.SegmentKeys in Go.
  const SEGMENT_KEYS = ['preset', 'threshold', 'colorPrecision', 'colorTolerance', 'maxColors', 'palette', 'filterSpeckle'];

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

  const api = { SEGMENT_KEYS, needsResegment, Requests };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  root.RekhaProtocol = api;
})(typeof self !== 'undefined' ? self : globalThis);

// Basic edits on a traced SVG, shared between the page and `node --test`:
// which paths form one shape, which shapes are the background, how a
// gradient is tinted, and the undo history. The page owns the DOM; this
// file owns the decisions. Plain script (no modules) like protocol.js.
(function (root) {
  'use strict';

  // groupShapes joins paths with identical data into one shape, in the
  // order the first of each appears. The writer paints a layered gradient
  // as a base path and translucent overlays with the same outline, so a
  // viewer picking "that shape" means all of them.
  function groupShapes(paths) {
    const byD = new Map();
    const groups = [];
    paths.forEach((p, i) => {
      let g = byD.get(p.d);
      if (!g) { g = []; byD.set(p.d, g); groups.push(g); }
      g.push(i);
    });
    return groups;
  }

  const BORDER_SLACK = 1.5;   // px a box may fall short of an edge
  const BACKGROUND_SHARE = 0.4; // least share of the image a background covers

  // backgroundShapes returns the indices of the shapes that read as the
  // background: a shape whose box touches at least three borders and
  // whose area is at least BACKGROUND_SHARE of the image (its own area
  // when given, else its box), plus every shape of the same fill that
  // touches a border. Shapes are {bbox: {x, y, width, height}, fill,
  // area?}.
  function backgroundShapes(shapes, w, h) {
    const touches = (b) => {
      let n = 0;
      if (b.x <= BORDER_SLACK) n++;
      if (b.y <= BORDER_SLACK) n++;
      if (b.x + b.width >= w - BORDER_SLACK) n++;
      if (b.y + b.height >= h - BORDER_SLACK) n++;
      return n;
    };
    const out = new Set();
    const fills = new Set();
    shapes.forEach((s, i) => {
      const area = s.area != null ? s.area : s.bbox.width * s.bbox.height;
      if (touches(s.bbox) >= 3 && area >= BACKGROUND_SHARE * w * h) { out.add(i); fills.add(s.fill); }
    });
    if (out.size) {
      shapes.forEach((s, i) => { if (fills.has(s.fill) && touches(s.bbox) >= 1) out.add(i); });
    }
    return [...out].sort((a, b) => a - b);
  }

  // pruneDefs removes the gradients no fill refers to, and the defs
  // block when it is left empty. Works on the writer's own markup: one
  // gradient element per id, no nesting.
  function pruneDefs(svg) {
    const used = new Set();
    for (const m of svg.matchAll(/url\(#([^)]+)\)/g)) used.add(m[1]);
    svg = svg.replace(/<(linear|radial)Gradient id="([^"]+)"[\s\S]*?<\/\1Gradient>/g, (all, kind, id) => (used.has(id) ? all : ''));
    return svg.replace(/<defs>\s*<\/defs>\n?/, '');
  }

  // History holds the states of the drawing (serialised SVG), the current
  // one included, up to cap states; push after an edit, undo and redo
  // return the state to show or null.
  class History {
    constructor(initial, cap) {
      this.states = [initial];
      this.at = 0;
      this.cap = cap || 30;
    }
    get current() { return this.states[this.at]; }
    get canUndo() { return this.at > 0; }
    get canRedo() { return this.at < this.states.length - 1; }
    push(state) {
      this.states.length = this.at + 1;
      this.states.push(state);
      if (this.states.length > this.cap) this.states.shift();
      this.at = this.states.length - 1;
    }
    undo() { return this.canUndo ? this.states[--this.at] : null; }
    redo() { return this.canRedo ? this.states[++this.at] : null; }
  }

  function parseHex(s) {
    s = s.trim().replace(/^#/, '');
    if (s.length === 3) s = s.replace(/./g, (c) => c + c);
    const n = parseInt(s, 16);
    return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
  }
  function hex(rgb) {
    return '#' + rgb.map((v) => Math.max(0, Math.min(255, Math.round(v))).toString(16).padStart(2, '0')).join('');
  }

  // hsl converts a hex colour to [hue 0..360, saturation 0..100,
  // lightness 0..100], rounded; hexFromHSL is its inverse.
  function hsl(s) {
    const [r, g, b] = parseHex(s).map((v) => v / 255);
    const max = Math.max(r, g, b), min = Math.min(r, g, b);
    const l = (max + min) / 2;
    let h = 0, sat = 0;
    if (max !== min) {
      const d = max - min;
      sat = l > 0.5 ? d / (2 - max - min) : d / (max + min);
      if (max === r) h = ((g - b) / d + (g < b ? 6 : 0)) * 60;
      else if (max === g) h = ((b - r) / d + 2) * 60;
      else h = ((r - g) / d + 4) * 60;
    }
    return [Math.round(h), Math.round(sat * 100), Math.round(l * 100)];
  }
  function hexFromHSL(h, sat, l) {
    sat /= 100; l /= 100;
    const k = (n) => (n + h / 30) % 12;
    const a = sat * Math.min(l, 1 - l);
    const f = (n) => l - a * Math.max(-1, Math.min(k(n) - 3, 9 - k(n), 1));
    return hex([f(0) * 255, f(8) * 255, f(4) * 255]);
  }

  // tintStops repaints a gradient: the first stop becomes `to`, and
  // every other stop moves by the same change in hue, saturation and
  // lightness, so a ramp keeps its shading and its spread of hues under
  // the new colour. A grey first stop has no hue to move from, so every
  // stop then takes the hue of `to`. Saturation and lightness clamp.
  function tintStops(stops, to) {
    if (!stops.length) return stops;
    const [h1, s1, l1] = hsl(to);
    const [h0, s0, l0] = hsl(stops[0]);
    const clamp = (v) => Math.max(0, Math.min(100, v));
    return stops.map((st, i) => {
      if (i === 0) return hex(parseHex(to));
      const [h, sat, l] = hsl(st);
      const hue = s0 < 5 || sat < 5 ? h1 : (h + h1 - h0 + 360) % 360;
      return hexFromHSL(hue, clamp(sat + s1 - s0), clamp(l + l1 - l0));
    });
  }

  // selectionText is the status line for a selection covering `share`
  // of the image.
  function selectionText(count, share) {
    if (!count) return '';
    const pct = share < 0.01 ? 'under 1%' : Math.round(share * 100) + '%';
    return `${count} shape${count === 1 ? '' : 's'} selected, ${pct} of the image.`;
  }

  const api = { groupShapes, backgroundShapes, pruneDefs, History, tintStops, selectionText, parseHex, hex, hsl };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  root.RekhaEdit = api;
})(typeof self !== 'undefined' ? self : globalThis);

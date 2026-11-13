'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const { groupShapes, backgroundShapes, pruneDefs, History, tintStops, selectionText, hsl } = require('./edit.js');

test('groupShapes joins paths with identical data into one shape, in first-seen order', () => {
  const paths = [{ d: 'A', fill: '#000' }, { d: 'B', fill: 'url(#g0)' }, { d: 'B', fill: 'url(#g1)' }, { d: 'C', fill: '#fff' }];
  assert.deepEqual(groupShapes(paths), [[0], [1, 2], [3]]);
});

test('backgroundShapes takes a big shape touching three borders', () => {
  const shapes = [
    { bbox: { x: 0, y: 0, width: 100, height: 100 }, fill: '#fff' },
    { bbox: { x: 20, y: 20, width: 50, height: 50 }, fill: '#f00' },
  ];
  assert.deepEqual(backgroundShapes(shapes, 100, 100), [0]);
});

test('backgroundShapes leaves a big shape that touches only two borders', () => {
  const shapes = [{ bbox: { x: 0, y: 20, width: 100, height: 60 }, fill: "#fff" }];
  assert.deepEqual(backgroundShapes(shapes, 100, 100), []);
});

test('backgroundShapes leaves a small shape touching every border', () => {
  // A thin frame has a full bbox but little area: the fill test uses the
  // shape's own area, not its box.
  const shapes = [{ bbox: { x: 0, y: 0, width: 100, height: 100 }, area: 800, fill: '#fff' }];
  assert.deepEqual(backgroundShapes(shapes, 100, 100), []);
});

test('backgroundShapes also takes border shapes of the same colour as the background', () => {
  const shapes = [
    { bbox: { x: 0, y: 0, width: 100, height: 100 }, fill: '#fff' },
    { bbox: { x: 0, y: 90, width: 10, height: 10 }, fill: '#fff' },   // corner, same colour
    { bbox: { x: 40, y: 40, width: 10, height: 10 }, fill: '#fff' },  // inside, same colour: a highlight, keep
    { bbox: { x: 0, y: 0, width: 10, height: 10 }, fill: '#f00' },    // corner, other colour: keep
  ];
  assert.deepEqual(backgroundShapes(shapes, 100, 100), [0, 1]);
});

test('backgroundShapes allows a pixel of slack at the borders', () => {
  const shapes = [{ bbox: { x: 0.6, y: 0.4, width: 99, height: 99.2 }, fill: '#fff' }];
  assert.deepEqual(backgroundShapes(shapes, 100, 100), [0]);
});

test('pruneDefs drops gradients nothing refers to and keeps the rest', () => {
  const svg = '<svg><defs><linearGradient id="g0"><stop/></linearGradient><radialGradient id="g1"><stop/></radialGradient></defs>\n<path fill="url(#g1)" d="M0 0"/>\n</svg>';
  assert.equal(pruneDefs(svg), '<svg><defs><radialGradient id="g1"><stop/></radialGradient></defs>\n<path fill="url(#g1)" d="M0 0"/>\n</svg>');
});

test('pruneDefs removes an emptied defs block', () => {
  const svg = '<svg>\n<defs><linearGradient id="g0"><stop/></linearGradient></defs>\n<path fill="#000" d="M0 0"/>\n</svg>';
  assert.equal(pruneDefs(svg), '<svg>\n<path fill="#000" d="M0 0"/>\n</svg>');
});

test('pruneDefs matches ids exactly, not by prefix', () => {
  const svg = '<svg><defs><linearGradient id="g1"><stop/></linearGradient><linearGradient id="g10"><stop/></linearGradient></defs><path fill="url(#g10)" d="M0 0"/></svg>';
  assert.equal(pruneDefs(svg), '<svg><defs><linearGradient id="g10"><stop/></linearGradient></defs><path fill="url(#g10)" d="M0 0"/></svg>');
});

test('History undoes and redoes, and a new edit drops the redo branch', () => {
  const h = new History('a');
  assert.equal(h.canUndo, false);
  h.push('b'); h.push('c');
  assert.equal(h.undo(), 'b');
  assert.equal(h.undo(), 'a');
  assert.equal(h.undo(), null);
  assert.equal(h.redo(), 'b');
  h.push('d');
  assert.equal(h.canRedo, false);
  assert.equal(h.redo(), null);
  assert.equal(h.current, 'd');
});

test('History keeps at most its cap of states', () => {
  const h = new History('0', 3);
  for (let i = 1; i <= 5; i++) h.push(String(i));
  assert.equal(h.undo(), '4');
  assert.equal(h.undo(), '3');
  assert.equal(h.undo(), null);
});

test('tintStops gives the first stop the new colour and moves the rest with it', () => {
  // A blue ramp made red: the first stop is exactly the colour asked,
  // the second keeps its 15 degrees of hue and its 10 points of
  // lightness away from the first, so the gradient still shades.
  const out = tintStops(['#00aeda', '#005da9'], '#ff0000');
  assert.equal(out[0], '#ff0000');
  const [h, sat, l] = hsl(out[1]);
  assert.equal(h, 15);
  assert.equal(sat, 100);
  assert.equal(l, 40);
});

test('tintStops keeps a spread of hues: yellow to magenta made red stays a ramp', () => {
  const out = tintStops(['#fecd4b', '#f453c9'], '#ff0000');
  assert.notEqual(out[0], out[1]);
  assert.ok(Math.abs(hsl(out[1])[0] - hsl(out[0])[0]) > 180, 'hues differ as before');
});

test('tintStops gives every stop the new hue when the ramp was grey', () => {
  const out = tintStops(['#808080', '#c0c0c0'], '#ff0000');
  assert.equal(hsl(out[1])[0], 0);
  assert.ok(hsl(out[1])[2] > hsl(out[0])[2]);
});

test('tintStops keeps a lighter stop lighter and clamps at white', () => {
  assert.deepEqual(tintStops(['#808080', '#f0f0f0'], '#ffffff'), ['#ffffff', '#ffffff']);
  const out = tintStops(['#808080', '#a0a0a0'], '#404040');
  assert.equal(out[0], '#404040');
  assert.ok(hsl(out[1])[2] > hsl(out[0])[2]);
});

test('tintStops keeps three-digit and upper-case input readable', () => {
  assert.deepEqual(tintStops(['#FFF'], '#000000'), ['#000000']);
});

test('selectionText reports the count and the share of the image', () => {
  assert.equal(selectionText(1, 0.123), '1 shape selected, 12% of the image.');
  assert.equal(selectionText(3, 0.004), '3 shapes selected, under 1% of the image.');
  assert.equal(selectionText(0, 0), '');
});

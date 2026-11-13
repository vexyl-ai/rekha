'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const { SEGMENT_KEYS, needsResegment, Requests, progressText, Viewport } = require('./protocol.js');

test('segment keys', () => {
  assert.deepEqual(SEGMENT_KEYS, ['auto', 'preset', 'threshold', 'colorPrecision', 'colorTolerance', 'maxColors', 'palette', 'filterSpeckle', 'gradients', 'gradientTolerance']);
});

test('needsResegment on preset change', () => {
  assert.equal(needsResegment({ preset: 'bw' }, { preset: 'poster' }), true);
});

test('finish-only change', () => {
  assert.equal(needsResegment({ preset: 'bw', refine: false }, { preset: 'bw', refine: true }), false);
});

test('null prev resegments', () => {
  assert.equal(needsResegment(null, { preset: 'bw' }), true);
});

test('palette array compared by value', () => {
  assert.equal(needsResegment({ palette: ['#000000'] }, { palette: ['#000000'] }), false);
  assert.equal(needsResegment({ palette: ['#000000'] }, { palette: ['#ffffff'] }), true);
});

test('stale results are ignored', () => {
  const r = new Requests();
  const a = r.next();
  const b = r.next();
  assert.equal(a, 1);
  assert.equal(b, 2);
  assert.equal(r.accept(a), false);
  assert.equal(r.accept(b), true);
});

const { Handles } = require('./protocol.js');

test('adopting a handle returns the previous one to free', () => {
  const h = new Handles();
  assert.equal(h.adopt(1), null);
  assert.equal(h.adopt(2), 1);
  assert.equal(h.current, 2);
});

test('release returns the current handle once', () => {
  const h = new Handles();
  h.adopt(5);
  assert.equal(h.release(), 5);
  assert.equal(h.release(), null);
  assert.equal(h.current, null);
});

const { imageFileFromItems } = require('./protocol.js');

test('paste picks the first image item', () => {
  const png = { kind: 'file', type: 'image/png', getAsFile: () => ({ name: 'a.png' }) };
  const items = [
    { kind: 'string', type: 'text/plain', getAsFile: () => null },
    png,
    { kind: 'file', type: 'image/jpeg', getAsFile: () => ({ name: 'b.jpg' }) },
  ];
  assert.deepEqual(imageFileFromItems(items), { name: 'a.png' });
});

test('paste without an image gives null', () => {
  assert.equal(imageFileFromItems([{ kind: 'string', type: 'text/plain', getAsFile: () => null }]), null);
  assert.equal(imageFileFromItems(null), null);
});

test('gradient keys re-segment', () => {
  assert.ok(SEGMENT_KEYS.includes('gradients') && SEGMENT_KEYS.includes('gradientTolerance'));
  assert.equal(needsResegment({ gradients: false }, { gradients: true }), true);
});

test('progress text names the stage and the count', () => {
  assert.equal(progressText('quantise', 0, 0), 'Quantising colours…');
  assert.equal(progressText('curves', 312, 1204), 'Fitting curves… 312 of 1,204');
  assert.equal(progressText('curves', 1204, 1204), 'Fitting curves… 1,204 of 1,204');
  assert.equal(progressText('nonsense', 0, 0), 'Working…');
});

test('viewport fits the image centred in the view', () => {
  const v = new Viewport();
  v.setView(1000, 600);
  v.setImage(2000, 1000);
  assert.deepEqual([v.z, v.x, v.y], [0.5, 0, 50]);
  assert.equal(v.fitted, true);
});

test('viewport zooms about the cursor and clamps to the edges', () => {
  const v = new Viewport();
  v.setView(1000, 600);
  v.setImage(2000, 1000);
  v.zoomAt(2, 500, 300);
  assert.deepEqual([v.z, v.x, v.y], [1, -500, -200]);
  assert.equal(v.fitted, false);
  v.pan(-2000, 0);
  assert.equal(v.x, -1000);
  v.pan(0, 5000);
  assert.equal(v.y, 0);
});

test('viewport centres an image smaller than the view', () => {
  const v = new Viewport();
  v.setView(1000, 600);
  v.setImage(2000, 1000);
  v.zoomAt(0.5, 0, 0);
  assert.deepEqual([v.z, v.x, v.y], [0.25, 250, 175]);
});

test('viewport zoom is bounded', () => {
  const v = new Viewport();
  v.setView(1000, 600);
  v.setImage(2000, 1000);
  v.zoomAt(1e6, 0, 0);
  assert.equal(v.z, 32);
  v.zoomAt(1e-6, 0, 0);
  assert.equal(v.z, 0.125); // a quarter of the fit scale
});

test('viewport refits on resize while fitted, keeps the zoom otherwise', () => {
  const v = new Viewport();
  v.setView(1000, 600);
  v.setImage(2000, 1000);
  v.setView(500, 500);
  assert.deepEqual([v.z, v.x, v.y], [0.25, 0, 125]);
  v.zoomTo(1, 250, 250);
  v.setView(400, 400);
  assert.equal(v.z, 1);
  assert.ok(v.x <= 0 && v.x >= 400 - 2000);
});

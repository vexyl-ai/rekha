'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const { SEGMENT_KEYS, needsResegment, Requests } = require('./protocol.js');

test('segment keys', () => {
  assert.deepEqual(SEGMENT_KEYS, ['preset', 'threshold', 'colorPrecision', 'colorTolerance', 'maxColors', 'palette', 'filterSpeckle']);
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

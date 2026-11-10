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

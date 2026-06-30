import assert from 'node:assert/strict'
import test from 'node:test'

import { restoredReaderScrollTop } from '../src/utils/readerScrollAnchor.js'

test('keeps the reading paragraph at the same viewport offset', () => {
  assert.equal(restoredReaderScrollTop({
    scrollTop: 420,
    previousOffset: 120,
    currentOffset: 760,
    maxScroll: 2400,
  }), 1060)
})

test('clamps restored reader scroll positions to the available range', () => {
  assert.equal(restoredReaderScrollTop({
    scrollTop: 50,
    previousOffset: 400,
    currentOffset: 100,
    maxScroll: 2000,
  }), 0)
  assert.equal(restoredReaderScrollTop({
    scrollTop: 1900,
    previousOffset: 100,
    currentOffset: 500,
    maxScroll: 2100,
  }), 2100)
})

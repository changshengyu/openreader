import assert from 'node:assert/strict'
import test from 'node:test'
import {
  readerTocTargetIndex,
  toggleReaderTocReverse,
} from '../src/utils/readerToc.js'

test('clamps reader table-of-contents targets to available chapters', () => {
  assert.equal(readerTocTargetIndex(-2, 10), 0)
  assert.equal(readerTocTargetIndex(4.8, 10), 4)
  assert.equal(readerTocTargetIndex(30, 10), 9)
  assert.equal(readerTocTargetIndex(3, 0), 0)
})

test('toggles reader table-of-contents order', () => {
  assert.equal(toggleReaderTocReverse(false), true)
  assert.equal(toggleReaderTocReverse(true), false)
})

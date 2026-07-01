import assert from 'node:assert/strict'
import test from 'node:test'
import {
  readerBlockTextOffset,
  readerScrollTextOffset,
  readerTextProgress,
  readerViewportAnchorY,
  selectVisibleReaderBlock,
} from '../src/utils/readerVisibility.js'

const viewport = {
  top: 100,
  bottom: 900,
  left: 0,
  right: 600,
  height: 800,
}

test('uses the capped 32 percent viewport reading anchor', () => {
  assert.equal(readerViewportAnchorY(viewport), 280)
  assert.equal(readerViewportAnchorY({
    top: 50,
    height: 300,
  }), 146)
})

test('selects the visible block covering the reading anchor', () => {
  const first = { id: 1 }
  const second = { id: 2 }
  assert.equal(selectVisibleReaderBlock([
    {
      node: first,
      rect: { top: 110, bottom: 240, left: 10, right: 590 },
    },
    {
      node: second,
      rect: { top: 240, bottom: 420, left: 10, right: 590 },
    },
  ], viewport), second)
})

test('falls back to the nearest visible block and rejects horizontal misses', () => {
  const near = { id: 'near' }
  const far = { id: 'far' }
  assert.equal(selectVisibleReaderBlock([
    {
      node: far,
      rect: { top: 500, bottom: 600, left: 10, right: 590 },
    },
    {
      node: near,
      rect: { top: 350, bottom: 400, left: 10, right: 590 },
    },
    {
      node: { id: 'outside' },
      rect: { top: 260, bottom: 320, left: 700, right: 800 },
    },
  ], viewport), near)
})

test('converts paragraph and scroll geometry into text progress', () => {
  assert.equal(readerBlockTextOffset({
    blockPosition: 100,
    textLength: 200,
    blockRect: { top: 180, height: 200 },
    viewport,
  }), 200)
  assert.equal(readerScrollTextOffset({
    scrollTop: 800,
    scrollHeight: 2400,
    clientHeight: 800,
    textLength: 1000,
  }), 500)
  assert.equal(readerTextProgress(500, 1000), 0.5)
  assert.equal(readerTextProgress(1500, 1000), 1)
})

import assert from 'node:assert/strict'
import test from 'node:test'
import {
  findVisibleReaderBlock,
  readerBlockTextOffset,
  readerScrollTextOffset,
  readerTextProgress,
  readerViewportAnchorY,
  selectTopVisibleReaderBlock,
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

test('keeps searching wrapped flip columns after a vertically low offscreen block', () => {
  const wrapped = { id: 'wrapped' }
  const nodes = [
    {
      getBoundingClientRect: () => ({ top: 980, bottom: 1080, left: -400, right: -20 }),
    },
    {
      ...wrapped,
      getBoundingClientRect: () => ({ top: 200, bottom: 360, left: 10, right: 590 }),
    },
  ]
  assert.equal(findVisibleReaderBlock(nodes, viewport, 8, false), nodes[1])
})

test('selects the first heading or paragraph below the upstream top boundary', () => {
  const heading = { id: 'heading' }
  const middleParagraph = { id: 'middle' }
  assert.equal(selectTopVisibleReaderBlock([
    {
      node: heading,
      rect: { top: 110, bottom: 170, left: 10, right: 590 },
    },
    {
      node: middleParagraph,
      rect: { top: 240, bottom: 420, left: 10, right: 590 },
    },
  ], viewport, 50), heading)

  const previousChapterTail = { id: 'previous-tail' }
  const nextChapterHeading = { id: 'next-heading' }
  assert.equal(selectTopVisibleReaderBlock([
    {
      node: previousChapterTail,
      rect: { top: 80, bottom: 149, left: 10, right: 590 },
    },
    {
      node: nextChapterHeading,
      rect: { top: 160, bottom: 220, left: 10, right: 590 },
    },
  ], viewport, 50), nextChapterHeading)
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

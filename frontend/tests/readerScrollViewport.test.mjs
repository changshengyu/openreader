import assert from 'node:assert/strict'
import test from 'node:test'
import {
  createDocumentReaderScrollViewport,
  readerElementScrollTop,
  shouldUseDocumentReaderScroll,
} from '../src/utils/readerScrollViewport.js'

test('uses the upstream document scroll host only for mobile vertical text modes', () => {
  for (const mode of ['page', 'scroll', 'scroll2']) {
    assert.equal(shouldUseDocumentReaderScroll({ mobile: true, mode, format: 'text' }), true)
  }
  assert.equal(shouldUseDocumentReaderScroll({ mobile: false, mode: 'page', format: 'text' }), false)
  assert.equal(shouldUseDocumentReaderScroll({ mobile: true, mode: 'flip', format: 'text' }), false)
  assert.equal(shouldUseDocumentReaderScroll({ mobile: true, mode: 'page', format: 'epub' }), false)
  assert.equal(shouldUseDocumentReaderScroll({ mobile: true, mode: 'page', format: 'audio' }), false)
  assert.equal(shouldUseDocumentReaderScroll({ mobile: true, mode: 'page', format: 'text', comic: true }), false)
})

test('document scroll adapter exposes one viewport while forwarding root position', () => {
  const root = {
    scrollTop: 120,
    scrollHeight: 2400,
    scrollWidth: 390,
    clientHeight: 844,
    clientWidth: 390,
  }
  const documentTarget = {
    scrollingElement: root,
    documentElement: root,
    body: { scrollTop: 0 },
  }
  const scrollCalls = []
  const windowTarget = {
    innerWidth: 390,
    innerHeight: 844,
    visualViewport: { width: 390, height: 812 },
    scrollTo: (...args) => scrollCalls.push(args),
  }
  const viewport = createDocumentReaderScrollViewport({ documentTarget, windowTarget })

  assert.equal(viewport.scrollTop, 120)
  viewport.scrollTop = 320
  assert.equal(root.scrollTop, 320)
  assert.equal(documentTarget.body.scrollTop, 320)
  assert.equal(viewport.scrollHeight, 2400)
  assert.equal(viewport.clientHeight, 812)
  assert.deepEqual(viewport.getBoundingClientRect(), {
    top: 0,
    right: 390,
    bottom: 812,
    left: 0,
    width: 390,
    height: 812,
    x: 0,
    y: 0,
  })

  viewport.scrollTo({ top: 480, behavior: 'auto' })
  assert.deepEqual(scrollCalls, [[{ top: 480, behavior: 'auto' }]])
})

test('maps an element rectangle to the active scroll host coordinate space', () => {
  const viewport = {
    scrollTop: 500,
    getBoundingClientRect: () => ({ top: 0 }),
  }
  const element = {
    getBoundingClientRect: () => ({ top: 260 }),
    offsetTop: 999,
  }
  assert.equal(readerElementScrollTop(viewport, element), 760)
})

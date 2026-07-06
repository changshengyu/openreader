import assert from 'node:assert/strict'
import test from 'node:test'
import { ref, shallowRef } from 'vue'
import {
  epubChapterIndexForResourceURL,
  useReaderEpubFrame,
} from '../src/composables/useReaderEpubFrame.js'

function createController() {
  const calls = []
  const frameWindow = {
    postMessage: (data, origin) => calls.push(['post', JSON.parse(data), origin]),
  }
  const frame = shallowRef({ contentWindow: frameWindow })
  const controller = useReaderEpubFrame({
    frame,
    resourceUrl: ref('/api/epub-resource/token/OPS/one.xhtml'),
    expectedOrigin: 'https://reader.example',
    viewportHeight: () => 800,
    styleText: () => 'body { color: #123; }',
    onReady: () => calls.push(['ready']),
    onLoad: data => calls.push(['load', data]),
    onHeight: height => calls.push(['height', height]),
    onClick: point => calls.push(['click', point]),
    onHash: rect => calls.push(['hash', rect]),
    onKeydown: event => calls.push(['keydown', event]),
    onPreview: data => calls.push(['preview', data]),
  })
  return { calls, controller, frameWindow }
}

function message(fixture, event, data, overrides = {}) {
  fixture.controller.handleMessage({
    source: fixture.frameWindow,
    origin: 'https://reader.example',
    data: JSON.stringify({ event, data }),
    ...overrides,
  })
}

test('accepts only messages from the active same-origin EPUB iframe', () => {
  const fixture = createController()
  message(fixture, 'click', { clientX: 10, clientY: 20 }, { origin: 'https://evil.example' })
  message(fixture, 'click', { clientX: 10, clientY: 20 }, { source: {} })
  assert.deepEqual(fixture.calls, [])
})

test('initializes style/height and forwards the upstream EPUB bridge events', () => {
  const fixture = createController()
  message(fixture, 'inited')
  assert.deepEqual(fixture.calls, [
    ['post', { event: 'setStyle', style: 'body { color: #123; }' }, 'https://reader.example'],
    ['post', { event: 'requestHeight' }, 'https://reader.example'],
    ['ready'],
  ])

  fixture.calls.length = 0
  message(fixture, 'load', { path: 'OPS/one.xhtml' })
  message(fixture, 'setHeight', 320)
  message(fixture, 'click', { clientX: 10, clientY: 20 })
  message(fixture, 'clickHash', { top: 45 })
  message(fixture, 'keydown', { key: 'ArrowRight', keyCode: 39 })
  message(fixture, 'previewImageList', { imageList: ['/one.jpg'], imageIndex: 0 })
  assert.deepEqual(fixture.calls, [
    ['load', { path: 'OPS/one.xhtml' }],
    ['height', 640],
    ['click', { clientX: 10, clientY: 20 }],
    ['hash', { top: 45 }],
    ['keydown', { key: 'ArrowRight', keyCode: 39 }],
    ['preview', { imageList: ['/one.jpg'], imageIndex: 0 }],
  ])
})

test('maps a navigated resource URL back to the matching chapter', () => {
  const chapters = [
    { index: 0, resourcePath: 'OPS/one.xhtml' },
    { index: 1, resourcePath: 'OPS/Text/two.xhtml' },
  ]
  assert.equal(
    epubChapterIndexForResourceURL(
      '/api/epub-resource/token/OPS/Text/two.xhtml#part',
      chapters,
    ),
    1,
  )
  assert.equal(
    epubChapterIndexForResourceURL('/api/epub-resource/token/OPS/missing.xhtml', chapters),
    -1,
  )
})

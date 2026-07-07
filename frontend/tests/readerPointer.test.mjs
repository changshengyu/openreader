import assert from 'node:assert/strict'
import test from 'node:test'
import { reactive, ref } from 'vue'
import { useReaderPointer } from '../src/composables/useReaderPointer.js'

function createController(overrides = {}) {
  const calls = []
  const reader = reactive({
    clickMethod: 'slide',
    mode: 'flip',
  })
  const rect = { left: 0, top: 0, width: 300, height: 600 }
  const pageEl = ref({
    getBoundingClientRect: () => rect,
  })
  let timestamp = 1000
  const controller = useReaderPointer({
    reader,
    pageEl,
    isMobileReader: ref(true),
    isOverlayOpen: ref(false),
    isAudio: ref(false),
    autoReading: ref(false),
    mobileChromeVisible: ref(true),
    scheduleSelectedTextOperation: delay => {
      calls.push(['selection', delay])
      return false
    },
    suppressContentClick: delay => calls.push(['suppress', delay]),
    consumeSuppressedContentClick: () => false,
    nextPage: () => calls.push(['next']),
    previousPage: () => calls.push(['previous']),
    toggleChrome: () => calls.push(['chrome']),
    now: () => timestamp,
    windowTarget: { innerWidth: 300, innerHeight: 600 },
    ...overrides,
  })
  return {
    calls,
    controller,
    pageEl,
    reader,
    setTimestamp: value => {
      timestamp = value
    },
  }
}

test('treats small finger movement as a center tap', () => {
  const fixture = createController()
  fixture.controller.handleTouchStart({
    touches: [{ clientX: 150, clientY: 300 }],
  })
  fixture.setTimestamp(1100)
  fixture.controller.handleTouchMove({
    touches: [{ clientX: 158, clientY: 305 }],
    preventDefault: () => fixture.calls.push(['prevent']),
    stopPropagation: () => fixture.calls.push(['stop']),
  })
  fixture.controller.handleTouchEnd({
    changedTouches: [{ clientX: 158, clientY: 305 }],
  })
  assert.deepEqual(fixture.calls, [
    ['selection', 200],
    ['suppress', 360],
    ['selection', 0],
    ['chrome'],
  ])
})

test('handles horizontal flip swipes without triggering taps', () => {
  const fixture = createController()
  fixture.controller.handleTouchStart({
    touches: [{ clientX: 240, clientY: 300 }],
  })
  fixture.controller.handleTouchMove({
    touches: [{ clientX: 120, clientY: 305 }],
    preventDefault: () => fixture.calls.push(['prevent']),
    stopPropagation: () => fixture.calls.push(['stop']),
  })
  fixture.controller.handleTouchEnd({
    changedTouches: [{ clientX: 120, clientY: 305 }],
  })
  assert.deepEqual(fixture.calls, [
    ['prevent'],
    ['stop'],
    ['selection', 200],
    ['suppress', 360],
    ['next'],
  ])
})

test('gives selected text priority over touch navigation', () => {
  const fixture = createController({
    scheduleSelectedTextOperation: delay => {
      fixture.calls.push(['selection', delay])
      return delay === 200
    },
  })
  fixture.controller.handleTouchStart({
    touches: [{ clientX: 240, clientY: 300 }],
  })
  fixture.controller.handleTouchEnd({
    changedTouches: [{ clientX: 120, clientY: 300 }],
  })
  assert.deepEqual(fixture.calls, [
    ['selection', 200],
    ['suppress', undefined],
  ])
})

test('ignores synthetic touch clicks and maps desktop reader clicks', () => {
  const mobile = createController()
  mobile.controller.handleTouchStart({
    touches: [{ clientX: 150, clientY: 300 }],
  })
  mobile.controller.handleTouchEnd({
    changedTouches: [{ clientX: 150, clientY: 300 }],
  })
  mobile.calls.length = 0
  mobile.controller.handleContentClick({
    button: 0,
    defaultPrevented: false,
    clientX: 280,
    clientY: 300,
    target: { closest: () => null },
  })
  assert.deepEqual(mobile.calls, [])

  const desktop = createController({ isMobileReader: ref(false) })
  desktop.controller.handleContentClick({
    button: 0,
    defaultPrevented: false,
    clientX: 280,
    clientY: 300,
    target: { closest: () => null },
  })
  assert.deepEqual(desktop.calls, [
    ['selection', 0],
    ['next'],
  ])
})

test('audio chapters only toggle chrome from the center and never page', () => {
  const fixture = createController({ isAudio: ref(true) })
  fixture.controller.handleContentClick({
    button: 0,
    defaultPrevented: false,
    clientX: 280,
    clientY: 300,
    target: { closest: () => null },
  })
  fixture.controller.handleContentClick({
    button: 0,
    defaultPrevented: false,
    clientX: 150,
    clientY: 300,
    target: { closest: () => null },
  })
  fixture.controller.handleTapZone('right')
  fixture.controller.handleTapZone('center')
  assert.deepEqual(fixture.calls, [
    ['selection', 0],
    ['selection', 0],
    ['chrome'],
    ['chrome'],
  ])
})

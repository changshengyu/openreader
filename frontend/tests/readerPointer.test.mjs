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
    ttsBarVisible: ref(false),
    isAudio: ref(false),
    autoReading: ref(false),
    mobileChromeVisible: ref(true),
    scheduleSelectedTextOperation: (delay, selectionOptions) => {
      calls.push(['selection', delay, selectionOptions])
      return false
    },
    preparePageAnimation: () => calls.push(['prepare-animation']),
    releasePageAnimationPreparation: () => calls.push(['release-animation']),
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
    ['prepare-animation'],
    ['selection', 0, { retry: false }],
    ['suppress', 360],
    ['release-animation'],
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
    ['prepare-animation'],
    ['prevent'],
    ['stop'],
    ['selection', 0, { retry: false }],
    ['suppress', 360],
    ['release-animation'],
    ['next'],
  ])
})

test('keeps a running click animation for taps and only cancels after a real drag begins', () => {
  const fixture = createController({
    reader: reactive({ clickMethod: 'slide', mode: 'page' }),
    cancelPageAnimation: () => fixture.calls.push(['cancel-animation']),
  })
  fixture.controller.handleTouchStart({
    touches: [{ clientX: 150, clientY: 500 }],
  })
  fixture.controller.handleTouchMove({
    touches: [{ clientX: 155, clientY: 493 }],
    preventDefault: () => fixture.calls.push(['prevent']),
    stopPropagation: () => fixture.calls.push(['stop']),
  })
  assert.deepEqual(fixture.calls, [['prepare-animation']])

  fixture.controller.handleTouchMove({
    touches: [{ clientX: 151, clientY: 470 }],
    preventDefault: () => fixture.calls.push(['prevent']),
    stopPropagation: () => fixture.calls.push(['stop']),
  })
  fixture.controller.handleTouchMove({
    touches: [{ clientX: 150, clientY: 430 }],
    preventDefault: () => fixture.calls.push(['prevent']),
    stopPropagation: () => fixture.calls.push(['stop']),
  })
  assert.deepEqual(fixture.calls, [['prepare-animation'], ['cancel-animation']])
})

test('gives selected text priority over touch navigation', () => {
  const fixture = createController({
    scheduleSelectedTextOperation: (delay, selectionOptions) => {
      fixture.calls.push(['selection', delay, selectionOptions])
      return true
    },
  })
  fixture.controller.handleTouchStart({
    touches: [{ clientX: 240, clientY: 300 }],
  })
  fixture.controller.handleTouchEnd({
    changedTouches: [{ clientX: 120, clientY: 300 }],
  })
  assert.deepEqual(fixture.calls, [
    ['prepare-animation'],
    ['selection', 0, { retry: false }],
    ['suppress', undefined],
    ['release-animation'],
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
    ['selection', 0, { retry: false }],
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
    ['selection', 0, { retry: false }],
    ['release-animation'],
    ['selection', 0, { retry: false }],
    ['release-animation'],
    ['chrome'],
    ['release-animation'],
    ['release-animation'],
    ['chrome'],
  ])
})

test('keeps page actions but not mobile chrome toggles while the upstream-style TTS bar is open', () => {
  const fixture = createController({
    ttsBarVisible: ref(true),
    reader: reactive({ clickMethod: 'slide', mode: 'page' }),
  })
  fixture.controller.handleContentClick({
    button: 0,
    defaultPrevented: false,
    clientX: 150,
    clientY: 300,
    target: { closest: () => null },
  })
  fixture.controller.handleTapZone('center')
  fixture.controller.handleContentClick({
    button: 0,
    defaultPrevented: false,
    clientX: 150,
    clientY: 520,
    target: { closest: () => null },
  })
  fixture.controller.handleTapZone('lower')
  assert.deepEqual(fixture.calls, [
    ['selection', 0, { retry: false }],
    ['release-animation'],
    ['release-animation'],
    ['selection', 0, { retry: false }],
    ['next'],
    ['next'],
  ])
})

test('keeps delayed selection retries for a long press without turning it into a page tap', () => {
  const fixture = createController({
    reader: reactive({ clickMethod: 'slide', mode: 'page' }),
  })
  fixture.controller.handleTouchStart({
    touches: [{ clientX: 150, clientY: 500 }],
  })
  fixture.setTimestamp(1500)
  fixture.controller.handleTouchEnd({
    changedTouches: [{ clientX: 150, clientY: 500 }],
  })
  assert.deepEqual(fixture.calls, [
    ['prepare-animation'],
    ['selection', 0, { retry: true }],
    ['suppress', undefined],
    ['release-animation'],
  ])
})

test('releases an unused prepared layer when the browser cancels the touch sequence', () => {
  const fixture = createController({
    reader: reactive({ clickMethod: 'slide', mode: 'page' }),
  })
  fixture.controller.handleTouchStart({
    touches: [{ clientX: 150, clientY: 500 }],
  })
  fixture.controller.handleTouchCancel()
  assert.deepEqual(fixture.calls, [
    ['prepare-animation'],
    ['release-animation'],
  ])
})

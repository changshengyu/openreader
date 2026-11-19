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
    ['selection', 0, { retry: false }],
    ['suppress', 360],
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
    ['selection', 0, { retry: false }],
    ['suppress', 360],
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
  assert.deepEqual(fixture.calls, [])

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
  assert.deepEqual(fixture.calls, [['cancel-animation']])
})

test('touchstart cancels only deferred settlement and progress work, not active visual motion', () => {
  const fixture = createController({
    reader: reactive({ clickMethod: 'slide', mode: 'page' }),
    preparePageInput: () => fixture.calls.push(['prepare-input']),
    cancelPageAnimation: () => fixture.calls.push(['cancel-animation']),
  })
  fixture.controller.handleTouchStart({
    touches: [{ clientX: 150, clientY: 500 }],
  })
  assert.deepEqual(fixture.calls, [['prepare-input']])
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
    ['selection', 0, { retry: false }],
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
    ['selection', 0, { retry: false }],
    ['chrome'],
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
    ['selection', 0, { retry: true }],
    ['suppress', undefined],
  ])
})

test('cancels a touch sequence without preparing a full-chapter layer', () => {
  const fixture = createController({
    reader: reactive({ clickMethod: 'slide', mode: 'page' }),
  })
  fixture.controller.handleTouchStart({
    touches: [{ clientX: 150, clientY: 500 }],
  })
  fixture.controller.handleTouchCancel()
  assert.deepEqual(fixture.calls, [])
})

test('short touch taps on reader buttons such as 重新加载 never page or toggle the tool layer', () => {
  const button = {
    tagName: 'BUTTON',
    closest: selector => (selector.split(',').map(part => part.trim()).includes('button') ? button : null),
  }
  const buttonLabel = { tagName: 'SPAN', closest: selector => button.closest(selector) }
  const text = { tagName: 'P', closest: () => null }
  const pageActions = ['previous', 'next', 'chrome']
  for (const [mode, point, textAction] of [
    ['page', { clientX: 150, clientY: 60 }, 'previous'],
    ['page', { clientX: 150, clientY: 540 }, 'next'],
    ['page', { clientX: 150, clientY: 300 }, 'chrome'],
    ['flip', { clientX: 30, clientY: 300 }, 'previous'],
    ['flip', { clientX: 270, clientY: 300 }, 'next'],
    ['scroll', { clientX: 150, clientY: 60 }, 'previous'],
    ['scroll2', { clientX: 150, clientY: 540 }, 'next'],
  ]) {
    for (const target of [button, buttonLabel]) {
      const mobileChromeVisible = ref(true)
      const fixture = createController({ reader: reactive({ clickMethod: 'slide', mode }), mobileChromeVisible })
      fixture.controller.handleTouchStart({ touches: [point], target })
      fixture.setTimestamp(1080)
      fixture.controller.handleTouchEnd({ changedTouches: [point], target })
      const label = `${mode} tap on ${target.tagName} at ${point.clientX},${point.clientY}`
      assert.deepEqual(fixture.calls.filter(([name]) => pageActions.includes(name)), [], `${label} reached the page actions`)
      assert.equal(mobileChromeVisible.value, true, `${label} hid the tool layer`)
    }

    // The same tap on reader text keeps the existing tap-point mapping.
    const fixture = createController({ reader: reactive({ clickMethod: 'slide', mode }) })
    fixture.controller.handleTouchStart({ touches: [point], target: text })
    fixture.setTimestamp(1080)
    fixture.controller.handleTouchEnd({ changedTouches: [point], target: text })
    assert.deepEqual(fixture.calls.filter(([name]) => pageActions.includes(name)), [[textAction]], `${mode} text tap mapping changed`)
  }
})

test('reports only a trusted single-finger vertical drag in the reading area', () => {
  const reported = []
  const overlay = ref(false)
  const fixture = createController({
    reader: reactive({ clickMethod: 'slide', mode: 'page' }),
    isOverlayOpen: overlay,
    onNativeScrollInput: delta => reported.push(delta),
  })
  const point = (x, y, identifier = 1) => ({ clientX: x, clientY: y, identifier })
  const start = () => fixture.controller.handleTouchStart({ touches: [point(150, 300)] })
  const move = (touches, attributes = {}) => fixture.controller.handleTouchMove({
    touches,
    isTrusted: true,
    ...attributes,
  })
  start()
  move([point(150, 295)])
  start()
  move([point(180, 290)])
  start()
  move([point(150, 270), point(90, 270, 2)])
  start()
  move([point(150, 270, 2)])
  start()
  move([point(150, 270)], { isTrusted: false })
  start()
  overlay.value = true
  move([point(150, 270)])
  assert.deepEqual(reported, [])

  start()
  overlay.value = false
  move([point(150, 275)])
  move([point(152, 245)])
  assert.deepEqual(reported, [25, 30])
})

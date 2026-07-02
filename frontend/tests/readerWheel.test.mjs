import assert from 'node:assert/strict'
import test from 'node:test'
import { reactive, ref } from 'vue'
import { useReaderWheel } from '../src/composables/useReaderWheel.js'

function createController(overrides = {}) {
  const calls = []
  const reader = reactive({
    fontSize: 18,
    lineHeight: 2,
    animateDuration: 200,
  })
  const target = {
    closest: () => null,
  }
  const shellEl = ref({
    contains: candidate => candidate?.inside !== false,
  })
  const contentEl = ref({
    clientHeight: 400,
    scrollHeight: 1200,
    scrollTop: 300,
  })
  let timestamp = 1000
  const controller = useReaderWheel({
    reader,
    shellEl,
    contentEl,
    isOverlayOpen: ref(false),
    isScrollRead: ref(false),
    nextPage: () => calls.push(['next']),
    previousPage: () => calls.push(['previous']),
    now: () => timestamp,
    windowTarget: { innerHeight: 800 },
    ...overrides,
  })
  function event(deltaY, eventOverrides = {}) {
    return {
      target,
      deltaX: 0,
      deltaY,
      deltaMode: 0,
      preventDefault: () => calls.push(['prevent']),
      ...eventOverrides,
    }
  }
  return {
    calls,
    contentEl,
    controller,
    event,
    setTimestamp: value => {
      timestamp = value
    },
    target,
  }
}

test('turns wheel movement into throttled page navigation', () => {
  const fixture = createController()
  fixture.controller.handle(fixture.event(120))
  fixture.setTimestamp(1100)
  fixture.controller.handle(fixture.event(120))
  fixture.setTimestamp(1300)
  fixture.controller.handle(fixture.event(-120))
  assert.deepEqual(fixture.calls, [
    ['prevent'],
    ['next'],
    ['prevent'],
    ['prevent'],
    ['previous'],
  ])
})

test('scrolls continuous content and crosses chapters at boundaries', () => {
  const fixture = createController({ isScrollRead: ref(true) })
  fixture.controller.handle(fixture.event(100))
  assert.equal(fixture.contentEl.value.scrollTop, 400)
  fixture.contentEl.value.scrollTop = 800
  fixture.controller.handle(fixture.event(100))
  fixture.contentEl.value.scrollTop = 0
  fixture.controller.handle(fixture.event(-100))
  assert.deepEqual(fixture.calls, [
    ['prevent'],
    ['prevent'],
    ['next'],
    ['prevent'],
    ['previous'],
  ])
})

test('ignores duplicate, outside, and interactive wheel events', () => {
  const fixture = createController()
  const duplicate = fixture.event(120, { _openReaderWheelHandled: true })
  fixture.controller.handle(duplicate)
  fixture.controller.handle(fixture.event(120, { target: { inside: false } }))
  fixture.controller.handle(fixture.event(120, {
    target: { closest: () => ({ tagName: 'INPUT' }) },
  }))
  assert.deepEqual(fixture.calls, [])
})

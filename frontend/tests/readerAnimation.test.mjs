import assert from 'node:assert/strict'
import test from 'node:test'
import { createReaderScrollAnimator } from '../src/utils/readerAnimation.js'

function createClock() {
  let now = 0
  let nextId = 0
  const frames = new Map()
  const tasks = new Map()
  return {
    now: () => now,
    requestFrame(callback) {
      nextId += 1
      frames.set(nextId, callback)
      return nextId
    },
    cancelFrame(id) {
      frames.delete(id)
    },
    scheduleTask(callback) {
      nextId += 1
      tasks.set(nextId, callback)
      return nextId
    },
    cancelTask(id) {
      tasks.delete(id)
    },
    step(time) {
      now = time
      const pending = [...frames.values()]
      frames.clear()
      pending.forEach(callback => callback(time))
    },
    flushTasks() {
      const pending = [...tasks.values()]
      tasks.clear()
      pending.forEach(callback => callback())
    },
  }
}

test('uses the configured duration instead of collapsing every positive value to smooth', () => {
  const clock = createClock()
  const element = { scrollTop: 100, scrollHeight: 2000, clientHeight: 800 }
  const completed = []
  const animator = createReaderScrollAnimator(clock)

  assert.equal(animator.scrollBy(element, 600, 500, () => completed.push(true)), true)
  clock.step(100)
  assert(element.scrollTop > 100 && element.scrollTop < 700)
  clock.step(250)
  assert(element.scrollTop > 300 && element.scrollTop < 700)
  clock.step(500)
  assert.equal(element.scrollTop, 700)
  assert.deepEqual(completed, [true])
  assert.equal(animator.isActive(), false)
})

test('jumps synchronously at zero milliseconds and clamps to the scroll range', () => {
  const clock = createClock()
  const element = { scrollTop: 700, scrollHeight: 1200, clientHeight: 500 }
  let completed = 0
  const animator = createReaderScrollAnimator(clock)

  assert.equal(animator.scrollBy(element, 900, 0, () => { completed += 1 }), true)
  assert.equal(element.scrollTop, 700)
  assert.equal(completed, 1)
  assert.equal(animator.isActive(), false)
})

test('blocks overlapping page animations and supports cancellation', () => {
  const clock = createClock()
  const element = { scrollTop: 0, scrollHeight: 2000, clientHeight: 800 }
  let completed = 0
  const animator = createReaderScrollAnimator(clock)

  assert.equal(animator.scrollBy(element, 600, 500, () => { completed += 1 }), true)
  assert.equal(animator.scrollBy(element, 600, 100, () => { completed += 1 }), false)
  clock.step(100)
  const cancelledTop = element.scrollTop
  animator.cancel()
  clock.step(500)
  assert.equal(element.scrollTop, cancelledTop)
  assert.equal(completed, 0)
  assert.equal(animator.isActive(), false)
})

test('starts responsive mobile page motion before a zero-timestamp first frame', () => {
  const clock = createClock()
  let scrollTop = 100
  const writes = []
  const element = {
    get scrollTop() {
      return scrollTop
    },
    set scrollTop(value) {
      scrollTop = value
      writes.push(value)
    },
    scrollHeight: 2000,
    clientHeight: 800,
  }
  const visualElement = {
    style: { willChange: '' },
    animate() {
      assert.fail('mobile text paging must not animate the full chapter body')
    },
  }
  let completed = 0
  const animator = createReaderScrollAnimator(clock)

  assert.equal(animator.scrollBy(
    element,
    600,
    300,
    () => { completed += 1 },
    { easing: 'responsive', visualElement },
  ), true)
  assert(scrollTop > 100, `touchend left the first painted position at the origin: ${scrollTop}`)
  const startedTop = scrollTop
  assert(
    startedTop - 100 <= 2,
    `the synchronous seed jumped too much text before the first frame: ${startedTop}`,
  )
  clock.step(0)
  assert(scrollTop >= startedTop, `the first rAF moved back to the origin: ${scrollTop}/${startedTop}`)
  clock.step(16)
  assert(scrollTop >= 106, `the first refresh interval remained in a dead zone: ${scrollTop}`)
  assert(scrollTop <= 124, `the first refresh interval jumped too much text: ${scrollTop}`)
  clock.step(32)
  assert(scrollTop >= 130, `the second refresh interval remained in a dead zone: ${scrollTop}`)
  assert(scrollTop <= 150, `the second refresh interval accelerated too abruptly: ${scrollTop}`)
  assert.equal(visualElement.style.willChange, '')
  clock.step(150)
  assert(scrollTop > 300 && scrollTop < 500)
  clock.step(300)
  assert.equal(scrollTop, 700)
  assert(writes.length >= 4, 'the lightweight path must advance scrollTop on refresh frames')
  assert.equal(completed, 1)
  assert.equal(animator.isActive(), false)
})

test('keeps the visible frame-scroll position after a touch or wheel cancellation', () => {
  const clock = createClock()
  let scrollTop = 100
  const writes = []
  const element = {
    get scrollTop() {
      return scrollTop
    },
    set scrollTop(value) {
      scrollTop = value
      writes.push(value)
    },
    scrollHeight: 2000,
    clientHeight: 800,
  }
  let completed = 0
  const animator = createReaderScrollAnimator(clock)

  animator.scrollBy(
    element,
    600,
    300,
    () => { completed += 1 },
    { easing: 'responsive' },
  )
  clock.step(150)
  const visibleTop = scrollTop
  animator.cancel()
  clock.step(300)

  assert.equal(Math.round(scrollTop), Math.round(visibleTop))
  assert(writes.length >= 1)
  assert.equal(completed, 0)
  assert.equal(animator.isActive(), false)
})

test('presents the responsive target before running heavy completion work in a later task', () => {
  const clock = createClock()
  const element = { scrollTop: 0, scrollHeight: 2000, clientHeight: 800 }
  const completed = []
  const animator = createReaderScrollAnimator(clock)

  animator.scrollBy(
    element,
    600,
    300,
    () => completed.push(element.scrollTop),
    { easing: 'responsive', finish: 'after-paint' },
  )
  clock.step(300)

  assert.equal(element.scrollTop, 600, 'the visual target must be committed in the final animation frame')
  assert.deepEqual(completed, [], 'chapter/layout settlement must not run inside the final animation frame')
  assert.equal(animator.isActive(), true, 'the handoff window must remain cancellable and block overlap')

  clock.flushTasks()
  assert.deepEqual(completed, [600])
  assert.equal(animator.isActive(), false)
})

test('continues a buffered visual page in the final frame without an intermediate settlement task', () => {
  const clock = createClock()
  const element = { scrollTop: 0, scrollHeight: 3000, clientHeight: 800 }
  const animator = createReaderScrollAnimator(clock)
  let visualSegments = 0
  let settled = 0

  const startSegment = () => animator.scrollBy(
    element,
    600,
    300,
    () => { settled += 1 },
    {
      easing: 'responsive',
      finish: 'after-paint',
      onVisualFinish: () => {
        visualSegments += 1
        if (visualSegments !== 1) return false
        return startSegment()
      },
    },
  )

  assert.equal(startSegment(), true)
  clock.step(300)
  assert(
    element.scrollTop > 600,
    `the buffered page must seed visible movement before the first endpoint can paint: ${element.scrollTop}`,
  )
  assert.equal(settled, 0)
  assert.equal(animator.isActive(), true)

  clock.flushTasks()
  assert.equal(settled, 0, 'the first visual segment must not schedule business settlement')

  clock.step(600)
  assert.equal(element.scrollTop, 1200)
  assert.equal(settled, 0)
  clock.flushTasks()
  assert.equal(settled, 1, 'the buffered chain must settle exactly once after its final visual segment')
  assert.equal(animator.isActive(), false)
})

test('cancels an after-paint completion before stale settlement can run', () => {
  const clock = createClock()
  const element = { scrollTop: 0, scrollHeight: 2000, clientHeight: 800 }
  let completed = 0
  const animator = createReaderScrollAnimator(clock)

  animator.scrollBy(
    element,
    600,
    300,
    () => { completed += 1 },
    { easing: 'responsive', finish: 'after-paint' },
  )
  clock.step(300)
  animator.cancel()
  clock.flushTasks()

  assert.equal(element.scrollTop, 600)
  assert.equal(completed, 0)
  assert.equal(animator.isActive(), false)
})

test('lets a new visual page take over an after-paint settlement handoff', () => {
  const clock = createClock()
  const element = { scrollTop: 0, scrollHeight: 3000, clientHeight: 800 }
  let settled = 0
  const animator = createReaderScrollAnimator(clock)

  animator.scrollBy(
    element,
    600,
    300,
    () => { settled += 1 },
    { easing: 'responsive', finish: 'after-paint' },
  )
  clock.step(300)
  assert.equal(element.scrollTop, 600)
  assert.equal(animator.isActive(), true)
  assert.equal(animator.takeOverPendingFinish(), true)
  assert.equal(animator.isActive(), false)

  assert.equal(animator.scrollBy(
    element,
    600,
    300,
    () => { settled += 1 },
    { easing: 'responsive', finish: 'after-paint' },
  ), true)
  assert(element.scrollTop > 600, 'the replacement page must seed motion in the takeover input task')
  clock.flushTasks()
  assert.equal(settled, 0, 'the superseded handoff must never run its old settlement')
})

test('keeps zero-duration paging synchronous even when the positive path finishes after paint', () => {
  const clock = createClock()
  const element = { scrollTop: 0, scrollHeight: 2000, clientHeight: 800 }
  let completed = 0
  const animator = createReaderScrollAnimator(clock)

  animator.scrollBy(
    element,
    600,
    0,
    () => { completed += 1 },
    { easing: 'responsive', finish: 'after-paint' },
  )

  assert.equal(element.scrollTop, 600)
  assert.equal(completed, 1)
  assert.equal(animator.isActive(), false)
})

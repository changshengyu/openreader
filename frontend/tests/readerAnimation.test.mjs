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
    step(time, executionTime = time) {
      now = executionTime
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

test('matches the upstream power-cubic ease-in-out without a synchronous seed', () => {
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

  assert.equal(animator.scrollBy(
    element,
    600,
    300,
    () => { completed += 1 },
  ), true)
  assert.equal(scrollTop, 100, 'upstream does not mutate scrollTop before the first animation frame')
  clock.step(0)
  assert.equal(scrollTop, 100)
  clock.step(75)
  assert.equal(scrollTop, 137.5)
  clock.step(150)
  assert.equal(scrollTop, 400)
  clock.step(225)
  assert.equal(scrollTop, 662.5)
  clock.step(300)
  assert.equal(scrollTop, 700)
  assert.equal(writes.length, 5)
  assert.equal(completed, 1)
  assert.equal(animator.isActive(), false)
})

test('uses the callback execution clock when the browser supplies a stale frame timestamp', () => {
  const clock = createClock()
  let scrollTop = 100
  const element = {
    get scrollTop() {
      return scrollTop
    },
    set scrollTop(value) {
      scrollTop = value
    },
    scrollHeight: 2000,
    clientHeight: 800,
  }
  const animator = createReaderScrollAnimator(clock)

  assert.equal(animator.scrollBy(element, 600, 300), true)
  clock.step(0, 16)

  const progress = 16 / 300
  const expected = 100 + 600 * (4 * progress * progress * progress)
  assert.equal(
    scrollTop,
    expected,
    'reader-dev reads Date.now() inside the callback instead of treating a stale rAF argument as elapsed=0',
  )
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

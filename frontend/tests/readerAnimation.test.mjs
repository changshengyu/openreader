import assert from 'node:assert/strict'
import test from 'node:test'
import { createReaderScrollAnimator } from '../src/utils/readerAnimation.js'

function createClock() {
  let now = 0
  let nextId = 0
  const frames = new Map()
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
    step(time) {
      now = time
      const pending = [...frames.values()]
      frames.clear()
      pending.forEach(callback => callback(time))
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

function createVisualAnimationFixture() {
  const calls = []
  const animation = {
    currentTime: 0,
    effect: {
      getComputedTiming: () => ({ progress: 0 }),
    },
    cancel() {
      calls.push(['cancel'])
    },
    onfinish: null,
  }
  const visualElement = {
    style: {
      willChange: '',
    },
    animate(keyframes, timing) {
      calls.push(['animate', keyframes, timing])
      return animation
    },
  }
  return { animation, calls, visualElement }
}

test('runs mobile page motion on the composited body and commits scrollTop once at settlement', () => {
  const fixture = createVisualAnimationFixture()
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
  const animator = createReaderScrollAnimator()

  assert.equal(animator.scrollBy(
    element,
    600,
    300,
    () => { completed += 1 },
    { visualElement: fixture.visualElement },
  ), true)
  assert.equal(fixture.calls[0][0], 'animate')
  assert.equal(fixture.calls[0][2].duration, 300)
  assert.equal(writes.length, 0, 'composited motion must not write scrollTop on every frame')
  assert.equal(fixture.visualElement.style.willChange, 'transform')

  fixture.animation.onfinish()
  assert.deepEqual(writes, [700])
  assert.equal(completed, 1)
  assert.equal(animator.isActive(), false)
  assert.equal(fixture.visualElement.style.willChange, '')
})

test('commits the visible composited offset before a touch or wheel cancellation', () => {
  const fixture = createVisualAnimationFixture()
  fixture.animation.currentTime = 150
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
  const animator = createReaderScrollAnimator()

  animator.scrollBy(
    element,
    600,
    300,
    () => { completed += 1 },
    { visualElement: fixture.visualElement },
  )
  animator.cancel()

  assert.equal(Math.round(scrollTop), 400)
  assert.deepEqual(writes.map(Math.round), [400])
  assert.equal(completed, 0)
  assert.equal(animator.isActive(), false)
  assert.equal(fixture.visualElement.style.willChange, '')
  assert.deepEqual(fixture.calls.map(call => call[0]), ['animate', 'cancel'])
})

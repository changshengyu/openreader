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

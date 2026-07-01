import assert from 'node:assert/strict'
import test from 'node:test'
import { effectScope } from 'vue'
import { useReaderToast } from '../src/composables/useReaderToast.js'

test('replaces previous toast timers and clears the active message', () => {
  const callbacks = new Map()
  const cleared = []
  let timerId = 0
  const scope = effectScope()
  const toast = scope.run(() => useReaderToast({
    setTimeout: callback => {
      timerId += 1
      callbacks.set(timerId, callback)
      return timerId
    },
    clearTimeout: id => {
      cleared.push(id)
      callbacks.delete(id)
    },
  }))

  toast.show('第一条', 1200)
  toast.show('第二条', 1600)
  assert.deepEqual(cleared, [1])
  assert.equal(toast.message.value, '第二条')
  callbacks.get(2)()
  assert.equal(toast.message.value, '')
  scope.stop()
})

test('keeps zero-duration messages until explicitly cleared', () => {
  const scope = effectScope()
  const toast = scope.run(() => useReaderToast())
  toast.show('朗读中', 0)
  assert.equal(toast.message.value, '朗读中')
  toast.clear()
  assert.equal(toast.message.value, '')
  scope.stop()
})

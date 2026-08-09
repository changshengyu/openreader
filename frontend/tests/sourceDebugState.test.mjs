import assert from 'node:assert/strict'
import test from 'node:test'
import {
  createSourceDebugHistory,
  loadSourceDebugSources,
  saveSourceDebugSources,
  sourceDebugStorageKey,
} from '../src/utils/sourceDebugState.js'

function memoryStorage() {
  const values = new Map()
  return {
    getItem: key => values.has(key) ? values.get(key) : null,
    setItem: (key, value) => values.set(key, String(value)),
    removeItem: key => values.delete(key),
  }
}

test('source-debug history keeps the fixed 50-step old/now/new contract', () => {
  const storage = memoryStorage()
  const history = createSourceDebugHistory({ storage, scope: 'user:1', initial: { name: '0' } })
  for (let index = 1; index <= 55; index += 1) {
    history.commit({ name: String(index) })
  }

  assert.equal(history.current().name, '55')
  assert.equal(history.snapshot().old.length, 50)
  assert.equal(history.undo().name, '54')
  assert.equal(history.redo().name, '55')
  assert.equal(history.snapshot().new.length, 0)
})

test('source-debug local source list and history are schema-versioned and account scoped', () => {
  const storage = memoryStorage()
  saveSourceDebugSources(storage, 'user:1', [{ bookSourceUrl: 'https://one.example' }])

  assert.deepEqual(loadSourceDebugSources(storage, 'user:1'), [{ bookSourceUrl: 'https://one.example' }])
  assert.deepEqual(loadSourceDebugSources(storage, 'user:2'), [])
  assert.notEqual(sourceDebugStorageKey('user:1', 'sources'), sourceDebugStorageKey('user:2', 'sources'))

  const first = createSourceDebugHistory({ storage, scope: 'user:1', initial: { name: 'first' } })
  first.commit({ name: 'private draft' })
  const second = createSourceDebugHistory({ storage, scope: 'user:2', initial: { name: 'second' } })
  assert.equal(second.current().name, 'second')
})

test('source-debug history clones values so callers cannot mutate persisted snapshots', () => {
  const storage = memoryStorage()
  const initial = { nested: { value: 1 } }
  const history = createSourceDebugHistory({ storage, scope: 'user:1', initial })
  initial.nested.value = 99
  assert.equal(history.current().nested.value, 1)

  const current = history.current()
  current.nested.value = 42
  assert.equal(history.current().nested.value, 1)
})

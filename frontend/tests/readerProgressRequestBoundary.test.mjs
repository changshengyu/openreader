import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const readerStoreSource = readFileSync(new URL('../src/stores/reader.js', import.meta.url), 'utf8')

test('regenerates an oversized session progress client ID before requests', async () => {
  const progress = await import('../src/utils/readerProgressPersistence.js')
  assert.equal(typeof progress.readOrCreateReaderClientId, 'function')
  assert.equal(progress.MAX_READER_PROGRESS_CLIENT_ID_BYTES, 128)

  const writes = []
  const storage = {
    getItem: key => key === 'openreader_reader_client_id' ? 'x'.repeat(129) : null,
    setItem: (key, value) => writes.push([key, value]),
  }
  const clientId = progress.readOrCreateReaderClientId(storage, () => 'web-replacement')
  assert.equal(clientId, 'web-replacement')
  assert.deepEqual(writes, [['openreader_reader_client_id', 'web-replacement']])
  assert.match(readerStoreSource, /readOrCreateReaderClientId\(sessionStorage, makeClientId\)/)
})

test('preserves a bounded opaque progress client ID', async () => {
  const progress = await import('../src/utils/readerProgressPersistence.js')
  const existing = 'c'.repeat(128)
  let generated = 0
  let writes = 0
  const storage = {
    getItem: () => existing,
    setItem: () => { writes += 1 },
  }
  const clientId = progress.readOrCreateReaderClientId(storage, () => {
    generated += 1
    return 'web-new'
  })
  assert.equal(clientId, existing)
  assert.equal(generated, 0)
  assert.equal(writes, 0)
})

test('counts UTF-8 bytes and survives restricted session storage', async () => {
  const progress = await import('../src/utils/readerProgressPersistence.js')
  assert.equal(progress.isReaderProgressClientId('界'.repeat(42)), true)
  assert.equal(progress.isReaderProgressClientId('界'.repeat(43)), false)

  const storage = {
    getItem: () => { throw new Error('blocked') },
    setItem: () => { throw new Error('blocked') },
  }
  assert.equal(
    progress.readOrCreateReaderClientId(storage, () => 'web-private-mode'),
    'web-private-mode',
  )
})

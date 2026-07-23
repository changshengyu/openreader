import assert from 'node:assert/strict'
import test from 'node:test'
import { initializeReaderTheme } from '../src/utils/readerSettingsBootstrap.js'

test('authenticated startup applies automatic theme only after remote reader settings settle', async () => {
  const calls = []
  let finishLoad
  const loading = initializeReaderTheme({
    authenticated: true,
    loadSettings: () => new Promise(resolve => {
      calls.push('load')
      finishLoad = resolve
    }),
    applyTheme: () => calls.push('theme'),
  })

  await Promise.resolve()
  assert.deepEqual(calls, ['load'])
  finishLoad()
  await loading
  assert.deepEqual(calls, ['load', 'theme'])
})

test('anonymous startup applies the local automatic theme without loading remote settings', async () => {
  const calls = []
  await initializeReaderTheme({
    authenticated: false,
    loadSettings: () => calls.push('load'),
    applyTheme: () => calls.push('theme'),
  })
  assert.deepEqual(calls, ['theme'])
})

test('theme application remains available when remote settings cannot be loaded', async () => {
  const calls = []
  await initializeReaderTheme({
    authenticated: true,
    loadSettings: async () => {
      calls.push('load')
      throw new Error('offline')
    },
    applyTheme: () => calls.push('theme'),
  })
  assert.deepEqual(calls, ['load', 'theme'])
})

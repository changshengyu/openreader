import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const readerSource = readFileSync(
  new URL('../src/views/Reader.vue', import.meta.url),
  'utf8',
)

test('mobile Reader settings binds one mode-change handler', () => {
  const mobileSettings = readerSource.match(
    /<ReaderMobileWorkspacePanel[\s\S]*?<ReaderSettingsPanel[\s\S]*?\/>[\s\S]*?<\/ReaderMobileWorkspacePanel>/,
  )?.[0] || ''
  const bindings = mobileSettings.match(/@mode-change="onModeChange"/g) || []

  assert.notEqual(mobileSettings, '', 'mobile settings panel must remain in the Reader scene')
  assert.equal(bindings.length, 1, 'one settings gesture must trigger one Reader mode transition')
})

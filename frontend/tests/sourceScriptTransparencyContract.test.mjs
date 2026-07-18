import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const sourceManager = readFileSync(resolve(__dirname, '../src/components/workspace/SourceManager.vue'), 'utf8')

test('source import preview and editor disclose unsupported runtime capabilities', () => {
  assert.match(sourceManager, /importSourceCompatibilityHint\(source\)/, 'each import row must expose a safe compatibility reason')
  assert.match(sourceManager, /class="source-compatibility-warning"/, 'the source editor must keep a visible compatibility warning')
  assert.match(sourceManager, /editorCompatibility/, 'the editor warning must react to the current unsaved form and rules')
  assert.match(sourceManager, /配置会保留/, 'the warning must explain that unsupported fields round-trip instead of being deleted')
  assert.match(sourceManager, /当前服务不会执行/, 'the warning must explain the runtime boundary before save/use')
})

test('source debug translates structured unsupported errors without hiding safe JSON', () => {
  assert.match(sourceManager, /debugCompatibilityMessage/, 'debug must derive a readable compatibility message from code/stage')
  assert.match(sourceManager, /source_rule_unsupported/, 'debug must recognize the backend unsupported-rule code')
  assert.match(sourceManager, /class="debug-compatibility-warning"/, 'debug must render the readable compatibility result')
  assert.match(sourceManager, /debugResultText/, 'the existing structured safe JSON must remain visible')
})

import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const sourceAPIPath = resolve(__dirname, '../src/api/sources.js')
const sourceManagerPath = resolve(__dirname, '../src/components/workspace/SourceManager.vue')

test('uses a dedicated cached-invalid-source API instead of a live health probe on entry', () => {
  const sourceAPI = readFileSync(sourceAPIPath, 'utf8')
  const sourceManager = readFileSync(sourceManagerPath, 'utf8')

  assert.match(sourceAPI, /export function listInvalidSources\(\)\s*\{\s*return api\.get\('\/sources\/invalid'\)/)
  assert.match(sourceManager, /listInvalidSources,/)
  assert.match(sourceManager, /async function loadInvalidSourceHealth\(\)/)

  const healthIntent = sourceManager.match(/if \(intent === 'health'\) \{([\s\S]*?)\n  \}/)?.[1] || ''
  assert.match(healthIntent, /failedOnly\.value = true/)
  assert.match(healthIntent, /await loadInvalidSourceHealth\(\)/)
  assert.doesNotMatch(healthIntent, /batchTestSources\(|checkInvalidSources\(/)
})

test('maps persisted failed rows into the existing failed-only source state and resets it on close', () => {
  const sourceManager = readFileSync(sourceManagerPath, 'utf8')

  assert.match(sourceManager, /health\.value\[item\.id\] = \{[\s\S]*?ok: false/)
  assert.match(sourceManager, /message: item\.errorMessage \|\| '请求书源失败'/)
  assert.match(sourceManager, /health\.value = \{\}/)
  assert.match(sourceManager, /failedOnly\.value = false/)
})

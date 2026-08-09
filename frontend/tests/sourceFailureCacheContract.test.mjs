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
  assert.match(sourceManager, /async function loadInvalidSourceHealth\(parentOperation = null\)/)

  const handleOpen = sourceManager.match(/async function handleOpen\(\) \{([\s\S]*?)\n\}/)?.[1] || ''
  assert.match(handleOpen, /await loadInvalidSourceHealth\(operation\)/)
  assert.doesNotMatch(handleOpen, /batchTestSources\(|checkInvalidSources\(/)
})

test('maps persisted failed rows into the failure table and resets only upstream close state', () => {
  const sourceManager = readFileSync(sourceManagerPath, 'utf8')

  assert.match(sourceManager, /failureSources\.value = \(Array\.isArray\(data\) \? data : \[\]\)\.map/)
  assert.match(sourceManager, /errorMessage: visibleFailureCategory\(source\.errorMessage\)/)
  const close = sourceManager.match(/function handleClosed\(\) \{([\s\S]*?)\n\}/)?.[1] || ''
  assert.match(close, /selectedGroup\.value = ''/)
  assert.doesNotMatch(close, /failureSources\.value = \[\]/)
})

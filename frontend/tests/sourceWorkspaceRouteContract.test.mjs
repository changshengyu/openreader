import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const routerPath = resolve(__dirname, '../src/router/index.js')
const layoutPath = resolve(__dirname, '../src/layouts/AppLayout.vue')

test('keeps legacy source URLs as root-workspace overlay intent redirects', () => {
  const router = readFileSync(routerPath, 'utf8')

  assert.doesNotMatch(router, /const Sources\s*=\s*\(\)\s*=>/)
  assert.match(router, /function sourceOverlayIntentFromLegacy\(to\)/)
  assert.match(router, /path:\s*'\/sources',[\s\S]*?redirect:\s*to\s*=>[\s\S]*?overlay:\s*'sources'/)
  assert.match(router, /sourceAction:\s*sourceOverlayIntentFromLegacy\(to\)/)
  assert.match(router, /to\.query\.panel === 'remote'/)
  assert.match(router, /\['import', 'health', 'debug'\]\.includes\(to\.query\.action\)/)
})

test('opens every Index source action in the shared overlay instead of a route page', () => {
  const layout = readFileSync(layoutPath, 'utf8')

  assert.match(layout, /\{ key: 'sources',[\s\S]*?action:\s*\(\) => overlay\.openSourceManage\('manage'\)/)
  assert.match(layout, /\{ key: 'importSources',[\s\S]*?action:\s*\(\) => overlay\.openSourceManage\('import'\)/)
  assert.match(layout, /\{ key: 'remoteSources',[\s\S]*?action:\s*\(\) => overlay\.openSourceManage\('remote'\)/)
  assert.match(layout, /\{ key: 'sourceHealth',[\s\S]*?action:\s*\(\) => overlay\.openSourceManage\('health'\)/)
  assert.match(layout, /\{ key: 'sourceDebug',[\s\S]*?action:\s*\(\) => overlay\.openSourceManage\('debug'\)/)
  assert.match(layout, /function openRouteSourceManageOverlay\(\)/)
  assert.match(layout, /overlay\.openSourceManage\(route\.query\.sourceAction\)/)
  assert.match(layout, /function clearRouteSourceManageOverlayIntent\(\)/)
})

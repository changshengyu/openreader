import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const routerPath = resolve(__dirname, '../src/router/index.js')
const layoutPath = resolve(__dirname, '../src/layouts/AppLayout.vue')

test('keeps ordinary legacy source URLs as root-workspace overlay intents and translates debug separately', () => {
  const router = readFileSync(routerPath, 'utf8')

  assert.doesNotMatch(router, /const Sources\s*=\s*\(\)\s*=>/)
  assert.match(router, /function sourceOverlayIntentFromLegacy\(to\)/)
  assert.match(router, /path:\s*'\/sources',[\s\S]*?redirect:\s*to\s*=>[\s\S]*?overlay:\s*'sources'/)
  assert.match(router, /sourceAction:\s*sourceOverlayIntentFromLegacy\(to\)/)
  assert.match(router, /to\.query\.panel === 'remote'/)
  assert.match(router, /\['import', 'health'\]\.includes\(to\.query\.action\)/)
  assert.match(router, /action === 'debug'[\s\S]*?name:\s*'source-debug'/)
})

test('opens ordinary Index source actions in the shared overlay and debugger in its upstream workspace', () => {
  const layout = readFileSync(layoutPath, 'utf8')

  assert.match(layout, /\{ key: 'sources',[\s\S]*?action:\s*\(\) => overlay\.openSourceManage\('manage'\)/)
  assert.match(layout, /\{ key: 'importSources',[\s\S]*?action:\s*\(\) => overlay\.openSourceManage\('import'\)/)
  assert.match(layout, /\{ key: 'remoteSources',[\s\S]*?action:\s*\(\) => overlay\.openSourceManage\('remote'\)/)
  assert.match(layout, /\{ key: 'sourceHealth',[\s\S]*?action:\s*\(\) => overlay\.openSourceManage\('health'\)/)
  assert.match(layout, /\{ key: 'sourceDebug',[\s\S]*?action:\s*openSourceDebugWorkspace/)
  assert.match(layout, /window\.open\([^)]*'_target'/s)
  assert.match(layout, /function openRouteSourceManageOverlay\(\)/)
  assert.match(layout, /overlay\.openSourceManage\(route\.query\.sourceAction\)/)
  assert.match(layout, /function clearRouteSourceManageOverlayIntent\(\)/)
})

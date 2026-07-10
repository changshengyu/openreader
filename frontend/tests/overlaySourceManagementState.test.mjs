import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'
import { createPinia, setActivePinia } from 'pinia'
import { useOverlayStore } from '../src/stores/overlay.js'

const __dirname = dirname(fileURLToPath(import.meta.url))
const overlayHostPath = resolve(__dirname, '../src/components/GlobalOverlayHost.vue')
const sourceOverlayPath = resolve(__dirname, '../src/components/overlays/OverlaySources.vue')
const sourceManagerPath = resolve(__dirname, '../src/components/workspace/SourceManager.vue')

function createOverlay() {
  setActivePinia(createPinia())
  return useOverlayStore()
}

test('owns one resettable source-management overlay intent', () => {
  const overlay = createOverlay()

  assert.equal(overlay.sourceManageVisible, false)
  assert.equal(overlay.sourceManageIntent, 'manage')

  overlay.openSourceManage('import')
  assert.equal(overlay.sourceManageVisible, true)
  assert.equal(overlay.sourceManageIntent, 'import')

  overlay.openSourceManage('debug')
  assert.equal(overlay.sourceManageVisible, true)
  assert.equal(overlay.sourceManageIntent, 'debug', 'a second sidebar action replaces the active source-manager intent')

  overlay.closeSourceManage()
  assert.equal(overlay.sourceManageVisible, false)
  assert.equal(overlay.sourceManageIntent, 'manage', 'closing must not leave a stale remote/import/health/debug intent')
})

test('hosts SourceManager as a single overlay body instead of creating a parallel source flow', () => {
  const host = readFileSync(overlayHostPath, 'utf8')
  const overlay = readFileSync(sourceOverlayPath, 'utf8')
  const sourceManager = readFileSync(sourceManagerPath, 'utf8')

  assert.match(host, /OverlaySources/)
  assert.match(overlay, /v-model="overlay\.sourceManageVisible"/)
  assert.match(overlay, /<SourceManager\s+embedded/)
  assert.match(overlay, /:intent="overlay\.sourceManageIntent"/)
  assert.match(sourceManager, /embedded:\s*\{ type: Boolean, default: false \}/)
  assert.match(sourceManager, /intent:\s*\{ type: String, default: 'manage' \}/)
})

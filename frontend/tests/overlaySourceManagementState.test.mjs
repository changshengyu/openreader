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
  assert.equal(overlay.sourceManageIntent, 'manage', 'the standalone debugger must not become a source-manager overlay intent')

  overlay.closeSourceManage()
  assert.equal(overlay.sourceManageVisible, false)
  assert.equal(overlay.sourceManageIntent, 'manage', 'closing must not leave a stale remote/import/health intent')
})

test('hosts ordinary source management as one overlay while debugger remains a separate upstream workspace', () => {
  const host = readFileSync(overlayHostPath, 'utf8')
  const overlay = readFileSync(sourceOverlayPath, 'utf8')
  const sourceManager = readFileSync(sourceManagerPath, 'utf8')

  assert.match(host, /OverlaySources/)
  assert.match(overlay, /<SourceManager/)
  assert.match(overlay, /:visible="overlay\.sourceManageVisible && isManagerIntent"/)
  assert.match(overlay, /:failure-mode="overlay\.sourceManageIntent === 'health'"/)
  assert.match(overlay, /<SourceTransferOverlay/)
  assert.match(sourceManager, /visible:\s*\{ type: Boolean, default: false \}/)
  assert.match(sourceManager, /failureMode:\s*\{ type: Boolean, default: false \}/)
  assert.doesNotMatch(sourceManager, /title="书源调试"|showDebug|debugKeyword|testSourceChapter|testSourceContent/, 'the retired three-probe dialog must not remain in SourceManager')
})

test('opens the upstream-style failure view without starting a live test', () => {
  const sourceManager = readFileSync(sourceManagerPath, 'utf8')
  const handleOpen = sourceManager.match(/async function handleOpen\(\) \{([\s\S]*?)\n\}/)?.[1] || ''

  assert.match(sourceManager, /const isFailureMode = computed\(\(\) => props\.failureMode\)/)
  assert.match(handleOpen, /if \(isFailureMode\.value\) await loadInvalidSourceHealth\(operation\)/)
  assert.doesNotMatch(handleOpen, /checkInvalidSources\(/)
  assert.match(
    sourceManager,
    /const sourceRows = \[\.\.\.sources\.value\]/,
    'an explicit test from an empty failure view must still test the complete source set',
  )
})

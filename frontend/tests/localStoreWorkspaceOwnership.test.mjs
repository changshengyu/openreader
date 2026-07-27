import assert from 'node:assert/strict'
import { existsSync, readFileSync } from 'node:fs'
import test from 'node:test'

const managerUrl = new URL('../src/components/workspace/LocalStoreManager.vue', import.meta.url)
const legacyViewUrl = new URL('../src/views/LocalStore.vue', import.meta.url)
const overlaySource = readFileSync(new URL('../src/components/overlays/OverlayLocalStore.vue', import.meta.url), 'utf8')

test('owns LocalStore through one workspace manager body', () => {
  assert.equal(existsSync(managerUrl), true, 'the workspace-owned LocalStore manager must exist')
  assert.equal(existsSync(legacyViewUrl), false, 'the unreachable standalone LocalStore view must be removed')

  const managerSource = readFileSync(managerUrl, 'utf8')
  assert.doesNotMatch(managerSource, /defineProps\(\s*\{[\s\S]*?\bembedded\b/, 'the manager must not retain an embedded/page dual-shape prop')
  assert.doesNotMatch(managerSource, /\bstore-head\b|<h1[^>]*>本地书仓<\/h1>|<el-dialog/, 'the manager body must not own a competing page or dialog shell')
})

test('keeps the sole LocalStore dialog in the root overlay', () => {
  assert.match(overlaySource, /LocalStoreManager/, 'the root overlay must mount the workspace manager')
  assert.match(overlaySource, /workspace\/LocalStoreManager\.vue/, 'the root overlay must import the workspace-owned manager')
  assert.doesNotMatch(overlaySource, /views\/LocalStore\.vue/, 'the root overlay must not import a legacy view')
  assert.doesNotMatch(overlaySource, /<LocalStore\s+embedded/, 'the root overlay must not retain the obsolete embedded protocol')
  assert.match(overlaySource, /title="书仓文件管理"/, 'the sole dialog retains the fixed upstream label')
  assert.match(overlaySource, /:fullscreen="isMobile"/, 'the sole dialog remains fullscreen on compact screens')
  assert.match(overlaySource, /destroy-on-close/, 'closing the manager must reset its workspace state')
})

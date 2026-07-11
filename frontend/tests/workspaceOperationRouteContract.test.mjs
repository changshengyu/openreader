import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const routerSource = readFileSync(new URL('../src/router/index.js', import.meta.url), 'utf8')
const layoutSource = readFileSync(new URL('../src/layouts/AppLayout.vue', import.meta.url), 'utf8')
const overlaySource = readFileSync(new URL('../src/stores/overlay.js', import.meta.url), 'utf8')
const hostSource = readFileSync(new URL('../src/components/GlobalOverlayHost.vue', import.meta.url), 'utf8')
const settingsSource = readFileSync(new URL('../src/views/Settings.vue', import.meta.url), 'utf8')

test('keeps LocalStore and every legacy Settings panel as root-workspace overlay intents', () => {
  assert.match(routerSource, /function\s+workspaceOverlayIntentFromLegacy\s*\(/, 'router must centralize legacy local-store/settings intent normalization')
  assert.match(routerSource, /path:\s*'\/local-store'[\s\S]*?redirect:\s*to\s*=>\s*workspaceOverlayIntentFromLegacy\(to,\s*'local-store'\)/, 'local-store must be a compatibility redirect, not a product page')
  assert.match(routerSource, /path:\s*'\/settings'[\s\S]*?redirect:\s*to\s*=>\s*workspaceOverlayIntentFromLegacy\(to,\s*'settings'\)/, 'settings must be a compatibility redirect, not a product page')
  for (const panel of ['account', 'backup', 'cache', 'webdav', 'reader', 'replace', 'rss', 'admin']) {
    assert.match(routerSource, new RegExp(`['"]${panel}['"]`), `legacy settings panel ${panel} must have an explicit intent mapping`)
  }
})

test('hydrates and clears operation intents through shared root overlay state only', () => {
  assert.match(overlaySource, /workspaceSettingsVisible:\s*false/, 'overlay store needs one workspace settings visibility state')
  assert.match(overlaySource, /workspaceSettingsPanel:\s*'account'/, 'workspace settings must default to account')
  assert.match(overlaySource, /openWorkspaceSettings\(/, 'workspace settings need a shared open action')
  assert.match(layoutSource, /openRouteWorkspaceOperationOverlay\(/, 'AppLayout must hydrate root operation query intents')
  assert.match(layoutSource, /clearRouteWorkspaceOperationOverlayIntent\(/, 'AppLayout must clear only closed operation intents')
  assert.match(layoutSource, /overlay\.openLocalStore\(/, 'local-store intent must reuse the existing overlay controller')
  assert.match(layoutSource, /overlay\.openWebDAV\(/, 'WebDAV intent must reuse the existing overlay controller')
  assert.match(layoutSource, /overlay\.openBackup\(/, 'backup intent must reuse the existing overlay controller')
  assert.match(layoutSource, /overlay\.openWorkspaceSettings\(/, 'account/cache/reader intents must use one settings overlay')
})

test('keeps settings as one workspace body while operation panels use their canonical overlays', () => {
  assert.match(hostSource, /<OverlayWorkspaceSettings/, 'the shared overlay host must mount workspace settings once')
  for (const panel of ['账户', '缓存', '阅读']) {
    assert.match(settingsSource, new RegExp(`label="${panel}"`), `workspace settings must retain ${panel}`)
  }
  assert.doesNotMatch(settingsSource, /label="备份"|label="WebDAV"|label="替换规则"|label="RSS"|label="用户管理"/, 'non-workspace operation panels must not keep a second Settings implementation')
  assert.doesNotMatch(settingsSource, /from '\.\.\/api\/backup'|RSSManager|WebDAVBrowser/, 'backup, RSS, and WebDAV must stay in their canonical overlays')
})

test('shows the manager-only user workspace entry only to administrators', () => {
  assert.match(layoutSource, /userStore\.profile\?\.role\s*===\s*'admin'/, 'the upstream manager-mode entry must be derived from the current role')
  assert.match(layoutSource, /key:\s*'userManage'/, 'the admin entry remains available to administrators')
})

import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const routerSource = readFileSync(new URL('../src/router/index.js', import.meta.url), 'utf8')
const layoutSource = readFileSync(new URL('../src/layouts/AppLayout.vue', import.meta.url), 'utf8')
const overlaySource = readFileSync(new URL('../src/stores/overlay.js', import.meta.url), 'utf8')
const hostSource = readFileSync(new URL('../src/components/GlobalOverlayHost.vue', import.meta.url), 'utf8')
const settingsSource = readFileSync(new URL('../src/views/Settings.vue', import.meta.url), 'utf8')
const localStoreOverlaySource = readFileSync(new URL('../src/components/overlays/OverlayLocalStore.vue', import.meta.url), 'utf8')
const webdavOverlaySource = readFileSync(new URL('../src/components/overlays/OverlayWebDAV.vue', import.meta.url), 'utf8')
const backupOverlaySource = readFileSync(new URL('../src/components/overlays/OverlayBackups.vue', import.meta.url), 'utf8')
const replaceOverlaySource = readFileSync(new URL('../src/components/overlays/OverlayReplaceRules.vue', import.meta.url), 'utf8')
const userOverlaySource = readFileSync(new URL('../src/components/overlays/OverlayUserManagement.vue', import.meta.url), 'utf8')

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

test('keeps upstream-style file operations in root dialogs instead of side drawers', () => {
  for (const [name, source, state] of [
    ['LocalStore', localStoreOverlaySource, 'localStoreVisible'],
    ['WebDAV', webdavOverlaySource, 'webdavVisible'],
    ['backup', backupOverlaySource, 'backupVisible'],
  ]) {
    assert.match(source, /<el-dialog/, `${name} must use the upstream workspace dialog shell`)
    assert.doesNotMatch(source, /<el-drawer/, `${name} must not retain a side/bottom drawer shell`)
    assert.match(source, new RegExp(`v-model="overlay\\.${state}"`), `${name} must retain the shared overlay state`)
    assert.match(source, /:fullscreen="isMobile"/, `${name} must be fullscreen on the compact/mobile interface`)
    assert.match(source, /destroy-on-close/, `${name} must recreate its root state after close`)
  }
  assert.match(hostSource, /<OverlayLocalStore\s+:is-mobile="isMobileOverlay"/, 'LocalStore must receive the shared compact-interface state')
  assert.match(hostSource, /<OverlayWebDAV\s+:is-mobile="isMobileOverlay"/, 'WebDAV must receive the shared compact-interface state')
  assert.match(hostSource, /<OverlayBackups\s+:is-mobile="isMobileOverlay"/, 'backup must receive the shared compact-interface state')
})

test('keeps ReplaceRule and UserManage manager roots in upstream-style dialogs', () => {
  for (const [name, source, state, loader] of [
    ['ReplaceRule', replaceOverlaySource, 'replaceRulesVisible', 'loadReplaceRules'],
    ['UserManage', userOverlaySource, 'userManageVisible', 'loadUsers'],
  ]) {
    assert.match(source, /<el-dialog/, `${name} manager must use the upstream root dialog shell`)
    assert.doesNotMatch(source, /<el-drawer/, `${name} manager must not retain a side/bottom drawer shell`)
    assert.match(source, new RegExp(`v-model="overlay\\.${state}"`), `${name} must retain shared overlay state`)
    assert.match(source, /:fullscreen="isMobile"/, `${name} manager must be fullscreen on the compact interface`)
    assert.match(source, /destroy-on-close/, `${name} manager must reset its closed surface`)
    assert.match(source, new RegExp(`@open="${loader}"`), `${name} must freshly load when its root dialog opens`)
    assert.match(source, /@closed="resetManager"/, `${name} must clear only manager state when it closes`)
  }
  assert.match(hostSource, /<OverlayReplaceRules\s+:is-mobile="isMobileOverlay"/, 'ReplaceRule must receive compact-interface state without drawer dimensions')
  assert.match(hostSource, /<OverlayUserManagement\s+:is-mobile="isMobileOverlay"/, 'UserManage must receive compact-interface state without drawer dimensions')
})

import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const routerSource = readFileSync(new URL('../src/router/index.js', import.meta.url), 'utf8')
const layoutSource = readFileSync(new URL('../src/layouts/AppLayout.vue', import.meta.url), 'utf8')
const hostSource = readFileSync(new URL('../src/components/GlobalOverlayHost.vue', import.meta.url), 'utf8')
const localStoreOverlaySource = readFileSync(new URL('../src/components/overlays/OverlayLocalStore.vue', import.meta.url), 'utf8')
const webdavOverlaySource = readFileSync(new URL('../src/components/overlays/OverlayWebDAV.vue', import.meta.url), 'utf8')
const replaceOverlaySource = readFileSync(new URL('../src/components/overlays/OverlayReplaceRules.vue', import.meta.url), 'utf8')
const userOverlaySource = readFileSync(new URL('../src/components/overlays/OverlayUserManagement.vue', import.meta.url), 'utf8')

test('keeps LocalStore and overlay-backed legacy Settings panels as root-workspace intents', () => {
  assert.match(routerSource, /function\s+workspaceOverlayIntentFromLegacy\s*\(/, 'router must centralize legacy local-store/settings intent normalization')
  assert.match(routerSource, /path:\s*'\/local-store'[\s\S]*?redirect:\s*to\s*=>\s*workspaceOverlayIntentFromLegacy\(to,\s*'local-store'\)/, 'local-store must be a compatibility redirect, not a product page')
  assert.match(routerSource, /path:\s*'\/settings'[\s\S]*?redirect:\s*to\s*=>\s*workspaceOverlayIntentFromLegacy\(to,\s*'settings'\)/, 'settings must be a compatibility redirect, not a product page')
  for (const panel of ['account', 'backup', 'cache', 'webdav', 'reader', 'replace', 'rss', 'admin']) {
    assert.match(routerSource, new RegExp(`['"]${panel}['"]`), `legacy settings panel ${panel} must have an explicit intent mapping`)
  }
  assert.match(routerSource, /workspaceFocus/, 'account and cache compatibility links must focus their persistent sidebar sections')
  assert.match(routerSource, /workspaceNotice/, 'reader compatibility links must explicitly explain that settings moved into Reader')
  assert.match(routerSource, /backup:\s*'webdav'/, 'legacy backup management links must open the sole WebDAV manager')
  assert.doesNotMatch(routerSource, /workspace-settings/, 'legacy Settings links must not recreate the retired generic settings drawer')
})

test('hydrates only real root operation overlays and consumes sidebar/Reader compatibility intents', () => {
  assert.match(layoutSource, /openRouteWorkspaceOperationOverlay\(/, 'AppLayout must hydrate root operation query intents')
  assert.match(layoutSource, /clearRouteWorkspaceOperationOverlayIntent\(/, 'AppLayout must clear only closed operation intents')
  assert.match(layoutSource, /overlay\.openLocalStore\(/, 'local-store intent must reuse the existing overlay controller')
  assert.match(layoutSource, /overlay\.openWebDAV\(/, 'WebDAV intent must reuse the existing overlay controller')
  assert.doesNotMatch(layoutSource, /overlay\.openBackup\(/, 'upstream direct backup must not open a second manager')
  assert.match(layoutSource, /action:\s*runWorkspaceBackup/, 'ordinary backup must be a direct confirmed workspace action')
  assert.match(layoutSource, /action:\s*runWorkspacePortableBackup/, 'the portable extension must remain a separately named direct action')
  assert.match(layoutSource, /consumeWorkspaceFocusIntent\(/, 'AppLayout must reveal the persistent account/cache sidebar section for old links')
  assert.match(layoutSource, /consumeWorkspaceNoticeIntent\(/, 'AppLayout must surface the Reader-settings migration notice for old links')
  assert.match(layoutSource, /backupUserConfig\(/, 'user configuration backup must be a dedicated settings flush, not a full backup dialog')
  assert.match(layoutSource, /reader\.saveReaderSettings\(\{\s*force:\s*true\s*\}\)/, 'explicit backup must force only the current reader snapshot')
  assert.match(layoutSource, /preferences\.savePreference\('shelf',\s*\{\s*force:\s*true\s*\}\)/, 'explicit backup must force the shelf snapshot')
  assert.match(layoutSource, /preferences\.savePreference\('search',\s*\{\s*force:\s*true\s*\}\)/, 'explicit backup must force the search snapshot')
  assert.match(layoutSource, /reader\.loadReaderSettings\(\{\s*createIfMissing:\s*false\s*\}\)/, 'explicit sync must not create a missing reader backup')
  assert.match(layoutSource, /preferences\.loadPreferences\(\{\s*createIfMissing:\s*false\s*\}\)/, 'explicit sync must not create missing preference backups')
  assert.doesNotMatch(layoutSource, /openWorkspaceSettings\(/, 'sidebar account/cache actions must not open a global settings surface')
})

test('keeps configuration in the workspace sidebar and reader settings in Reader only', () => {
  assert.doesNotMatch(hostSource, /<OverlayWorkspaceSettings/, 'the shared overlay host must not mount a generic settings drawer')
  assert.doesNotMatch(hostSource, /OverlayWorkspaceSettings/, 'the retired generic settings overlay must have no host import')
  assert.match(layoutSource, /key:\s*'account'/, 'the persistent user-space section must remain addressable')
  assert.match(layoutSource, /key:\s*'cache'/, 'the persistent cache section must remain addressable')
  assert.match(layoutSource, /syncUserConfig\(/, 'the sidebar must retain configuration restore')
  assert.doesNotMatch(layoutSource, /global-workspace-settings-drawer/, 'AppLayout must not retain Drawer-specific ownership')
})

test('shows the manager-only user workspace entry only to administrators', () => {
  assert.match(layoutSource, /userStore\.profile\?\.role\s*===\s*'admin'/, 'the upstream manager-mode entry must be derived from the current role')
  assert.match(layoutSource, /key:\s*'userManage'/, 'the admin entry remains available to administrators')
})

test('keeps LocalStore and WebDAV workspace entries independently permission-scoped', () => {
  assert.match(layoutSource, /const canAccessLocalStore = computed\(/, 'LocalStore visibility must derive from its own permission')
  assert.match(layoutSource, /const canAccessWebDAV = computed\(/, 'WebDAV visibility must derive from its own permission')
  assert.match(layoutSource, /typeof explicit === 'boolean' \? explicit : canAccessLocalStore\.value/, 'legacy nullable WebDAV permission must fall back to LocalStore only until explicitly changed')
  assert.match(layoutSource, /canAccessLocalStore\.value[\s\S]*?key: 'localStore'/, 'LocalStore menu entry must not be shown after its permission is revoked')
  assert.match(layoutSource, /canAccessWebDAV\.value[\s\S]*?key: 'webdav'/, 'WebDAV/backup section must not be shown after WebDAV permission is revoked')
})

test('keeps upstream-style file operations in one root manager instead of a duplicate backup dialog', () => {
  for (const [name, source, state] of [
    ['LocalStore', localStoreOverlaySource, 'localStoreVisible'],
    ['WebDAV', webdavOverlaySource, 'webdavVisible'],
  ]) {
    assert.match(source, /<el-dialog/, `${name} must use the upstream workspace dialog shell`)
    assert.doesNotMatch(source, /<el-drawer/, `${name} must not retain a side/bottom drawer shell`)
    assert.match(source, new RegExp(`v-model="overlay\\.${state}"`), `${name} must retain the shared overlay state`)
    assert.match(source, /:fullscreen="isMobile"/, `${name} must be fullscreen on the compact/mobile interface`)
    assert.match(source, /destroy-on-close/, `${name} must recreate its root state after close`)
  }
  assert.match(hostSource, /<OverlayLocalStore\s+:is-mobile="isMobileOverlay"/, 'LocalStore must receive the shared compact-interface state')
  assert.match(hostSource, /<OverlayWebDAV\s+:is-mobile="isMobileOverlay"/, 'WebDAV must receive the shared compact-interface state')
  assert.doesNotMatch(hostSource, /OverlayBackups/, 'the shared host must not mount a second backup manager')
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

test('keeps UserManage protected rows and metadata aligned with the upstream manager contract', () => {
  assert.doesNotMatch(userOverlaySource, /<el-select v-model="userDraft\.role"/, 'manager-created users must not expose an administrator role selector')
  assert.match(userOverlaySource, /prop="lastActiveAt" label="最近活跃"/, 'manager must display upstream-equivalent activity metadata')
  assert.match(userOverlaySource, /prop="createdAt" label="注册时间"/, 'manager must display upstream-equivalent registration metadata')
  assert.match(userOverlaySource, /isUserMutable\(row\)/, 'protected rows must gate mutable permission and password controls')
  assert.match(userOverlaySource, /formatUserTime\(/, 'manager must use one deterministic time/empty-state formatter')
  assert.match(userOverlaySource, /canAccessWebdav/, 'manager must expose the upstream WebDAV permission separately from LocalStore')
  assert.match(userOverlaySource, /active-text="WebDAV"/, 'the WebDAV control must remain visible in desktop, mobile, and create-user flows')
  assert.doesNotMatch(userOverlaySource, /清理不活跃用户/, 'the non-upstream destructive cleanup entry must not be exposed in the manager UI')
})

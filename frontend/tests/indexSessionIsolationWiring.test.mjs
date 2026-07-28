import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

function source(path) {
  return readFileSync(new URL(path, import.meta.url), 'utf8')
}

const app = source('../src/App.vue')
const authDialog = source('../src/components/AuthDialog.vue')
const login = source('../src/views/Login.vue')
const user = source('../src/stores/user.js')
const search = source('../src/views/Search.vue')
const discover = source('../src/views/Discover.vue')
const explorePopover = source('../src/components/workspace/ExploreWorkspacePopover.vue')
const sidebarSearch = source('../src/composables/useAppSidebarSearch.js')
const layout = source('../src/layouts/AppLayout.vue')

test('the authenticated shell stays blocked until reauthentication routing is settled', () => {
  assert.match(app, /v-else-if="isLoggedIn && !authenticatedSessionBlocked"/)
  assert.match(app, /v-else-if="isLoggedIn" class="workspace-auth-blocked"/)
  assert.match(app, /authenticatedSessionBlocked/)
})

test('the login route stays mounted until its success callback settles routing', () => {
  const loginRouteIndex = app.indexOf('<router-view v-if="isLoginRoute" />')
  const authenticatedReaderIndex = app.indexOf('<template v-else-if="isReader && isLoggedIn && !authenticatedSessionBlocked">')
  assert.notEqual(loginRouteIndex, -1)
  assert.notEqual(authenticatedReaderIndex, -1)
  assert(loginRouteIndex < authenticatedReaderIndex)
  assert.doesNotMatch(app, /<router-view v-else-if="isLoginRoute"\s*\/>/)
})

test('both dialog and page login settle an account switch before unblocking the shell', () => {
  assert.match(authDialog, /if \(!result\.sameAuthenticatedScope\)[\s\S]*?router\.replace\(\{ name: 'home' \}\)/)
  assert.doesNotMatch(authDialog, /\['reader', 'remote-reader'\]\.includes\(route\.name\)[\s\S]*?!result\.sameAuthenticatedScope/)
  assert.match(login, /async function handleSuccess\(result = \{\}\)/)
  assert.match(login, /result\.previousScope[\s\S]*?!result\.sameAuthenticatedScope[\s\S]*?router\.replace\(\{ name: 'home' \}\)/)
  assert.match(user, /const workspace = useIndexWorkspaceStore\(\)[\s\S]*?workspace\.suspendSessionState\(\)/)
  assert.match(user, /workspace\.resetSessionState\(\)/)
  assert.match(user, /sameAuthenticatedScope[\s\S]*?resumeSuspendedSession\(\)[\s\S]*?discardSuspendedSession\(\)/)
})

test('Search guards source initialization, local search, import, and temporary Reader handoff by authenticated operation', () => {
  assert.match(search, /createAuthenticatedOperationGuard/)
  assert.match(search, /searchSessionOperations/)
  assert.match(search, /async function loadSources\(\)[\s\S]*?canCommit/)
  assert.match(search, /async function searchLocalBooks\(\)[\s\S]*?captureWorkspaceRequest[\s\S]*?canCommit/)
  assert.match(search, /async function importLocalPaths\(paths\)[\s\S]*?canCommit/)
  assert.match(search, /async function openRemoteReader\(item\)[\s\S]*?canCommit/)
  assert.match(search, /onBeforeUnmount\(\(\) => \{[\s\S]*?searchSessionOperations\.reset\(\)/)
})

test('Explore chooser/results and sidebar source hydration retire account-stale callbacks', () => {
  assert.match(explorePopover, /createAuthenticatedOperationGuard/)
  assert.match(explorePopover, /captureWorkspaceSession/)
  assert.match(explorePopover, /onBeforeUnmount/)
  assert.match(explorePopover, /requestGate\.invalidate\(\)/)
  assert.match(discover, /captureWorkspaceRequest/)
  assert.match(sidebarSearch, /createAuthenticatedOperationGuard/)
  assert.match(sidebarSearch, /function dispose\(\)/)
  assert.match(layout, /disposeSidebarSearch/)
})

test('route BookInfo hydration cannot reopen a cleared account overlay', () => {
  assert.match(layout, /routeBookInfoOperations\s*=\s*useAuthenticatedOperationGuard\(\)/)
  assert.match(layout, /async function openRouteBookInfoOverlay\(\)[\s\S]*?routeBookInfoOperations\.begin/)
  assert.match(layout, /routeBookInfoOperations\.canCommit/)
  assert.match(layout, /routeBookInfoOperations\.reset\(\)/)
})

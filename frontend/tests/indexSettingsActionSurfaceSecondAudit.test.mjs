import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import { useIndexWorkspaceRefresh } from '../src/composables/useIndexWorkspaceRefresh.js'

const layout = readFileSync(new URL('../src/layouts/AppLayout.vue', import.meta.url), 'utf8')
const home = readFileSync(new URL('../src/views/Home.vue', import.meta.url), 'utf8')

function deferred() {
  let resolve
  let reject
  const promise = new Promise((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

function createRefreshFixture(overrides = {}) {
  const calls = []
  const identity = { scope: 'user:1', token: 'token-1' }
  const controller = useIndexWorkspaceRefresh({
    getIdentity: () => ({ ...identity }),
    refreshShelf: async () => calls.push('shelf'),
    refreshSources: async () => calls.push('sources'),
    refreshPreferences: async () => calls.push('preferences'),
    refreshReaderSettings: async () => calls.push('reader-settings'),
    refreshOverlays: async () => calls.push('overlays'),
    refreshCacheStats: async () => calls.push('cache-stats'),
    onSuccess: message => calls.push(['success', message]),
    onWarning: message => calls.push(['warning', message]),
    onError: (error, fallback) => calls.push(['error', error?.message, fallback]),
    ...overrides,
  })
  return {
    calls,
    controller,
    setIdentity(value) {
      Object.assign(identity, value)
    },
  }
}

function assertLabelsInOrder(source, labels) {
  let previous = -1
  for (const label of labels) {
    const position = source.indexOf(label)
    assert(position >= 0, `missing ${label}`)
    assert(position > previous, `${label} is out of order`)
    previous = position
  }
}

test('Index sidebar keeps the fixed-upstream action ownership and order', () => {
  const backendSection = layout.slice(
    layout.indexOf("key: 'backend'"),
    layout.indexOf("key: 'sources'"),
  )
  assert.match(backendSection, /backendConnected\.value \? '后端在线' : '后端未连接'/)
  assert.match(backendSection, /action:\s*\(\) => refreshHealthInfo\(true\)/)
  assert.doesNotMatch(backendSection, /refreshShelfData/)

  const sourceSection = layout.slice(
    layout.indexOf("key: 'sources'"),
    layout.indexOf("key: 'bookshelf'"),
  )
  const sourceLabels = ['书源管理', '探索书源', '导入书源', '远程书源', '失效书源', '调试书源']
  assertLabelsInOrder(sourceSection, sourceLabels.map(label => `label: '${label}'`))

  const shelfSection = layout.slice(
    layout.indexOf("key: 'bookshelf'"),
    layout.indexOf("key: 'account'"),
  )
  const shelfLabels = ['书籍管理', '分组管理', '导入书籍', '浏览书仓', '刷新缓存']
  assertLabelsInOrder(shelfSection, shelfLabels)
  assert.doesNotMatch(shelfSection, /key:\s*'home'|label:\s*'书架'/)
  assert.match(shelfSection, /key:\s*'refreshWorkspace'.*action:\s*refreshWorkspaceData/s)

  const navSource = layout.slice(layout.indexOf('const navSections'), layout.indexOf('const {\n  quickSearch'))
  assert.doesNotMatch(navSource, /key:\s*'other'|label:\s*'RSS'|label:\s*'替换规则'/)
  assert(navSource.indexOf("key: 'webdav'") < navSource.indexOf("key: 'cache'"))
})

test('Home preserves the upstream ordinary-shelf title action sequence', () => {
  const actionTemplate = home.slice(
    home.indexOf('<div class="title-actions">'),
    home.indexOf('</div>', home.indexOf('<div class="title-actions">')),
  )
  const labels = ['编辑', '刷新', 'RSS', '书海']
  for (let index = 1; index < labels.length; index += 1) {
    assert(actionTemplate.indexOf(labels[index - 1]) < actionTemplate.indexOf(labels[index]))
  }
  assert.doesNotMatch(actionTemplate, /view-switch|网格显示|列表显示/)
})

test('workspace refresh reloads every current-account workspace owner', async () => {
  const fixture = createRefreshFixture()

  assert.equal(await fixture.controller.refresh(), true)
  assert.deepEqual(fixture.calls, [
    'shelf',
    'sources',
    'preferences',
    'reader-settings',
    'overlays',
    'cache-stats',
    ['success', '工作台缓存已刷新'],
  ])
  assert.equal(fixture.controller.loading.value, false)
})

test('workspace refresh keeps successful owners and reports partial failures', async () => {
  const fixture = createRefreshFixture({
    refreshSources: async () => {
      fixture.calls.push('sources')
      throw new Error('source offline')
    },
  })

  assert.equal(await fixture.controller.refresh(), true)
  assert(fixture.calls.includes('shelf'))
  assert(fixture.calls.includes('preferences'))
  assert.deepEqual(fixture.calls.at(-1), ['warning', '工作台已刷新，部分数据刷新失败：书源'])
  assert.equal(fixture.controller.loading.value, false)
})

test('workspace refresh retires stale account results without touching new loading ownership', async () => {
  const firstShelf = deferred()
  const secondShelf = deferred()
  const shelfRequests = [firstShelf, secondShelf]
  const fixture = createRefreshFixture({
    refreshShelf: () => shelfRequests.shift().promise,
  })

  const first = fixture.controller.refresh()
  fixture.setIdentity({ scope: 'user:2', token: 'token-2' })
  const second = fixture.controller.refresh()
  firstShelf.resolve(true)
  await first

  assert.equal(fixture.controller.loading.value, true)
  assert.equal(fixture.calls.some(row => Array.isArray(row)), false)

  secondShelf.resolve(true)
  await second

  assert.equal(fixture.controller.loading.value, false)
  assert.deepEqual(fixture.calls.at(-1), ['success', '工作台缓存已刷新'])
})

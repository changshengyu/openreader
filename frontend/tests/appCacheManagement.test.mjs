import assert from 'node:assert/strict'
import test from 'node:test'
import {
  formatSize,
  useAppCacheManagement,
} from '../src/composables/useAppCacheManagement.js'

function createController(overrides = {}) {
  const calls = []
  const identity = { scope: 'user:1', token: 'token-1' }
  let browserStats = {
    total: { files: 5, size: 3072 },
    groups: {
      bookSourceList: { files: 2, size: 1024 },
      rssSources: { files: 0, size: 0 },
      chapterList: { files: 1, size: 1024 },
      chapterContent: { files: 2, size: 1024 },
    },
  }
  const controller = useAppCacheManagement({
    getServerStats: async () => ({ data: { files: 3, size: 2048 } }),
    getBrowserStats: async () => browserStats,
    clearServerCache: async () => ({
      data: { clearedFiles: 3, clearedSize: 2048 },
    }),
    clearBrowserGroup: async (group, scope) => {
      calls.push(['clear-browser', group, scope])
      browserStats = {
        total: { files: 3, size: 2048 },
        groups: {
          ...browserStats.groups,
          [group]: { files: 0, size: 0 },
        },
      }
      return 2
    },
    confirm: async (...args) => calls.push(['confirm', ...args]),
    getIdentity: () => ({ ...identity }),
    onSuccess: message => calls.push(['success', message]),
    onInfo: message => calls.push(['info', message]),
    onError: (...args) => calls.push(['error', ...args]),
    ...overrides,
  })
  return {
    calls,
    controller,
    setBrowserStats: value => {
      browserStats = value
    },
    setIdentity: value => {
      Object.assign(identity, value)
    },
  }
}

function deferred() {
  let resolve
  let reject
  const promise = new Promise((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

test('loads combined stats and builds visible browser cache actions', async () => {
  const fixture = createController()
  await fixture.controller.loadStats()

  assert.equal(fixture.controller.sectionTitle.value, '本地缓存 5.0 KB')
  assert.equal(
    fixture.controller.clearServerLabel.value,
    '清空服务器缓存 2.0 KB',
  )
  assert.deepEqual(
    fixture.controller.browserNavItems.value.map(item => [
      item.key,
      item.label,
    ]),
    [
      ['clear-bookSourceList', '清空书源缓存 1.0 KB'],
      ['clear-rssSources', '清空RSS源缓存'],
      ['clear-chapterList', '清空章节列表缓存 1.0 KB'],
      ['clear-chapterContent', '清空章节内容缓存 1.0 KB'],
    ],
  )
  assert.equal(fixture.controller.loading.value, false)
})

test('clears server cache and refreshes both stats', async () => {
  const fixture = createController()
  await fixture.controller.clearServer()

  assert.deepEqual(fixture.calls, [
    [
      'confirm',
      '确定清理服务器章节缓存吗？清理后阅读时会重新加载远程章节内容。',
      '清理缓存',
      { type: 'warning' },
    ],
    ['success', '已清理 3 个文件，释放 2.0 KB'],
  ])
  assert.equal(fixture.controller.clearingServer.value, false)
  assert.equal(fixture.controller.serverStats.value.size, 2048)
})

test('clears a populated browser group and reports empty groups', async () => {
  const fixture = createController()
  await fixture.controller.loadStats()
  await fixture.controller.clearBrowser('bookSourceList')
  await fixture.controller.clearBrowser('rssSources')

  assert.deepEqual(fixture.calls, [
    [
      'confirm',
      '确定清理当前浏览器的书源缓存吗？清理后会在需要时重新加载。',
      '清理浏览器缓存',
      { type: 'warning' },
    ],
    ['clear-browser', 'bookSourceList', 'user:1'],
    ['success', '已清理书源缓存 2 项'],
    ['info', 'RSS源缓存为空'],
  ])
  assert.equal(fixture.controller.clearingBrowserGroup.value, '')
})

test('keeps safe empty stats when either stats provider fails', async () => {
  const fixture = createController({
    getServerStats: async () => {
      throw new Error('server unavailable')
    },
    getBrowserStats: async () => {
      throw new Error('browser unavailable')
    },
  })
  await fixture.controller.loadStats()

  assert.deepEqual(fixture.controller.serverStats.value, {})
  assert.deepEqual(fixture.controller.browserStats.value, {
    total: { files: 0, size: 0 },
    groups: {},
  })
  assert.equal(fixture.controller.sectionTitle.value, '本地缓存')
  assert.equal(formatSize(1024 * 1024 * 2), '2.0 MB')
})

test('commits only the newest stats request and keeps its loading state owned', async () => {
  const firstServer = deferred()
  const firstBrowser = deferred()
  const secondServer = deferred()
  const secondBrowser = deferred()
  const serverRequests = [firstServer, secondServer]
  const browserRequests = [firstBrowser, secondBrowser]
  const browserScopes = []
  const fixture = createController({
    getServerStats: () => serverRequests.shift().promise,
    getBrowserStats: scope => {
      browserScopes.push(scope)
      return browserRequests.shift().promise
    },
  })

  const first = fixture.controller.loadStats()
  fixture.setIdentity({ scope: 'user:2', token: 'token-2' })
  const second = fixture.controller.loadStats()
  firstServer.resolve({ data: { files: 1, size: 100 } })
  firstBrowser.resolve({
    total: { files: 1, size: 100 },
    groups: {},
  })
  await first

  assert.equal(fixture.controller.loading.value, true)
  assert.deepEqual(fixture.controller.serverStats.value, {})

  secondServer.resolve({ data: { files: 2, size: 200 } })
  secondBrowser.resolve({
    total: { files: 2, size: 200 },
    groups: {},
  })
  await second

  assert.deepEqual(browserScopes, ['user:1', 'user:2'])
  assert.equal(fixture.controller.serverStats.value.size, 200)
  assert.equal(fixture.controller.browserStats.value.total.size, 200)
  assert.equal(fixture.controller.loading.value, false)
})

test('does not clear browser cache if identity changes while confirmation is open', async () => {
  const confirmation = deferred()
  const cleared = []
  const fixture = createController({
    confirm: () => confirmation.promise,
    clearBrowserGroup: async (...args) => {
      cleared.push(args)
      return 1
    },
  })
  await fixture.controller.loadStats()

  const clearing = fixture.controller.clearBrowser('bookSourceList')
  fixture.setIdentity({ scope: 'user:2', token: 'token-2' })
  confirmation.resolve(true)
  await clearing

  assert.deepEqual(cleared, [])
  assert.equal(fixture.controller.clearingBrowserGroup.value, '')
})

test('freezes clear scope and resetScope retires old account state and busy ownership', async () => {
  const deletion = deferred()
  const cleared = []
  const fixture = createController({
    clearBrowserGroup: async (...args) => {
      cleared.push(args)
      return deletion.promise
    },
  })
  await fixture.controller.loadStats()

  const clearing = fixture.controller.clearBrowser('bookSourceList')
  await Promise.resolve()
  assert.deepEqual(cleared, [['bookSourceList', 'user:1']])
  assert.equal(fixture.controller.clearingBrowserGroup.value, 'bookSourceList')

  fixture.setIdentity({ scope: 'user:2', token: 'token-2' })
  fixture.controller.resetScope()
  assert.equal(fixture.controller.clearingBrowserGroup.value, '')
  assert.deepEqual(fixture.controller.serverStats.value, {})
  assert.equal(fixture.controller.browserStats.value.total.files, 0)

  deletion.resolve(1)
  await clearing
  assert.equal(fixture.calls.some(call => call[0] === 'success'), false)
})

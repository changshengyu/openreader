import assert from 'node:assert/strict'
import test from 'node:test'
import { reactive } from 'vue'
import { useAppSidebarSearch } from '../src/composables/useAppSidebarSearch.js'

function createController(overrides = {}) {
  const calls = []
  const route = reactive({ name: 'home', query: {} })
  const preferences = reactive({
    search: {
      searchType: 'all',
      group: '',
      sourceId: '',
      concurrent: 60,
    },
    setSearchConfig(config) {
      Object.assign(this.search, config)
      calls.push(['config', config])
    },
  })
  const controller = useAppSidebarSearch({
    route,
    preferences,
    router: {
      push: payload => calls.push(['push', payload]),
      replace: payload => calls.push(['replace', payload]),
    },
    listSources: async () => ({
      data: [
        { id: 1, name: '甲源', group: '分组甲', enabled: true },
        { id: 2, name: '乙源', group: '分组甲', enabled: true },
        { id: 3, name: '停用源', group: '分组乙', enabled: false },
      ],
    }),
    cacheFirstRequest: async request => request(),
    networkFirstRequest: async request => request(),
    removeBrowserCache: async key => calls.push(['remove-cache', key]),
    getUserScope: () => 'user-7',
    onWarning: message => calls.push(['warning', message]),
    afterNavigate: () => calls.push(['after-navigate']),
    afterSourcesUpdated: () => calls.push(['after-sources']),
    ...overrides,
  })
  return { calls, controller, preferences, route }
}

test('loads enabled source groups and initializes empty search choices', async () => {
  const fixture = createController()
  await fixture.controller.loadSources()

  assert.deepEqual(
    fixture.controller.enabledSources.value.map(source => source.id),
    [1, 2],
  )
  assert.deepEqual(fixture.controller.sourceGroups.value, [
    { label: '分组甲', value: '分组甲', count: 2 },
  ])
  assert.equal(fixture.preferences.search.group, '分组甲')
  assert.equal(fixture.preferences.search.sourceId, 1)
  assert.equal(fixture.controller.sourceCacheKey(), 'bookSourceList@user-7')
})

test('builds remote and local routes from the active search preference', () => {
  const fixture = createController()
  fixture.controller.quickSearch.value = '  关键字  '
  fixture.preferences.search.searchType = 'group'
  fixture.preferences.search.group = '玄幻'
  fixture.preferences.search.concurrent = 16
  fixture.controller.goSearch()

  assert.deepEqual(fixture.calls, [
    [
      'push',
      {
        name: 'search',
        query: {
          q: '关键字',
          searchType: 'group',
          concurrent: 16,
          group: '玄幻',
        },
      },
    ],
    ['after-navigate'],
  ])

  fixture.calls.length = 0
  fixture.controller.goSearchRoute('local')
  assert.deepEqual(fixture.calls, [
    ['push', { name: 'search', query: { mode: 'local', q: '关键字' } }],
    ['after-navigate'],
  ])
})

test('uses the shared Index workspace callback without replacing the current route scene', () => {
  const fixture = createController({
    onWorkspaceSearch: query => fixture.calls.push(['workspace-search', query]),
  })
  fixture.controller.quickSearch.value = '  工作台搜索  '
  fixture.preferences.search.searchType = 'single'
  fixture.preferences.search.sourceId = 8

  fixture.controller.goSearch()
  fixture.controller.goSearchRoute('local')

  assert.deepEqual(fixture.calls, [
    ['workspace-search', {
      q: '工作台搜索',
      searchType: 'single',
      concurrent: 60,
      sourceId: 8,
    }],
    ['workspace-search', { mode: 'local', q: '工作台搜索' }],
  ])
})

test('warns for a blank primary search and synchronizes route keywords', async () => {
  const fixture = createController()
  fixture.controller.goSearch()
  assert.deepEqual(fixture.calls, [['warning', '请输入关键词进行搜索']])

  fixture.route.name = 'search'
  fixture.route.query = { q: '路由词', mode: 'local' }
  await Promise.resolve()
  assert.equal(fixture.controller.quickSearch.value, '路由词')

  fixture.controller.clearSearchQuery()
  assert.deepEqual(fixture.calls.at(-1), [
    'replace',
    { name: 'search', query: { mode: 'local' } },
  ])

  fixture.route.name = 'settings'
  await Promise.resolve()
  assert.equal(fixture.controller.quickSearch.value, '')
})

test('invalidates the scoped source cache before refreshing dependent stats', async () => {
  const fixture = createController()
  await fixture.controller.handleSourcesUpdated()

  assert.deepEqual(fixture.calls, [
    ['remove-cache', 'bookSourceList@user-7'],
    ['config', { group: '分组甲' }],
    ['config', { sourceId: 1 }],
    ['after-sources'],
  ])
})

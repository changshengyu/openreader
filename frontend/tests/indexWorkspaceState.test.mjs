import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'
import { createPinia, setActivePinia } from 'pinia'
import { useIndexWorkspaceStore } from '../src/stores/indexWorkspace.js'

const __dirname = dirname(fileURLToPath(import.meta.url))
const storePath = resolve(__dirname, '../src/stores/indexWorkspace.js')
const searchViewPath = resolve(__dirname, '../src/views/Search.vue')
const discoverViewPath = resolve(__dirname, '../src/views/Discover.vue')
const homeViewPath = resolve(__dirname, '../src/views/Home.vue')

function createWorkspace() {
  setActivePinia(createPinia())
  return useIndexWorkspaceStore()
}

test('uses one upstream-style result scene for shelf, search, explore, and back-to-shelf', () => {
  const workspace = createWorkspace()

  assert.equal(workspace.mode, 'shelf')
  assert.deepEqual(workspace.resultRows, [])
  assert.deepEqual(workspace.continuation, {
    page: 1,
    lastIndex: -1,
    hasMore: false,
    loading: false,
  })

  workspace.beginSearch({
    keyword: '  雪中悍刀行  ',
    mode: 'remote',
    searchType: 'group',
    group: '玄幻',
    concurrent: 16,
  })
  workspace.replaceResultRows([
    { key: 'source-a:1', title: '雪中悍刀行' },
  ], {
    page: 1,
    lastIndex: 6,
    hasMore: true,
  })
  workspace.rememberResultScroll(318)

  assert.equal(workspace.mode, 'search')
  assert.equal(workspace.searchRevision, 1)
  assert.deepEqual(workspace.search, {
    keyword: '雪中悍刀行',
    mode: 'remote',
    searchType: 'group',
    group: '玄幻',
    sourceId: '',
    concurrent: 16,
  })
  assert.deepEqual(workspace.continuation, {
    page: 1,
    lastIndex: 6,
    hasMore: true,
    loading: false,
  })
  assert.equal(workspace.resultScrollTop, 318)

  workspace.appendResultRows([
    { key: 'source-a:2', title: '剑来' },
  ], {
    page: 2,
    lastIndex: 12,
    hasMore: false,
  })
  assert.deepEqual(workspace.resultRows.map(row => row.title), ['雪中悍刀行', '剑来'])
  assert.deepEqual(workspace.continuation, {
    page: 2,
    lastIndex: 12,
    hasMore: false,
    loading: false,
  })

  workspace.showExploreResults([
    { key: 'source-b:1', title: '诡秘之主' },
  ], {
    sourceId: 7,
    sourceGroup: '推荐',
    url: 'https://source.example/explore',
    name: '热门',
    page: 1,
    hasMore: true,
  })

  assert.equal(workspace.mode, 'explore')
  assert.equal(workspace.exploreRevision, 0, 'applying a result page must not retrigger the Explore entry flow')
  assert.deepEqual(workspace.resultRows.map(row => row.title), ['诡秘之主'])
  assert.deepEqual(workspace.explore, {
    sourceId: 7,
    sourceGroup: '推荐',
    url: 'https://source.example/explore',
    name: '热门',
    sourceName: '',
  })
  assert.deepEqual(workspace.continuation, {
    page: 1,
    lastIndex: -1,
    hasMore: true,
    loading: false,
  })
  assert.equal(workspace.resultScrollTop, 0)

  workspace.backToShelf()

  assert.equal(workspace.mode, 'shelf')
  assert.deepEqual(workspace.resultRows, [])
  assert.deepEqual(workspace.continuation, {
    page: 1,
    lastIndex: -1,
    hasMore: false,
    loading: false,
  })
  assert.equal(workspace.resultScrollTop, 0)
  assert.equal(workspace.search.keyword, '雪中悍刀行', 'returning only clears result state, not the saved search configuration')
})

test('opens Explore as a chooser request and only enters result mode after an entry resolves', () => {
  const workspace = createWorkspace()

  workspace.requestExplore({
    sourceId: 7,
    sourceGroup: '推荐',
    url: 'https://source.example/explore',
    name: '热门',
    sourceName: '示例书源',
  })

  assert.equal(workspace.mode, 'shelf', 'opening Explore must leave the current Index body intact')
  assert.equal(workspace.exploreChooserRevision, 1, 'each Explore trigger must be observable by the long-lived chooser')
  assert.deepEqual(workspace.explore, {
    sourceId: 7,
    sourceGroup: '推荐',
    url: 'https://source.example/explore',
    name: '热门',
    sourceName: '示例书源',
  })

  workspace.showExploreResults([{ key: 'source-b:1', title: '诡秘之主' }], {
    ...workspace.explore,
    page: 1,
    hasMore: true,
  })

  assert.equal(workspace.mode, 'explore', 'only a selected/resolved entry may replace the shelf with Explore results')
  assert.equal(workspace.exploreChooserRevision, 1, 'result updates must not reopen the chooser')
})

test('keeps result pagination and scroll restoration in the workspace state without a route dependency', () => {
  const workspace = createWorkspace()
  workspace.beginSearch({ keyword: '长夜', sourceId: 4, searchType: 'single' })
  workspace.setResultLoading(true)
  workspace.replaceResultRows([{ key: 'one', title: '长夜余火' }], { page: 3, lastIndex: 21, hasMore: true })
  workspace.rememberResultScroll(501.9)

  assert.equal(workspace.continuation.loading, false, 'a completed result replacement clears the loading flag')
  assert.equal(workspace.continuation.page, 3)
  assert.equal(workspace.continuation.lastIndex, 21)
  assert.equal(workspace.resultScrollTop, 501.9)

  workspace.rememberResultScroll(-3)
  assert.equal(workspace.resultScrollTop, 0, 'invalid scroll values must not move the restored list above its origin')

  const source = readFileSync(storePath, 'utf8')
  assert.doesNotMatch(source, /vue-router|router\.push|router\.replace/, 'the shared workspace state must be usable without a route scene transition')
})

test('keeps Search and Explore result cards as root-workspace bodies rather than standalone pages or source choosers', () => {
  const searchView = readFileSync(searchViewPath, 'utf8')
  const discoverView = readFileSync(discoverViewPath, 'utf8')
  const homeView = readFileSync(homeViewPath, 'utf8')

  assert.match(searchView, /useIndexWorkspaceStore/)
  assert.match(searchView, /workspace\.replaceResultRows\(/)
  assert.match(searchView, /workspace\.mode\s*!==\s*'search'/)
  assert.doesNotMatch(searchView, /defineProps\s*\(/, 'Search must not preserve an optional standalone-page prop')
  assert.doesNotMatch(searchView, /!embedded|v-else\s+class="search-head"/, 'Search must not preserve a standalone page branch')
  assert.doesNotMatch(searchView, /route\.query\.(?:mode|q|searchType|group|sourceId|concurrent)/, 'Search must initialize only from the shared workspace intent')
  assert.match(discoverView, /useIndexWorkspaceStore/)
  assert.match(discoverView, /workspace\.appendResultRows\(/)
  assert.doesNotMatch(discoverView, /defineProps\s*\(/, 'Discover must not preserve an optional standalone-page prop')
  assert.doesNotMatch(discoverView, /!embedded|v-else\s+class="discover-head"/, 'Discover must not preserve a standalone page branch')
  assert.doesNotMatch(discoverView, /listExploreSources|source-panel|source-group-tabs|el-collapse-item/, 'Discover result body must not recreate the upstream Explore chooser')
  assert.match(homeView, /useIndexWorkspaceStore/)
  assert.match(homeView, /workspace\.backToShelf\(\)/)
})

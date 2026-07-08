import assert from 'node:assert/strict'
import test from 'node:test'
import { computed, ref } from 'vue'
import { useReaderPanels } from '../src/composables/useReaderPanels.js'

function createController(overrides = {}) {
  const calls = []
  const book = ref({ id: 7, sourceId: 2, title: '测试书' })
  const state = {
    mobileChromeVisible: ref(true),
    settingsVisible: ref(false),
    bookmarkVisible: ref(false),
    searchVisible: ref(false),
    sourceVisible: ref(false),
    cacheVisible: ref(false),
    clickZoneVisible: ref(false),
    customBg: ref(''),
    sliderLineHeight: ref(0),
  }
  const controller = useReaderPanels({
    book,
    bookId: ref(7),
    isRemoteBook: computed(() => Number(book.value?.sourceId || 0) > 0),
    bookProgress: ref(42),
    bookProgressLabel: ref('42.0%'),
    ...state,
    getCustomBgColor: () => '#efe4c5',
    getLineHeight: () => 1.8,
    refreshBrowserCachedChapters: () => calls.push(['refresh-cache']),
    saveProgress: payload => calls.push(['save', payload]),
    navigate: async route => calls.push(['navigate', route]),
    defer: task => {
      calls.push(['defer'])
      task()
    },
    focusContentSearch: () => calls.push(['focus-search']),
    closeBookInfo: () => calls.push(['close-info']),
    openBookInfoOverlay: (...args) => calls.push(['open-info', ...args]),
    openReplaceRulesOverlay: () => calls.push(['replace-rules']),
    openToc: () => calls.push(['open-toc']),
    ensureCategoriesLoaded: async () => calls.push(['load-categories']),
    openBookGroup: (...args) => calls.push(['open-group', ...args]),
    getCategoryName: () => '收藏',
    refreshCatalog: () => calls.push(['refresh-catalog']),
    clearCache: () => calls.push(['clear-cache']),
    ...overrides,
  })
  return { book, calls, controller, state }
}

test('opens reader panels while preserving their existing visibility side effects', () => {
  const fixture = createController()
  fixture.controller.openSettings()
  assert.equal(fixture.state.mobileChromeVisible.value, true)
  assert.equal(fixture.state.settingsVisible.value, true)
  assert.equal(fixture.state.customBg.value, '#efe4c5')
  assert.equal(fixture.state.sliderLineHeight.value, 1.8)

  fixture.controller.openSettings()
  assert.equal(fixture.state.mobileChromeVisible.value, true)
  assert.equal(fixture.state.settingsVisible.value, false)

  fixture.controller.openSettings()
  assert.equal(fixture.state.settingsVisible.value, true)

  fixture.controller.showClickZone()
  assert.equal(fixture.state.settingsVisible.value, false)
  assert.equal(fixture.state.clickZoneVisible.value, true)

  fixture.controller.openCache()
  fixture.controller.openBookmarks()
  fixture.controller.openContentSearch()
  fixture.controller.openReplaceRules()
  assert.equal(fixture.state.mobileChromeVisible.value, true)
  assert.equal(fixture.state.cacheVisible.value, true)
  assert.equal(fixture.state.bookmarkVisible.value, true)
  assert.equal(fixture.state.searchVisible.value, true)
  assert.deepEqual(fixture.calls, [
    ['refresh-cache'],
    ['defer'],
    ['focus-search'],
    ['replace-rules'],
  ])
})

test('saves progress before navigating back to the shelf', async () => {
  const fixture = createController()
  fixture.state.mobileChromeVisible.value = true
  await fixture.controller.goShelf()
  assert.equal(fixture.state.mobileChromeVisible.value, true)
  assert.deepEqual(fixture.calls, [
    ['save', { force: true, background: true }],
    ['navigate', { name: 'home' }],
  ])
})

test('opens plain reader BookInfo without injecting toolbar shortcut actions', () => {
  const fixture = createController()
  fixture.controller.openBookInfo()
  const remoteOptions = fixture.calls[0][2]
  assert.equal(remoteOptions.statusLabel, '阅读中 · 42.0%')
  assert.equal(remoteOptions.statusType, 'success')
  assert.equal(remoteOptions.progress, 42)
  assert.equal('actions' in remoteOptions, false)
  assert.equal(fixture.state.mobileChromeVisible.value, true)

  fixture.calls.length = 0
  fixture.book.value = { id: 7, sourceId: 0, title: '本地书' }
  fixture.controller.openSource()
  fixture.controller.openCache()
  fixture.controller.openBookInfo()
  const localOptions = fixture.calls[0][2]
  assert.equal('actions' in localOptions, false)
  assert.equal(fixture.state.sourceVisible.value, false)
})

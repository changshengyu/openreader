import assert from 'node:assert/strict'
import test from 'node:test'
import { ref } from 'vue'
import { useReaderCatalogActions } from '../src/composables/useReaderCatalogActions.js'

function createController(overrides = {}) {
  const calls = []
  const book = ref({ id: 7, sourceId: 2, title: '旧书名' })
  const chapters = ref([{ id: 1 }])
  const currentIndex = ref(2)
  let overlayBook = { id: 7, title: '旧书名' }
  const controller = useReaderCatalogActions({
    book,
    bookId: ref(7),
    chapters,
    currentIndex,
    canChangeLocalTocRule: ref(true),
    chooseLocalTocRule: async () => '^第.+章$',
    runTocRefreshing: task => task(),
    refreshLocalBook: async (...args) => {
      calls.push(['refresh-local', ...args])
      return {
        book: { id: 7, sourceId: 0, title: '本地新书名' },
        chapterCount: 3,
      }
    },
    refreshRemoteBook: async id => {
      calls.push(['refresh-remote', id])
      return { book: { id: 7, sourceId: 2, title: '远程新书名' } }
    },
    invalidateDataCache: async payload => calls.push(['invalidate', payload]),
    resetChapterCaches: async payload => calls.push(['reset', payload]),
    mergeLoadedBook: incoming => ({ retained: true, ...incoming }),
    upsertBook: row => calls.push(['upsert', row]),
    getOverlayBook: () => overlayBook,
    setOverlayBook: row => {
      overlayBook = row
      calls.push(['overlay', row])
    },
    writeDataCache: async payload => calls.push(['cache', payload]),
    loadChapters: async () => {
      chapters.value = [{ id: 11 }, { id: 12 }, { id: 13 }]
      calls.push(['load-chapters'])
      return chapters.value
    },
    loadChapter: async (...args) => calls.push(['load-chapter', ...args]),
    refreshBrowserCachedChapters: async () => calls.push(['browser-cached']),
    locateCurrentTocChapter: () => calls.push(['locate-toc']),
    getCurrentOffset: () => 88,
    getCurrentChapterPercent: () => 0.45,
    fetchChapters: async id => {
      calls.push(['fetch-chapters', id])
      return [{ id: 21 }, { id: 22 }]
    },
    resetContentSearch: () => calls.push(['reset-search']),
    refreshSourceCandidates: async () => calls.push(['refresh-sources']),
    closeSourceDrawer: () => calls.push(['close-source']),
    notify: (...args) => calls.push(['notify', ...args]),
    onError: (...args) => calls.push(['error', ...args]),
    ...overrides,
  })
  return {
    book,
    calls,
    chapters,
    controller,
    currentIndex,
    getOverlayBook: () => overlayBook,
  }
}

test('does nothing when changing a local TOC rule is cancelled', async () => {
  const fixture = createController({
    chooseLocalTocRule: async () => null,
  })
  await fixture.controller.changeLocalTocRule()
  assert.deepEqual(fixture.calls, [])
})

test('refreshes a local catalog and preserves the existing transaction order', async () => {
  const fixture = createController()
  await fixture.controller.changeLocalTocRule()
  assert.deepEqual(fixture.calls, [
    ['refresh-local', 7, { tocRule: '^第.+章$' }],
    ['invalidate', { chapters: true, book: true }],
    ['reset', { clearBrowser: true }],
    ['upsert', { retained: true, id: 7, sourceId: 0, title: '本地新书名' }],
    ['overlay', { retained: true, id: 7, sourceId: 0, title: '本地新书名' }],
    ['cache', { bookData: { retained: true, id: 7, sourceId: 0, title: '本地新书名' } }],
    ['load-chapters'],
    ['load-chapter', 2, 0, { refresh: true, saveAfterLoad: true }],
    ['browser-cached'],
    ['locate-toc'],
    ['notify', '目录规则已更新，共 3 章'],
  ])
})

test('refreshes a remote catalog while restoring the chapter position', async () => {
  const fixture = createController()
  await fixture.controller.refreshRemoteCatalog()
  assert.deepEqual(fixture.calls, [
    ['refresh-remote', 7],
    ['invalidate', { book: true, chapters: true }],
    ['reset', { clearBrowser: true }],
    ['upsert', { retained: true, id: 7, sourceId: 2, title: '远程新书名' }],
    ['cache', { bookData: { retained: true, id: 7, sourceId: 2, title: '远程新书名' } }],
    ['load-chapters'],
    ['load-chapter', 2, 88, { restorePercent: 0.45, refresh: true }],
    ['overlay', { retained: true, id: 7, sourceId: 2, title: '远程新书名' }],
    ['notify', '目录已刷新', 1400],
  ])
})

test('applies a source change with the new catalog before refreshing candidates', async () => {
  const fixture = createController()
  await fixture.controller.applySourceChange({
    book: { id: 7, sourceId: 9, title: '换源后' },
    previousBook: { id: 7, sourceId: 2 },
  })
  assert.deepEqual(fixture.chapters.value, [{ id: 21 }, { id: 22 }])
  assert.equal(fixture.currentIndex.value, 1)
  assert.deepEqual(fixture.calls, [
    ['invalidate', { book: true, chapters: true }],
    ['reset', { clearBrowser: true, book: { id: 7, sourceId: 2 } }],
    ['upsert', { retained: true, id: 7, sourceId: 9, title: '换源后' }],
    ['fetch-chapters', 7],
    ['cache', {
      bookData: { retained: true, id: 7, sourceId: 9, title: '换源后' },
      chaptersData: [{ id: 21 }, { id: 22 }],
    }],
    ['load-chapter', 1, 0],
    ['reset-search'],
    ['refresh-sources'],
    ['close-source'],
  ])
})

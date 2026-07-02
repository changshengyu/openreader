import assert from 'node:assert/strict'
import test from 'node:test'
import { computed, ref } from 'vue'
import { useReaderChapterMaintenance } from '../src/composables/useReaderChapterMaintenance.js'

function createController(overrides = {}) {
  const calls = []
  const bookId = ref(7)
  const book = ref({ id: 7, sourceId: 2 })
  const chapters = ref([{ id: 1 }])
  const currentIndex = ref(4)
  const controller = useReaderChapterMaintenance({
    book,
    bookId,
    chapters,
    currentIndex,
    isRemoteBook: computed(() => Number(book.value?.sourceId || 0) > 0),
    fetchChapters: async id => {
      calls.push(['fetch', id])
      return [{ id: 11 }, { id: 12 }]
    },
    writeDataCache: async payload => calls.push(['cache', payload]),
    clearMemory: (...args) => calls.push(['memory', ...args]),
    resetBrowserState: () => calls.push(['reset-browser']),
    clearBrowserCache: async (...args) => {
      calls.push(['clear-browser', ...args])
      return 3
    },
    loadChapter: async (...args) => calls.push(['load-chapter', ...args]),
    getCurrentOffset: () => 88,
    clearServerCache: async ids => {
      calls.push(['clear-server', ids])
      return { cleared: 5 }
    },
    clearCurrentBrowserCache: async () => {
      calls.push(['clear-current-browser'])
      return 2
    },
    notify: message => calls.push(['notify', message]),
    onError: (...args) => calls.push(['error', ...args]),
    ...overrides,
  })
  return {
    book,
    bookId,
    calls,
    chapters,
    controller,
    currentIndex,
  }
}

test('loads chapters, clamps the index, and writes the reader cache', async () => {
  const fixture = createController()
  assert.deepEqual(await fixture.controller.loadChapters(), [{ id: 11 }, { id: 12 }])
  assert.equal(fixture.currentIndex.value, 1)
  assert.deepEqual(fixture.calls, [
    ['fetch', 7],
    ['cache', { bookId: 7, chaptersData: [{ id: 11 }, { id: 12 }] }],
  ])
})

test('discards a chapter response after navigating to another book', async () => {
  let resolveFetch
  const fixture = createController({
    fetchChapters: () => new Promise(resolve => {
      resolveFetch = resolve
    }),
  })
  const pending = fixture.controller.loadChapters()
  fixture.bookId.value = 8
  resolveFetch([{ id: 99 }])
  assert.deepEqual(await pending, [{ id: 1 }])
  assert.deepEqual(fixture.chapters.value, [{ id: 1 }])
  assert.equal(fixture.calls.length, 0)
})

test('resets memory and browser cache while tolerating browser cleanup failure', async () => {
  const fixture = createController({
    clearBrowserCache: async () => {
      throw new Error('unavailable')
    },
  })
  assert.equal(await fixture.controller.resetCaches({ clearBrowser: true }), 0)
  assert.deepEqual(fixture.calls, [
    ['memory', fixture.book.value, 7],
    ['reset-browser'],
  ])
})

test('reloads the current chapter and clears remote caches with unchanged messages', async () => {
  const fixture = createController()
  await fixture.controller.reloadChapter()
  await fixture.controller.clearCurrentBookCache()
  assert.deepEqual(fixture.calls, [
    ['load-chapter', 4, 88, { refresh: true }],
    ['notify', '章节已重新载入'],
    ['clear-server', [7]],
    ['clear-current-browser'],
    ['fetch', 7],
    ['cache', { bookId: 7, chaptersData: [{ id: 11 }, { id: 12 }] }],
    ['notify', '已清理服务器 5 章，本地 2 章'],
  ])
})

test('does not clear cache for a local book', async () => {
  const fixture = createController()
  fixture.book.value = { id: 7, sourceId: 0 }
  await fixture.controller.clearCurrentBookCache()
  assert.deepEqual(fixture.calls, [])
})

import assert from 'node:assert/strict'
import test from 'node:test'
import { reactive } from 'vue'
import { useOverlayBookCacheState } from '../src/composables/useOverlayBookCacheState.js'

test('owns cache counts, reader cache refresh, and merged book broadcasts', async () => {
  const calls = []
  const overlay = reactive({
    bookInfoBook: { id: 1, title: '旧书名' },
  })
  const bookshelf = reactive({
    books: [{
      id: 1,
      title: '书架书名',
      progress: { bookId: 1, chapterIndex: 3 },
    }],
    upsertBook: book => calls.push(['upsert', book]),
  })
  const controller = useOverlayBookCacheState({
    overlay,
    bookshelf,
    getManagedBooks: () => bookshelf.books,
    countBrowserCachedChapters: async rows => {
      calls.push(['count', rows.map(row => row.id)])
      return { 1: 2 }
    },
    listBrowserCachedChapters: async (book, id) => {
      calls.push(['list-browser', book.id, id])
      return { 0: true, 1: true, 2: true }
    },
    clearBrowserChapterCache: async (book, id) => {
      calls.push(['clear-browser', book.id, id])
    },
    invalidateReaderData: async (id, options) => {
      calls.push(['invalidate', id, options])
    },
    listChapters: async id => {
      calls.push(['list-chapters', id])
      return { data: [{ id: 10, index: 0, title: '第一章' }] }
    },
    writeReaderData: async (id, payload) => {
      calls.push(['write-reader', id, payload])
    },
    mergeBook: (current, incoming) => ({
      ...current,
      ...incoming,
      progress: current?.progress,
    }),
    emitBookInfoUpdated: book => calls.push(['emit-info', book]),
    emitReaderBookDataUpdated: detail => calls.push(['emit-reader', detail]),
  })

  await controller.refreshManagedBrowserCacheCounts()
  await controller.refreshBookInfoBrowserCacheCount({ id: 1 })
  assert.equal(controller.localCacheCount({ id: 1 }), 3)

  await controller.invalidateBookReaderCaches({ id: 1 }, { clearBrowser: true })
  assert.equal(controller.localCacheCount({ id: 1 }), 0)

  const chapters = await controller.refreshBookChaptersCache({ id: 1, title: '新书名' })
  const merged = controller.applyUpdatedBookToOverlay(
    { id: 1, title: '新书名' },
    chapters,
  )
  assert.deepEqual(merged.progress, { bookId: 1, chapterIndex: 3 })
  assert.equal(overlay.bookInfoBook.title, '新书名')

  controller.updateServerCacheCount(merged, 4)
  assert.equal(overlay.bookInfoBook.cachedChapterCount, 4)
  assert.equal(controller.serverCacheCount(overlay.bookInfoBook), 4)
  assert.deepEqual(calls.map(call => call[0]), [
    'count',
    'list-browser',
    'invalidate',
    'clear-browser',
    'list-chapters',
    'write-reader',
    'upsert',
    'emit-info',
    'emit-reader',
    'upsert',
  ])
})

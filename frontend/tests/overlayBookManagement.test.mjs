import assert from 'node:assert/strict'
import test from 'node:test'
import { reactive } from 'vue'
import { useOverlayBookManagement } from '../src/composables/useOverlayBookManagement.js'
import { cancelAllBookManagementCacheJobs } from '../src/composables/useOverlayBookItemActions.js'

function createController(overrides = {}) {
  const calls = []
  const books = reactive([
    { id: 1, title: '本地/书', sourceId: 0, categoryIds: [2] },
    { id: 2, title: '远程书', sourceId: 8, categoryIds: [3] },
    { id: 3, title: '另一远程书', sourceId: 9, categoryIds: [2] },
  ])
  const bookshelf = {
    batchSetCategory: async (...args) => calls.push(['set-category', ...args]),
    batchCacheBooks: async ids => {
      calls.push(['batch-cache', ids])
      return { cached: 6, requested: 8 }
    },
    batchClearCache: async ids => {
      calls.push(['batch-clear', ids])
      return { cleared: ids.length * 2 }
    },
    batchDeleteBooks: async ids => calls.push(['batch-delete', ids]),
    exportSelectedBooks: async (ids, format) => {
      calls.push(['export', ids, format])
      return { ids, format }
    },
    loadBooks: async options => calls.push(['load-books', options]),
    upsertBook: book => calls.push(['upsert', book]),
  }
  const controller = useOverlayBookManagement({
    bookshelf,
    getManagedBooks: () => books,
    getFilteredManagedBooks: () => books.slice(0, 2),
    getBookProgress: book => (
      book.id === 2 ? { chapterIndex: 4 } : { chapterIndex: 0 }
    ),
    cacheBookContent: async (id, payload) => {
      calls.push(['cache-server', id, payload])
      return {
        data: {
          cached: 2,
          requested: 3,
          book: { id, cachedChapterCount: 2 },
        },
      }
    },
    listChapters: async id => {
      calls.push(['chapters', id])
      return { data: [{ id: 10 }, { id: 11 }] }
    },
    cacheBrowserChapters: async (book, id, chapters, options) => {
      calls.push(['cache-browser', book.id, id, chapters, options])
      return { cached: 2, requested: 2 }
    },
    clearBrowserChapterCache: async (book, id) => {
      calls.push(['clear-browser', book.id, id])
      return 5
    },
    updateServerCacheCount: (book, count) => {
      calls.push(['server-count', book.id, count])
    },
    refreshManagedBrowserCacheCounts: async () => {
      calls.push(['refresh-managed-cache'])
    },
    saveBlob: (blob, filename) => calls.push(['save-blob', blob, filename]),
    confirm: async (...args) => calls.push(['confirm', ...args]),
    now: () => 123,
    onSuccess: message => calls.push(['success', message]),
    onInfo: message => calls.push(['info', message]),
    onError: (...args) => calls.push(['error', ...args]),
    ...overrides,
  })
  return { books, calls, controller, bookshelf }
}

test('owns desktop and mobile book selection state', () => {
  const fixture = createController()
  fixture.controller.onManageSelectionChange([
    { id: 1 },
    { id: 2 },
  ])
  fixture.controller.toggleManagedBook(3, true)
  fixture.controller.toggleManagedBook(2, false)
  assert.deepEqual(fixture.controller.selectedBookIds.value, [1, 3])

  fixture.controller.selectAllManagedBooks()
  assert.deepEqual(fixture.controller.selectedBookIds.value, [1, 2])
  fixture.controller.clearManagedSelection()
  assert.deepEqual(fixture.controller.selectedBookIds.value, [])
})

test('adds and removes categories only for matching selected books', async () => {
  const fixture = createController()
  fixture.controller.selectedBookIds.value = [1, 2]
  await fixture.controller.batchAddCategory({ id: 7, name: '收藏' })
  await fixture.controller.batchRemoveCategory({ id: 2, name: '本地组' })

  assert.deepEqual(fixture.calls, [
    ['set-category', [1, 2], 7, { action: 'category-add' }],
    ['success', '已添加到“收藏”分组'],
    ['set-category', [1], 2, { action: 'category-remove' }],
    ['success', '已从“本地组”分组移除'],
  ])
  assert.equal(fixture.controller.batchBusy.value, false)

  fixture.calls.length = 0
  fixture.controller.selectedBookIds.value = [2]
  await fixture.controller.batchRemoveCategory({ id: 2, name: '本地组' })
  assert.deepEqual(fixture.calls, [
    ['info', '选中书籍不在该分组中'],
  ])
})

test('confirms batch deletion for the current selection', async () => {
  const fixture = createController()
  fixture.controller.selectedBookIds.value = [1, 3]
  await fixture.controller.batchDeleteBooks()

  assert.deepEqual(fixture.calls, [
    ['confirm', '确定删除选中的 2 本书吗？', '批量删除', { type: 'warning' }],
    ['batch-delete', [1, 3]],
    ['success', '已批量删除'],
  ])
  assert.deepEqual(fixture.controller.selectedBookIds.value, [])
})

test('routes BookManage server and browser caching across the whole catalogue', async () => {
  const fixture = createController()
  await fixture.controller.cacheBook(fixture.books[0], 'cacheBook')
  assert.deepEqual(fixture.calls, [
    ['info', '本地书无需服务器缓存'],
  ])

  fixture.calls.length = 0
  await fixture.controller.cacheBook(fixture.books[1], 'cacheBook')
  assert.deepEqual(fixture.calls, [
    ['cache-server', 2, { all: true, chapterIndex: 0, refresh: false }],
    ['upsert', { id: 2, cachedChapterCount: 2 }],
    ['success', '已缓存 2/3 章'],
  ])
  assert.equal(fixture.controller.cachingBookId.value, null)

  fixture.calls.length = 0
  await fixture.controller.cacheBook(fixture.books[0], 'cacheBookLocal')
  assert.equal(fixture.calls[0][0], 'chapters')
  assert.deepEqual(fixture.calls[0].slice(1), [1])
  assert.equal(fixture.calls[1][0], 'cache-browser')
  assert.deepEqual(fixture.calls[1].slice(1, 4), [1, 1, [{ id: 10 }, { id: 11 }]])
  assert.equal(fixture.calls[1][4].startIndex, 0)
  assert.equal(fixture.calls[1][4].count, true)
  assert.equal(fixture.calls[1][4].concurrency, 2)
  assert.equal(typeof fixture.calls[1][4].cancelled, 'function')
  assert.equal(typeof fixture.calls[1][4].onProgress, 'function')
  assert.deepEqual(fixture.calls.slice(2), [
    ['success', '已缓存到浏览器 2/2 章'],
    ['refresh-managed-cache'],
  ])
})

test('streams per-book server-cache progress and cancels it from the active control', async () => {
  let aborted = false
  const fixture = createController({
    cacheBookContentStream: (id, payload, { signal, onEvent }) => {
      fixture.calls.push(['cache-stream', id, payload])
      onEvent({
        event: 'message',
        data: { bookId: id, cached: 1, requested: 1, total: 3, chapterIndex: 4, failed: 0 },
      })
      return new Promise((resolve, reject) => {
        signal.addEventListener('abort', () => {
          aborted = true
          const error = new Error('aborted')
          error.name = 'AbortError'
          reject(error)
        })
      })
    },
  })
  const book = fixture.books[1]
  const pending = fixture.controller.cacheBook(book, 'cacheBook')

  assert.equal(fixture.controller.isCachingBook(book), true)
  assert.equal(fixture.controller.cacheProgressLabel(book), '1/3')
  assert.equal(fixture.controller.cancelBookCache(book), true)
  await pending

  assert.equal(aborted, true)
  assert.equal(fixture.controller.isCachingBook(book), false)
  assert.equal(fixture.controller.cacheProgressLabel(book), '')
  assert.deepEqual(fixture.calls, [
    ['cache-stream', 2, { all: true, chapterIndex: 0, refresh: false }],
    ['info', '正在停止服务器缓存'],
    ['info', '已取消服务器缓存'],
  ])
})

test('allows independent server cache jobs and cancels only the selected book', async () => {
  const pendingByBook = new Map()
  const fixture = createController({
    cacheBookContentStream: (id, payload, { signal }) => new Promise((resolve, reject) => {
      fixture.calls.push(['cache-stream', id, payload])
      pendingByBook.set(id, { resolve, reject, signal })
      signal.addEventListener('abort', () => {
        const error = new Error('aborted')
        error.name = 'AbortError'
        reject(error)
      })
    }),
  })

  const first = fixture.controller.cacheBook(fixture.books[1], 'cacheBook')
  const second = fixture.controller.cacheBook(fixture.books[2], 'cacheBook')
  assert.equal(fixture.controller.isCachingBook(fixture.books[1]), true)
  assert.equal(fixture.controller.isCachingBook(fixture.books[2]), true)
  assert.equal(fixture.controller.cancelBookCache(fixture.books[1]), true)
  await first
  assert.equal(pendingByBook.get(2).signal.aborted, true)
  assert.equal(fixture.controller.isCachingBook(fixture.books[1]), false)
  assert.equal(fixture.controller.isCachingBook(fixture.books[2]), true)

  pendingByBook.get(3).resolve({ cachedCount: 2, successCount: 2, failedCount: 0, processed: 2, total: 2 })
  await second
  assert.equal(fixture.controller.isCachingBook(fixture.books[2]), false)
  assert.equal(pendingByBook.get(3).signal.aborted, false)
})

test('cancels a browser cache queue without cancelling another book job', async () => {
  let finishFirstBrowser
  const fixture = createController({
    cacheBrowserChapters: async (book, id, chapters, options) => {
      fixture.calls.push(['cache-browser', book.id, id, chapters, options])
      if (book.id === 1) {
        await new Promise(resolve => { finishFirstBrowser = resolve })
        return { cached: 1, requested: 2, cancelled: options.cancelled() }
      }
      return { cached: 2, requested: 2, cancelled: false }
    },
  })
  const pending = fixture.controller.cacheBook(fixture.books[0], 'cacheBookLocal')
  await Promise.resolve()
  assert.equal(fixture.controller.isCachingBook(fixture.books[0]), true)
  assert.equal(fixture.controller.cancelBookCache(fixture.books[0]), true)
  finishFirstBrowser()
  await pending
  assert.equal(fixture.controller.isCachingBook(fixture.books[0]), false)
})

test('finishes browser cancellation while its catalogue is still loading', async () => {
  let finishCatalogue
  const fixture = createController({
    listChapters: id => new Promise(resolve => {
      fixture.calls.push(['chapters', id])
      finishCatalogue = resolve
    }),
  })
  const book = fixture.books[0]
  const pending = fixture.controller.cacheBook(book, 'cacheBookLocal')
  assert.equal(fixture.controller.isCachingBook(book), true)
  assert.equal(fixture.controller.cancelBookCache(book), true)
  finishCatalogue({ data: [{ id: 10 }] })
  await pending

  assert.equal(fixture.controller.isCachingBook(book), false)
  assert.deepEqual(fixture.calls, [
    ['chapters', 1],
    ['info', '正在停止浏览器缓存'],
    ['info', '已取消浏览器缓存'],
  ])
})

test('aborts every active cache job when the authenticated session is cleared', async () => {
  const fixture = createController({
    cacheBookContentStream: (_id, _payload, { signal }) => new Promise((_resolve, reject) => {
      signal.addEventListener('abort', () => {
        const error = new Error('aborted')
        error.name = 'AbortError'
        reject(error)
      })
    }),
  })
  const first = fixture.controller.cacheBook(fixture.books[1], 'cacheBook')
  const second = fixture.controller.cacheBook(fixture.books[2], 'cacheBook')
  assert.equal(fixture.controller.isCachingBook(fixture.books[1]), true)
  assert.equal(fixture.controller.isCachingBook(fixture.books[2]), true)

  cancelAllBookManagementCacheJobs()
  await Promise.all([first, second])
  assert.equal(fixture.controller.isCachingBook(fixture.books[1]), false)
  assert.equal(fixture.controller.isCachingBook(fixture.books[2]), false)
})

test('clears both cache layers and sanitizes single-book export names', async () => {
  const fixture = createController()
  await fixture.controller.cacheBook(fixture.books[1], 'deleteBookCache')
  await fixture.controller.cacheBook(fixture.books[0], 'deleteBookLocalCache')
  await fixture.controller.exportBook(fixture.books[0], 'epub')

  assert.deepEqual(fixture.calls, [
    ['confirm', '确认要删除服务器上《远程书》的缓存章节吗？', '提示', { type: 'warning' }],
    ['batch-clear', [2]],
    ['server-count', 2, 0],
    ['success', '已清理 2 个章节缓存'],
    ['confirm', '确认要删除浏览器中《本地/书》的缓存章节吗？', '提示', { type: 'warning' }],
    ['clear-browser', 1, 1],
    ['refresh-managed-cache'],
    ['success', '已清理浏览器缓存 5 章'],
    ['export', [1], 'epub'],
    ['save-blob', { ids: [1], format: 'epub' }, '本地-书.epub'],
    ['success', '已导出《本地/书》'],
  ])
  assert.equal(fixture.controller.exportBookFilename({ id: 9 }, 'weird'), 'book-9.txt')
  assert.equal(fixture.controller.batchBusy.value, false)
  assert.equal(fixture.controller.isCachingBook(fixture.books[0]), false)
})

test('shares busy state and normalizes unsupported single-book exports to txt', async () => {
  const fixture = createController()
  let finishExport
  fixture.bookshelf.exportSelectedBooks = async () => new Promise((resolve) => {
    finishExport = resolve
  })

  const pending = fixture.controller.exportBook(fixture.books[1], 'json')
  assert.equal(fixture.controller.batchBusy.value, true)

  finishExport({ format: 'json' })
  await pending
  assert.equal(fixture.controller.batchBusy.value, false)
  assert.deepEqual(fixture.calls, [
    ['save-blob', { format: 'json' }, '远程书.txt'],
    ['success', '已导出《远程书》'],
  ])
})

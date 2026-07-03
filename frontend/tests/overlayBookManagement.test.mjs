import assert from 'node:assert/strict'
import test from 'node:test'
import { reactive } from 'vue'
import { useOverlayBookManagement } from '../src/composables/useOverlayBookManagement.js'

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
    refreshBookInfoBrowserCacheCount: async book => {
      calls.push(['refresh-info-cache', book.id])
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

test('filters remote books for batch server cache and clear operations', async () => {
  const fixture = createController()
  fixture.controller.selectedBookIds.value = [1, 2, 3]
  assert.deepEqual(fixture.controller.selectedRemoteBookIds(), [2, 3])

  await fixture.controller.batchCacheBooks()
  await fixture.controller.batchClearCache()

  assert.deepEqual(fixture.calls, [
    ['batch-cache', [2, 3]],
    ['success', '已缓存 6/8 章'],
    ['load-books', { force: true, all: true }],
    ['confirm', '确定清理选中 2 本远程书的章节缓存吗？', '清理缓存', { type: 'warning' }],
    ['batch-clear', [2, 3]],
    ['success', '已清理 4 个章节缓存'],
    ['server-count', 2, 0],
    ['server-count', 3, 0],
  ])
  assert.equal(fixture.controller.batchBusy.value, false)
})

test('confirms batch deletion and exports the current selection', async () => {
  const fixture = createController()
  fixture.controller.selectedBookIds.value = [1, 3]
  await fixture.controller.batchExportBooks()
  await fixture.controller.batchDeleteBooks()

  assert.deepEqual(fixture.calls, [
    ['export', [1, 3], 'json'],
    ['save-blob', { ids: [1, 3], format: 'json' }, 'openreader-books-2.json'],
    ['success', '已导出 2 本书'],
    ['confirm', '确定删除选中的 2 本书吗？', '批量删除', { type: 'warning' }],
    ['batch-delete', [1, 3]],
    ['success', '已批量删除'],
  ])
  assert.deepEqual(fixture.controller.selectedBookIds.value, [])
})

test('routes server and browser caching while starting from reading progress', async () => {
  const fixture = createController()
  await fixture.controller.cacheBook(fixture.books[0], 'cacheBook')
  assert.deepEqual(fixture.calls, [
    ['info', '本地书无需服务器缓存'],
  ])

  fixture.calls.length = 0
  await fixture.controller.cacheBook(fixture.books[1], 'cacheBook')
  assert.deepEqual(fixture.calls, [
    ['cache-server', 2, { all: true, count: 20, chapterIndex: 4 }],
    ['upsert', { id: 2, cachedChapterCount: 2 }],
    ['success', '已缓存 2/3 章'],
  ])
  assert.equal(fixture.controller.cachingBookId.value, null)

  fixture.calls.length = 0
  await fixture.controller.cacheBook(fixture.books[0], 'cacheBookLocal')
  assert.deepEqual(fixture.calls, [
    ['chapters', 1],
    [
      'cache-browser',
      1,
      1,
      [{ id: 10 }, { id: 11 }],
      { startIndex: 0, count: 100 },
    ],
    ['success', '已缓存到浏览器 2/2 章'],
    ['refresh-managed-cache'],
    ['refresh-info-cache', 1],
  ])
})

test('clears both cache layers and sanitizes single-book export names', async () => {
  const fixture = createController()
  await fixture.controller.cacheBook(fixture.books[1], 'deleteBookCache')
  await fixture.controller.cacheBook(fixture.books[0], 'deleteBookLocalCache')
  await fixture.controller.exportBook(fixture.books[0], 'epub')

  assert.deepEqual(fixture.calls, [
    ['batch-clear', [2]],
    ['server-count', 2, 0],
    ['success', '已清理 2 个章节缓存'],
    ['clear-browser', 1, 1],
    ['refresh-managed-cache'],
    ['refresh-info-cache', 1],
    ['success', '已清理浏览器缓存 5 章'],
    ['export', [1], 'epub'],
    ['save-blob', { ids: [1], format: 'epub' }, '本地-书.epub'],
    ['success', '已导出《本地/书》'],
  ])
  assert.equal(fixture.controller.exportBookFilename({ id: 9 }, 'weird'), 'book-9.txt')
  assert.equal(fixture.controller.batchBusy.value, false)
  assert.equal(fixture.controller.cachingBookId.value, null)
})

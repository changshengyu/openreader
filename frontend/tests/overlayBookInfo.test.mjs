import assert from 'node:assert/strict'
import test from 'node:test'
import { reactive } from 'vue'
import { useOverlayBookInfo } from '../src/composables/useOverlayBookInfo.js'

function createController(overrides = {}) {
  const calls = []
  const overlay = reactive({
    bookInfoBook: {
      id: 1,
      title: '旧书名',
      author: '作者',
      intro: '简介',
      sourceId: 8,
      categoryIds: [2, 3],
      canUpdate: true,
    },
    bookEditBook: null,
    bookEditVisible: false,
  })
  const bookshelf = reactive({
    books: [
      {
        id: 1,
        title: '书架书名',
        progress: { bookId: 1, chapterIndex: 4 },
      },
      { id: 2, title: '第二本' },
    ],
    upsertBook: book => calls.push(['upsert', book]),
    loadBooks: async options => calls.push(['load-books', options]),
  })
  const controller = useOverlayBookInfo({
    overlay,
    bookshelf,
    getManagedBooks: () => bookshelf.books,
    countBrowserCachedChapters: async books => {
      calls.push(['count-browser', books.map(book => book.id)])
      return { 1: 2, 2: 4 }
    },
    listBrowserCachedChapters: async (book, id) => {
      calls.push(['list-browser', book.id, id])
      return { 0: true, 2: true, 4: true }
    },
    clearBrowserChapterCache: async (book, id) => {
      calls.push(['clear-browser', book.id, id])
      return 3
    },
    invalidateReaderData: async (id, options) => {
      calls.push(['invalidate-reader', id, options])
    },
    listChapters: async id => {
      calls.push(['list-chapters', id])
      return { data: [{ id: 10, index: 0, title: '第一章' }] }
    },
    writeReaderData: async (id, payload) => {
      calls.push(['write-reader', id, payload])
    },
    refreshLocalBook: async id => {
      calls.push(['refresh-local', id])
      return {
        data: {
          book: { id, title: '刷新后', chapterCount: 1 },
          chapterCount: 1,
        },
      }
    },
    uploadAsset: async payload => {
      calls.push(['upload', payload])
      return { data: { url: '/assets/new-cover.jpg' } }
    },
    updateBook: async (id, payload) => {
      calls.push(['update-book', id, payload])
      return { data: { ...overlay.bookInfoBook, ...payload, id } }
    },
    mergeBook: (current, incoming) => ({
      ...current,
      ...incoming,
      progress: current?.progress,
    }),
    emitBookInfoUpdated: book => calls.push(['emit-info', book]),
    emitReaderBookDataUpdated: detail => calls.push(['emit-reader', detail]),
    onSuccess: message => calls.push(['success', message]),
    onError: (...args) => calls.push(['error', ...args]),
    ...overrides,
  })
  return { calls, controller, overlay, bookshelf }
}

test('loads aggregate and single-book browser cache counts with safe fallbacks', async () => {
  const fixture = createController()
  await fixture.controller.refreshManagedBrowserCacheCounts()
  await fixture.controller.refreshBookInfoBrowserCacheCount({ id: 1 })

  assert.deepEqual(fixture.controller.localCacheCounts.value, { 1: 3, 2: 4 })
  assert.equal(fixture.controller.localCacheCount({ id: 1 }), 3)
  assert.deepEqual(fixture.calls, [
    ['count-browser', [1, 2]],
    ['list-browser', 1, 1],
  ])

  const failed = createController({
    countBrowserCachedChapters: async () => {
      throw new Error('cache unavailable')
    },
    listBrowserCachedChapters: async () => {
      throw new Error('cache unavailable')
    },
  })
  await failed.controller.refreshManagedBrowserCacheCounts()
  await failed.controller.refreshBookInfoBrowserCacheCount({ id: 1 })
  assert.deepEqual(failed.controller.localCacheCounts.value, { 1: 0, 2: 0 })
})

test('merges updated books before broadcasting to the shelf and reader', () => {
  const fixture = createController()
  const chapters = [{ id: 10, index: 0 }]
  const updated = fixture.controller.applyUpdatedBookToOverlay(
    { id: 1, title: '新书名' },
    chapters,
  )

  assert.equal(updated.title, '新书名')
  assert.deepEqual(updated.progress, { bookId: 1, chapterIndex: 4 })
  assert.equal(fixture.overlay.bookInfoBook.title, '新书名')
  assert.deepEqual(fixture.calls, [
    ['upsert', updated],
    ['emit-info', updated],
    ['emit-reader', { bookId: 1, book: updated, chapters }],
  ])

  fixture.controller.updateServerCacheCount(updated, -3)
  assert.equal(fixture.overlay.bookInfoBook.cachedChapterCount, 0)
  assert.equal(fixture.controller.serverCacheCount(fixture.overlay.bookInfoBook), 0)
})

test('invalidates reader caches and refreshes chapter cache with a book-only fallback', async () => {
  const fixture = createController()
  const book = { id: 1, title: '本地书' }
  fixture.controller.setLocalCacheCount(1, 5)
  await fixture.controller.invalidateBookReaderCaches(book, { clearBrowser: true })
  const chapters = await fixture.controller.refreshBookChaptersCache(book)

  assert.deepEqual(chapters, [{ id: 10, index: 0, title: '第一章' }])
  assert.equal(fixture.controller.localCacheCount(book), 0)
  assert.deepEqual(fixture.calls, [
    ['invalidate-reader', 1, { book: true, chapters: true }],
    ['clear-browser', 1, 1],
    ['list-chapters', 1],
    ['write-reader', 1, { bookData: book, chaptersData: chapters }],
  ])

  const failed = createController({
    listChapters: async () => {
      throw new Error('catalog failed')
    },
  })
  assert.equal(await failed.controller.refreshBookChaptersCache(book), null)
  assert.deepEqual(failed.calls, [
    ['write-reader', 1, { bookData: book }],
  ])
})

test('saves edited metadata through the shared merge and broadcast path', async () => {
  const fixture = createController()
  fixture.overlay.bookEditBook = {
    id: 1,
    title: '旧书名',
    categoryIds: [2, 3],
    canUpdate: false,
  }
  fixture.overlay.bookEditVisible = true

  await fixture.controller.saveEditedBook({
    title: '编辑后',
    author: '新作者',
  })

  assert.deepEqual(fixture.calls[0], [
    'update-book',
    1,
    {
      title: '编辑后',
      author: '新作者',
      categoryIds: [2, 3],
      canUpdate: false,
    },
  ])
  assert.equal(fixture.overlay.bookEditBook.title, '编辑后')
  assert.equal(fixture.overlay.bookEditVisible, false)
  assert.equal(fixture.controller.editingBookSaving.value, false)
  assert.deepEqual(fixture.calls.at(-1), ['success', '书籍已更新'])
})

test('refreshes a local book, rebuilds reader caches, and refreshes its cache count', async () => {
  const fixture = createController()
  const book = { id: 1, title: '本地书', sourceId: 0 }

  await fixture.controller.refreshLocalBookInfo(book)

  assert.deepEqual(fixture.calls.map(call => call[0]), [
    'refresh-local',
    'invalidate-reader',
    'clear-browser',
    'list-chapters',
    'write-reader',
    'upsert',
    'emit-info',
    'emit-reader',
    'list-browser',
    'success',
  ])
  assert.equal(fixture.overlay.bookInfoBook.title, '刷新后')
  assert.equal(fixture.controller.localCacheCount(book), 3)
  assert.equal(fixture.controller.refreshingBookId.value, null)
  assert.deepEqual(fixture.calls.at(-1), ['success', '本地书已刷新，共 1 章'])
})

test('uploads custom covers and toggles remote update state with precise patch payloads', async () => {
  const fixture = createController()
  const file = { name: 'cover.jpg' }
  await fixture.controller.uploadBookInfoCover(file)

  assert.deepEqual(fixture.calls[0], ['upload', { file, type: 'cover' }])
  assert.deepEqual(fixture.calls[1], [
    'update-book',
    1,
    {
      customCoverUrl: '/assets/new-cover.jpg',
    },
  ])
  assert.equal(fixture.controller.coverUploadingBookId.value, null)

  fixture.calls.length = 0
  await fixture.controller.toggleBookCanUpdate(false)
  assert.deepEqual(fixture.calls[0], [
    'update-book',
    1,
    {
      canUpdate: false,
    },
  ])
  assert.equal(fixture.controller.updatingBookId.value, null)
  assert.deepEqual(fixture.calls.at(-1), ['success', '已关闭追更'])
})

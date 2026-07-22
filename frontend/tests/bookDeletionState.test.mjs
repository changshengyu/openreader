import assert from 'node:assert/strict'
import test, { after, afterEach } from 'node:test'
import { createServer } from 'vite'

class MemoryStorage {
  constructor() {
    this.values = new Map()
  }

  get length() {
    return this.values.size
  }

  clear() {
    this.values.clear()
  }

  getItem(key) {
    return this.values.has(String(key)) ? this.values.get(String(key)) : null
  }

  key(index) {
    return [...this.values.keys()][index] ?? null
  }

  removeItem(key) {
    this.values.delete(String(key))
  }

  setItem(key, value) {
    this.values.set(String(key), String(value))
  }
}

class TestCustomEvent extends Event {
  constructor(type, options = {}) {
    super(type)
    this.detail = options.detail
  }
}

const storage = new MemoryStorage()
const windowTarget = new EventTarget()
windowTarget.localStorage = storage
windowTarget.location = { protocol: 'http:', host: 'openreader.test' }
globalThis.window = windowTarget
globalThis.localStorage = storage
globalThis.CustomEvent = TestCustomEvent

const vite = await createServer({
  root: new URL('..', import.meta.url).pathname,
  appType: 'custom',
  logLevel: 'silent',
  server: { middlewareMode: true },
})
after(() => vite.close())

const { createPinia, setActivePinia } = await import('pinia')
const { default: api } = await vite.ssrLoadModule('/src/api/client.js')
const { useBookshelfStore } = await vite.ssrLoadModule('/src/stores/bookshelf.js')
const { useReaderStore } = await vite.ssrLoadModule('/src/stores/reader.js')

const originalDelete = api.delete
const originalPost = api.post

afterEach(() => {
  api.delete = originalDelete
  api.post = originalPost
  storage.clear()
})

function tokenFor(userId) {
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url')
  const payload = Buffer.from(JSON.stringify({ userId })).toString('base64url')
  return `${header}.${payload}.book-delete-test`
}

function freshStores() {
  storage.clear()
  storage.setItem('openreader_token', tokenFor(1))
  setActivePinia(createPinia())
  return {
    bookshelf: useBookshelfStore(),
    reader: useReaderStore(),
  }
}

function progress(bookId) {
  return {
    bookId,
    chapterId: bookId * 10,
    chapterIndex: 2,
    offset: 80,
    percent: 0.4,
    updatedAt: '2026-07-22T00:00:00Z',
  }
}

function deletedEvents() {
  const events = []
  const listener = event => events.push(event.detail?.ids)
  windowTarget.addEventListener('openreader:books-deleted', listener)
  return {
    events,
    dispose: () => windowTarget.removeEventListener('openreader:books-deleted', listener),
  }
}

test('sync deletion clears shelf, local progress and browser chapters, then emits one normalized event', async () => {
  const { bookshelf, reader } = freshStores()
  const book = { id: 7, title: 'Book', author: 'A', url: 'url-7' }
  bookshelf.books = [book, { id: 8, title: 'Keep' }]
  reader.applyProgress(progress(7))
  storage.setItem('localCache@user:1@Book_A@url-7@chapterContent-0', JSON.stringify({ content: 'cached' }))
  const observed = deletedEvents()

  await bookshelf.removeBookLocal('7')

  assert.deepEqual(bookshelf.books.map(row => row.id), [8])
  assert.equal(reader.progressByBook[7], undefined)
  assert.equal(storage.getItem('openreader_chapter_progress@user:1@7'), null)
  assert.equal(storage.getItem('localCache@user:1@Book_A@url-7@chapterContent-0'), null)
  assert.deepEqual(observed.events, [[7]])
  observed.dispose()
})

test('sync deletion can recover cached book identity before removing a shelf snapshot', async () => {
  const { bookshelf, reader } = freshStores()
  const book = { id: 7, title: 'Cached', author: 'B', url: 'cached-url-7' }
  const shelfCacheKey = 'localCache@bookshelf@getBookshelf:{"all":true}:user:1'
  storage.setItem(shelfCacheKey, JSON.stringify([book]))
  storage.setItem('localCache@user:1@Cached_B@cached-url-7@chapterContent-0', JSON.stringify({ content: 'cached' }))
  reader.applyProgress(progress(7))

  await bookshelf.removeBookLocal(7)

  assert.equal(storage.getItem('localCache@user:1@Cached_B@cached-url-7@chapterContent-0'), null)
  assert.deepEqual(JSON.parse(storage.getItem(shelfCacheKey)), [])
  assert.equal(reader.progressByBook[7], undefined)
})

test('direct delete changes no local consumer before the API succeeds', async () => {
  const { bookshelf, reader } = freshStores()
  const book = { id: 7, title: 'Keep on failure' }
  bookshelf.books = [book]
  reader.applyProgress(progress(7))
  const observed = deletedEvents()
  api.delete = async () => {
    throw new Error('delete failed')
  }

  await assert.rejects(bookshelf.removeBook(7), /delete failed/)

  assert.deepEqual(bookshelf.books, [book])
  assert.deepEqual(reader.progressByBook[7], progress(7))
  assert.deepEqual(observed.events, [])
  observed.dispose()
})

test('batch delete reconciles only server-confirmed ids and emits one idempotent transaction', async () => {
  const { bookshelf, reader } = freshStores()
  bookshelf.books = [
    { id: 7, title: 'Deleted' },
    { id: 8, title: 'Not returned' },
    { id: 9, title: 'Unselected' },
  ]
  ;[7, 8, 9].forEach(id => reader.applyProgress(progress(id)))
  const observed = deletedEvents()
  api.post = async (url) => {
    assert.equal(url, '/books/batch')
    return { data: { affected: 1, deletedIds: [7, 7] } }
  }

  await bookshelf.batchDeleteBooks([7, 8])

  assert.deepEqual(bookshelf.books.map(book => book.id), [8, 9])
  assert.equal(reader.progressByBook[7], undefined)
  assert.deepEqual(reader.progressByBook[8], progress(8))
  assert.deepEqual(observed.events, [[7]])
  observed.dispose()
})

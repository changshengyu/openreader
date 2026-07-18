import { defineStore } from 'pinia'
import { batchBooks, createBook, deleteBook, exportBooks, listBooks } from '../api/books'
import { createCategory, deleteCategory, listCategories, reorderCategories, updateCategory } from '../api/categories'
import api from '../api/client'
import { useReaderStore } from './reader'
import { clearBookBrowserChapterCache } from '../utils/bookChapterCache'
import { newestProgress, sortByShelfOrder } from '../utils/bookOrder'
import { getBrowserCache, listBrowserCacheKeys, setBrowserCache } from '../utils/browserCache'
import { bookCategoryIds } from '../utils/bookCategory'
import { currentUserScope } from '../utils/authScope'
import { createAuthenticatedOperationGuard } from '../utils/authenticatedOperation'
import { createShelfRequestRevisionGate } from '../utils/shelfRequestRevision'
import { resolveShelfNetworkFirst } from '../utils/shelfNetworkFirst'

function asList(data) {
  if (Array.isArray(data)) return data
  if (Array.isArray(data?.list)) return data.list
  if (Array.isArray(data?.items)) return data.items
  if (Array.isArray(data?.data)) return data.data
  return []
}

function sortBooks(books) {
  const reader = useReaderStore()
  const values = asList(books).map(book => {
    const progress = newestProgress(book?.progress || null, reader.progressByBook?.[book?.id] || null)
    if (!progress || progress === book?.progress) return book
    return { ...book, progress }
  })
  return sortByShelfOrder(values, reader.progressByBook)
}

function sortCategories(categories) {
  return asList(categories).sort((a, b) => (a.sortOrder || 0) - (b.sortOrder || 0) || String(a.name || '').localeCompare(String(b.name || '')))
}

async function clearDeletedBookBrowserCache(book, bookId) {
  if (!book) return
  try {
    await clearBookBrowserChapterCache(book, bookId)
  } catch {
    // Deletion has already succeeded remotely; stale browser entries are
    // cache-only data and will be retried/overwritten on the next read.
  }
}

function normalizeLoadOptions(options = {}) {
  return options === true ? { force: true } : { ...(options || {}) }
}

const REFRESH_DEDUPE_MS = 1200
const MEMORY_CACHE_MS = 5000
const SHELF_CACHE_KEY = 'bookshelf@getBookshelf'
const CATEGORY_CACHE_KEY = 'bookshelf@getCategories'
let booksRequest = null
let booksRequestKey = ''
let categoriesRequest = null
const booksRevision = createShelfRequestRevisionGate()
const categoryOperations = createAuthenticatedOperationGuard()

export const useBookshelfStore = defineStore('bookshelf', {
  state: () => ({
    shelfScope: currentUserScope(),
    books: [],
    categories: [],
    selectedCategoryId: '',
    loading: false,
    booksLoadedAt: 0,
    booksLoadedKey: '',
    categoriesLoadedAt: 0,
  }),
  actions: {
    ensureShelfScope() {
      const scope = currentUserScope()
      if (!this.shelfScope) {
        this.shelfScope = scope
        return scope
      }
      if (this.shelfScope !== scope) {
        this.resetShelfState(scope)
      }
      return scope
    },
    resetShelfState(scope = currentUserScope()) {
      this.shelfScope = scope
      this.books = []
      this.categories = []
      this.selectedCategoryId = ''
      this.loading = false
      this.booksLoadedAt = 0
      this.booksLoadedKey = ''
      this.categoriesLoadedAt = 0
      booksRequest = null
      booksRequestKey = ''
      categoriesRequest = null
      booksRevision.reset(scope)
      categoryOperations.reset()
    },
    async loadBooks(options = {}) {
      this.ensureShelfScope()
      const force = options === true || Boolean(options?.force)
      const all = Boolean(options?.all)
      const params = {}
      if (!all && this.selectedCategoryId) {
        params.categoryId = this.selectedCategoryId
      }
      const requestKey = JSON.stringify(params)
      const now = Date.now()
      if (!force && this.books.length > 0 && this.booksLoadedKey === requestKey && now - this.booksLoadedAt < MEMORY_CACHE_MS) {
        return this.books
      }
      if (!force && this.booksLoadedKey === requestKey && this.booksLoadedAt > 0 && now - this.booksLoadedAt < REFRESH_DEDUPE_MS) {
        return this.books
      }
      if (!force && booksRequest && booksRequestKey === requestKey) return booksRequest

      this.loading = this.books.length === 0
      booksRequestKey = requestKey
      const requestRevision = booksRevision.begin(this.shelfScope)
      const cacheKey = scopedShelfCacheKey(`${SHELF_CACHE_KEY}:${requestKey}`)
      const request = resolveShelfNetworkFirst({
        request: () => listBooks(params).then(({ data }) => asList(data)),
        readFallback: () => readShelfCacheEntry(cacheKey),
        isCurrent: () => booksRevision.canCommit(requestRevision, this.shelfScope),
        hasCurrent: () => this.books.length > 0,
      })
        .then((result) => {
          if (result.source === 'network') {
            syncServerProgressFromBooks(result.value)
            this.books = sortBooks(result.value)
            this.booksLoadedAt = Date.now()
            this.booksLoadedKey = requestKey
            writeShelfCache(cacheKey, this.books)
          } else if (result.source === 'fallback') {
            this.books = sortBooks(result.value)
            this.booksLoadedAt = 0
            this.booksLoadedKey = requestKey
          }
          return this.books
        })
        .finally(() => {
          if (booksRequest === request) {
            booksRequest = null
            booksRequestKey = ''
            this.loading = false
          }
        })
      booksRequest = request
      return booksRequest
    },
    async loadCategories(options = {}) {
      const scope = this.ensureShelfScope()
      const force = options === true || Boolean(options?.force)
      const now = Date.now()
      if (!force && this.categoriesLoadedAt > 0 && now - this.categoriesLoadedAt < REFRESH_DEDUPE_MS) {
        return this.categories
      }
      if (!force && categoriesRequest) return categoriesRequest

      const operation = categoryOperations.begin('categories')
      const cacheKey = scopedShelfCacheKey(CATEGORY_CACHE_KEY, scope)

      if (!force && this.categories.length === 0) {
        const cached = await readShelfCache(cacheKey)
        if (!categoryOperations.canCommit(operation)) return this.categories
        if (cached.length) {
          this.categories = sortCategories(cached)
          this.categoriesLoadedAt = Date.now()
        }
      }

      const request = listCategories()
        .then(({ data }) => {
          if (!categoryOperations.canCommit(operation)) return this.categories
          this.categories = sortCategories(data)
          this.categoriesLoadedAt = Date.now()
          writeShelfCache(cacheKey, this.categories)
          return this.categories
        })
        .catch((err) => {
          if (!categoryOperations.canCommit(operation)) return this.categories
          if (this.categories.length) return this.categories
          throw err
        })
        .finally(() => {
          if (categoriesRequest === request) categoriesRequest = null
        })
      categoriesRequest = request
      return categoriesRequest
    },
    async ensureBooksLoaded(options = {}) {
      this.ensureShelfScope()
      const force = options === true || Boolean(options?.force)
      if (force || !this.booksLoadedAt) {
        return this.loadBooks({ all: true, ...normalizeLoadOptions(options) })
      }
      return this.books
    },
    async ensureCategoriesLoaded(options = {}) {
      this.ensureShelfScope()
      const force = options === true || Boolean(options?.force)
      if (force || (!this.categories.length && !this.categoriesLoadedAt)) {
        return this.loadCategories(normalizeLoadOptions(options))
      }
      return this.categories
    },
    invalidateBooks() {
      this.booksLoadedAt = 0
      this.booksLoadedKey = ''
    },
    invalidateCategories() {
      this.categoriesLoadedAt = 0
    },
    invalidateShelf() {
      this.invalidateBooks()
      this.invalidateCategories()
    },
    async addCategory(category) {
      const { data } = await createCategory(category)
      const index = this.categories.findIndex(item => Number(item.id) === Number(data.id))
      this.categories = sortCategories(index >= 0
        ? this.categories.map(item => Number(item.id) === Number(data.id) ? data : item)
        : [...this.categories, data])
      this.invalidateCategories()
      return data
    },
    async selectCategory(categoryId) {
      this.selectedCategoryId = categoryId
      await this.loadBooks({ force: true })
    },
    async addBook(book) {
      const { data } = await createBook(book)
      booksRevision.mutate(this.shelfScope)
      this.books = sortBooks([data, ...this.books])
      this.invalidateBooks()
      syncCachedBookUpsert(data)
      return data
    },
    async removeBook(bookId) {
      const book = this.books.find(item => Number(item.id) === Number(bookId))
      await deleteBook(bookId)
      await clearDeletedBookBrowserCache(book, bookId)
      booksRevision.mutate(this.shelfScope)
      this.books = this.books.filter(book => book.id !== bookId)
      this.invalidateBooks()
      syncCachedBookRemoval(bookId)
    },
    removeBookLocal(bookId) {
      const book = this.books.find(item => Number(item.id) === Number(bookId))
      clearDeletedBookBrowserCache(book, bookId)
      booksRevision.mutate(this.shelfScope)
      this.books = this.books.filter(book => Number(book.id) !== Number(bookId))
      this.invalidateBooks()
      syncCachedBookRemoval(bookId)
    },
    upsertBook(book) {
      if (!book?.id) return
      booksRevision.mutate(this.shelfScope)
      const index = this.books.findIndex(item => Number(item.id) === Number(book.id))
      const nextBook = index >= 0 ? mergeShelfBook(this.books[index], book) : book
      if (nextBook?.progress?.bookId) {
        const reader = useReaderStore()
        reader.applyServerProgress(nextBook.progress)
      }
      const nextBooks = index >= 0
        ? this.books.map(item => Number(item.id) === Number(book.id) ? nextBook : item)
        : [nextBook, ...this.books]
      this.books = sortBooks(nextBooks)
      this.invalidateBooks()
      syncCachedBookUpsert(nextBook)
    },
    replaceCategories(categories) {
      categoryOperations.invalidate('categories')
      this.categories = sortCategories(categories)
      this.categoriesLoadedAt = Date.now()
      writeShelfCache(scopedShelfCacheKey(CATEGORY_CACHE_KEY), this.categories)
    },
    upsertCategory(category) {
      if (!category?.id) return
      categoryOperations.invalidate('categories')
      const index = this.categories.findIndex(item => Number(item.id) === Number(category.id))
      const nextCategories = index >= 0
        ? this.categories.map(item => Number(item.id) === Number(category.id) ? category : item)
        : [...this.categories, category]
      this.categories = sortCategories(nextCategories)
      this.categoriesLoadedAt = Date.now()
      writeShelfCache(scopedShelfCacheKey(CATEGORY_CACHE_KEY), this.categories)
    },
    removeCategoryLocal(categoryId) {
      categoryOperations.invalidate('categories')
      this.categories = this.categories.filter(category => Number(category.id) !== Number(categoryId))
      if (String(this.selectedCategoryId) === String(categoryId)) {
        this.selectedCategoryId = ''
      }
      this.invalidateCategories()
      writeShelfCache(scopedShelfCacheKey(CATEGORY_CACHE_KEY), this.categories)
    },
    applyBookProgress(progress, options = {}) {
      if (!progress?.bookId) return
      let changed = false
      const nextBooks = this.books.map(book => {
        if (Number(book.id) !== Number(progress.bookId)) return book
        const nextProgress = options.replace ? progress : newestProgress(book.progress || null, progress)
        if (nextProgress === book.progress) return book
        changed = true
        return { ...book, progress: nextProgress }
      })
      if (changed) {
        this.books = sortBooks(nextBooks)
        this.booksLoadedAt = Date.now()
        if (this.booksLoadedKey) writeShelfCache(scopedShelfCacheKey(`${SHELF_CACHE_KEY}:${this.booksLoadedKey}`), this.books)
        syncCachedBookProgress(progress, options)
      }
    },
    async batchDeleteBooks(bookIds) {
      const booksByID = new Map(this.books.map(book => [Number(book.id), book]))
      const { data } = await batchBooks({ action: 'delete', bookIds })
      const deletedIds = Array.isArray(data?.deletedIds) && data.deletedIds.length ? data.deletedIds : bookIds
      const deletedSet = new Set(deletedIds.map(id => Number(id)))
      await Promise.all(deletedIds.map(bookId => (
        clearDeletedBookBrowserCache(booksByID.get(Number(bookId)), bookId)
      )))
      booksRevision.mutate(this.shelfScope)
      this.books = this.books.filter(book => !deletedSet.has(Number(book.id)))
      this.invalidateBooks()
      deletedIds.forEach(bookId => syncCachedBookRemoval(bookId))
    },
    async batchSetCategory(bookIds, categoryId, options = {}) {
      const action = options.action || 'category'
      const payload = Array.isArray(categoryId)
        ? { action, bookIds, categoryIds: categoryId }
        : { action, bookIds, categoryId }
      const { data } = await batchBooks(payload)
      const updatedBooks = asList(data?.books)
      if (updatedBooks.length) {
        updatedBooks.forEach(book => this.upsertBook(book))
        return
      }
      const idSet = new Set(bookIds.map(id => Number(id)))
      const nextBooks = this.books.map(book => {
        if (!idSet.has(Number(book.id))) return book
        const categoryIds = nextCategoryIdsForAction(book, categoryId, action)
        return { ...book, categoryId: categoryIds[0] || null, categoryIds }
      })
      booksRevision.mutate(this.shelfScope)
      this.books = sortBooks(nextBooks)
      this.invalidateBooks()
      nextBooks.filter(book => idSet.has(Number(book.id))).forEach(book => syncCachedBookUpsert(book))
    },
    async batchCacheBooks(bookIds) {
      const { data } = await batchBooks({ action: 'cache', bookIds })
      return data
    },
    async batchClearCache(bookIds) {
      const { data } = await batchBooks({ action: 'clear-cache', bookIds })
      return data
    },
    async exportSelectedBooks(bookIds, format = 'json') {
      const { data } = await exportBooks(bookIds, format)
      return data
    },
    async renameCategory(categoryId, payload) {
      const { data } = await updateCategory(categoryId, payload)
      const index = this.categories.findIndex(category => category.id === data.id)
      if (index >= 0) this.categories[index] = data
      this.invalidateCategories()
      return data
    },
    async setCategoryVisible(categoryId, show) {
      const { data } = await updateCategory(categoryId, { show })
      const index = this.categories.findIndex(category => category.id === data.id)
      if (index >= 0) this.categories[index] = data
      this.invalidateCategories()
      return data
    },
    async removeCategory(categoryId) {
      await deleteCategory(categoryId)
      this.categories = this.categories.filter(category => category.id !== categoryId)
      const changedIds = new Set()
      const nextBooks = this.books.map(book => {
        if (!bookHasCategory(book, categoryId)) return book
        changedIds.add(Number(book.id))
        const categoryIds = bookCategoryIds(book).filter(id => String(id) !== String(categoryId))
        return { ...book, categoryId: categoryIds[0] || null, categoryIds }
      })
      booksRevision.mutate(this.shelfScope)
      this.books = sortBooks(nextBooks)
      this.invalidateShelf()
      nextBooks.filter(book => changedIds.has(Number(book.id))).forEach(book => syncCachedBookUpsert(book))
    },
    async reorderCategoryIds(ids) {
      const { data } = await reorderCategories(ids)
      this.categories = asList(data)
      this.invalidateCategories()
      return data
    },
    async importTXT({ file, importToken, title, author, categoryId, categoryIds = [], tocRule }) {
      const form = new FormData()
      if (importToken) form.append('importToken', importToken)
      else form.append('file', file)
      if (title) form.append('title', title)
      if (author) form.append('author', author)
      const normalizedCategoryIds = Array.isArray(categoryIds)
        ? categoryIds.map(id => Number(id)).filter(id => Number.isFinite(id) && id > 0)
        : []
      if (normalizedCategoryIds.length) {
        normalizedCategoryIds.forEach(id => form.append('categoryIds', String(id)))
      } else if (categoryId) {
        form.append('categoryId', categoryId)
      }
      if (tocRule) form.append('tocRule', tocRule)

      const { data } = await api.post('/imports/books', form, {
        headers: { 'Content-Type': 'multipart/form-data' },
        timeout: 10 * 60 * 1000,
      })
      this.upsertBook(data)
      return data
    },
  },
})

async function readShelfCache(key) {
  try {
    return asList(await getBrowserCache(key))
  } catch {
    return []
  }
}

async function readShelfCacheEntry(key) {
  try {
    const cached = await getBrowserCache(key)
    return cached === null || cached === undefined ? null : asList(cached)
  } catch {
    return null
  }
}

function writeShelfCache(key, value) {
  setBrowserCache(key, asList(value)).catch(() => {})
}

function scopedShelfCacheKey(key, scope = currentUserScope()) {
  return `${key}:${scope}`
}

function syncServerProgressFromBooks(books) {
  const reader = useReaderStore()
  asList(books).forEach(book => {
    if (book?.progress?.bookId) reader.applyServerProgress(book.progress)
  })
}

async function syncCachedBookProgress(progress, options = {}) {
  if (!progress?.bookId) return
  try {
    const keys = await listBrowserCacheKeys(SHELF_CACHE_KEY)
    const scopedKeys = keys.filter(isCurrentUserShelfCacheKey)
    await Promise.all(scopedKeys.map(async (key) => {
      const cached = asList(await getBrowserCache(key))
      if (!cached.length) return
      let changed = false
      const next = cached.map(book => {
        if (Number(book.id) !== Number(progress.bookId)) return book
        const nextProgress = options.replace ? progress : newestProgress(book.progress || null, progress)
        if (nextProgress === book.progress) return book
        changed = true
        return { ...book, progress: nextProgress }
      })
      if (changed) await setBrowserCache(key, sortBooks(next))
    }))
  } catch {
    // Shelf memory state is authoritative; cache sync is a best-effort fast resume path.
  }
}

function isCurrentUserShelfCacheKey(key) {
  const value = String(key || '')
  const unprefixed = value.startsWith('localCache@') ? value.slice('localCache@'.length) : value
  return unprefixed.startsWith(`${SHELF_CACHE_KEY}:`) && unprefixed.endsWith(`:${currentUserScope()}`)
}

async function syncCachedBookUpsert(book) {
  if (!book?.id) return
  await mutateCachedShelfLists((rows, requestParams) => {
    const index = rows.findIndex(item => Number(item.id) === Number(book.id))
    if (index >= 0) {
      if (!matchesShelfRequest(book, requestParams)) {
        return rows.filter(item => Number(item.id) !== Number(book.id))
      }
      return rows.map(item => Number(item.id) === Number(book.id) ? mergeShelfBook(item, book) : item)
    }
    if (matchesShelfRequest(book, requestParams)) return [book, ...rows]
    return rows
  })
}

async function syncCachedBookRemoval(bookId) {
  if (!bookId) return
  await mutateCachedShelfLists(rows => rows.filter(book => Number(book.id) !== Number(bookId)))
}

async function mutateCachedShelfLists(mutator) {
  try {
    const keys = (await listBrowserCacheKeys(SHELF_CACHE_KEY)).filter(isCurrentUserShelfCacheKey)
    await Promise.all(keys.map(async (key) => {
      const cached = asList(await getBrowserCache(key))
      if (!cached.length) return
      const next = asList(mutator(cached, shelfRequestParamsFromCacheKey(key)))
      if (sameBookIdList(cached, next) && cached.every((book, index) => book === next[index])) return
      await setBrowserCache(key, sortBooks(next))
    }))
  } catch {
    // Cache updates are best-effort; the in-memory shelf and next network load remain authoritative.
  }
}

function shelfRequestParamsFromCacheKey(key) {
  const value = String(key || '')
  const unprefixed = value.startsWith('localCache@') ? value.slice('localCache@'.length) : value
  const suffix = `:${currentUserScope()}`
  if (!unprefixed.startsWith(`${SHELF_CACHE_KEY}:`) || !unprefixed.endsWith(suffix)) return {}
  const requestKey = unprefixed.slice(`${SHELF_CACHE_KEY}:`.length, -suffix.length)
  try {
    return JSON.parse(requestKey || '{}') || {}
  } catch {
    return {}
  }
}

function matchesShelfRequest(book, requestParams = {}) {
  if (!requestParams.categoryId) return true
  if (requestParams.categoryId === 'none') return bookCategoryIds(book).length === 0
  return bookHasCategory(book, requestParams.categoryId)
}

function sameBookIdList(a, b) {
  if (a.length !== b.length) return false
  return a.every((book, index) => Number(book.id) === Number(b[index]?.id))
}

export function mergeShelfBook(current, incoming) {
  if (!current) return incoming
  const next = { ...current, ...incoming }
  next.categoryIds = bookCategoryIds(next)
  next.categoryId = next.categoryIds[0] || null
  const progress = newestProgress(current.progress || null, incoming?.progress || null)
  if (progress) next.progress = progress
  next.shelfOrderAt = newestShelfOrderAt(current.shelfOrderAt, incoming?.shelfOrderAt)
  return next
}

export function bookHasCategory(book, categoryId) {
  return bookCategoryIds(book).some(id => String(id) === String(categoryId))
}

function nextCategoryIdsForAction(book, categoryIdOrIds, action) {
  const current = bookCategoryIds(book)
  const targetIds = Array.isArray(categoryIdOrIds)
    ? categoryIdOrIds.map(id => Number(id)).filter(id => Number.isFinite(id) && id > 0)
    : (categoryIdOrIds ? [Number(categoryIdOrIds)] : [])
  if (action === 'category-add') {
    return [...new Set([...current, ...targetIds])]
  }
  if (action === 'category-remove') {
    const removeSet = new Set(targetIds.map(id => String(id)))
    return current.filter(id => !removeSet.has(String(id)))
  }
  return [...new Set(targetIds)]
}

function newestShelfOrderAt(current, incoming) {
  const currentTime = Date.parse(current || '')
  const incomingTime = Date.parse(incoming || '')
  if (Number.isFinite(currentTime) && Number.isFinite(incomingTime)) {
    return incomingTime > currentTime ? incoming : current
  }
  return incoming || current
}

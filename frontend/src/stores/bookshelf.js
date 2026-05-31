import { defineStore } from 'pinia'
import { batchBooks, createBook, deleteBook, exportBooks, listBooks } from '../api/books'
import { createCategory, deleteCategory, listCategories, reorderCategories, updateCategory } from '../api/categories'
import api from '../api/client'
import { newestProgress, sortByShelfOrder } from '../utils/bookOrder'
import { getBrowserCache, setBrowserCache } from '../utils/browserCache'

function asList(data) {
  if (Array.isArray(data)) return data
  if (Array.isArray(data?.list)) return data.list
  if (Array.isArray(data?.items)) return data.items
  if (Array.isArray(data?.data)) return data.data
  return []
}

function sortBooks(books) {
  return sortByShelfOrder(asList(books))
}

const REFRESH_DEDUPE_MS = 1200
const SHELF_CACHE_KEY = 'bookshelf@getBookshelf'
const CATEGORY_CACHE_KEY = 'bookshelf@getCategories'
let booksRequest = null
let booksRequestKey = ''
let categoriesRequest = null

export const useBookshelfStore = defineStore('bookshelf', {
  state: () => ({
    books: [],
    categories: [],
    selectedCategoryId: '',
    loading: false,
    booksLoadedAt: 0,
    booksLoadedKey: '',
    categoriesLoadedAt: 0,
  }),
  actions: {
    async loadBooks(options = {}) {
      const force = options === true || Boolean(options?.force)
      const all = Boolean(options?.all)
      const params = {}
      if (!all && this.selectedCategoryId) {
        params.categoryId = this.selectedCategoryId
      }
      const requestKey = JSON.stringify(params)
      const now = Date.now()
      if (!force && this.books.length > 0 && this.booksLoadedKey === requestKey) {
        return this.books
      }
      if (!force && this.booksLoadedKey === requestKey && this.booksLoadedAt > 0 && now - this.booksLoadedAt < REFRESH_DEDUPE_MS) {
        return this.books
      }
      if (!force && booksRequest && booksRequestKey === requestKey) return booksRequest

      if (!force && this.books.length === 0) {
        const cached = await readShelfCache(`${SHELF_CACHE_KEY}:${requestKey}`)
        if (cached.length) {
          this.books = sortBooks(cached)
          this.booksLoadedAt = Date.now()
          this.booksLoadedKey = requestKey
        }
      }

      this.loading = this.books.length === 0
      booksRequestKey = requestKey
      const request = listBooks(params)
        .then(({ data }) => {
          this.books = sortBooks(data)
          this.booksLoadedAt = Date.now()
          this.booksLoadedKey = requestKey
          writeShelfCache(`${SHELF_CACHE_KEY}:${requestKey}`, this.books)
          return this.books
        })
        .catch((err) => {
          if (this.books.length) return this.books
          throw err
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
      const force = options === true || Boolean(options?.force)
      const now = Date.now()
      if (!force && this.categoriesLoadedAt > 0 && now - this.categoriesLoadedAt < REFRESH_DEDUPE_MS) {
        return this.categories
      }
      if (!force && categoriesRequest) return categoriesRequest

      if (!force && this.categories.length === 0) {
        const cached = await readShelfCache(CATEGORY_CACHE_KEY)
        if (cached.length) {
          this.categories = cached
          this.categoriesLoadedAt = Date.now()
        }
      }

      const request = listCategories()
        .then(({ data }) => {
          this.categories = asList(data)
          this.categoriesLoadedAt = Date.now()
          writeShelfCache(CATEGORY_CACHE_KEY, this.categories)
          return this.categories
        })
        .catch((err) => {
          if (this.categories.length) return this.categories
          throw err
        })
        .finally(() => {
          if (categoriesRequest === request) categoriesRequest = null
        })
      categoriesRequest = request
      return categoriesRequest
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
      this.categories.push(data)
      this.categories.sort((a, b) => (a.sortOrder || 0) - (b.sortOrder || 0) || a.name.localeCompare(b.name))
      this.invalidateCategories()
      return data
    },
    async selectCategory(categoryId) {
      this.selectedCategoryId = categoryId
      await this.loadBooks({ force: true })
    },
    async addBook(book) {
      const { data } = await createBook(book)
      this.books = sortBooks([data, ...this.books])
      this.invalidateBooks()
      return data
    },
    async removeBook(bookId) {
      await deleteBook(bookId)
      this.books = this.books.filter(book => book.id !== bookId)
      this.invalidateBooks()
    },
    upsertBook(book) {
      if (!book?.id) return
      const index = this.books.findIndex(item => item.id === book.id)
      const nextBooks = index >= 0
        ? this.books.map(item => item.id === book.id ? book : item)
        : [book, ...this.books]
      this.books = sortBooks(nextBooks)
      this.invalidateBooks()
    },
    applyBookProgress(progress) {
      if (!progress?.bookId) return
      let changed = false
      const nextBooks = this.books.map(book => {
        if (Number(book.id) !== Number(progress.bookId)) return book
        const nextProgress = newestProgress(book.progress || null, progress)
        if (nextProgress === book.progress) return book
        changed = true
        return { ...book, progress: nextProgress }
      })
      if (changed) {
        this.books = sortBooks(nextBooks)
        this.booksLoadedAt = Date.now()
        if (this.booksLoadedKey) writeShelfCache(`${SHELF_CACHE_KEY}:${this.booksLoadedKey}`, this.books)
      }
    },
    async batchDeleteBooks(bookIds) {
      await batchBooks({ action: 'delete', bookIds })
      this.books = this.books.filter(book => !bookIds.includes(book.id))
      this.invalidateBooks()
    },
    async batchSetCategory(bookIds, categoryId) {
      await batchBooks({ action: 'category', bookIds, categoryId })
      this.books = sortBooks(this.books.map(book => bookIds.includes(book.id) ? { ...book, categoryId } : book))
      this.invalidateBooks()
    },
    async batchCacheBooks(bookIds) {
      const { data } = await batchBooks({ action: 'cache', bookIds })
      return data
    },
    async batchClearCache(bookIds) {
      const { data } = await batchBooks({ action: 'clear-cache', bookIds })
      return data
    },
    async exportSelectedBooks(bookIds) {
      const { data } = await exportBooks(bookIds)
      return data
    },
    async renameCategory(categoryId, payload) {
      const { data } = await updateCategory(categoryId, payload)
      const index = this.categories.findIndex(category => category.id === data.id)
      if (index >= 0) this.categories[index] = data
      this.invalidateCategories()
      return data
    },
    async removeCategory(categoryId) {
      await deleteCategory(categoryId)
      this.categories = this.categories.filter(category => category.id !== categoryId)
      this.books = sortBooks(this.books.map(book => String(book.categoryId) === String(categoryId) ? { ...book, categoryId: null } : book))
      this.invalidateShelf()
    },
    async reorderCategoryIds(ids) {
      const { data } = await reorderCategories(ids)
      this.categories = asList(data)
      this.invalidateCategories()
      return data
    },
    async importTXT({ file, title, author, categoryId }) {
      const form = new FormData()
      form.append('file', file)
      if (title) form.append('title', title)
      if (author) form.append('author', author)
      if (categoryId) form.append('categoryId', categoryId)

      const { data } = await api.post('/imports/books', form, {
        headers: { 'Content-Type': 'multipart/form-data' },
      })
      await this.loadBooks({ force: true, all: true })
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

function writeShelfCache(key, value) {
  setBrowserCache(key, asList(value)).catch(() => {})
}

import { defineStore } from 'pinia'
import { batchBooks, createBook, deleteBook, exportBooks, listBooks } from '../api/books'
import { createCategory, deleteCategory, listCategories, reorderCategories, updateCategory } from '../api/categories'
import api from '../api/client'
import { sortByShelfOrder } from '../utils/bookOrder'

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

export const useBookshelfStore = defineStore('bookshelf', {
  state: () => ({
    books: [],
    categories: [],
    selectedCategoryId: '',
    loading: false,
  }),
  actions: {
    async loadBooks() {
      this.loading = true
      try {
        const params = {}
        if (this.selectedCategoryId) {
          params.categoryId = this.selectedCategoryId
        }
        const { data } = await listBooks(params)
        this.books = sortBooks(data)
      } finally {
        this.loading = false
      }
    },
    async loadCategories() {
      const { data } = await listCategories()
      this.categories = asList(data)
    },
    async addCategory(category) {
      const { data } = await createCategory(category)
      this.categories.push(data)
      this.categories.sort((a, b) => (a.sortOrder || 0) - (b.sortOrder || 0) || a.name.localeCompare(b.name))
      return data
    },
    async selectCategory(categoryId) {
      this.selectedCategoryId = categoryId
      await this.loadBooks()
    },
    async addBook(book) {
      const { data } = await createBook(book)
      this.books = sortBooks([data, ...this.books])
      return data
    },
    async removeBook(bookId) {
      await deleteBook(bookId)
      this.books = this.books.filter(book => book.id !== bookId)
    },
    upsertBook(book) {
      if (!book?.id) return
      const index = this.books.findIndex(item => item.id === book.id)
      const nextBooks = index >= 0
        ? this.books.map(item => item.id === book.id ? book : item)
        : [book, ...this.books]
      this.books = sortBooks(nextBooks)
    },
    async batchDeleteBooks(bookIds) {
      await batchBooks({ action: 'delete', bookIds })
      this.books = this.books.filter(book => !bookIds.includes(book.id))
    },
    async batchSetCategory(bookIds, categoryId) {
      await batchBooks({ action: 'category', bookIds, categoryId })
      this.books = sortBooks(this.books.map(book => bookIds.includes(book.id) ? { ...book, categoryId } : book))
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
      return data
    },
    async removeCategory(categoryId) {
      await deleteCategory(categoryId)
      this.categories = this.categories.filter(category => category.id !== categoryId)
      this.books = sortBooks(this.books.map(book => String(book.categoryId) === String(categoryId) ? { ...book, categoryId: null } : book))
    },
    async reorderCategoryIds(ids) {
      const { data } = await reorderCategories(ids)
      this.categories = asList(data)
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
      await this.loadBooks()
      return data
    },
  },
})

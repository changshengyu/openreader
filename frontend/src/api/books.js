import api from './client'

export function listBooks(params = {}) {
  return api.get('/books', { params })
}

export function createBook(payload) {
  return api.post('/books', payload)
}

export function createRemoteBook(payload) {
  return api.post('/books/remote', payload)
}

export function checkBookUpdates(payload = {}) {
  return api.post('/books/check-updates', payload)
}

export function batchBooks(payload) {
  return api.post('/books/batch', payload)
}

export function exportBooks(bookIds) {
  return api.post('/books/export', { bookIds }, { responseType: 'blob' })
}

export function getBook(id) {
  return api.get(`/books/${id}`)
}

export function updateBook(id, payload) {
  return api.put(`/books/${id}`, payload)
}

export function deleteBook(id) {
  return api.delete(`/books/${id}`)
}

export function refreshBook(id) {
  return api.post(`/books/${id}/refresh`)
}

export function refreshLocalBook(id, payload = undefined) {
  return api.post(`/books/${id}/refresh-local`, payload)
}

export function listTXTTocRules() {
  return api.get('/txt-toc-rules')
}

export function cacheBookContent(id, payload) {
  return api.post(`/books/${id}/cache`, payload)
}

export function updateBookCategory(id, categoryId) {
  return api.put(`/books/${id}/category`, { categoryId })
}

export function listBookSourceCandidates(id, params = {}) {
  return api.get(`/books/${id}/source-candidates`, { params })
}

export function changeBookSource(id, payload) {
  return api.post(`/books/${id}/change-source`, typeof payload === 'object' ? payload : { sourceId: payload })
}

export function searchBookContent(id, keyword, params = {}) {
  return api.get(`/books/${id}/search`, { params: { q: keyword, ...params } })
}

export function listChapters(id) {
  return api.get(`/books/${id}/chapters`)
}

export function getChapterContent(id, index) {
  return api.get(`/books/${id}/chapters/${index}/content`)
}

export function listBookmarks(id) {
  return api.get(`/books/${id}/bookmarks`)
}

export function createBookmark(id, payload) {
  return api.post(`/books/${id}/bookmarks`, payload)
}

export function updateBookmark(id, payload) {
  return api.put(`/bookmarks/${id}`, payload)
}

export function deleteBookmark(id) {
  return api.delete(`/bookmarks/${id}`)
}

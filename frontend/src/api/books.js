import api from './client.js'

export function listBooks(params = {}) {
  return api.get('/books', { params })
}

export function createBook(payload) {
  return api.post('/books', payload)
}

export function createRemoteBook(payload) {
  return api.post('/books/remote', payload)
}

export function previewLocalBook(file, payload = {}) {
  const form = new FormData()
  form.append('file', file)
  if (payload.title) form.append('title', payload.title)
  if (payload.author) form.append('author', payload.author)
  if (payload.tocRule) form.append('tocRule', payload.tocRule)
  return api.post('/imports/books/preview', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
}

export function checkBookUpdates(payload = {}) {
  return api.post('/books/check-updates', payload)
}

export function batchBooks(payload) {
  return api.post('/books/batch', payload)
}

export function exportBooks(bookIds, format = 'json') {
  return api.post('/books/export', { bookIds, format }, { responseType: 'blob' })
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

export function updateBookCategory(id, categoryIdOrIds) {
  if (Array.isArray(categoryIdOrIds)) {
    return api.put(`/books/${id}/category`, { categoryIds: categoryIdOrIds })
  }
  return api.put(`/books/${id}/category`, { categoryId: categoryIdOrIds })
}

export function listBookSourceCandidates(id, params = {}) {
  return api.get(`/books/${id}/source-candidates`, { params })
}

export function changeBookSource(id, payload) {
  return api.post(`/books/${id}/change-source`, typeof payload === 'object' ? payload : { sourceId: payload })
}

export function searchBookContent(id, keyword, params = {}) {
  return api.get(`/books/${id}/search`, {
    params: { q: keyword, ...params },
    timeout: 60000,
  })
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

export function createBookmarks(id, payloads) {
  return api.post(`/books/${id}/bookmarks/batch`, payloads)
}

export function updateBookmark(id, payload) {
  return api.put(`/bookmarks/${id}`, payload)
}

export function deleteBookmark(id) {
  return api.delete(`/bookmarks/${id}`)
}

export function deleteBookmarks(id, bookmarkIds) {
  return api.post(`/books/${id}/bookmarks/batch-delete`, { ids: bookmarkIds })
}

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
  if (payload.importToken) form.append('importToken', payload.importToken)
  else if (file) form.append('file', file)
  if (payload.title) form.append('title', payload.title)
  if (payload.author) form.append('author', payload.author)
  if (payload.tocRule) form.append('tocRule', payload.tocRule)
  return api.post('/imports/books/preview', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
    timeout: 10 * 60 * 1000,
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

export async function cacheBookContentStream(id, payload, options = {}) {
  const token = typeof localStorage === 'undefined'
    ? ''
    : localStorage.getItem('openreader_token')
  const response = await fetch(`/api/books/${id}/cache/stream`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: JSON.stringify(payload || {}),
    signal: options.signal,
  })
  if (!response.ok) {
    let data = null
    try {
      data = await response.json()
    } catch {
      // Keep proxy-generated non-JSON errors client-safe.
    }
    if (response.status === 401 && token && typeof window !== 'undefined') {
      if (localStorage.getItem('openreader_token') === token) {
        localStorage.removeItem('openreader_token')
      }
      window.__openreaderAuthRequired = { reason: 'session', rejectedToken: token }
      window.dispatchEvent(new CustomEvent('openreader:auth-required', {
        detail: window.__openreaderAuthRequired,
      }))
    }
    const error = new Error(data?.error || '缓存章节失败')
    error.response = { status: response.status, data }
    throw error
  }
  if (!response.body) throw new Error('缓存进度流不可用')

  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  let terminal = null
  const dispatch = block => {
    const lines = block.split(/\r?\n/)
    let event = 'message'
    const dataLines = []
    for (const line of lines) {
      if (line.startsWith('event:')) event = line.slice(6).trim() || 'message'
      if (line.startsWith('data:')) dataLines.push(line.slice(5).trimStart())
    }
    if (!dataLines.length) return
    let data
    try {
      data = JSON.parse(dataLines.join('\n'))
    } catch {
      return
    }
    options.onEvent?.({ event, data })
    if (event === 'error') {
      const error = new Error(data?.error || '缓存章节失败')
      error.stream = data
      throw error
    }
    if (event === 'end') terminal = data
  }

  try {
    while (true) {
      const { done, value } = await reader.read()
      buffer += decoder.decode(value || new Uint8Array(), { stream: !done })
      let boundary = buffer.indexOf('\n\n')
      while (boundary >= 0) {
        dispatch(buffer.slice(0, boundary))
        buffer = buffer.slice(boundary + 2)
        boundary = buffer.indexOf('\n\n')
      }
      if (done) break
    }
  } finally {
    reader.releaseLock()
  }
  if (!terminal) throw new Error('缓存进度流意外结束')
  return terminal
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

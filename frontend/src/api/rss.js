import api from './client'

export function listRSSSources() {
  return api.get('/rss/sources')
}

export function createRSSSource(payload) {
  return api.post('/rss/sources', payload)
}

export function updateRSSSource(id, payload) {
  return api.put(`/rss/sources/${id}`, payload)
}

export function deleteRSSSource(id) {
  return api.delete(`/rss/sources/${id}`)
}

export function refreshRSSSource(id, params = {}) {
  return api.post(`/rss/sources/${id}/refresh`, null, { params })
}

export function listRSSArticles(params = {}) {
  return api.get('/rss/articles', { params })
}

export function getRSSArticleContent(id, params = {}) {
  return api.get(`/rss/articles/${id}/content`, { params })
}

export function updateRSSArticle(id, payload) {
  return api.put(`/rss/articles/${id}`, payload)
}

import api from './client'

export function listSources() {
  return api.get('/sources')
}

export function listInvalidSources() {
  return api.get('/sources/invalid')
}

export function createSource(payload) {
  return api.post('/sources', payload)
}

export function getSource(id) {
  return api.get(`/sources/${id}`)
}

export function updateSource(id, payload) {
  return api.put(`/sources/${id}`, payload)
}

export function deleteSource(id) {
  return api.delete(`/sources/${id}`)
}

export function clearSources() {
  return api.delete('/sources')
}

export function defaultSourceStatus() {
  return api.get('/sources/default')
}

export function saveDefaultSources() {
  return api.post('/sources/default/save')
}

export function restoreDefaultSources() {
  return api.post('/sources/default/restore')
}

export function batchSources(payload) {
  return api.post('/sources/batch', payload)
}

export function importSources(form) {
  return api.post('/sources/import', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
}

export function exportSources(sourceIds = []) {
  const ids = Array.isArray(sourceIds) ? sourceIds.filter(Boolean) : []
  return api.get('/sources/export', {
    params: ids.length ? { sourceIds: ids.join(',') } : undefined,
    responseType: 'blob',
  })
}

export function importRemoteSource(url) {
  return api.post('/sources/remote', { url })
}

export function previewRemoteSource(url) {
  return api.post('/sources/remote-preview', { url })
}

export function batchTestSources(payload) {
  return api.post('/sources/batch-test', payload)
}

export function testSourceSearch(id, keyword) {
  return api.post(`/sources/${id}/test`, { keyword })
}

export function testSourceChapter(id, bookUrl) {
  return api.post(`/sources/${id}/test-chapter`, { bookUrl })
}

export function testSourceContent(id, chapterUrl) {
  return api.post(`/sources/${id}/test-content`, { chapterUrl })
}

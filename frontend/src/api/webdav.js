import api, { rootApi } from './client'

export function listWebDAV(path = '') {
  return rootApi.get(webdavURL(path), { responseType: 'text' })
}

export function uploadWebDAV({ path = '', file }) {
  return rootApi.put(webdavURL(joinPath(path, file.name)), file, {
    headers: { 'Content-Type': file.type || 'application/octet-stream' },
  })
}

export function createWebDAVDirectory({ path = '', name }) {
  return rootApi({ method: 'MKCOL', url: webdavURL(joinPath(path, name)) })
}

export function renameWebDAV({ path, newPath }) {
  return rootApi({
    method: 'MOVE',
    url: webdavURL(path),
    headers: { Destination: webdavURL(newPath) },
  })
}

export function deleteWebDAV(path) {
  return rootApi.delete(webdavURL(path))
}

export function downloadWebDAV(path) {
  return rootApi.get(webdavURL(path), { responseType: 'blob' })
}

export function importFromWebDAV(paths, categoryIds = []) {
  const items = Array.isArray(paths) && paths.length && typeof paths[0] === 'object' ? paths : []
  return api.post('/webdav/import', items.length ? { items, categoryIds } : { paths, categoryIds })
}

export function previewWebDAVImport(paths) {
  return api.post('/webdav/import-preview', { paths })
}

function webdavURL(path) {
  const clean = String(path || '').replace(/^\/+/, '')
  return `/webdav/${clean.split('/').map(encodeURIComponent).join('/')}`
}

function joinPath(base, name) {
  return [base, name].filter(Boolean).join('/')
}

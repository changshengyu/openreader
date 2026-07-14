import api from './client'

export function listLocalStore(path = '', recursive = undefined) {
  const params = { path }
  if (typeof recursive === 'boolean') params.recursive = recursive ? 1 : 0
  return api.get('/local-store', { params })
}

export function uploadToLocalStore({ path = '', file, files = [] }) {
  const form = new FormData()
  form.append('path', path)
  const selectedFiles = Array.isArray(files) && files.length ? files : [file]
  selectedFiles.filter(Boolean).forEach(item => form.append('file', item))
  return api.post('/local-store/upload', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
}

export function createLocalStoreDirectory({ path = '', name }) {
  return api.post('/local-store/directory', { path, name })
}

export function renameLocalStoreItem({ path, name }) {
  return api.put('/local-store/rename', { path, name })
}

export function deleteFromLocalStore(path) {
  return api.delete('/local-store', { params: { path } })
}

export function downloadFromLocalStore(path) {
  return api.get('/local-store/download', { params: { path }, responseType: 'blob' })
}

export function importFromLocalStore(paths, categoryIds = []) {
  const items = Array.isArray(paths) && paths.length && typeof paths[0] === 'object' ? paths : []
  return api.post('/local-store/import', items.length ? { items, categoryIds } : { paths, categoryIds })
}

export function previewLocalStoreImport(paths) {
  const items = Array.isArray(paths) && paths.length && typeof paths[0] === 'object' ? paths : []
  return api.post('/local-store/import-preview', items.length ? { items } : { paths })
}

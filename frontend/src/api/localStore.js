import api from './client'

export function listLocalStore(path = '', recursive = false) {
  return api.get('/local-store', { params: { path, recursive: recursive ? 1 : 0 } })
}

export function uploadToLocalStore({ path = '', file }) {
  const form = new FormData()
  form.append('path', path)
  form.append('file', file)
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

export function importFromLocalStore(paths, categoryId = null) {
  return api.post('/local-store/import', { paths, categoryId })
}

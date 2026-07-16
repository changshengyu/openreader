import api from './client'

export function triggerBackup() {
  return api.post('/backup/trigger')
}

export function triggerPortableBackup() {
  return api.post('/backup/portable/trigger')
}

export function listBackups() {
  return api.get('/backup/list')
}

export function downloadBackup(name) {
  return api.get(`/backup/download/${encodeURIComponent(name)}`, { responseType: 'blob' })
}

export function restoreLegadoBackup(form) {
  return api.post('/backup/restore-legado', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
}

export function restoreWebDAVBackup(path) {
  return api.post('/backup/restore-webdav', { path })
}

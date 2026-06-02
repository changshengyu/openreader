import api from './client'

export function uploadAsset({ file, type = 'misc' }) {
  const form = new FormData()
  form.append('file', file)
  form.append('type', type)
  return api.post('/uploads', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
}

export function deleteAsset(url) {
  return api.delete('/uploads', { data: { url } })
}

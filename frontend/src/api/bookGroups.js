import api from './client'

export function listBookGroups() {
  return api.get('/book-groups')
}

export function updateBuiltInBookGroup(key, payload) {
  return api.put(`/book-groups/${encodeURIComponent(key)}`, payload)
}

export function reorderBookGroups(keys) {
  return api.put('/book-groups/reorder', { keys })
}

import api from './client'

export function listReplaceRules() {
  return api.get('/replace-rules')
}

export function createReplaceRule(payload) {
  return api.post('/replace-rules', payload)
}

export function upsertReplaceRules(payloads) {
  return api.post('/replace-rules/batch', payloads)
}

export function updateReplaceRule(id, payload) {
  return api.put(`/replace-rules/${id}`, payload)
}

export function deleteReplaceRule(id) {
  return api.delete(`/replace-rules/${id}`)
}

export function deleteReplaceRules(ids) {
  return api.post('/replace-rules/batch-delete', { ids })
}

export function testReplaceRule(payload) {
  return api.post('/replace-rules/test', payload)
}

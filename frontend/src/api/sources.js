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

export async function debugSourceStream(id, keyword, options = {}) {
  const token = localStorage.getItem('openreader_token') || ''
  const response = await fetch(`/api/sources/${id}/debug/stream`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: JSON.stringify({ keyword }),
    signal: options.signal,
  })
  if (!response.ok) {
    const payload = await safeJSON(response)
    if (response.status === 401 && token && localStorage.getItem('openreader_token') === token) {
      localStorage.removeItem('openreader_token')
      window.dispatchEvent(new CustomEvent('openreader:auth-required', {
        detail: { reason: 'session', rejectedToken: token },
      }))
    }
    const error = new Error(payload?.error?.message || payload?.error || `HTTP ${response.status}`)
    error.response = { status: response.status, data: payload }
    throw error
  }
  if (!response.body) throw new Error('调试流不可用')

  const events = []
  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  while (true) {
    const { value, done } = await reader.read()
    buffer += decoder.decode(value || new Uint8Array(), { stream: !done })
    const blocks = buffer.replaceAll('\r\n', '\n').split('\n\n')
    buffer = blocks.pop() || ''
    for (const block of blocks) {
      const event = parseSourceDebugEvent(block)
      if (!event) continue
      events.push(event)
      options.onEvent?.(event)
    }
    if (done) break
  }
  if (buffer.trim()) {
    const event = parseSourceDebugEvent(buffer)
    if (event) {
      events.push(event)
      options.onEvent?.(event)
    }
  }
  return events
}

function parseSourceDebugEvent(block) {
  let type = 'message'
  const data = []
  for (const line of String(block || '').split('\n')) {
    if (line.startsWith('event:')) type = line.slice(6).trim()
    if (line.startsWith('data:')) data.push(line.slice(5).trim())
  }
  if (!data.length) return null
  try {
    return { type, data: JSON.parse(data.join('\n')) }
  } catch {
    return null
  }
}

async function safeJSON(response) {
  try {
    return await response.json()
  } catch {
    return {}
  }
}

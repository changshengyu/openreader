import api from './client.js'

export function createRemoteReaderSession(payload) {
  return api.post('/reader/remote-sessions', payload)
}

export function getRemoteReaderSession(id) {
  return api.get(`/reader/remote-sessions/${encodeURIComponent(id)}`)
}

export function getRemoteReaderChapterContent(id, index, options = {}) {
  return api.get(`/reader/remote-sessions/${encodeURIComponent(id)}/chapters/${index}/content`, {
    signal: options.signal,
  })
}

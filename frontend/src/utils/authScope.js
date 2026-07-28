export function currentUserScope(token = storedAuthToken()) {
  const payload = decodeJWTPayload(token)
  const userId = payload?.userId || payload?.sub || ''
  return userId ? `user:${userId}` : 'anonymous'
}

function storedAuthToken() {
  if (typeof localStorage === 'undefined') return ''
  return localStorage.getItem('openreader_token') || ''
}

function decodeJWTPayload(token) {
  try {
    const part = String(token || '').split('.')[1]
    if (!part) return null
    const normalized = part.replace(/-/g, '+').replace(/_/g, '/')
    const padded = normalized.padEnd(Math.ceil(normalized.length / 4) * 4, '=')
    const decoder = globalThis.atob || window.atob
    return JSON.parse(decoder(padded))
  } catch {
    return null
  }
}

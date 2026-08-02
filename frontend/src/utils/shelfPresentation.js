export function normalizeShelfEditQuery(value) {
  return String(value || '').trim().toLowerCase()
}

export function filterShelfBooksByEditQuery(books, query) {
  const rows = Array.isArray(books) ? books : []
  const normalized = normalizeShelfEditQuery(query)
  if (!normalized) return rows
  return rows.filter(book => (
    String(book?.title || book?.name || '').toLowerCase().includes(normalized)
    || String(book?.author || '').toLowerCase().includes(normalized)
  ))
}

export function relativeShelfTimeLabel(value, now = Date.now()) {
  const timestamp = typeof value === 'number' ? value : Date.parse(value)
  if (!Number.isFinite(timestamp)) return '最新'
  const seconds = Math.max(0, Math.floor((now - timestamp) / 1000))
  if (seconds <= 30) return '刚刚'
  if (seconds < 60) return `${seconds}秒前`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}分钟前`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}小时前`
  if (seconds < 2592000) return `${Math.floor(seconds / 86400)}天前`
  if (seconds < 31536000) return `${Math.floor(seconds / 2592000)}月前`
  return `${Math.floor(seconds / 31536000)}年前`
}

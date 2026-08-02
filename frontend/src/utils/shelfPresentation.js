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

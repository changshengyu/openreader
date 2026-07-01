export function normalizeImportedBookmarks(rows) {
  return (Array.isArray(rows) ? rows : [])
    .map(row => {
      const chapterIndex = Math.max(0, Math.floor(Number(row.chapterIndex ?? row.durChapterIndex ?? 0)))
      return {
        chapterIndex,
        offset: Math.max(0, Math.floor(Number(row.offset ?? 0))),
        percent: clampPercent(row.percent),
        title: String(row.title || row.chapterName || row.chapterTitle || `第 ${chapterIndex + 1} 章`).trim(),
        excerpt: String(row.excerpt || row.bookText || '').trim(),
        note: String(row.note || row.content || '').trim(),
      }
    })
    .filter(row => row.title || row.excerpt || row.note)
}

export function prependBookmarks(current, incoming) {
  return [...(Array.isArray(incoming) ? incoming : []), ...(Array.isArray(current) ? current : [])]
}

export function replaceBookmark(current, bookmark) {
  if (!bookmark?.id) return Array.isArray(current) ? current : []
  return (Array.isArray(current) ? current : []).map(item => (
    String(item.id) === String(bookmark.id) ? bookmark : item
  ))
}

export function removeBookmarkIds(current, ids) {
  const removed = new Set((Array.isArray(ids) ? ids : []).map(id => String(id)))
  return (Array.isArray(current) ? current : []).filter(item => !removed.has(String(item.id)))
}

export function bookmarkUpdateTargetsBook(event, bookId) {
  if (!bookId) return false
  const bookIds = event?.detail?.bookIds || []
  return !bookIds.length || bookIds.some(id => String(id) === String(bookId))
}

export function bookmarkReaderQuery(bookmark) {
  const percent = Number(bookmark?.percent)
  return {
    chapter: bookmark?.chapterIndex,
    offset: bookmark?.offset || 0,
    percent: Number.isFinite(percent) ? percent : undefined,
  }
}

export function parseBookmarkPercent(value) {
  if (value === undefined || value === null || value === '') return null
  const percent = Number(value)
  return Number.isFinite(percent) ? Math.max(0, Math.min(1, percent)) : null
}

function clampPercent(value) {
  const percent = Number(value)
  return Number.isFinite(percent) ? Math.max(0, Math.min(1, percent)) : 0
}

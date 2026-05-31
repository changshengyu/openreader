function toTime(value) {
  const time = new Date(value || 0).getTime()
  return Number.isFinite(time) ? time : 0
}

export function progressUpdatedAt(progress) {
  return toTime(progress?.updatedAt)
}

export function newestProgress(a, b) {
  if (!a) return b || null
  if (!b) return a
  const aTime = progressUpdatedAt(a)
  const bTime = progressUpdatedAt(b)
  if (bTime > aTime) return b
  if (aTime > bTime) return a
  if (b.chapterPercent !== undefined && a.chapterPercent === undefined) return b
  if (b.chapterTitle && !a.chapterTitle) return b
  return a
}

export function newestBookProgress(book, progressByBook) {
  return newestProgress(book?.progress || null, progressByBook?.[book?.id] || null)
}

function progressFor(book, progressByBook) {
  return newestBookProgress(book, progressByBook)
}

export function compareByShelfOrderWithProgress(progressByBook) {
  return (a, b) => {
    const aOrderAt = shelfOrderTime(a, progressByBook)
    const bOrderAt = shelfOrderTime(b, progressByBook)
    if (aOrderAt !== bOrderAt) return bOrderAt - aOrderAt
    return Number(b?.id || 0) - Number(a?.id || 0)
  }
}

export function compareByShelfOrder(a, b) {
  return compareByShelfOrderWithProgress()(a, b)
}

export function shelfOrderTime(book, progressByBook) {
  const explicitShelfAt = toTime(book?.shelfOrderAt)
  const progressAt = toTime(progressFor(book, progressByBook)?.updatedAt)
  const shelfAt = Math.max(toTime(book?.updatedAt), toTime(book?.createdAt))
  return Math.max(explicitShelfAt, progressAt, shelfAt)
}

export function sortByShelfOrder(books, progressByBook) {
  const list = Array.isArray(books) ? books : []
  return [...list].sort(compareByShelfOrderWithProgress(progressByBook))
}

export function compareRecentBook(a, b, progressByBook) {
  const aTime = shelfOrderTime(a, progressByBook)
  const bTime = shelfOrderTime(b, progressByBook)
  if (aTime !== bTime) return bTime - aTime
  return Number(b?.id || 0) - Number(a?.id || 0)
}

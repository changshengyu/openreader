function toTime(value) {
  const time = new Date(value || 0).getTime()
  return Number.isFinite(time) ? time : 0
}

function progressFor(book, progressByBook) {
  return progressByBook?.[book?.id] || book?.progress || null
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
  const progressAt = toTime(progressFor(book, progressByBook)?.updatedAt)
  const shelfAt = Math.max(toTime(book?.updatedAt), toTime(book?.createdAt))
  return Math.max(progressAt, shelfAt)
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

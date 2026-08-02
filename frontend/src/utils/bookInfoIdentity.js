export function bookInfoURL(book) {
  return String(book?.url || book?.bookUrl || book?.bookURL || '').trim()
}

export function findShelfBookByURL(book, shelfBooks = [], options = {}) {
  if (!book) return null
  const rows = Array.isArray(shelfBooks) ? shelfBooks : []
  const targetURL = bookInfoURL(book)
  if (targetURL) {
    return rows.find(item => bookInfoURL(item) === targetURL) || null
  }
  if (!options.allowIdFallback) return null
  const targetID = Number(book.id)
  if (!Number.isInteger(targetID) || targetID <= 0) return null
  return rows.find(item => Number(item?.id) === targetID) || null
}

export function isShelfBookInfo(book, shelfBooks = [], options = {}) {
  return Boolean(findShelfBookByURL(book, shelfBooks, options))
}

// The catalog row the Reader is on: chapters[currentIndex], used only while it belongs to the current book. Shelf
// catalog rows carry their bookId; temporary remote sessions serialize book.id and row.bookId as 0. A book switch
// replaces book before the new catalog arrives, so a row of the previous book is never paired with the new book.
export function readerCatalogRow(book, chapters, index) {
  const position = Number(index)
  if (!Array.isArray(chapters) || !Number.isInteger(position) || position < 0) return null
  const row = chapters[position]
  if (!row || persistedId(row.bookId) !== persistedId(book?.id)) return null
  return row
}

function persistedId(value) {
  const id = Number(value)
  return Number.isFinite(id) && id > 0 ? id : 0
}

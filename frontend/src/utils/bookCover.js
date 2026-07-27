export function bookCoverUrl(book) {
  const custom = String(book?.customCoverUrl || '').trim()
  if (custom) return custom
  if (book && Object.prototype.hasOwnProperty.call(book, 'coverResourceUrl')) {
    return String(book.coverResourceUrl || '').trim()
  }
  return String(book?.coverUrl || '').trim()
}

export function hasBookCover(book) {
  return Boolean(bookCoverUrl(book))
}

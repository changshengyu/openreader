export function bookCoverUrl(book) {
  return String(book?.customCoverUrl || book?.coverUrl || '').trim()
}

export function hasBookCover(book) {
  return Boolean(bookCoverUrl(book))
}

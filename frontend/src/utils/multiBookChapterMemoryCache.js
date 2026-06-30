export function createMultiBookChapterMemoryCache(maxBooks = 3) {
  const limit = Math.max(1, Math.floor(Number(maxBooks) || 1))
  const books = new Map()

  function touch(bookKey, chapters) {
    books.delete(bookKey)
    books.set(bookKey, chapters)
  }

  return {
    get(bookKey, index) {
      if (!bookKey || !books.has(bookKey)) return null
      const chapters = books.get(bookKey)
      touch(bookKey, chapters)
      return chapters.get(index) ?? null
    },

    set(bookKey, index, value) {
      if (!bookKey) return
      const chapters = books.get(bookKey) || new Map()
      chapters.set(index, value)
      touch(bookKey, chapters)
      while (books.size > limit) {
        books.delete(books.keys().next().value)
      }
    },

    clearBook(bookKey) {
      if (bookKey) books.delete(bookKey)
    },

    clearAll() {
      books.clear()
    },

    bookKeys() {
      return [...books.keys()]
    },
  }
}

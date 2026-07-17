export function refreshShelfAfterSyncConnect(bookshelf) {
  return Promise.all([
    bookshelf.loadCategories({ force: true }),
    bookshelf.loadBooks({ force: true, all: true }),
  ])
}

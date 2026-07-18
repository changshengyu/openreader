export function refreshShelfAfterSyncConnect(bookshelf) {
  return Promise.all([
    bookshelf.loadCategories({ force: true }),
    bookshelf.loadBooks({ force: true, all: true }),
  ])
}

export function createShelfForegroundReconciler({
  loadShelf,
  isVisible = () => true,
  now = () => Date.now(),
  interval = 30_000,
}) {
  let lastSuccessAt = 0
  let pending

  function refresh() {
    if (!isVisible()) return Promise.resolve(false)
    if (pending) return pending
    const startedAt = now()
    if (lastSuccessAt && startedAt - lastSuccessAt < interval) {
      return Promise.resolve(false)
    }

    let operation
    try {
      operation = loadShelf({ force: true, all: true })
    } catch (error) {
      return Promise.reject(error)
    }
    pending = Promise.resolve(operation)
      .then(() => {
        lastSuccessAt = now()
        return true
      })
      .finally(() => {
        pending = undefined
      })
    return pending
  }

  return { refresh }
}

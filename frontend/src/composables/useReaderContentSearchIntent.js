import { unref, watch } from 'vue'

export function useReaderContentSearchIntent(options) {
  return watch(
    () => unref(options.request)?.requestId,
    async (requestId) => {
      const request = unref(options.request)
      if (!requestId || !request || !matchesCurrentBook(request, options)) return
      await options.jumpToResult({
        ...(request.result || {}),
        query: request.query || request.result?.query || '',
      })
    },
  )
}

function matchesCurrentBook(request, options) {
  const book = unref(options.book)
  const currentBookId = Number(book?.id || unref(options.bookId))
  const requestBookId = Number(request.bookId)
  if (
    Number.isInteger(currentBookId)
    && currentBookId > 0
    && Number.isInteger(requestBookId)
    && requestBookId > 0
  ) {
    return currentBookId === requestBookId
  }
  const currentBookURL = String(book?.bookUrl || book?.url || '').trim()
  const requestBookURL = String(request.bookUrl || '').trim()
  return Boolean(currentBookURL && requestBookURL && currentBookURL === requestBookURL)
}

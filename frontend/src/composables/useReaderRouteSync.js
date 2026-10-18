import { unref, watch } from 'vue'
import { parseReaderRoutePercent } from '../utils/readerRoute.js'

export function useReaderRouteSync(options) {
  let suppressedPositionKey = ''

  function positionKey(values) {
    const position = Array.isArray(values)
      ? values
      : [values?.chapter, values?.offset, values?.percent]
    return position.map(value => value === undefined || value === null ? '' : String(value)).join('|')
  }

  function suppressNextPositionReload(position) {
    suppressedPositionKey = positionKey(position)
  }

  watch(options.bookId, async () => {
    options.onBookLoadStart?.()
    try {
      await options.loadBook()
    } catch (error) {
      options.onBookLoadError?.(error)
    }
  })

  // A route change that also switches the book belongs to the book load: it applies the new catalog first and then
  // locates the chapter and route line itself. The previous catalog must not pick or load a chapter for the new book.
  watch([options.bookId, options.positionQuery], async ([bookId, [chapter, offset, percent]], [previousBookId]) => {
    if (bookId !== previousBookId) {
      suppressedPositionKey = ''
      return
    }
    if (suppressedPositionKey) {
      const shouldSuppress = positionKey([chapter, offset, percent]) === suppressedPositionKey
      suppressedPositionKey = ''
      if (shouldSuppress) {
        await options.jumpToRouteLine()
        return
      }
    }
    const index = Number(chapter || 0)
    const nextOffset = Number(offset || 0)
    const restorePercent = parseReaderRoutePercent(percent)
    const shouldLoad = (
      index !== unref(options.currentIndex)
      || offset !== undefined
      || restorePercent !== null
    )
    if (shouldLoad) {
      await options.loadChapter(index, nextOffset, {
        restorePercent,
        saveAfterLoad: true,
      })
    }
    await options.jumpToRouteLine()
  })

  watch([options.bookId, options.searchQuery], async ([bookId], [previousBookId]) => {
    if (bookId !== previousBookId) return
    await options.jumpToRouteLine()
  })

  return {
    suppressNextPositionReload,
  }
}

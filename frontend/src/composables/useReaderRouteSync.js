import { unref, watch } from 'vue'
import { parseReaderRoutePercent } from '../utils/readerRoute.js'

export function useReaderRouteSync(options) {
  watch(options.bookId, async () => {
    options.onBookLoadStart?.()
    try {
      await options.loadBook()
    } catch (error) {
      options.onBookLoadError?.(error)
    }
  })

  watch(options.positionQuery, async ([chapter, offset, percent]) => {
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

  watch(options.searchQuery, async () => {
    await options.jumpToRouteLine()
  })
}

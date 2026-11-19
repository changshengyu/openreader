import { unref, watch } from 'vue'
import { isNavigationFailure, NavigationFailureType } from 'vue-router'
import { parseReaderRoutePercent } from '../utils/readerRoute.js'

export function useReaderRouteSync(options) {
  let pendingPosition = null

  async function replacePosition(location) {
    const router = options.router
    const position = { fullPath: router.resolve(location).fullPath }
    pendingPosition = position
    try {
      const failure = await router.replace(location)
      return (!failure || isNavigationFailure(failure, NavigationFailureType.duplicated))
        && router.currentRoute.value.fullPath === position.fullPath
    } finally {
      if (pendingPosition === position) pendingPosition = null
    }
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
    const position = pendingPosition
    pendingPosition = null
    if (bookId !== previousBookId) return
    if (position && position.fullPath === options.router.currentRoute.value.fullPath) {
      await options.jumpToRouteLine()
      return
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
    replacePosition,
  }
}

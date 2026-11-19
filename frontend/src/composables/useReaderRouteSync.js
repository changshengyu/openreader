import { unref, watch } from 'vue'
import { isNavigationFailure, NavigationFailureType } from 'vue-router'
import { parseReaderRoutePercent } from '../utils/readerRoute.js'

const readerPositionStateKey = 'openReaderPositionNavigation'
const readerPositionSession = Math.random().toString(36).slice(2)
let nextPositionId = 0

export function useReaderRouteSync(options) {
  let pendingPosition = null

  async function replacePosition(location) {
    const router = options.router
    const { fullPath, path, query, hash } = router.resolve(location)
    const position = { fullPath, id: `${readerPositionSession}:${++nextPositionId}` }
    pendingPosition = position
    try {
      const destination = typeof location === 'string' ? { path, query, hash } : location
      const failure = await router.replace({
        ...destination,
        state: { ...destination.state, [readerPositionStateKey]: position.id },
      })
      if (router.currentRoute.value.fullPath !== fullPath) return false
      if (isNavigationFailure(failure, NavigationFailureType.duplicated)) return true
      return !failure && router.options.history.state?.[readerPositionStateKey] === position.id
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
    // A user navigation to the same URL can commit while the internal replace is still pending.
    if (position && position.fullPath === options.router.currentRoute.value.fullPath
      && options.router.options.history.state?.[readerPositionStateKey] === position.id) {
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

import { unref, watch } from 'vue'
import { parseReaderRoutePercent } from '../utils/readerRoute.js'

export function useReaderRouteSync(options) {
  let suppressedPositionKey = ''
  // Set while the route change that opened another book is being flushed. The same change also changes the
  // position query, but the book load positions itself from the route once the new catalog arrives; loading a
  // chapter here would use the previous book's catalog (running title contract R5).
  let openingBook = false

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
    openingBook = true
    suppressedPositionKey = ''
    // Both watchers of one route change run in the same scheduler flush; this resets after it.
    Promise.resolve().then(() => {
      openingBook = false
    })
    options.onBookLoadStart?.()
    try {
      await options.loadBook()
    } catch (error) {
      options.onBookLoadError?.(error)
    }
  })

  watch(options.positionQuery, async ([chapter, offset, percent]) => {
    if (openingBook) {
      openingBook = false
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

  watch(options.searchQuery, async () => {
    await options.jumpToRouteLine()
  })

  return {
    suppressNextPositionReload,
  }
}

import { unref } from 'vue'
import {
  parseReaderRoutePercent,
  savedBookChapterPercent,
} from '../utils/readerRoute.js'

export function useReaderBookLoad(options) {
  async function reconcileServerProgress(serverSaved, reconcileOptions = {}) {
    const currentBookId = unref(options.bookId)
    if (!serverSaved?.bookId || Number(serverSaved.bookId) !== Number(currentBookId)) return
    const routeQuery = options.getRouteQuery()
    const canFollowServer = (
      reconcileOptions.resumeFromProgress
      || routeQuery.chapter === undefined
    )
    if (
      !canFollowServer
      || reconcileOptions.hasRouteOffset
      || reconcileOptions.routePercent !== null
    ) return
    if (
      reconcileOptions.baseline?.bookId
      && progressUpdatedAtMs(serverSaved)
        <= progressUpdatedAtMs(reconcileOptions.baseline)
    ) return
    if (
      options.progressKey(options.getCurrentProgress())
      !== reconcileOptions.baselineKey
    ) return

    const targetIndex = Math.max(
      0,
      Math.min(
        Number(serverSaved.chapterIndex || 0),
        Math.max(options.chapters.value.length - 1, 0),
      ),
    )
    const targetOffset = Math.max(0, Math.floor(Number(serverSaved.offset || 0)))
    const restorePercent = Number.isFinite(Number(serverSaved.chapterPercent))
      ? Math.max(0, Math.min(1, Number(serverSaved.chapterPercent)))
      : savedBookChapterPercent(serverSaved, options.chapters.value.length)
    await options.navigate({
      resume: '1',
      chapter: targetIndex,
      ...(targetOffset ? { offset: targetOffset } : {}),
      ...(restorePercent !== null
        ? { percent: Number(restorePercent.toFixed(6)) }
        : {}),
    })
    await options.loadChapter(targetIndex, targetOffset, {
      restorePercent,
      saveAfterLoad: false,
    })
    options.markProgressSaved(options.getCurrentProgress())
  }

  async function refreshCaches(refreshOptions = {}) {
    const targetBookId = unref(options.bookId)
    const requests = []
    if (refreshOptions.book) {
      requests.push(
        options.refreshBook(targetBookId)
          .then(response => ({ key: 'book', data: response.data })),
      )
    }
    if (refreshOptions.chapters) {
      requests.push(
        options.refreshChapters(targetBookId)
          .then(response => ({ key: 'chapters', data: response.data })),
      )
    }
    const rows = await Promise.all(requests)
    if (Number(unref(options.bookId)) !== Number(targetBookId)) return
    rows.forEach(row => {
      if (row.key === 'book' && row.data?.id) {
        options.book.value = options.mergeLoadedBook(row.data)
      }
      if (row.key === 'chapters' && Array.isArray(row.data)) {
        options.chapters.value = row.data
      }
    })
  }

  async function load() {
    options.cancelProgressSave()
    const targetBookId = unref(options.bookId)
    if (unref(options.isTemporaryReader)) {
      const response = await options.loadTemporaryReader(targetBookId)
      if (unref(options.bookId) !== targetBookId) return
      const temporaryBook = response?.book
      const temporaryChapters = response?.chapters
      if (!temporaryBook || !Array.isArray(temporaryChapters)) {
        throw new Error('remote reader session is invalid')
      }
      options.book.value = temporaryBook
      options.chapters.value = temporaryChapters
      options.resetSourceCandidates()
      const routeQuery = options.getRouteQuery()
      options.currentIndex.value = Math.max(0, Number(routeQuery.chapter || 0))
      const restorePercent = parseReaderRoutePercent(routeQuery.percent)
      await options.loadChapter(options.currentIndex.value, Number(routeQuery.offset || 0), {
        restorePercent,
        saveAfterLoad: false,
      })
      options.markProgressSaved(options.getCurrentProgress())
      await options.jumpToRouteLine()
      return
    }
    const bookmarksRequest = typeof options.loadBookmarks === 'function'
      ? options.loadBookmarks(targetBookId).catch(() => [])
      : null
    const progressRequest = options.reader
      .loadProgress(targetBookId, { preferLocal: true })
      .catch(() => null)
    const cachedProgress = options.reader.cachedProgress(targetBookId)
    const shelfBook = options.getShelfBook?.(targetBookId) || null
    if (shelfBook?.id) options.book.value = options.mergeLoadedBook(shelfBook)
    const bookRequest = options.loadCachedBook(targetBookId)
      .catch(error => {
        if (shelfBook?.id) return { error }
        throw error
      })
    const chapterResponse = await options.loadCachedChapters(targetBookId)
    if (Number(unref(options.bookId)) !== Number(targetBookId)) return

    let bookResponse = null
    if (!shelfBook?.id) {
      bookResponse = await bookRequest
      if (Number(unref(options.bookId)) !== Number(targetBookId)) return
      options.book.value = options.mergeLoadedBook(bookResponse.data)
    }

    const saved = cachedProgress?.bookId
      ? cachedProgress
      : await progressRequest
    if (Number(unref(options.bookId)) !== Number(targetBookId)) return

    options.chapters.value = chapterResponse.data
    if (options.book.value?.progress?.bookId) {
      options.reader.applyServerProgress(options.book.value.progress)
      options.bookshelf.applyBookProgress(options.book.value.progress)
    }
    if (saved?.bookId) {
      options.book.value = options.mergeBookProgress(options.book.value, saved)
    }
    options.resetSourceCandidates()
    if (saved?.bookId) options.bookshelf.applyBookProgress(saved)

    const routeQuery = options.getRouteQuery()
    const resumeFromProgress = routeQuery.resume === '1'
    const hasExplicitChapter = routeQuery.chapter !== undefined
    const shouldUseSavedPosition = resumeFromProgress || !hasExplicitChapter
    if (shouldUseSavedPosition && saved?.chapterIndex !== undefined) {
      options.currentIndex.value = saved.chapterIndex
    } else {
      options.currentIndex.value = Number(routeQuery.chapter || 0)
    }
    const hasRouteOffset = (
      !resumeFromProgress
      && routeQuery.offset !== undefined
    )
    const initialOffset = hasRouteOffset
      ? Number(routeQuery.offset || 0)
      : (shouldUseSavedPosition ? Number(saved?.offset || 0) : 0)
    const routePercent = resumeFromProgress
      ? null
      : parseReaderRoutePercent(routeQuery.percent)
    const savedPercent = shouldUseSavedPosition
      ? savedBookChapterPercent(saved, options.chapters.value.length)
      : null
    await options.loadChapter(options.currentIndex.value, initialOffset, {
      restorePercent: routePercent ?? (hasRouteOffset ? null : savedPercent),
      saveAfterLoad: false,
    })

    const initialProgressKey = options.progressKey(options.getCurrentProgress())
    progressRequest.then(serverSaved => {
      reconcileServerProgress(serverSaved, {
        baseline: saved,
        baselineKey: initialProgressKey,
        resumeFromProgress,
        hasRouteOffset,
        routePercent,
      }).catch(() => {})
    })
    if (chapterResponse.fromCache) refreshCaches({ chapters: true }).catch(() => {})
    bookmarksRequest?.then(data => {
      if (options.bookmarks && Number(unref(options.bookId)) === Number(targetBookId)) {
        options.bookmarks.value = data
      }
    }).catch(() => {})
    await options.jumpToRouteLine()

    if (!bookResponse) bookResponse = await bookRequest
    if (Number(unref(options.bookId)) !== Number(targetBookId)) return
    if (bookResponse?.data?.id) {
      options.book.value = options.mergeLoadedBook(bookResponse.data)
      if (saved?.bookId) {
        options.book.value = options.mergeBookProgress(options.book.value, saved)
      }
    }
    if (bookResponse?.fromCache) refreshCaches({ book: true }).catch(() => {})
  }

  return {
    load,
    reconcileServerProgress,
    refreshCaches,
  }
}

function progressUpdatedAtMs(progress) {
  const time = Date.parse(progress?.updatedAt || '')
  return Number.isFinite(time) ? time : 0
}

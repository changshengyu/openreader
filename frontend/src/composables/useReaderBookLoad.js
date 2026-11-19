import { getCurrentScope, onScopeDispose, unref } from 'vue'
import {
  parseReaderRoutePercent,
  savedBookChapterPercent,
} from '../utils/readerRoute.js'

export function useReaderBookLoad(options) {
  async function reconcileServerProgress(serverSaved, reconcileOptions, isCurrent) {
    if (!isCurrent() || !serverSaved?.bookId || Number(serverSaved.bookId) !== Number(unref(options.bookId))) return
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
    const restorePercent = savedBookChapterPercent(serverSaved, options.chapters.value.length)
    await trackRestore(serverSaved, async () => {
      const reachedPosition = await options.navigate({
        resume: '1',
        chapter: targetIndex,
        ...(targetOffset ? { offset: targetOffset } : {}),
        ...(restorePercent !== null
          ? { percent: Number(restorePercent.toFixed(6)) }
          : {}),
      })
      if (!reachedPosition || !isCurrent()) return

      await options.loadChapter(targetIndex, targetOffset, {
        restorePercent,
        saveAfterLoad: false,
        restoredProgress: serverSaved,
      })
    })
  }

  async function refreshCachedBook(targetBookId, isCurrent) {
    const { data } = await options.refreshBook(targetBookId, isCurrent)
    if (isCurrent() && data?.id) options.book.value = options.mergeLoadedBook(data)
  }

  async function refreshCachedChapters(targetBookId, isCurrent) {
    const { data } = await options.refreshChapters(targetBookId, isCurrent)
    if (isCurrent() && Array.isArray(data)) options.chapters.value = data
  }

  let loadGeneration = 0
  const trackRestore = options.trackRestore ?? ((progress, restore) => restore())
  if (getCurrentScope()) onScopeDispose(() => { loadGeneration++ })

  async function load() {
    const generation = ++loadGeneration
    const targetBookId = unref(options.bookId)
    const isCurrent = () => generation === loadGeneration && unref(options.bookId) === targetBookId
    try {
      await loadBook(targetBookId, isCurrent)
    } catch (error) {
      if (isCurrent()) throw error
    }
  }

  async function loadBook(targetBookId, isCurrent) {
    options.cancelProgressSave()
    // A catalog belongs to the book it was loaded for. Drop the previous book's rows before the first await so the
    // TOC, preloads, chapter clamping, progress and N% never size or label this book with them.
    options.chapters.value = []
    if (unref(options.isTemporaryReader)) {
      const response = await options.loadTemporaryReader(targetBookId)
      if (!isCurrent()) return
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
    const bookRequest = options.loadCachedBook(targetBookId, isCurrent)
      .catch(error => ({ error }))
    const chapterResponse = await options.loadCachedChapters(targetBookId, isCurrent)
    if (!isCurrent()) return

    let bookResponse = null
    if (!shelfBook?.id) {
      bookResponse = await bookRequest
      if (!isCurrent()) return
      if (bookResponse.error) throw bookResponse.error
      options.book.value = options.mergeLoadedBook(bookResponse.data)
    }

    const saved = cachedProgress?.bookId
      ? cachedProgress
      : await progressRequest
    if (!isCurrent()) return

    options.chapters.value = chapterResponse.data
    if (options.book.value?.progress?.bookId) {
      if (!options.book.value.progress.pendingSync) options.reader.applyServerProgress(options.book.value.progress)
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
    const restoredProgress = shouldUseSavedPosition && !hasRouteOffset && routePercent === null && saved?.bookId
      ? saved
      : null
    await trackRestore(restoredProgress, () => options.loadChapter(options.currentIndex.value, initialOffset, {
      restorePercent: routePercent ?? (hasRouteOffset ? null : savedPercent),
      saveAfterLoad: false,
      ...(restoredProgress ? { restoredProgress } : {}),
    }))

    if (!isCurrent()) return

    const initialProgressKey = options.progressKey(options.getCurrentProgress())
    progressRequest.then(serverSaved => {
      reconcileServerProgress(serverSaved, {
        baseline: saved,
        baselineKey: initialProgressKey,
        resumeFromProgress,
        hasRouteOffset,
        routePercent,
      }, isCurrent).catch(() => {})
    })
    if (chapterResponse.fromCache) refreshCachedChapters(targetBookId, isCurrent).catch(() => {})
    bookmarksRequest?.then(data => {
      if (options.bookmarks && isCurrent()) {
        options.bookmarks.value = data
      }
    }).catch(() => {})
    await options.jumpToRouteLine()

    if (!bookResponse) bookResponse = await bookRequest
    if (!isCurrent()) return
    if (bookResponse?.data?.id) {
      options.book.value = options.mergeLoadedBook(bookResponse.data)
      if (saved?.bookId) {
        options.book.value = options.mergeBookProgress(options.book.value, saved)
      }
    }
    if (bookResponse?.fromCache) refreshCachedBook(targetBookId, isCurrent).catch(() => {})
  }

  return {
    load,
  }
}

function progressUpdatedAtMs(progress) {
  const time = Date.parse(progress?.updatedAt || '')
  return Number.isFinite(time) ? time : 0
}

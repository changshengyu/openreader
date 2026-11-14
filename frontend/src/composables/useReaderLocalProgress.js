import { unref } from 'vue'
import {
  readerProgressBaseUpdatedAt,
  readerProgressNamesCatalogRow,
  readerProgressPayload,
  readerProgressSaveKey,
} from '../utils/readerProgressPersistence.js'

export function useReaderLocalProgress(options) {
  const now = options.now ?? (() => new Date())
  let lastLocalProgressKey = ''

  function serverBaseUpdatedAt(targetBookId = unref(options.bookId)) {
    return readerProgressBaseUpdatedAt(
      options.reader.progressByBook[targetBookId],
    )
  }

  function storedProgress(targetBookId) {
    return options.reader.cachedProgress
      ? options.reader.cachedProgress(targetBookId)
      : options.reader.progressByBook[targetBookId]
  }

  function currentPayload(snapshotOverride) {
    if (unref(options.isTemporaryReader)) return null
    const chapters = unref(options.chapters)
    // The audio player's payload is used only while it names the catalog row at its index (contract P1): after a
    // failed or pending load `chapter` is still the previous audio chapter.
    const override = options.getCurrentPayload?.()
    if (override && readerProgressNamesCatalogRow(override, chapters)) return override
    const snapshot = arguments.length
      ? snapshotOverride
      : options.getVisibleSnapshot()
    const bookId = unref(options.bookId)
    return readerProgressPayload({
      bookId,
      chapters,
      visibleSnapshot: snapshot,
      currentChapter: unref(options.chapter),
      currentChapterIndex: unref(options.currentIndex),
      currentOffset: snapshot ? 0 : options.getCurrentOffset(),
      currentChapterPercent: snapshot ? 0 : options.getCurrentPercent(),
      totalChapters: chapters.length,
      getStoredProgress: () => storedProgress(bookId),
    })
  }

  function upsert(progress, upsertOptions = {}) {
    if (!progress?.bookId) return
    const currentBook = unref(options.book)
    if (
      currentBook?.id
      && Number(currentBook.id) === Number(progress.bookId)
    ) {
      const nextBook = options.mergeBook(currentBook, {
        id: currentBook.id,
        progress,
        shelfOrderAt: progress.updatedAt,
      })
      options.book.value = nextBook
      options.bookshelf.upsertBook(nextBook)
      return
    }
    options.bookshelf.applyBookProgress(progress, upsertOptions)
  }

  function apply(payload = currentPayload(), applyOptions = {}) {
    if (!payload?.bookId || !unref(options.chapter)) return
    const nextPayload = {
      ...payload,
      baseUpdatedAt: payload.baseUpdatedAt
        || serverBaseUpdatedAt(payload.bookId),
    }
    const mode = options.getMode()
    const key = readerProgressSaveKey(nextPayload, mode)
    if (key === lastLocalProgressKey && !applyOptions.force) return
    lastLocalProgressKey = key
    options.reader.applyProgress({
      ...nextPayload,
      mode,
      updatedAt: now().toISOString(),
      pendingSync: true,
    })
    upsert(options.reader.progressByBook[nextPayload.bookId])
  }

  return {
    apply,
    currentPayload,
    serverBaseUpdatedAt,
    upsert,
  }
}

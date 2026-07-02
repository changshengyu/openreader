import { unref } from 'vue'
import {
  readerProgressBaseUpdatedAt,
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

  function currentPayload() {
    const snapshot = options.getVisibleSnapshot()
    return readerProgressPayload({
      bookId: unref(options.bookId),
      visibleSnapshot: snapshot,
      currentChapter: unref(options.chapter),
      currentChapterIndex: unref(options.currentIndex),
      currentOffset: snapshot ? 0 : options.getCurrentOffset(),
      currentChapterPercent: snapshot ? 0 : options.getCurrentPercent(),
      totalChapters: unref(options.chapters).length,
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
    const key = readerProgressSaveKey(nextPayload, options.reader.mode)
    if (key === lastLocalProgressKey && !applyOptions.force) return
    lastLocalProgressKey = key
    options.reader.applyProgress({
      ...nextPayload,
      mode: options.reader.mode,
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

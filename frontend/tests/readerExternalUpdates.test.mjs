import assert from 'node:assert/strict'
import test from 'node:test'
import { ref } from 'vue'
import { useReaderExternalUpdates } from '../src/composables/useReaderExternalUpdates.js'

function createController(overrides = {}) {
  const calls = []
  const book = ref({ id: 7, title: '旧书名' })
  const chapter = ref({ id: 12 })
  const chapters = ref([{ id: 11 }, { id: 12 }, { id: 13 }])
  const currentIndex = ref(1)
  const currentProgress = {
    bookId: 7,
    chapterId: 12,
    chapterIndex: 1,
    offset: 80,
    chapterPercent: 0.4,
  }
  const controller = useReaderExternalUpdates({
    bookId: ref(7),
    book,
    chapter,
    chapters,
    currentIndex,
    isRestoring: () => false,
    isProgressSaveBusy: () => false,
    progressKey: progress => progress
      ? `${progress.bookId}:${progress.chapterIndex}:${progress.offset}:${progress.chapterPercent}`
      : '',
    getCurrentProgress: () => currentProgress,
    cancelProgressSave: () => calls.push(['cancel']),
    navigate: async query => calls.push(['navigate', query]),
    loadChapter: async (...args) => calls.push(['load', ...args]),
    markProgressSaved: progress => calls.push(['mark', progress]),
    getCurrentOffset: () => 80,
    getCurrentPercent: () => 0.4,
    clearChapterCache: () => calls.push(['clear']),
    resetCachedChapters: () => calls.push(['reset-cache']),
    resetContentSearch: () => calls.push(['reset-search']),
    refreshCachedChapters: async () => calls.push(['refresh-cache']),
    onReplaceSuccess: () => calls.push(['replace-success']),
    onReplaceError: error => calls.push(['replace-error', error.message]),
    ...overrides,
  })
  return {
    book,
    calls,
    chapter,
    chapters,
    controller,
    currentIndex,
    currentProgress,
  }
}

test('applies different remote progress to the current book', async () => {
  const fixture = createController()
  await fixture.controller.handleProgressUpdated({
    detail: {
      progress: {
        bookId: 7,
        chapterId: 13,
        chapterIndex: 9,
        offset: 125.8,
        chapterPercent: 1.4,
      },
    },
  })
  assert.deepEqual(fixture.calls, [
    ['cancel'],
    ['navigate', { chapter: 2, offset: 125, percent: 1 }],
    ['load', 2, 125, { restorePercent: 1, saveAfterLoad: false }],
    ['mark', fixture.currentProgress],
  ])
})

test('ignores progress for other books, matching positions, or busy restoration', async () => {
  const fixture = createController()
  await fixture.controller.handleProgressUpdated({
    detail: { progress: { ...fixture.currentProgress, bookId: 8 } },
  })
  await fixture.controller.handleProgressUpdated({
    detail: { progress: fixture.currentProgress },
  })
  assert.deepEqual(fixture.calls, [])

  const busy = createController({ isRestoring: () => true })
  await busy.controller.handleProgressUpdated({
    detail: { progress: { ...busy.currentProgress, offset: 120 } },
  })
  assert.deepEqual(busy.calls, [])
})

test('updates book metadata without reloading and refreshes changed chapter lists', async () => {
  const fixture = createController()
  await fixture.controller.handleBookDataUpdated({
    detail: { bookId: 7, book: { id: 7, title: '新书名' } },
  })
  assert.equal(fixture.book.value.title, '新书名')
  assert.deepEqual(fixture.calls, [])

  await fixture.controller.handleBookDataUpdated({
    detail: {
      bookId: 7,
      chapters: [{ id: 21 }],
    },
  })
  assert.equal(fixture.currentIndex.value, 0)
  assert.deepEqual(fixture.chapters.value, [{ id: 21 }])
  assert.deepEqual(fixture.calls, [
    ['clear'],
    ['reset-cache'],
    ['reset-search'],
    ['refresh-cache'],
    ['load', 0, 80, { restorePercent: 0.4, refresh: true, saveAfterLoad: false }],
  ])
})

test('reloads the current chapter after replacement rules change', async () => {
  const fixture = createController()
  await fixture.controller.handleReplaceRulesUpdated()
  assert.deepEqual(fixture.calls, [
    ['load', 1, 80, { restorePercent: 0.4, refresh: true }],
    ['replace-success'],
  ])

  const failed = createController({
    loadChapter: async () => {
      throw new Error('reload failed')
    },
  })
  await failed.controller.handleReplaceRulesUpdated()
  assert.deepEqual(failed.calls, [['replace-error', 'reload failed']])
})

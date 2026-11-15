import assert from 'node:assert/strict'
import test from 'node:test'
import { reactive, ref } from 'vue'
import { useReaderLocalProgress } from '../src/composables/useReaderLocalProgress.js'
import { readerProgressPayload } from '../src/utils/readerProgressPersistence.js'

// docs/compat/reader-failure-progress-payload-fixed-baseline-second-audit-p0-contract.md P1, P3, P4.
// reader-dev names progress by catalog index only and the server takes the title from that row, so a payload can never
// pair one chapter's index with another chapter's id or title.

const TITLES = ['第一章 起点', '第二章 后邻', '第三章 平稳', '第四章 预载', '第五章 前邻', '第六章 失败', '第七章 后续', '第八章 终点']
const BOOK_ID = 7
const ROWS = TITLES.map((title, index) => ({ id: 11 + index, bookId: BOOK_ID, index, title }))
// The chapter object a successful content load leaves in `chapter` (the backend Chapter model).
const LOADED_FIFTH = { id: 15, bookId: BOOK_ID, index: 4, title: '第五章 前邻' }

function payloadFor(overrides = {}) {
  return readerProgressPayload({
    bookId: BOOK_ID,
    chapters: ROWS,
    visibleSnapshot: null,
    currentChapter: LOADED_FIFTH,
    currentChapterIndex: 4,
    currentOffset: 561,
    currentChapterPercent: 0.3,
    totalChapters: ROWS.length,
    ...overrides,
  })
}

test('a failed or loading chapter names its own catalog row at the chapter start (P1, P3)', () => {
  // `chapter` still holds the last loaded chapter (row 4) while currentIndex already points at the target (row 5).
  assert.deepEqual(payloadFor({ currentChapterIndex: 5, currentOffset: 1477, currentChapterPercent: 0.4 }), {
    bookId: BOOK_ID,
    chapterId: 16,
    chapterIndex: 5,
    offset: 0,
    percent: 5 / 8,
    chapterPercent: 0,
    chapterTitle: '第六章 失败',
  })
  // A first load has no loaded chapter at all.
  assert.deepEqual(payloadFor({ currentChapter: null, currentChapterIndex: 5, currentOffset: 0, currentChapterPercent: 0 }), {
    bookId: BOOK_ID,
    chapterId: 16,
    chapterIndex: 5,
    offset: 0,
    percent: 5 / 8,
    chapterPercent: 0,
    chapterTitle: '第六章 失败',
  })
})

test('a chapter without content keeps the saved position when the saved progress already names it', () => {
  // A resume of chapter 5 at offset 300 whose load fails: reader-dev leaves its in-chapter position alone (W5).
  const stored = { bookId: BOOK_ID, chapterId: 16, chapterIndex: 5, offset: 300, chapterPercent: 0.25 }
  assert.deepEqual(payloadFor({
    currentChapter: null,
    currentChapterIndex: 5,
    currentOffset: 0,
    currentChapterPercent: 0,
    getStoredProgress: () => stored,
  }), {
    bookId: BOOK_ID,
    chapterId: 16,
    chapterIndex: 5,
    offset: 300,
    percent: (5 + 0.25) / 8,
    chapterPercent: 0.25,
    chapterTitle: '第六章 失败',
  })
  // Saved progress of another chapter does not lend its offset.
  assert.equal(payloadFor({
    currentChapterIndex: 5,
    getStoredProgress: () => ({ bookId: BOOK_ID, chapterId: 15, chapterIndex: 4, offset: 561, chapterPercent: 0.3 }),
  })?.offset, 0)
})

test('a catalog row of another book or an unknown row never names this book\'s progress (P4)', () => {
  const otherBookRows = ROWS.map(row => ({ ...row, bookId: 3 }))
  assert.equal(payloadFor({
    chapters: otherBookRows,
    currentChapter: { ...LOADED_FIFTH, bookId: 3 },
  }), null, 'the previous book\'s catalog and chapter must not name book 7')
  assert.equal(payloadFor({
    chapters: otherBookRows,
    visibleSnapshot: { chapterIndex: 4, chapter: otherBookRows[4], offset: 40, chapterPercent: 0.1 },
  }), null, 'a visible snapshot of another book\'s row must not name book 7')
  assert.equal(payloadFor({ chapters: [], currentChapterIndex: 0 }), null, 'an empty catalog has no row to name')
})

test('a loaded chapter and a visible snapshot keep their identity and position (control)', () => {
  assert.deepEqual(payloadFor(), {
    bookId: BOOK_ID,
    chapterId: 15,
    chapterIndex: 4,
    offset: 561,
    percent: (4 + 0.3) / 8,
    chapterPercent: 0.3,
    chapterTitle: '第五章 前邻',
  })
  assert.deepEqual(payloadFor({
    currentChapterIndex: 5,
    visibleSnapshot: { chapterIndex: 5, chapter: ROWS[5], offset: 0, chapterPercent: 0 },
  }), {
    bookId: BOOK_ID,
    chapterId: 16,
    chapterIndex: 5,
    offset: 0,
    percent: 5 / 8,
    chapterPercent: 0,
    chapterTitle: '第六章 失败',
  })
})

function createLocalProgress(overrides = {}) {
  const calls = []
  const reader = reactive({
    progressByBook: {},
    applyProgress(progress) {
      this.progressByBook[progress.bookId] = progress
      calls.push(['apply', progress])
    },
  })
  const controller = useReaderLocalProgress({
    reader,
    bookshelf: {
      upsertBook: row => calls.push(['upsert', row]),
      applyBookProgress: progress => calls.push(['shelf', progress]),
    },
    bookId: ref(BOOK_ID),
    book: ref({ id: BOOK_ID, title: '失败态进度' }),
    chapter: ref(LOADED_FIFTH),
    chapters: ref(ROWS),
    currentIndex: ref(5),
    getVisibleSnapshot: () => null,
    getCurrentOffset: () => 1477,
    getCurrentPercent: () => 0.4,
    mergeBook: (current, incoming) => ({ ...current, ...incoming }),
    now: () => new Date('2026-11-14T00:00:00Z'),
    getMode: () => 'page',
    ...overrides,
  })
  return { calls, controller, reader }
}

test('the Reader payload names the target row while the previous chapter is still loaded (P1)', () => {
  const fixture = createLocalProgress()
  const payload = fixture.controller.currentPayload()
  assert.equal(payload?.chapterIndex, 5)
  assert.equal(payload?.chapterId, 16, `chapterId ${payload?.chapterId} belongs to row ${ROWS.findIndex(row => row.id === payload?.chapterId)}`)
  assert.equal(payload?.chapterTitle, '第六章 失败')
  assert.equal(payload?.offset, 0)
})

test('an audio payload of the previous chapter does not override the target row (P1)', () => {
  const previousAudio = { bookId: BOOK_ID, chapterId: 15, chapterIndex: 5, offset: 33, percent: 0.6, chapterPercent: 0.2, chapterTitle: '第五章 前邻' }
  const failed = createLocalProgress({ getCurrentPayload: () => previousAudio })
  assert.equal(failed.controller.currentPayload()?.chapterId, 16, 'a mismatched override must give way to the target row')

  const playing = { ...previousAudio, chapterId: 16, chapterTitle: '第六章 失败' }
  const healthy = createLocalProgress({ chapter: ref({ id: 16, bookId: BOOK_ID, index: 5, title: '第六章 失败' }), getCurrentPayload: () => playing })
  assert.deepEqual(healthy.controller.currentPayload(), playing, 'a matching audio payload is kept (control)')
})

test('a book switch whose catalog has not arrived writes nothing for the new book (P4)', () => {
  // The route already names book 9 while chapter, chapters and currentIndex still belong to book 7.
  const fixture = createLocalProgress({
    bookId: ref(9),
    book: ref({ id: 9, title: '切换目标' }),
    currentIndex: ref(4),
  })
  assert.equal(fixture.controller.currentPayload(), null)
  fixture.controller.apply()
  assert.deepEqual(fixture.calls.filter(call => call[0] === 'apply'), [], 'no local snapshot for book 9 may carry book 7\'s chapter')
})

test('the Reader payload keeps the saved offset of a failed resume (P3)', () => {
  const fixture = createLocalProgress({ chapter: ref(null) })
  fixture.reader.progressByBook[BOOK_ID] = { bookId: BOOK_ID, chapterId: 16, chapterIndex: 5, offset: 300, chapterPercent: 0.25, updatedAt: '2026-11-13T00:00:00Z' }
  const payload = fixture.controller.currentPayload()
  assert.equal(payload?.chapterId, 16)
  assert.equal(payload?.offset, 300)
  assert.equal(payload?.chapterPercent, 0.25)
})

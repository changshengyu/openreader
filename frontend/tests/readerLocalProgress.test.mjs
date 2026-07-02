import assert from 'node:assert/strict'
import test from 'node:test'
import { reactive, ref } from 'vue'
import { useReaderLocalProgress } from '../src/composables/useReaderLocalProgress.js'

function createController(overrides = {}) {
  const calls = []
  const book = ref({ id: 7, title: '测试书' })
  const chapter = ref({ id: 12, title: '第三章' })
  const reader = reactive({
    mode: 'scroll',
    progressByBook: {},
    applyProgress(progress) {
      this.progressByBook[progress.bookId] = progress
      calls.push(['apply', progress])
    },
  })
  const bookshelf = {
    upsertBook: row => calls.push(['upsert', row]),
    applyBookProgress: (progress, options) => calls.push(['shelf', progress, options]),
  }
  const controller = useReaderLocalProgress({
    reader,
    bookshelf,
    bookId: ref(7),
    book,
    chapter,
    chapters: ref(Array.from({ length: 10 }, () => ({}))),
    currentIndex: ref(2),
    getVisibleSnapshot: () => null,
    getCurrentOffset: () => 120,
    getCurrentPercent: () => 0.4,
    mergeBook: (current, incoming) => ({ ...current, ...incoming }),
    now: () => new Date('2026-07-02T00:00:00Z'),
    ...overrides,
  })
  return { book, calls, chapter, controller, reader }
}

test('builds progress from the current chapter when no visible snapshot exists', () => {
  const fixture = createController()
  assert.deepEqual(fixture.controller.currentPayload(), {
    bookId: 7,
    chapterId: 12,
    chapterIndex: 2,
    offset: 120,
    percent: 0.24,
    chapterPercent: 0.4,
    chapterTitle: '第三章',
  })
})

test('deduplicates local progress while allowing forced snapshots', () => {
  const fixture = createController()
  fixture.controller.apply()
  fixture.controller.apply()
  fixture.controller.apply(undefined, { force: true })
  assert.equal(fixture.calls.filter(call => call[0] === 'apply').length, 2)
  assert.equal(fixture.calls.filter(call => call[0] === 'upsert').length, 2)
  assert.equal(fixture.book.value.progress.pendingSync, true)
  assert.equal(fixture.book.value.shelfOrderAt, '2026-07-02T00:00:00.000Z')
})

test('preserves a pending server base and updates unrelated shelf progress', () => {
  const fixture = createController()
  fixture.reader.progressByBook[7] = {
    pendingSync: true,
    baseUpdatedAt: 'server-v1',
    updatedAt: 'local-v2',
  }
  assert.equal(fixture.controller.serverBaseUpdatedAt(), 'server-v1')
  fixture.controller.upsert({ bookId: 8, updatedAt: 'other-v1' }, { replace: true })
  assert.deepEqual(fixture.calls, [
    ['shelf', { bookId: 8, updatedAt: 'other-v1' }, { replace: true }],
  ])
})

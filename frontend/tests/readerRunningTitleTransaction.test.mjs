import assert from 'node:assert/strict'
import test from 'node:test'
import { computed, effectScope, nextTick, reactive, ref, watch } from 'vue'
import { useReaderChapterLoader } from '../src/composables/useReaderChapterLoader.js'
import { useReaderRouteSync } from '../src/composables/useReaderRouteSync.js'

// docs/compat/reader-running-title-load-failure-fixed-baseline-second-audit-p0-contract.md R1-R5 (section 8.1 item 3):
// the running title switches to the target catalog row as soon as navigation starts, keeps it while loading, after
// a failure and during a retry, ignores superseded loads, and never pairs an ordinal with another book's row. A
// synchronous watcher records every value, including ones no DOM update ever shows (`log`); a pre-flush watcher
// records what each DOM update shows (`painted`).

async function runningTitle() {
  try {
    return await import('../src/utils/readerRunningTitle.js')
  } catch (error) {
    assert.fail(`running title helper must exist at frontend/src/utils/readerRunningTitle.js: ${error.message}`)
  }
}

const BOOK_ONE = { id: 1, title: '书一' }
const BOOK_TWO = { id: 2, title: '书二' }
const catalog = (bookId, titles) => titles.map((title, index) => ({ id: bookId * 100 + index, bookId, index, title }))
const ONE = catalog(1, ['甲', '乙', '丙'])
const SHELF_ONE = catalog(1, ['甲一', '甲二', '甲三 当前', '甲四', '甲五'])
const SHELF_TWO = catalog(2, ['乙一', '乙二', '乙三', '乙四 目标', '乙五'])

function httpError(status) {
  const error = new Error(`Request failed with status code ${status}`)
  error.response = { status, data: { error: 'failed to load chapter content' } }
  return error
}

async function flush() {
  for (let round = 0; round < 6; round += 1) {
    await nextTick()
    await Promise.resolve()
  }
}

async function createReader({ book = BOOK_ONE, chapters = ONE, index = 0 } = {}) {
  const { runningChapterLabelFor } = await runningTitle()
  const scope = effectScope()
  const pending = []
  const state = {
    book: ref(book),
    bookId: ref(book?.id ?? 0),
    chapters: ref(chapters),
    currentIndex: ref(index),
    chapter: ref(chapters[index] || null),
    content: ref(''),
    chapterLoadError: ref(''),
    log: [],
    painted: [],
    loads: [],
    pending,
  }
  scope.run(() => {
    state.label = computed(() => runningChapterLabelFor({
      book: state.book.value,
      chapters: state.chapters.value,
      index: state.currentIndex.value,
    }))
    watch(state.label, value => state.log.push(value), { flush: 'sync' })
    watch(state.label, value => state.painted.push(value), { flush: 'pre' })
    state.loader = useReaderChapterLoader({
      chapters: state.chapters,
      currentIndex: state.currentIndex,
      mobileChromeVisible: ref(true),
      restoringPosition: ref(false),
      chapterLoaded: ref(true),
      chapterLoadError: state.chapterLoadError,
      chapterLoading: ref(false),
      chapter: state.chapter,
      content: state.content,
      cachedImages: ref({}),
      chapterFormat: ref('text'),
      epubResource: ref(null),
      audioResource: ref(null),
      page: ref(0),
      chapterBlocks: ref([]),
      progressVersion: ref(0),
      isContinuousScrollRead: ref(false),
      getScopeKey: () => String(state.bookId.value),
      cancelProgressSave: () => {},
      getMemoryContent: () => null,
      loadContent: (target) => new Promise((resolve, reject) => {
        state.loads.push(target)
        pending.push({ index: target, resolve, reject })
      }),
      makeChapterBlock: (target, row, text) => ({ index: target, id: row?.id, content: text }),
      updateLayout: () => {},
      restorePosition: async () => {},
      preloadNearby: () => {},
      saveProgress: async () => {},
      markProgressSaved: () => {},
      getCurrentProgress: () => null,
      computeChapterWindow: async () => {},
      formatError: () => '服务器处理失败，请稍后重试',
      nextFrame: async () => {},
    })
  })
  state.label.value
  state.scope = scope
  state.take = (target) => {
    const at = pending.findIndex(row => row.index === target)
    assert.notEqual(at, -1, `no pending content request for chapter ${target}`)
    return pending.splice(at, 1)[0]
  }
  return state
}

const contentFor = (row) => ({ chapter: { id: row.id, index: row.index, title: row.title }, content: `${row.title}正文`, format: 'text' })

test('switches to the target catalog row as soon as a load starts and keeps it while loading', async () => {
  const reader = await createReader()
  assert.equal(reader.label.value, '第 1 章 甲')
  const loading = reader.loader.load(2)
  assert.equal(reader.label.value, '第 3 章 丙', 'the label must name the target while its content loads')
  reader.take(2).resolve(contentFor(ONE[2]))
  await loading
  assert.equal(reader.label.value, '第 3 章 丙')
  assert.deepEqual(reader.log, ['第 3 章 丙'], 'no value may pair the new ordinal with the previous chapter')
  reader.scope.stop()
})

test('keeps the target label after a failure and during the retry', async () => {
  const reader = await createReader()
  const failing = reader.loader.load(2)
  reader.take(2).reject(httpError(500))
  await failing
  assert.equal(reader.label.value, '第 3 章 丙')
  assert.notEqual(reader.chapterLoadError.value, '', 'the failure is still reported')
  assert.equal(reader.chapter.value.title, '甲', 'a failed load commits no chapter')
  const retry = reader.loader.load(2, 0, { refresh: true })
  assert.equal(reader.label.value, '第 3 章 丙')
  reader.take(2).resolve(contentFor(ONE[2]))
  await retry
  assert.deepEqual(reader.log, ['第 3 章 丙'])
  reader.scope.stop()
})

test('a superseded load that finishes late leaves the latest target in the label', async () => {
  const reader = await createReader()
  const slow = reader.loader.load(1)
  const fast = reader.loader.load(2)
  reader.take(2).resolve(contentFor(ONE[2]))
  await fast
  reader.take(1).resolve(contentFor(ONE[1]))
  await slow
  assert.equal(reader.label.value, '第 3 章 丙')
  assert.equal(reader.chapter.value.title, '丙')
  assert.deepEqual(reader.log, ['第 2 章 乙', '第 3 章 丙'])
  reader.scope.stop()
})

test('is empty until the catalog arrives, then names the target directly', async () => {
  const reader = await createReader({ chapters: [], index: 3 })
  assert.equal(reader.label.value, '', 'no guessed ordinal before the catalog')
  reader.chapters.value = ONE
  reader.currentIndex.value = 2
  await nextTick()
  assert.equal(reader.label.value, '第 3 章 丙')
  assert(!reader.log.some(value => value.startsWith('第 4 章')), `a guessed ordinal leaked: ${JSON.stringify(reader.log)}`)
  reader.scope.stop()
})

// R5: switching books inside the Reader. The new book is not on the shelf, so its book object only arrives with the
// book request; the same route change also changes the position query. The order follows useReaderBookLoad.load.
for (const outcome of ['loads', 'fails']) {
  test(`a Reader book switch never pairs an ordinal with the other book's row (new chapter ${outcome})`, async () => {
    const reader = await createReader({ book: BOOK_ONE, chapters: SHELF_ONE, index: 2 })
    const query = reactive({ chapter: '2' })
    const catalogGate = {}
    const catalogArrived = new Promise(resolve => { catalogGate.release = resolve })
    let bookLoad = null
    reader.scope.run(() => {
      useReaderRouteSync({
        bookId: reader.bookId,
        currentIndex: reader.currentIndex,
        positionQuery: () => [query.chapter, query.offset, query.percent],
        searchQuery: () => [],
        loadBook: () => {
          bookLoad = (async () => {
            await catalogArrived
            reader.book.value = BOOK_TWO
            await Promise.resolve()
            reader.chapters.value = SHELF_TWO
            reader.currentIndex.value = 3
            await reader.loader.load(3)
          })()
          return bookLoad
        },
        loadChapter: (index, offset, options) => reader.loader.load(index, offset, options),
        jumpToRouteLine: async () => {},
      })
    })
    assert.equal(reader.label.value, '第 3 章 甲三 当前')

    reader.bookId.value = 2
    query.chapter = '3'
    await flush()
    assert.deepEqual(reader.loads, [], 'no chapter of the new book may load against the previous catalog')
    catalogGate.release()
    await flush()
    const request = reader.take(3)
    if (outcome === 'loads') request.resolve(contentFor(SHELF_TWO[3]))
    else request.reject(httpError(500))
    await bookLoad
    await flush()

    assert.equal(reader.label.value, '第 4 章 乙四 目标')
    // Every value, even between two synchronous writes, comes from one row of one book.
    const sameRow = new Set(['', ...[[SHELF_ONE, 2], ...SHELF_TWO.map((_, at) => [SHELF_TWO, at])]
      .map(([rows, at]) => `第 ${at + 1} 章 ${rows[at].title}`)])
    const spliced = reader.log.filter(value => !sameRow.has(value))
    assert.deepEqual(spliced, [], `an ordinal was paired with another row or book: ${JSON.stringify(reader.log)}`)
    // What a DOM update shows: the old book's full label, an empty label or the new target, nothing else.
    const allowed = new Set(['第 3 章 甲三 当前', '', '第 4 章 乙四 目标'])
    const shown = reader.painted.filter(value => !allowed.has(value))
    assert.deepEqual(shown, [], `only the old book's full label, an empty label or the new target may show: ${JSON.stringify(reader.painted)}`)
    assert.deepEqual(reader.loads, [3], 'the new book loads its target chapter once, after its catalog')
    reader.scope.stop()
  })
}

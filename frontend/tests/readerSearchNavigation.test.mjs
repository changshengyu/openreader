import assert from 'node:assert/strict'
import test from 'node:test'
import { ref } from 'vue'
import { useReaderSearchNavigation } from '../src/composables/useReaderSearchNavigation.js'

function createFixture(overrides = {}) {
  const chapterEl = { dataset: { index: '1' } }
  const paragraphs = [
    {
      textContent: '第一段没有目标',
      offsetTop: 100,
      offsetLeft: 0,
      closest: () => chapterEl,
    },
    {
      textContent: '目标，目标！',
      offsetTop: 420,
      offsetLeft: 800,
      closest: () => chapterEl,
    },
  ]
  const scope = { querySelectorAll: () => paragraphs }
  const body = {
    querySelector: () => scope,
    querySelectorAll: () => paragraphs,
  }
  const loaded = []
  const navigated = []
  const saved = []
  const options = {
    keyword: ref('目标'),
    contentEl: ref({ scrollTop: 0 }),
    contentBody: ref(body),
    currentIndex: ref(1),
    chapterBlocks: ref([{ index: 1, id: 11, title: '第二章', content: '正文' }]),
    chapters: ref([{ id: 10, title: '第一章' }, { id: 11, title: '第二章' }]),
    chapter: ref({ id: 11, title: '第二章' }),
    content: ref('正文'),
    page: ref(0),
    pageCount: ref(4),
    pageWidth: ref(400),
    getMode: () => 'scroll',
    getRouteQuery: () => ({}),
    closeDrawer: () => {},
    navigate: async query => navigated.push(query),
    loadChapter: async (index, loadOptions) => loaded.push({ index, loadOptions }),
    flashParagraph: () => {},
    saveProgress: () => saved.push(true),
    ...overrides,
  }
  return {
    controller: useReaderSearchNavigation(options),
    loaded,
    navigated,
    options,
    paragraphs,
    saved,
  }
}

test('jumps to the requested occurrence and scrolls the matching paragraph', () => {
  const { controller, options, paragraphs, saved } = createFixture()
  assert.equal(controller.jumpToMatch({
    query: '目标',
    resultCountWithinChapter: 1,
  }), true)
  assert.equal(options.contentEl.value.scrollTop, paragraphs[1].offsetTop - 80)
  assert.equal(saved.length, 1)
})

test('loads same chapter directly and navigates before loading another chapter', async () => {
  const same = createFixture()
  await same.controller.jumpToResult({
    chapterIndex: 1,
    percent: 0.3,
    query: '目标',
    resultCountWithinChapter: 0,
  })
  assert.deepEqual(same.navigated, [])
  assert.deepEqual(same.loaded, [{
    index: 1,
    loadOptions: { restorePercent: 0.3, saveAfterLoad: true },
  }])

  const other = createFixture()
  await other.controller.jumpToResult({
    chapterIndex: 3,
    percent: 0.6,
    lineIndex: 1,
  })
  assert.deepEqual(other.navigated, [{ chapter: 3, percent: 0.6 }])
  assert.deepEqual(other.loaded, [{
    index: 3,
    loadOptions: { restorePercent: 0.6, saveAfterLoad: true },
  }])
})

test('restores a bookmark by paragraph context after route offset restoration', async () => {
  const fixture = createFixture()
  assert.equal(fixture.controller.jumpToBookmarkContext('目标 目标'), true)
  assert.equal(fixture.options.contentEl.value.scrollTop, fixture.paragraphs[1].offsetTop - 80)

  const failures = []
  const missing = createFixture({
    getRouteQuery: () => ({ bookmark: '不存在的书签上下文' }),
    onBookmarkNotFound: () => failures.push('missing'),
  })
  await missing.controller.jumpToRouteLine()
  assert.deepEqual(failures, ['missing'])
})

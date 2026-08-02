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

test('does not trim the exact query before locating a result', () => {
  const { controller, options, paragraphs } = createFixture()
  paragraphs[0].textContent = '目标'
  paragraphs[1].textContent = '前文 目标 后文'

  assert.equal(controller.jumpToMatch({
    query: ' 目标 ',
    resultCountWithinChapter: 0,
  }), true)
  assert.equal(options.contentEl.value.scrollTop, paragraphs[1].offsetTop - 80)
})

test('maps a fragmented flip paragraph from rendered column geometry instead of offsetLeft', () => {
  const fixture = createFixture({
    getMode: () => 'flip',
    contentEl: ref({
      getBoundingClientRect: () => ({ left: 0 }),
    }),
    page: ref(0),
    pageCount: ref(5),
    pageWidth: ref(374),
  })
  fixture.paragraphs[1].offsetLeft = 0
  fixture.paragraphs[1].getBoundingClientRect = () => ({ left: 390 })

  fixture.controller.jumpToParagraph(fixture.paragraphs[1], { save: false, flash: false })

  assert.equal(fixture.options.page.value, 1)
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
  assert.deepEqual(same.loaded, [], 'an upstream same-chapter result must reposition without reloading content')
  assert.equal(same.saved.length, 1)
  await same.controller.jumpToResult({
    chapterIndex: 1,
    percent: 0.3,
    query: '目标',
    resultCountWithinChapter: 0,
  })
  assert.deepEqual(same.loaded, [], 'selecting the identical result again must still avoid a reload')
  assert.equal(same.saved.length, 2, 'the identical result must remain a repeatable Reader event')

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

test('rebuilds the target continuous window before locating a cross-chapter result', async () => {
  const events = []
  const fixture = createFixture({
    isContinuousScrollRead: ref(true),
    navigate: async query => events.push(['navigate', query]),
    loadChapter: async (index, loadOptions) => events.push(['load', index, loadOptions]),
    rebuildContinuousWindow: async index => events.push(['rebuild', index]),
  })

  await fixture.controller.jumpToResult({
    chapterIndex: 3,
    percent: 0.4,
    query: '目标',
    resultCountWithinChapter: 0,
  })

  assert.deepEqual(events, [
    ['navigate', { chapter: 3, percent: 0.4 }],
    ['load', 3, { restorePercent: 0.4, saveAfterLoad: true }],
    ['rebuild', 3],
  ])
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

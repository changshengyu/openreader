import assert from 'node:assert/strict'
import test from 'node:test'
import { ref } from 'vue'
import { useReaderNavigation } from '../src/composables/useReaderNavigation.js'
import { READER_CHAPTER_END_OFFSET } from '../src/utils/readerPosition.js'

function createNavigation(overrides = {}) {
  const navigated = []
  const saved = []
  const scheduled = []
  const options = {
    contentEl: ref(null),
    contentBody: ref(null),
    chapterBlocks: ref([]),
    chapters: ref([
      { id: 1, title: '第一章' },
      { id: 2, title: '第二章' },
      { id: 3, title: '第三章' },
    ]),
    currentIndex: ref(1),
    chapter: ref({ id: 2, title: '第二章' }),
    content: ref('正文'),
    page: ref(1),
    pageCount: ref(3),
    progressVersion: ref(0),
    isContinuousScrollRead: ref(false),
    isVerticalRead: ref(false),
    getMode: () => 'flip',
    getAnimateDuration: () => 200,
    scrollStep: () => 600,
    scrollBehavior: () => 'smooth',
    jumpToParagraph: () => {},
    closeToc: () => {},
    navigate: async query => navigated.push(query),
    saveProgress: () => saved.push(true),
    scheduleProgressSave: delay => scheduled.push(delay),
    ...overrides,
  }
  return {
    navigation: useReaderNavigation(options),
    navigated,
    options,
    saved,
    scheduled,
  }
}

test('moves within flip pages before crossing chapter boundaries', async () => {
  const { navigation, options, navigated, saved } = createNavigation()
  await navigation.previousPage()
  assert.equal(options.page.value, 0)
  assert.equal(saved.length, 1)
  assert.deepEqual(navigated, [])

  await navigation.nextPage()
  assert.equal(options.page.value, 1)
  assert.equal(saved.length, 2)
})

test('routes to adjacent chapters at page boundaries', async () => {
  const previous = createNavigation({
    page: ref(0),
  })
  await previous.navigation.previousPage()
  assert.deepEqual(previous.navigated, [{
    chapter: 0,
    offset: READER_CHAPTER_END_OFFSET,
  }])

  const next = createNavigation({
    page: ref(2),
  })
  await next.navigation.nextPage()
  assert.deepEqual(next.navigated, [{ chapter: 2 }])
})

test('scrolls vertical pages and schedules progress without changing chapters', async () => {
  const animationCalls = []
  const fixture = createNavigation({
    contentEl: ref({
      scrollTop: 700,
      scrollHeight: 3000,
      clientHeight: 800,
      scrollBy: () => assert.fail('native smooth scrolling must not own the configured duration'),
    }),
    isVerticalRead: ref(true),
    getMode: () => 'scroll',
    scrollAnimator: {
      isActive: () => false,
      scrollBy: (element, delta, duration, onFinish) => {
        animationCalls.push({ element, delta, duration })
        onFinish()
        return true
      },
    },
  })
  await fixture.navigation.previousPage()
  await fixture.navigation.nextPage()

  assert.deepEqual(animationCalls.map(({ delta, duration }) => ({ delta, duration })), [
    { delta: -600, duration: 200 },
    { delta: 600, duration: 200 },
  ])
  assert.deepEqual(fixture.scheduled, [60, 60])
  assert.deepEqual(fixture.navigated, [])
})

test('rebuilds an explicitly selected loaded chapter before jumping in continuous mode', async () => {
  const calls = []
  const targetChapter = {
    offsetTop: 900,
    offsetHeight: 700,
    querySelector: () => null,
  }
  const fixture = createNavigation({
    contentEl: ref({
      scrollTop: 200,
      clientHeight: 600,
      scrollTo: value => calls.push(['scroll', value]),
    }),
    contentBody: ref({
      querySelector: selector => selector.includes('"2"') ? targetChapter : null,
    }),
    chapterBlocks: ref([
      { index: 1, id: 2, title: '第二章', content: '正文 1' },
      { index: 2, id: 3, title: '第三章', content: '正文 2' },
    ]),
    isContinuousScrollRead: ref(true),
    getMode: () => 'scroll2',
    rebuildContinuousWindow: async index => calls.push(['rebuild', index]),
  })

  await fixture.navigation.goChapter(2)
  assert.deepEqual(calls, [
    ['rebuild', 2],
    ['scroll', { top: 900, behavior: 'smooth' }],
  ])
  assert.equal(fixture.options.currentIndex.value, 2)
  assert.deepEqual(fixture.navigated, [])
})

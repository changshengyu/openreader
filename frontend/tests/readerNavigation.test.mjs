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
    scheduleSettlementTask: callback => {
      callback()
      return 1
    },
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

test('releases vertical paging immediately and defers only the settlement task boundary', async () => {
  const settled = []
  const tasks = new Map()
  let nextTaskId = 0
  let finishAnimation
  const fixture = createNavigation({
    contentEl: ref({
      scrollTop: 1000,
      scrollHeight: 3000,
      clientHeight: 800,
    }),
    isVerticalRead: ref(true),
    getMode: () => 'page',
    onVerticalPageSettled: () => settled.push('settled'),
    scheduleSettlementTask: (callback, delay) => {
      nextTaskId += 1
      tasks.set(nextTaskId, { callback, delay })
      return nextTaskId
    },
    cancelSettlementTask: id => tasks.delete(id),
    scrollAnimator: {
      isActive: () => Boolean(finishAnimation),
      scrollBy: (_element, _delta, _duration, onFinish) => {
        finishAnimation = () => {
          finishAnimation = null
          onFinish()
        }
        return true
      },
    },
  })

  await fixture.navigation.nextPage()
  assert.deepEqual(settled, [])
  finishAnimation()
  assert.deepEqual(settled, [])
  assert.equal(tasks.size, 1)
  const [{ callback, delay }] = tasks.values()
  assert.equal(delay, 0, 'upstream releases transforming immediately; it only delays progress persistence')
  callback()
  assert.deepEqual(settled, ['settled'])
})

test('uses upstream cubic paging, rejects overlap, and accepts a new tap after motion', async () => {
  const animations = []
  const tasks = new Map()
  let nextTaskId = 0
  let active = false
  let settled = 0
  const fixture = createNavigation({
    contentEl: ref({
      scrollTop: 1000,
      scrollHeight: 4000,
      clientHeight: 800,
    }),
    isVerticalRead: ref(true),
    getMode: () => 'scroll2',
    onVerticalPageSettled: () => { settled += 1 },
    scheduleSettlementTask: (callback, delay) => {
      nextTaskId += 1
      tasks.set(nextTaskId, { callback, delay })
      return nextTaskId
    },
    cancelSettlementTask: id => tasks.delete(id),
    scrollAnimator: {
      cancel: () => { active = false },
      isActive: () => active,
      scrollBy: (_element, delta, duration, onFinish, animationOptions) => {
        active = true
        animations.push({ delta, duration, onFinish, animationOptions })
        return true
      },
    },
  })

  await fixture.navigation.nextPage()
  await fixture.navigation.nextPage()
  await fixture.navigation.previousPage()
  assert.equal(animations.length, 1, 'upstream transforming guard must reject both overlap directions')
  assert.equal(animations[0].animationOptions, undefined, 'vertical click paging must use the default upstream cubic curve')

  active = false
  animations[0].onFinish()
  assert.equal(tasks.size, 1)
  assert.equal(fixture.navigation.isVerticalScrollSyncSuppressed(), true)

  await fixture.navigation.nextPage()
  assert.equal(animations.length, 2, 'a new tap after visual completion must start immediately')
  assert.equal(tasks.size, 0, 'new motion must cancel stale delayed settlement')
  assert.equal(settled, 0)

  active = false
  animations[1].onFinish()
  const [{ callback }] = tasks.values()
  callback()
  assert.equal(settled, 1)
  assert.equal(fixture.navigation.isVerticalScrollSyncSuppressed(), false)
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
    scrollAnimator: {
      cancel: () => calls.push(['cancel']),
      isActive: () => false,
      scrollBy: () => false,
      scrollTo: (element, top, duration) => {
        calls.push(['animate-scroll', top, duration])
        element.scrollTop = top
        return true
      },
    },
  })

  await fixture.navigation.goChapter(2)
  assert.deepEqual(calls, [
    ['cancel'],
    ['rebuild', 2],
    ['animate-scroll', 900, 200],
  ])
  assert.equal(fixture.options.currentIndex.value, 2)
  assert.deepEqual(fixture.navigated, [])
})

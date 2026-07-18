import assert from 'node:assert/strict'
import test from 'node:test'
import { nextTick, ref } from 'vue'
import { useReaderChapterReady } from '../src/composables/useReaderChapterReady.js'

function createController() {
  const currentIndex = ref(0)
  const chapterLoaded = ref(true)
  const chapterLoading = ref(false)
  const chapterLoadError = ref('')
  const scope = ref('book:1')
  const controller = useReaderChapterReady({
    currentIndex,
    chapterLoaded,
    chapterLoading,
    chapterLoadError,
    getScopeKey: () => scope.value,
  })
  return {
    chapterLoadError,
    chapterLoaded,
    chapterLoading,
    controller,
    currentIndex,
    scope,
  }
}

test('resolves only after the requested chapter reaches its real ready state', async () => {
  const fixture = createController()
  fixture.chapterLoaded.value = false
  const pending = fixture.controller.wait(2)
  let settled = false
  pending.finally(() => {
    settled = true
  })

  fixture.currentIndex.value = 2
  fixture.chapterLoading.value = true
  await nextTick()
  assert.equal(settled, false)

  fixture.chapterLoading.value = false
  fixture.chapterLoaded.value = true
  await pending
  assert.equal(settled, true)
})

test('rejects a chapter-ready waiter on load failure or book scope change', async () => {
  const failed = createController()
  failed.chapterLoaded.value = false
  const failedWait = failed.controller.wait(1)
  failed.currentIndex.value = 1
  failed.chapterLoadError.value = '章节加载失败'
  await assert.rejects(failedWait, /章节加载失败/)

  const stale = createController()
  stale.chapterLoaded.value = false
  const staleWait = stale.controller.wait(1)
  stale.scope.value = 'book:2'
  await assert.rejects(staleWait, /reader scope changed/)
})

test('aborts an obsolete chapter-ready waiter without leaving a live transaction', async () => {
  const fixture = createController()
  fixture.chapterLoaded.value = false
  const abortController = new AbortController()
  const pending = fixture.controller.wait(2, { signal: abortController.signal })
  abortController.abort()
  await assert.rejects(pending, error => error?.name === 'AbortError')
  assert.equal(fixture.controller.pendingCount(), 0)
  fixture.controller.dispose()
})

import assert from 'node:assert/strict'
import test from 'node:test'
import { effectScope, ref } from 'vue'
import { useReaderToc } from '../src/composables/useReaderToc.js'

// Error surface contract R5 (sections 7 and 13 item 6): the catalog panel's 刷新 after a failed book load reloads the
// whole book, so the route or saved target chapter loads once the catalog arrives.

function createToc(overrides = {}) {
  const calls = []
  const scope = effectScope()
  let toc
  scope.run(() => {
    toc = useReaderToc({
      chapters: ref([]),
      isRemoteBook: ref(false),
      isTemporaryReader: ref(false),
      onUnavailable: () => calls.push(['unavailable']),
      refreshCachedChapters: async () => calls.push(['cached']),
      syncCurrentChapter: () => calls.push(['sync']),
      goChapter: async index => calls.push(['go', index]),
      refreshRemoteCatalog: async () => calls.push(['remote-catalog']),
      refreshLocalCatalog: async () => calls.push(['local-catalog']),
      ...overrides,
    })
  })
  return { calls, scope, toc }
}

test('refreshing the catalog after a failed book load reloads the whole book', async () => {
  const fixture = createToc({
    isBookLoadFailed: () => true,
    reloadBook: async () => {
      fixture.calls.push(['reload-book'])
      return true
    },
  })
  await fixture.toc.refresh()
  assert.deepEqual(
    fixture.calls.filter(call => ['reload-book', 'remote-catalog', 'local-catalog'].includes(call[0])),
    [['reload-book']],
  )
  assert.equal(fixture.toc.refreshing.value, false)
  fixture.scope.stop()
})

test('refreshing a healthy local catalog keeps the local catalog refresh', async () => {
  const fixture = createToc({
    isBookLoadFailed: () => false,
    reloadBook: async () => {
      fixture.calls.push(['reload-book'])
      return true
    },
  })
  await fixture.toc.refresh()
  assert.deepEqual(
    fixture.calls.filter(call => ['reload-book', 'remote-catalog', 'local-catalog'].includes(call[0])),
    [['local-catalog']],
  )
  fixture.scope.stop()
})

import assert from 'node:assert/strict'
import test from 'node:test'
import { ref } from 'vue'
import { useReaderToc } from '../src/composables/useReaderToc.js'
import {
  readerTocTargetIndex,
  toggleReaderTocReverse,
} from '../src/utils/readerToc.js'

test('clamps reader table-of-contents targets to available chapters', () => {
  assert.equal(readerTocTargetIndex(-2, 10), 0)
  assert.equal(readerTocTargetIndex(4.8, 10), 4)
  assert.equal(readerTocTargetIndex(30, 10), 9)
  assert.equal(readerTocTargetIndex(3, 0), 0)
})

test('toggles reader table-of-contents order', () => {
  assert.equal(toggleReaderTocReverse(false), true)
  assert.equal(toggleReaderTocReverse(true), false)
})

function createToc(failed) {
  const calls = []
  const toc = useReaderToc({
    chapters: ref(Array.from({ length: 8 }, (_, index) => ({ id: 11 + index, bookId: 7, index, title: `第${index + 1}章` }))),
    currentIndex: ref(5),
    goChapter: async index => calls.push(['go', index]),
    isCurrentChapterFailed: () => failed,
    reloadCurrentChapter: async index => calls.push(['reload', index]),
  })
  return { calls, toc }
}

// docs/compat/reader-failure-progress-payload-fixed-baseline-second-audit-p0-contract.md P6: reader-dev's catalog press is
// getContent(index), so pressing the failed current chapter retries it.
test('the catalog reloads a failed current chapter instead of jumping within it (P6)', async () => {
  const { calls, toc } = createToc(true)
  await toc.jump(5)
  assert.deepEqual(calls, [['reload', 5]])
})

test('the catalog still jumps for a healthy current chapter and for other chapters (P6 control)', async () => {
  const healthy = createToc(false)
  await healthy.toc.jump(5)
  assert.deepEqual(healthy.calls, [['go', 5]])
  const other = createToc(true)
  await other.toc.jump(4)
  assert.deepEqual(other.calls, [['go', 4]])
})

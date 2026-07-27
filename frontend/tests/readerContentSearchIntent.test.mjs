import assert from 'node:assert/strict'
import test from 'node:test'
import { nextTick, ref } from 'vue'
import { useReaderContentSearchIntent } from '../src/composables/useReaderContentSearchIntent.js'

test('consumes repeated same-result requests and ignores another book', async () => {
  const request = ref(null)
  const calls = []
  const stop = useReaderContentSearchIntent({
    request,
    book: ref({ id: 7, bookUrl: 'https://book.example/7' }),
    bookId: ref(7),
    jumpToResult: async result => calls.push(result),
  })

  const result = { chapterIndex: 3, resultCountWithinChapter: 1 }
  request.value = {
    requestId: 1,
    bookId: 7,
    bookUrl: 'https://book.example/7',
    query: '目标',
    result,
  }
  await nextTick()
  assert.deepEqual(calls, [{ ...result, query: '目标' }])

  request.value = {
    ...request.value,
    requestId: 2,
  }
  await nextTick()
  assert.deepEqual(calls, [
    { ...result, query: '目标' },
    { ...result, query: '目标' },
  ])

  request.value = {
    ...request.value,
    requestId: 3,
    bookId: 8,
    bookUrl: 'https://book.example/8',
  }
  await nextTick()
  assert.equal(calls.length, 2)
  stop()
})

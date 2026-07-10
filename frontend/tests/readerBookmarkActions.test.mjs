import assert from 'node:assert/strict'
import test from 'node:test'
import { ref } from 'vue'
import { useReaderBookmarkActions } from '../src/composables/useReaderBookmarkActions.js'

function createActions(overrides = {}) {
  const formCalls = []
  const actions = useReaderBookmarkActions({
    book: ref({ id: 7, title: '测试书', author: '测试作者' }),
    chapter: ref({ id: 12, title: '第三章' }),
    currentIndex: ref(2),
    getOffset: () => 240,
    getPercent: () => 0.4,
    getExcerpt: () => '当前摘录',
    openForm: async (...args) => {
      formCalls.push(args)
      return { saved: false }
    },
    ...overrides,
  })
  return { actions, formCalls }
}

test('routes current, selected-text, and note bookmarks through the shared form without direct writes', async () => {
  const { actions, formCalls } = createActions()
  await actions.createCurrent()
  await actions.createFromSelectedText(`  ${'选'.repeat(520)}  `)
  await actions.openNote()

  assert.equal(formCalls.length, 3)
  assert.deepEqual(formCalls[0], [{ id: 7, title: '测试书', author: '测试作者' }, {
    chapterId: 12,
    chapterIndex: 2,
    offset: 240,
    percent: 0.4,
    title: '第三章',
    excerpt: '当前摘录',
    note: '',
  }, { mode: 'create' }])
  assert.equal(formCalls[1][1].excerpt.length, 500)
  assert.equal(formCalls[2][1].note, '')
})

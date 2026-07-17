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
    getSelectedTextContext: () => ({ excerpt: '定位段落\n后续段落' }),
    onSelectedTextNotFound: () => formCalls.push(['not-found']),
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
  const currentDraft = actions.currentDraft()
  assert.equal(formCalls.length, 0)
  assert.deepEqual(currentDraft, {
    chapterId: 12,
    chapterIndex: 2,
    offset: 240,
    percent: 0.4,
    title: '第三章',
    excerpt: '当前摘录',
    note: '',
  })
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
  assert.equal(formCalls[1][1].excerpt, '定位段落\n后续段落')
  assert.equal(formCalls[2][1].note, '')
})

test('does not expose a current-page draft without a readable chapter or excerpt', () => {
  assert.equal(createActions({ chapter: ref(null) }).actions.currentDraft(), null)
  assert.equal(createActions({ getExcerpt: () => '' }).actions.currentDraft(), null)
})

test('does not open a bookmark form when selected text cannot map to reader paragraphs', async () => {
  const { actions, formCalls } = createActions({
    getSelectedTextContext: () => null,
  })
  await actions.createFromSelectedText('无法定位的选择')
  assert.deepEqual(formCalls, [['not-found']])
})

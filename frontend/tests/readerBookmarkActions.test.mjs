import assert from 'node:assert/strict'
import test from 'node:test'
import { ref } from 'vue'
import { useReaderBookmarkActions } from '../src/composables/useReaderBookmarkActions.js'

function createActions(overrides = {}) {
  const created = []
  const actions = useReaderBookmarkActions({
    chapter: ref({ id: 12, title: '第三章' }),
    currentIndex: ref(2),
    getOffset: () => 240,
    getPercent: () => 0.4,
    getExcerpt: () => '当前摘录',
    create: async payload => {
      created.push(payload)
      return payload
    },
    onToast: () => {},
    ...overrides,
  })
  return { actions, created }
}

test('creates current, selected-text, and note bookmarks from one position source', async () => {
  const { actions, created } = createActions()
  await actions.createCurrent()
  await actions.createFromSelectedText(`  ${'选'.repeat(520)}  `)
  actions.openNote()
  actions.noteText.value = '  我的笔记  '
  await actions.saveNote()

  assert.equal(created.length, 3)
  assert.deepEqual(created[0], {
    chapterId: 12,
    chapterIndex: 2,
    offset: 240,
    percent: 0.4,
    title: '第三章',
    excerpt: '当前摘录',
  })
  assert.equal(created[1].excerpt.length, 500)
  assert.equal(created[2].note, '我的笔记')
  assert.equal(actions.noteVisible.value, false)
})

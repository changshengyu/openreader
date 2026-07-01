import assert from 'node:assert/strict'
import test from 'node:test'
import { ref } from 'vue'
import { useReaderBookmarkActions } from '../src/composables/useReaderBookmarkActions.js'

function createActions(overrides = {}) {
  const created = []
  const navigated = []
  const reloaded = []
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
    update: async (_id, payload) => payload,
    remove: async () => {},
    removeMany: async () => [],
    importPayloads: async payloads => payloads,
    confirm: async () => {},
    closeDrawer: () => {},
    reloadCurrent: async target => reloaded.push(target),
    navigate: async query => navigated.push(query),
    onToast: () => {},
    onSuccess: () => {},
    onError: () => {},
    ...overrides,
  })
  return { actions, created, navigated, reloaded }
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

test('routes same-chapter bookmarks locally and other chapters through navigation', async () => {
  const { actions, navigated, reloaded } = createActions()
  await actions.jump({ chapterIndex: 2, offset: 88, percent: 1.5 })
  await actions.jump({ chapterIndex: 4, offset: 20, percent: 0.25 })

  assert.deepEqual(reloaded, [{ offset: 88, percent: 1 }])
  assert.deepEqual(navigated, [{
    chapter: 4,
    offset: 20,
    percent: 0.25,
  }])
})

test('opens and saves bookmark editor state through the shared update action', async () => {
  const updates = []
  const { actions } = createActions({
    update: async (id, payload) => {
      updates.push({ id, payload })
      return { id, ...payload }
    },
  })
  actions.openEditor({ id: 7, title: '旧标题', excerpt: '摘录', note: '笔记' })
  actions.draft.title = '新标题'
  await actions.saveEdit()

  assert.deepEqual(updates, [{
    id: 7,
    payload: { title: '新标题', excerpt: '摘录', note: '笔记' },
  }])
  assert.equal(actions.editorVisible.value, false)
})

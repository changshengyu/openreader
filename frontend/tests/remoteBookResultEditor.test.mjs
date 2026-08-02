import assert from 'node:assert/strict'
import test from 'node:test'
import { useRemoteBookResultEditor } from '../src/composables/useRemoteBookResultEditor.js'

function deferred() {
  let resolve
  let reject
  const promise = new Promise((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

function fixture(overrides = {}) {
  let identity = { scope: 'user-a', token: 'token-a' }
  const creates = []
  const upserts = []
  const successes = []
  const errors = []
  const editor = useRemoteBookResultEditor({
    getAuthenticatedIdentity: () => identity,
    confirm: async () => true,
    createRemoteBook: async payload => {
      creates.push(payload)
      return { data: { id: 9, ...payload } }
    },
    upsertBook: book => upserts.push(book),
    onSuccess: message => successes.push(message),
    onError: (_error, fallback) => errors.push(fallback),
    ...overrides,
  })
  return {
    editor,
    creates,
    upserts,
    successes,
    errors,
    setIdentity(value) {
      identity = value
    },
  }
}

const resultBook = {
  title: '测试书',
  author: '作者',
  bookUrl: 'https://books.example/1',
  sourceId: 7,
  sourceName: '测试源',
}

test('result editor rejects malformed JSON and each missing required field without writing', async () => {
  const state = fixture()
  state.editor.open(resultBook)

  for (const [content, message] of [
    ['{', '书籍信息必须是JSON格式'],
    [JSON.stringify({ ...resultBook, title: '', name: '' }), '书籍名称不能为空'],
    [JSON.stringify({ ...resultBook, bookUrl: '', url: '' }), '书籍链接不能为空'],
    [JSON.stringify({ ...resultBook, sourceId: 0, bookSourceId: 0 }), '书籍来源不能为空'],
  ]) {
    state.editor.content.value = content
    await state.editor.save()
    assert.equal(state.errors.at(-1), message)
  }

  assert.equal(state.creates.length, 0)
  assert.equal(state.editor.visible.value, true)
})

test('cancelling result editor confirmation keeps the editor open and performs zero writes', async () => {
  const state = fixture({ confirm: async () => { throw new Error('cancel') } })
  state.editor.open(resultBook)
  await state.editor.save()
  assert.equal(state.creates.length, 0)
  assert.equal(state.editor.visible.value, true)
})

test('successful result edit writes once, upserts once and closes the editor', async () => {
  const state = fixture()
  state.editor.open(resultBook)
  state.editor.content.value = JSON.stringify({ ...resultBook, title: '修改后' })
  const saved = await state.editor.save()
  await state.editor.save()

  assert.equal(saved.title, '修改后')
  assert.equal(state.creates.length, 1)
  assert.equal(state.upserts.length, 1)
  assert.deepEqual(state.successes, ['修改书籍成功'])
  assert.equal(state.editor.visible.value, false)
})

test('failed save stays editable and account changes reject late writes', async () => {
  const failure = fixture({ createRemoteBook: async () => { throw new Error('offline') } })
  failure.editor.open(resultBook)
  await failure.editor.save()
  assert.equal(failure.editor.visible.value, true)
  assert.equal(failure.editor.saving.value, false)
  assert.equal(failure.errors.at(-1), '保存书籍失败')

  const response = deferred()
  const state = fixture({ createRemoteBook: () => response.promise })
  state.editor.open(resultBook)
  const saving = state.editor.save()
  await Promise.resolve()
  state.setIdentity({ scope: 'user-b', token: 'token-b' })
  response.resolve({ data: { id: 10, ...resultBook } })
  assert.equal(await saving, null)
  assert.equal(state.upserts.length, 0)
  assert.equal(state.successes.length, 0)
  assert.equal(state.editor.visible.value, true)
  assert.equal(state.editor.saving.value, false)
})

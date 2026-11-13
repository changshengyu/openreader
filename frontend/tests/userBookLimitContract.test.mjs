import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

import { useRemoteBookAddToShelf } from '../src/composables/useRemoteBookAddToShelf.js'
import { useRemoteBookResultEditor } from '../src/composables/useRemoteBookResultEditor.js'
import { useStorageImportWorkflow } from '../src/composables/useStorageImportWorkflow.js'

// Contract: docs/compat/user-book-limit-fixed-baseline-second-audit-p2-contract.md
// reader-dev BookController.kt#saveBook answers 超过用户书籍数上限, which
// plugins/axios.js shows as Message.error. OpenReader answers 403
// {"error":"book limit reached"} (per item in batch imports).
const __dirname = dirname(fileURLToPath(import.meta.url))
const read = path => readFileSync(resolve(__dirname, '..', path), 'utf8')
const optionalModule = async path => import(new URL(`../${path}`, import.meta.url)).catch(() => null)

const UPSTREAM_TEXT = '超过用户书籍数上限'
const httpError = (status, error) => ({ response: { status, data: { error } } })

// The two readError shapes that format add-to-shelf failures today:
// OverlayBookInfo.vue, and Search.vue / Discover.vue.
const bookInfoReadError = (error, fallback) => error?.response?.data?.error?.message ||
  error?.response?.data?.error ||
  fallback
const workspaceReadError = (error, fallback) => error?.response?.data?.error?.message ||
  error?.response?.data?.error ||
  error?.message ||
  fallback

test('bookLimit.js turns only the book-limit refusal into the upstream text', async () => {
  const limit = await optionalModule('src/utils/bookLimit.js')
  assert.ok(limit?.bookLimitMessage, 'src/utils/bookLimit.js must export bookLimitMessage')
  assert.equal(limit.BOOK_LIMIT_MESSAGE, UPSTREAM_TEXT)
  assert.equal(limit.bookLimitMessage(httpError(403, 'book limit reached')), UPSTREAM_TEXT)
  // Batch imports report the refusal per item as a plain string.
  assert.equal(limit.bookLimitMessage('book limit reached'), UPSTREAM_TEXT)
  for (const other of [
    httpError(403, 'user limit reached'),
    httpError(500, 'book limit reached'),
    httpError(400, 'book title is required'),
    new Error('offline'),
    'failed to import book',
    '',
    null,
    undefined,
  ]) {
    assert.equal(limit.bookLimitMessage(other), '', JSON.stringify(other))
  }
})

test('bookLimitDisplayError hands callers the upstream text and leaves other errors alone', async () => {
  const limit = await optionalModule('src/utils/bookLimit.js')
  assert.ok(limit?.bookLimitDisplayError, 'src/utils/bookLimit.js must export bookLimitDisplayError')
  const shown = limit.bookLimitDisplayError(httpError(403, 'book limit reached'))
  assert.equal(bookInfoReadError(shown, '加入书架失败'), UPSTREAM_TEXT)
  assert.equal(workspaceReadError(shown, '加入书架失败'), UPSTREAM_TEXT)
  const other = httpError(400, 'source not found')
  assert.equal(limit.bookLimitDisplayError(other), other)
  const offline = new Error('offline')
  assert.equal(limit.bookLimitDisplayError(offline), offline)
})

test('adding a remote book at the limit reports the upstream text to every caller', async () => {
  const errors = []
  const controller = useRemoteBookAddToShelf({
    buildPayload: book => ({ title: book.title }),
    createRemoteBook: async () => {
      throw httpError(403, 'book limit reached')
    },
    upsertBook: () => assert.fail('a refused add must not upsert'),
    onSuccess: () => assert.fail('a refused add must not report success'),
    onError: (error, fallback) => errors.push([error, fallback]),
  })
  const added = await controller.addRemoteBook({ title: '上限书', bookUrl: 'https://books.example/limit' }, { key: 'limit' })
  assert.equal(added, null)
  assert.equal(errors.length, 1)
  const [error, fallback] = errors[0]
  assert.equal(fallback, '加入书架失败')
  assert.equal(bookInfoReadError(error, fallback), UPSTREAM_TEXT)
  assert.equal(workspaceReadError(error, fallback), UPSTREAM_TEXT)

  // Any other failure still reaches the caller unchanged.
  const offline = new Error('offline')
  const passed = []
  const other = useRemoteBookAddToShelf({
    buildPayload: book => ({ title: book.title }),
    createRemoteBook: async () => {
      throw offline
    },
    upsertBook: () => {},
    onSuccess: () => {},
    onError: (error, fallback) => passed.push([error, fallback]),
  })
  await other.addRemoteBook({ title: '离线书' }, { key: 'offline' })
  assert.deepEqual(passed, [[offline, '加入书架失败']])
})

test('saving an edited search result at the limit reports the upstream text', async () => {
  const errors = []
  const editor = useRemoteBookResultEditor({
    getAuthenticatedIdentity: () => ({ scope: 'user:1', token: 'token-1' }),
    confirm: async () => true,
    createRemoteBook: async () => {
      throw httpError(403, 'book limit reached')
    },
    upsertBook: () => assert.fail('a refused save must not upsert'),
    onSuccess: () => assert.fail('a refused save must not report success'),
    onError: (error, fallback) => errors.push([error, fallback]),
  })
  editor.open({ title: '编辑书', bookUrl: 'https://books.example/edit', sourceId: 7, sourceName: '测试源' })
  assert.equal(await editor.save(), null)
  assert.equal(errors.length, 1)
  const [error, fallback] = errors[0]
  assert.equal(fallback, '保存书籍失败')
  assert.equal(workspaceReadError(error, fallback), UPSTREAM_TEXT)
  assert.equal(bookInfoReadError(error, fallback), UPSTREAM_TEXT)
})

function stagedRow(path, token) {
  return {
    path,
    importToken: token,
    book: { title: path.replace(/\.txt$/, ''), author: '', chapterCount: 1, chapters: [{ index: 0, title: '第一章' }] },
  }
}

test('a storage import item refused at the limit shows the upstream text', async () => {
  const shown = []
  const workflow = useStorageImportWorkflow({
    preview: async () => ({ items: [stagedRow('limit.txt', 'a'.repeat(48))] }),
    importItem: async () => ({ imported: [{ path: 'limit.txt', error: 'book limit reached' }] }),
    onError: message => shown.push(message),
    onComplete: () => {},
  })
  await workflow.start({ source: 'local-store', paths: ['limit.txt'] })
  assert.equal(await workflow.confirmCurrent(), false)
  assert.equal(workflow.currentRow.value.lastError, UPSTREAM_TEXT)
  assert.deepEqual(shown, [UPSTREAM_TEXT])
})

test('a direct upload refused at the limit shows the upstream text', async () => {
  const shown = []
  const file = { name: 'limit.txt' }
  const workflow = useStorageImportWorkflow({
    preview: async () => ({ items: [{ key: 'direct:0', ...stagedRow('limit.txt', 'b'.repeat(48)) }] }),
    importItem: async () => {
      throw httpError(403, 'book limit reached')
    },
    onError: message => shown.push(message),
    onComplete: () => {},
  })
  await workflow.start({ source: 'direct', files: [file] })
  assert.equal(await workflow.confirmCurrent(), false)
  assert.equal(workflow.currentRow.value.lastError, UPSTREAM_TEXT)
  assert.deepEqual(shown, [UPSTREAM_TEXT])
})

test('the search workspace local import shows the upstream text when an item hits the limit', () => {
  const search = read('src/views/Search.vue')
  assert.match(search, /import \{[^}]*\bBOOK_LIMIT_MESSAGE\b[^}]*\bbookLimitMessage\b[^}]*\} from '\.\.\/utils\/bookLimit'/)
  const start = search.indexOf('async function importLocalPaths(')
  assert.notEqual(start, -1)
  const body = search.slice(start, search.indexOf('\nfunction ', start))
  assert.match(body, /imported\.some\(item => bookLimitMessage\(item\.error\)\)/)
  assert.match(body, /ElMessage\.error\(BOOK_LIMIT_MESSAGE\)/)
})

test('authErrors.js keeps its own table untouched by the book limit', () => {
  const authErrors = read('src/utils/authErrors.js')
  assert.doesNotMatch(authErrors, /book limit|书籍数上限/)
})

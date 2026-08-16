import assert from 'node:assert/strict'
import test from 'node:test'

import { useStorageImportWorkflow } from '../src/composables/useStorageImportWorkflow.js'

function deferred() {
  let resolve
  let reject
  const promise = new Promise((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

function previewRow(path, token, overrides = {}) {
  return {
    path,
    importToken: token,
    book: {
      title: path.replace(/\.txt$/, ''),
      author: '',
      chapterCount: 1,
      chapters: [{ index: 0, title: '第一章' }],
      ...overrides.book,
    },
    ...overrides,
  }
}

function createWorkflow(overrides = {}) {
  const calls = []
  const completed = []
  const workflow = useStorageImportWorkflow({
    preview: async (_source, payload) => {
      calls.push(['preview', payload])
      return { items: overrides.previewItems || [] }
    },
    importItem: async (source, item, categoryIds) => {
      calls.push(['import', source, item.path, item.importToken, [...categoryIds]])
      return { imported: [{ path: item.path, book: { id: calls.length, title: item.title } }] }
    },
    onComplete: summary => completed.push(summary),
    ...overrides,
  })
  return { workflow, calls, completed }
}

test('opens one valid staged preview directly in the single-book confirmation with no default group', async () => {
  const { workflow, calls } = createWorkflow({
    previewItems: [previewRow('one.txt', 'a'.repeat(48))],
  })

  await workflow.start({ source: 'local-store', paths: ['one.txt'] })

  assert.equal(workflow.phase.value, 'single')
  assert.equal(workflow.currentRow.value.path, 'one.txt')
  assert.deepEqual(workflow.currentRow.value.categoryIds, [])
  assert.deepEqual(calls, [['preview', ['one.txt']]])
})

test('treats a staged empty catalogue as a valid upstream local-book preview', async () => {
  const { workflow } = createWorkflow({
    previewItems: [previewRow('empty.txt', 'z'.repeat(48), {
      book: { chapterCount: 0, chapters: [] },
    })],
  })

  await workflow.start({ source: 'local-store', paths: ['empty.txt'] })

  assert.equal(workflow.phase.value, 'single')
  assert.equal(workflow.currentRow.value.chapterCount, 0)
  assert.equal(workflow.currentRow.value.valid, true)
})

test('distinguishes multi-book batch, sequential, and close cancellation transitions', async () => {
  const rows = [previewRow('one.txt', 'a'.repeat(48)), previewRow('two.txt', 'b'.repeat(48))]

  const batch = createWorkflow({ previewItems: rows })
  await batch.workflow.start({ source: 'webdav', paths: ['one.txt', 'two.txt'] })
  assert.equal(batch.workflow.phase.value, 'choose-mode')
  batch.workflow.chooseBatch()
  assert.equal(batch.workflow.phase.value, 'batch-groups')
  batch.workflow.cancelBatchGroups()
  assert.equal(batch.workflow.phase.value, 'idle')
  assert.equal(batch.calls.filter(([kind]) => kind === 'import').length, 0, 'cancelling shared groups must not write a book')

  const sequential = createWorkflow({ previewItems: rows })
  await sequential.workflow.start({ source: 'webdav', paths: ['one.txt', 'two.txt'] })
  sequential.workflow.chooseSequential()
  assert.equal(sequential.workflow.currentLabel.value, '（1/2）')
  sequential.workflow.skipCurrent()
  assert.equal(sequential.workflow.currentRow.value.path, 'two.txt', 'cancelling one sequential item must advance to the next one')
  await sequential.workflow.confirmCurrent()
  assert.equal(sequential.workflow.phase.value, 'idle')
  assert.deepEqual(sequential.calls.filter(([kind]) => kind === 'import'), [
    ['import', 'webdav', 'two.txt', 'b'.repeat(48), []],
  ])

  const closed = createWorkflow({ previewItems: rows })
  await closed.workflow.start({ source: 'local-store', paths: ['one.txt', 'two.txt'] })
  closed.workflow.cancelMode()
  assert.equal(closed.workflow.phase.value, 'idle')
  assert.equal(closed.calls.filter(([kind]) => kind === 'import').length, 0, 'closing the mode chooser cancels the whole flow')
})

test('runs batch imports one staged item at a time in preview order after explicit shared group confirmation', async () => {
  const { workflow, calls, completed } = createWorkflow({
    previewItems: [previewRow('one.txt', 'a'.repeat(48)), previewRow('two.txt', 'b'.repeat(48))],
  })

  await workflow.start({ source: 'local-store', paths: ['one.txt', 'two.txt'] })
  workflow.chooseBatch()
  workflow.batchCategoryIds.value = [9, 3, 9]
  await workflow.confirmBatch()

  assert.equal(workflow.phase.value, 'idle')
  assert.deepEqual(calls.filter(([kind]) => kind === 'import'), [
    ['import', 'local-store', 'one.txt', 'a'.repeat(48), [9, 3]],
    ['import', 'local-store', 'two.txt', 'b'.repeat(48), [9, 3]],
  ])
  assert.deepEqual(completed, [{ succeeded: 2, failed: 0, skipped: 0 }])
})

test('keeps a failed staged item in its single dialog and reparses the same token before import', async () => {
  const token = 'c'.repeat(48)
  const { workflow, calls } = createWorkflow({
    previewItems: [previewRow('retry.txt', token)],
    preview: async (_source, payload) => {
      calls.push(['preview', payload])
      if (Array.isArray(payload) && payload[0]?.importToken) {
        return { items: [previewRow('retry.txt', token, { book: { chapterCount: 2, chapters: [{ index: 0, title: '第一章' }, { index: 1, title: '第二章' }] } })] }
      }
      return { items: [previewRow('retry.txt', token)] }
    },
    importItem: async () => ({ imported: [{ path: 'retry.txt', error: 'temporary import failure' }] }),
  })

  await workflow.start({ source: 'local-store', paths: ['retry.txt'] })
  workflow.currentRow.value.tocRule = '^== .+ ==$'
  await workflow.reparse(workflow.currentRow.value)
  assert.equal(workflow.currentRow.value.importToken, token)
  assert.equal(workflow.currentRow.value.chapterCount, 2)
  assert.deepEqual(calls[1], ['preview', [{ key: 'retry.txt', path: 'retry.txt', importToken: token, title: 'retry', author: '', tocRule: '^== .+ ==$' }]])

  const imported = await workflow.confirmCurrent()
  assert.equal(imported, false)
  assert.equal(workflow.phase.value, 'single', 'a failed current import must stay available for retry or cancellation')
  assert.match(workflow.currentRow.value.lastError, /temporary import failure/)
})

test('stops a batch after session invalidation without upserting or completing the old flow', async () => {
  const firstResponse = deferred()
  let current = true
  const imported = []
  const { workflow, calls, completed } = createWorkflow({
    previewItems: [previewRow('one.txt', 'a'.repeat(48)), previewRow('two.txt', 'b'.repeat(48))],
    operationGuard: {
      begin: key => ({ key }),
      canCommit: () => current,
      reset: () => {
        current = false
      },
    },
    importItem: async (source, item, categoryIds) => {
      calls.push(['import', source, item.path, item.importToken, [...categoryIds]])
      if (item.path === 'one.txt') return firstResponse.promise
      return { imported: [{ path: item.path, book: { id: 2, title: item.title } }] }
    },
    onImported: book => imported.push(book),
  })

  await workflow.start({ source: 'local-store', paths: ['one.txt', 'two.txt'] })
  workflow.chooseBatch()
  const pending = workflow.confirmBatch()
  await Promise.resolve()
  current = false
  firstResponse.resolve({
    imported: [{ path: 'one.txt', book: { id: 1, title: 'one' } }],
  })
  assert.equal(await pending, false)

  assert.deepEqual(calls.filter(([kind]) => kind === 'import').map(([, , path]) => path), ['one.txt'])
  assert.deepEqual(imported, [])
  assert.deepEqual(completed, [])
})

test('a new direct selection aborts the old preview generation without a late error', async () => {
  const errors = []
  const requests = []
  const workflow = useStorageImportWorkflow({
    preview: async (_source, files, options) => {
      requests.push({ files, signal: options.signal })
      if (requests.length === 2) {
        return { items: [previewRow('second.txt', 'b'.repeat(48))] }
      }
      return new Promise((_resolve, reject) => {
        options.signal.addEventListener('abort', () => {
          reject(Object.assign(new Error('cancelled'), { code: 'ERR_CANCELED' }))
        }, { once: true })
      })
    },
    importItem: async () => ({ imported: [] }),
    onError: error => errors.push(error),
  })

  const first = workflow.start({ source: 'direct', files: [{ name: 'first.txt' }] })
  await Promise.resolve()
  const second = workflow.start({ source: 'direct', files: [{ name: 'second.txt' }] })

  assert.equal(await second, true)
  assert.equal(await first, false)
  assert.equal(requests[0].signal.aborted, true)
  assert.deepEqual(workflow.rows.value.map(row => row.path), ['second.txt'])
  assert.deepEqual(errors, [])
})

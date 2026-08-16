import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

import { previewDirectLocalBooks } from '../src/api/books.js'
import { useStorageImportWorkflow } from '../src/composables/useStorageImportWorkflow.js'

const directOverlaySource = readFileSync(new URL('../src/components/overlays/OverlayBookImport.vue', import.meta.url), 'utf8')
const sharedOverlaySource = readFileSync(new URL('../src/components/overlays/OverlayStorageImport.vue', import.meta.url), 'utf8')

function directPreviewRow(key, path, token) {
  return {
    key,
    path,
    importToken: token,
    book: {
      title: path.replace(/\.txt$/, ''),
      author: '',
      chapterCount: 1,
      chapters: [{ index: 0, title: '第一章' }],
    },
  }
}

test('the direct chooser is multiple and delegates confirmation to the shared import workflow', () => {
  assert.match(directOverlaySource, /<el-upload[^>]*\bmultiple\b[^>]*>/, 'the direct browser chooser must accept multiple files')
  assert.match(directOverlaySource, /openStorageImport\('direct', files\)/, 'the chooser must hand the ordered File list to the root import controller')
  assert.match(sharedOverlaySource, /useStorageImportWorkflow/, 'direct import must reuse the storage confirmation state machine')
  assert.match(sharedOverlaySource, /source === 'direct'/, 'the shared controller must provide a direct-file adapter')
  assert.doesNotMatch(directOverlaySource, /useOverlayBookImport/, 'the legacy single-file confirmation controller must be removed')
  assert.doesNotMatch(directOverlaySource, /v-model="draft\.(?:title|author|categoryIds|tocRule)"/, 'the chooser must not retain a second metadata confirmation form')
})

test('the shared confirmation keeps every non-empty TXT rule manually selectable', () => {
  assert.match(sharedOverlaySource, /listTXTTocRules/)
  assert.match(sharedOverlaySource, /filter\(rule => rule\?\.rule\)/, 'only structurally broken rules may be hidden')
  assert.doesNotMatch(sharedOverlaySource, /filter\(rule => rule\?\.enable/, 'disabled auto-detection rules must remain manually selectable')
  assert.match(sharedOverlaySource, /allow-create/, 'custom TXT rules must remain editable')
})

test('direct previews upload one file at a time and preserve parser failures in selection order', async () => {
  const first = deferred()
  const second = deferred()
  const calls = []
  const request = previewDirectLocalBooks([
    { name: 'same.txt', marker: 1 },
    { name: 'same.txt', marker: 2 },
  ], {
    previewBook: (file, payload, options) => {
      calls.push({ file, payload, options })
      return calls.length === 1 ? first.promise : second.promise
    },
  })

  assert.equal(calls.length, 1, 'the second upload must wait for the first preview')
  first.resolve({
    data: { title: '第一本', chapterCount: 1, importToken: 'a'.repeat(48) },
  })
  await Promise.resolve()
  assert.equal(calls.length, 2)
  second.reject(Object.assign(new Error('目录损坏'), {
    response: { data: { error: '目录损坏', importToken: 'b'.repeat(48) } },
  }))

  const result = await request
  assert.deepEqual(result.items.map(item => item.key), ['direct:0', 'direct:1'])
  assert.equal(result.items[0].book.title, '第一本')
  assert.equal(result.items[1].error, '目录损坏')
  assert.equal(result.items[1].importToken, 'b'.repeat(48))
})

test('cancelling a direct preview stops the ordered upload queue', async () => {
  const controller = new AbortController()
  let calls = 0
  const request = previewDirectLocalBooks([
    { name: 'first.txt' },
    { name: 'second.txt' },
  ], {
    signal: controller.signal,
    previewBook: (_file, _payload, options) => {
      calls += 1
      return new Promise((_resolve, reject) => {
        options.signal.addEventListener('abort', () => {
          reject(Object.assign(new Error('cancelled'), { code: 'ERR_CANCELED' }))
        }, { once: true })
      })
    },
  })

  controller.abort()
  await assert.rejects(request, error => error?.code === 'ERR_CANCELED')
  assert.equal(calls, 1)
})

test('shared import workflow accepts ordered direct files and keeps duplicate filenames as distinct rows', async () => {
  const files = [
    { name: 'same.txt', marker: 1 },
    { name: 'same.txt', marker: 2 },
  ]
  const calls = []
  const workflow = useStorageImportWorkflow({
    preview: async (source, payload) => {
      calls.push(['preview', source, payload])
      return {
        items: [
          directPreviewRow('direct:0', 'same.txt', 'a'.repeat(48)),
          directPreviewRow('direct:1', 'same.txt', 'b'.repeat(48)),
        ],
      }
    },
    importItem: async () => ({ imported: [] }),
  })

  const started = await workflow.start({ source: 'direct', files })

  assert.equal(started, true)
  assert.deepEqual(calls, [['preview', 'direct', files]])
  assert.equal(workflow.phase.value, 'choose-mode')
  assert.deepEqual(workflow.rows.value.map(row => row.key), ['direct:0', 'direct:1'])
  assert.deepEqual(workflow.rows.value.map(row => row.path), ['same.txt', 'same.txt'])
})

test('direct selection admits exactly 64 visible files and rejects 65 before preview', async () => {
  let previewCalls = 0
  const workflow = useStorageImportWorkflow({
    preview: async (_source, files) => {
      previewCalls += 1
      return {
        items: files.map((file, index) => directPreviewRow(`direct:${index}`, file.name, String(index).padStart(48, 'a').slice(-48))),
      }
    },
    importItem: async () => ({ imported: [] }),
  })
  const accepted = Array.from({ length: 64 }, (_, index) => ({ name: `book-${index}.txt` }))
  const rejected = [...accepted, { name: 'book-64.txt' }]

  assert.equal(await workflow.start({ source: 'direct', files: accepted }), true)
  assert.equal(workflow.rows.value.length, 64)
  assert.equal(previewCalls, 1)

  assert.equal(await workflow.start({ source: 'direct', files: rejected }), false)
  assert.equal(workflow.phase.value, 'idle')
  assert.equal(workflow.rows.value.length, 0)
  assert.equal(previewCalls, 1, 'over-cardinality selection must fail before any preview request')
})

test('direct confirmation submits only the staged token and selected groups', async () => {
  const calls = []
  const workflow = useStorageImportWorkflow({
    preview: async (source, payload) => {
      calls.push(['preview', source, payload])
      return { items: [directPreviewRow('direct:0', 'book.txt', 'c'.repeat(48))] }
    },
    importItem: async (source, item, categoryIds) => {
      calls.push(['import', source, item, categoryIds])
      return {
        imported: [{ key: item.key, path: item.path, book: { id: 9, title: item.title } }],
      }
    },
  })

  const file = { name: 'book.txt', privateBytes: 'must not survive preview' }
  assert.equal(await workflow.start({ source: 'direct', files: [file] }), true)
  workflow.currentRow.value.categoryIds = [3, '4']
  assert.equal(await workflow.confirmCurrent(), true)

  const importCall = calls.find(call => call[0] === 'import')
  assert.equal(importCall[1], 'direct')
  assert.equal(importCall[2].importToken, 'c'.repeat(48))
  assert.equal('file' in importCall[2], false)
  assert.equal('privateBytes' in importCall[2], false)
  assert.deepEqual(importCall[3], [3, 4])
})

test('direct selection rejects any hidden legacy format before previewing valid neighbors', async () => {
  let previewCalls = 0
  const workflow = useStorageImportWorkflow({
    preview: async () => {
      previewCalls += 1
      return { items: [] }
    },
    importItem: async () => ({ imported: [] }),
  })

  const started = await workflow.start({
    source: 'direct',
    files: [{ name: 'visible.epub' }, { name: 'hidden.pdf' }],
  })

  assert.equal(started, false)
  assert.equal(workflow.phase.value, 'idle')
  assert.equal(previewCalls, 0)
})

function deferred() {
  let resolve
  let reject
  const promise = new Promise((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

import { useStorageImportWorkflow } from '../src/composables/useStorageImportWorkflow.js'

const directOverlaySource = readFileSync(new URL('../src/components/overlays/OverlayBookImport.vue', import.meta.url), 'utf8')

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
  assert.match(directOverlaySource, /useStorageImportWorkflow/, 'direct import must reuse the storage confirmation state machine')
  assert.doesNotMatch(directOverlaySource, /useOverlayBookImport/, 'the legacy single-file confirmation controller must be removed')
  assert.doesNotMatch(directOverlaySource, /v-model="draft\.(?:title|author|categoryIds|tocRule)"/, 'the chooser must not retain a second metadata confirmation form')
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

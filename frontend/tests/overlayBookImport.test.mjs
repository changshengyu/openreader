import assert from 'node:assert/strict'
import test from 'node:test'
import { nextTick, ref } from 'vue'
import { useOverlayBookImport } from '../src/composables/useOverlayBookImport.js'

function createController(overrides = {}) {
  const calls = []
  const visible = ref(true)
  const controller = useOverlayBookImport({
    visible,
    loadCategories: async () => calls.push(['categories']),
    listTocRules: async () => {
      calls.push(['toc-rules'])
      return {
        data: [
          { id: 1, name: '章节', rule: '^第.+章$', enable: true },
          { id: 2, name: '停用', rule: '^卷', enable: false },
          { id: 3, name: '空规则', rule: '', enable: true },
        ],
      }
    },
    previewBook: async (file, payload) => {
      calls.push(['preview', file, payload])
      return {
        data: {
          title: '解析书名',
          author: '解析作者',
          chapterCount: 3,
          importToken: 'staged-token',
        },
      }
    },
    importBook: async payload => {
      calls.push(['import', payload])
      return { title: payload.title, chapterCount: 3 }
    },
    close: () => calls.push(['close']),
    onSuccess: message => calls.push(['success', message]),
    onError: (...args) => calls.push(['error', ...args]),
    ...overrides,
  })
  return { calls, controller, visible }
}

test('loads categories and filters enabled TXT TOC rules once', async () => {
  const fixture = createController()
  await fixture.controller.open()
  await fixture.controller.loadTocRules()
  await fixture.controller.loadTocRules()
  assert.deepEqual(fixture.calls, [
    ['categories'],
    ['toc-rules'],
  ])
  assert.deepEqual(fixture.controller.tocRuleOptions.value, [
    { id: 1, name: '章节', rule: '^第.+章$', enable: true },
  ])
})

test('picks an EPUB file, previews metadata, and applies its default TOC rule', async () => {
  const fixture = createController()
  const file = { name: 'book.epub' }
  await fixture.controller.pickFile({ raw: file })
  assert.equal(fixture.controller.isEPUB.value, true)
  assert.equal(fixture.controller.draft.tocRule, 'spin+toc')
  assert.equal(fixture.controller.draft.title, '解析书名')
  assert.equal(fixture.controller.draft.author, '解析作者')
  assert.equal(fixture.controller.importToken.value, 'staged-token')
  assert.deepEqual(fixture.calls, [
    ['preview', file, {
      title: '',
      author: '',
      tocRule: 'spin+toc',
    }],
  ])
})

test('clears stale previews and reports parsing failures', async () => {
  const failure = new Error('bad file')
  const fixture = createController({
    previewBook: async () => {
      throw failure
    },
  })
  fixture.controller.previewData.value = { title: '旧预览' }
  await fixture.controller.pickFile({ raw: { name: 'book.txt' } })
  assert.equal(fixture.controller.previewData.value, null)
  assert.equal(fixture.controller.previewing.value, false)
  assert.deepEqual(fixture.calls, [
    ['toc-rules'],
    ['error', failure, '解析书籍失败'],
  ])
})

test('keeps an empty explicit catalogue as a normal staged preview', async () => {
	const fixture = createController({
		previewBook: async () => ({
			data: {
				title: '空目录书',
				chapterCount: 0,
				chapters: [],
				importToken: 'retry-token',
			},
		}),
	})

	await fixture.controller.pickFile({ raw: { name: 'book.txt' } })

	assert.equal(fixture.controller.previewData.value.chapterCount, 0)
	assert.equal(fixture.controller.importToken.value, 'retry-token')
	assert.equal(fixture.controller.previewError.value, '')
	assert.deepEqual(fixture.calls, [['toc-rules']])
})

test('reuses the staged upload for TOC reparsing and final import', async () => {
  const fixture = createController()
  const file = { name: 'book.txt' }
  await fixture.controller.pickFile({ raw: file })
  fixture.controller.draft.tocRule = '^卷.+$'
  fixture.calls.length = 0

  await fixture.controller.preview()
  assert.deepEqual(fixture.calls[0], [
    'preview',
    file,
    {
      title: '解析书名',
      author: '解析作者',
      tocRule: '^卷.+$',
      importToken: 'staged-token',
    },
  ])

  fixture.calls.length = 0
  await fixture.controller.importBook()
  assert.equal(fixture.calls[0][1].importToken, 'staged-token')
})

test('imports the confirmed preview, resets state, and closes the dialog', async () => {
  const fixture = createController()
  const file = { name: 'book.txt' }
  fixture.controller.draft.file = file
  fixture.controller.draft.title = '测试书'
  fixture.controller.draft.author = '作者'
  fixture.controller.draft.categoryIds = ['2', '3']
  fixture.controller.draft.tocRule = '^第.+章$'
  fixture.controller.previewData.value = { chapterCount: 3 }
  fixture.controller.importToken.value = 'staged-token'
  await nextTick()
  fixture.calls.length = 0
  await fixture.controller.importBook()
  assert.equal(fixture.calls[0][0], 'import')
  assert.equal(fixture.calls[0][1].file.name, file.name)
  assert.equal(fixture.calls[0][1].importToken, 'staged-token')
  assert.equal(fixture.calls[0][1].title, '测试书')
  assert.equal(fixture.calls[0][1].author, '作者')
  assert.deepEqual([...fixture.calls[0][1].categoryIds], ['2', '3'])
  assert.equal(fixture.calls[0][1].tocRule, '^第.+章$')
  assert.deepEqual(fixture.calls.slice(1), [
    ['success', '已导入《测试书》，共 3 章'],
    ['close'],
  ])
  assert.equal(fixture.controller.draft.file, null)
  assert.equal(fixture.controller.previewData.value, null)
  assert.equal(fixture.controller.importToken.value, '')
  assert.equal(fixture.controller.importing.value, false)
})

test('resets import state when the dialog closes', async () => {
  const fixture = createController()
  fixture.controller.draft.file = { name: 'book.pdf' }
  fixture.controller.draft.title = '临时'
  fixture.controller.previewData.value = { chapterCount: 1 }
  fixture.visible.value = false
  await nextTick()
  assert.equal(fixture.controller.draft.file, null)
  assert.equal(fixture.controller.draft.title, '')
  assert.equal(fixture.controller.previewData.value, null)
})

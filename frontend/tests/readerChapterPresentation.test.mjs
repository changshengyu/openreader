import assert from 'node:assert/strict'
import test from 'node:test'
import { reactive, ref } from 'vue'
import { useReaderChapterPresentation } from '../src/composables/useReaderChapterPresentation.js'

function createController() {
  const reader = reactive({ chineseFont: '简体' })
  const chapters = ref([
    { id: 11, title: '第一章 爱国' },
    { id: 12 },
  ])
  return {
    chapters,
    reader,
    controller: useReaderChapterPresentation({ reader, chapters }),
  }
}

test('formats chapter text with the active simplified or traditional setting', () => {
  const fixture = createController()
  assert.equal(fixture.controller.formatChineseText('愛國'), '爱国')
  fixture.reader.chineseFont = '繁体'
  assert.equal(fixture.controller.formatChineseText('爱国'), '愛國')
  assert.equal(fixture.controller.displayChapterTitle('第一章 爱国'), '第一章 愛國')
})

test('parses chapter paragraphs while preserving source positions', () => {
  const fixture = createController()
  const paragraphs = fixture.controller.makeParagraphs('  第一段  \n\n第二段', '标题')
  assert.deepEqual(paragraphs, [
    { type: 'text', text: '第一段', pos: 4, endPos: 7 },
    { type: 'text', text: '第二段', pos: 9, endPos: 12 },
  ])
})

test('builds chapter blocks from row and catalog fallbacks', () => {
  const fixture = createController()
  const fromCatalog = fixture.controller.makeChapterBlock(0, null, '正文')
  assert.equal(fromCatalog.id, 11)
  assert.equal(fromCatalog.title, '第一章 爱国')
  assert.equal(fromCatalog.content, '正文')
  assert.deepEqual(fromCatalog.imageUrls, [])

  const generated = fixture.controller.makeChapterBlock(1, { id: 22 }, '')
  assert.equal(generated.id, 22)
  assert.equal(generated.title, '第 2 章')
})

test('reads the final paragraph boundary as chapter text length', () => {
  const fixture = createController()
  assert.equal(fixture.controller.chapterBlockTextLength({ paragraphs: [] }), 0)
  assert.equal(fixture.controller.chapterBlockTextLength({
    paragraphs: [
      { pos: 2, endPos: 8 },
      { pos: 10, endPos: 16 },
    ],
  }), 16)
})

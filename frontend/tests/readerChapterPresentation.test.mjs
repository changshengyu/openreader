import assert from 'node:assert/strict'
import test from 'node:test'
import { reactive, ref } from 'vue'
import { useReaderChapterPresentation } from '../src/composables/useReaderChapterPresentation.js'

function createController() {
  const reader = reactive({ chineseFont: '简体' })
  const book = ref({ id: 7, url: 'https://example.com/book.txt' })
  const chapters = ref([
    { id: 11, title: '第一章 爱国' },
    { id: 12 },
  ])
  return {
    book,
    chapters,
    reader,
    controller: useReaderChapterPresentation({ reader, book, chapters }),
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

test('preserves safe upstream inline html while keeping searchable text', () => {
  const fixture = createController()
  const paragraphs = fixture.controller.makeParagraphs(
    '她说<ruby>愛<rt>あい</rt></ruby><em>很好</em><br>继续',
    '标题',
  )
  assert.deepEqual(paragraphs, [
    {
      type: 'text',
      text: '她说爱あい很好继续',
      html: '她说<ruby>爱<rt>あい</rt></ruby><em>很好</em><br>继续',
      pos: 4,
      endPos: 48,
    },
  ])
})

test('strips unsafe html from reader text blocks', () => {
  const fixture = createController()
  const paragraphs = fixture.controller.makeParagraphs(
    '正文<script>alert(1)</script><span onclick="x()">保留</span><img src="javascript:alert(1)">',
    '标题',
  )
  assert.deepEqual(paragraphs, [
    {
      type: 'text',
      text: '正文保留',
      html: '正文<span>保留</span>',
      pos: 4,
      endPos: 60,
    },
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

test('preserves upstream volume chapter semantics', () => {
  const fixture = createController()
  fixture.chapters.value[0].isVolume = true
  const volume = fixture.controller.makeChapterBlock(0, null, '卷首语\n敬请期待')
  assert.equal(volume.isVolume, true)
  assert.equal(volume.volumeText, '卷首语\n敬请期待')

  const regular = fixture.controller.makeChapterBlock(1, { id: 22, isVolume: false }, '正文')
  assert.equal(regular.isVolume, false)
  assert.equal(regular.volumeText, '')
})

test('marks image and cbz chapters with upstream comic semantics', () => {
  const fixture = createController()
  const imageBlock = fixture.controller.makeChapterBlock(0, null, '<img src="/comic/1.jpg" alt="图">')
  assert.equal(imageBlock.isComic, true)
  assert.equal(imageBlock.isCBZ, undefined)
  assert.equal(imageBlock.hideTitle, undefined)
  assert.deepEqual(imageBlock.imageUrls, ['http://localhost/comic/1.jpg'])

  fixture.book.value = { id: 7, url: '/library/demo.CBZ?cache=1#page' }
  const cbzBlock = fixture.controller.makeChapterBlock(0, null, '<img src="/comic/1.jpg">')
  assert.equal(cbzBlock.isCBZ, true)
  assert.equal(cbzBlock.isComic, true)
  assert.equal(cbzBlock.hideTitle, true)

  fixture.book.value = { id: 7, originalFile: 'uploads/archive.cbz' }
  assert.equal(fixture.controller.makeChapterBlock(0, null, '正文').hideTitle, true)

  fixture.book.value = { id: 7, libraryPath: 'books/archive.CBZ?download=1#cover' }
  assert.equal(fixture.controller.makeChapterBlock(0, null, '正文').hideTitle, true)
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

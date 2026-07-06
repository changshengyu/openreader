import assert from 'node:assert/strict'
import test from 'node:test'
import { parseReaderContentBlocks } from '../src/utils/readerContent.js'

function withOrigin(origin, callback) {
  const previous = globalThis.location
  globalThis.location = { origin }
  try {
    return callback()
  } finally {
    if (previous === undefined) {
      delete globalThis.location
    } else {
      globalThis.location = previous
    }
  }
}

test('resolves upstream image source attributes and API root placeholders', () => {
  withOrigin('https://reader.example', () => {
    assert.deepEqual(
      parseReaderContentBlocks('<img data-src="__API_ROOT__/cover.jpg" alt="封面" data-image-style="full">', '标题'),
      [{
        type: 'image',
        src: 'https://reader.example/cover.jpg',
        alt: '封面',
        imageStyle: 'FULL',
        text: '',
        pos: 4,
        endPos: 76,
      }],
    )

    assert.equal(
      parseReaderContentBlocks('<img data-original="/one.png">', '')[0].src,
      'https://reader.example/one.png',
    )
    assert.equal(
      parseReaderContentBlocks('<img data-url="https://cdn.example/two.png">', '')[0].src,
      'https://cdn.example/two.png',
    )
  })
})

test('preserves mixed text and image source positions', () => {
  withOrigin('https://reader.example', () => {
    const blocks = parseReaderContentBlocks('前文<img src="/comic/1.jpg" title="图">后文', '标题')
    assert.equal(blocks.length, 3)
    assert.deepEqual(blocks[0], {
      type: 'text',
      text: '前文',
      pos: 4,
      endPos: 6,
    })
    assert.equal(blocks[1].type, 'image')
    assert.equal(blocks[1].src, 'https://reader.example/comic/1.jpg')
    assert.equal(blocks[1].alt, '图')
    assert.equal(blocks[1].pos, 6)
    assert.deepEqual(blocks[2], {
      type: 'text',
      text: '后文',
      pos: blocks[1].endPos,
      endPos: blocks[1].endPos + 2,
    })
  })
})

test('rejects unsafe image URLs while preserving safe inline text', () => {
  const blocks = parseReaderContentBlocks(
    '正文<script>alert(1)</script><span onclick="x()">保留</span><img src="javascript:alert(1)">',
    '标题',
  )
  assert.deepEqual(blocks, [
    {
      type: 'text',
      text: '正文保留',
      html: '正文<span>保留</span>',
      pos: 4,
      endPos: 60,
    },
  ])
})

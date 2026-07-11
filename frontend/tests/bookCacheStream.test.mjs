import assert from 'node:assert/strict'
import test from 'node:test'
import { cacheBookContentStream } from '../src/api/books.js'

test('parses authenticated cache SSE progress and terminal events', async () => {
  const previousFetch = globalThis.fetch
  const events = []
  globalThis.fetch = async (url, init) => {
    assert.equal(url, '/api/books/7/cache/stream')
    assert.equal(init.method, 'POST')
    assert.equal(init.headers['Content-Type'], 'application/json')
    assert.deepEqual(JSON.parse(init.body), { all: true, count: 2, chapterIndex: 4 })
    return new Response([
      'event: message\n',
      'data: {"bookId":7,"cached":1,"requested":1,"total":2,"chapterIndex":4,"failed":0}\n\n',
      'event: end\n',
      'data: {"bookId":7,"cached":2,"requested":2,"failed":0,"book":{"id":7}}\n\n',
    ].join(''), {
      status: 200,
      headers: { 'Content-Type': 'text/event-stream' },
    })
  }
  try {
    const result = await cacheBookContentStream(7, {
      all: true,
      count: 2,
      chapterIndex: 4,
    }, {
      onEvent: event => events.push(event),
    })
    assert.deepEqual(events, [
      {
        event: 'message',
        data: { bookId: 7, cached: 1, requested: 1, total: 2, chapterIndex: 4, failed: 0 },
      },
      {
        event: 'end',
        data: { bookId: 7, cached: 2, requested: 2, failed: 0, book: { id: 7 } },
      },
    ])
    assert.deepEqual(result, { bookId: 7, cached: 2, requested: 2, failed: 0, book: { id: 7 } })
  } finally {
    globalThis.fetch = previousFetch
  }
})

test('surfaces a terminal cache SSE error to the active controller', async () => {
  const previousFetch = globalThis.fetch
  globalThis.fetch = async () => new Response(
    'event: error\ndata: {"bookId":7,"error":"未能缓存章节内容"}\n\n',
    { status: 200, headers: { 'Content-Type': 'text/event-stream' } },
  )
  try {
    await assert.rejects(
      () => cacheBookContentStream(7, { all: true }),
      /未能缓存章节内容/,
    )
  } finally {
    globalThis.fetch = previousFetch
  }
})

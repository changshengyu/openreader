import assert from 'node:assert/strict'
import test from 'node:test'
import {
  browserLocalCacheKeyMetadata,
  clearBrowserLocalCacheGroup,
  currentBrowserLocalCacheStats,
} from '../src/utils/localCacheStats.js'

const VALUES = new Map([
  ['localCache@bookSourceList@user:1', { id: 'source-a' }],
  ['localCache@bookSourceList@user:2', { id: 'source-b' }],
  ['localCache@rssSources@user:1', { id: 'rss-a' }],
  ['localCache@rssSources@user:2', { id: 'rss-b' }],
  ['localCache@reader@user:1@chapters:7', ['chapter-a']],
  ['localCache@reader@user:2@chapters:8', ['chapter-b']],
  ['localCache@reader@user:1@book:7', { title: 'book-a' }],
  ['localCache@reader@user:2@book:8', { title: 'book-b' }],
  ['localCache@user:1@Book_A@url-a@chapterContent-0', { content: 'content-a' }],
  ['localCache@user:2@Book_B@url-b@chapterContent-0', { content: 'content-b' }],
  ['localCache@bookshelf@getBookshelf:{"all":true}:user:1', [{ id: 7 }]],
  ['localCache@bookshelf@getBookshelf:{"all":true}:user:2', [{ id: 8 }]],
  ['localCache@Legacy_Author@legacy-url@chapterContent-0', { content: 'legacy' }],
  ['localCache@random-bookSourceList-note', { note: true }],
])

function cacheAdapters(removed = []) {
  return {
    listKeys: async () => [...VALUES.keys()],
    getCache: async key => VALUES.get(key),
    removeCache: async key => removed.push(key),
  }
}

test('classifies only exact cache keys owned by the captured user scope', () => {
  assert.deepEqual(
    browserLocalCacheKeyMetadata('localCache@bookSourceList@user:1', 'user:1'),
    { owned: true, group: 'bookSourceList' },
  )
  assert.deepEqual(
    browserLocalCacheKeyMetadata('localCache@rssSources@user:1', 'user:1'),
    { owned: true, group: 'rssSources' },
  )
  assert.deepEqual(
    browserLocalCacheKeyMetadata('localCache@reader@user:1@chapters:7', 'user:1'),
    { owned: true, group: 'chapterList' },
  )
  assert.deepEqual(
    browserLocalCacheKeyMetadata('localCache@user:1@Book_A@url-a@chapterContent-0', 'user:1'),
    { owned: true, group: 'chapterContent' },
  )
  assert.deepEqual(
    browserLocalCacheKeyMetadata('localCache@reader@user:1@book:7', 'user:1'),
    { owned: true, group: '' },
  )
  assert.deepEqual(
    browserLocalCacheKeyMetadata('localCache@bookshelf@getBookshelf:{"all":true}:user:1', 'user:1'),
    { owned: true, group: '' },
  )
})

test('fails closed for other users, unowned legacy cache, and substring collisions', () => {
  for (const key of [
    'localCache@bookSourceList@user:2',
    'localCache@rssSources@user:2',
    'localCache@reader@user:2@chapters:8',
    'localCache@user:2@Book_B@url-b@chapterContent-0',
    'localCache@Legacy_Author@legacy-url@chapterContent-0',
    'localCache@random-bookSourceList-note',
  ]) {
    assert.deepEqual(
      browserLocalCacheKeyMetadata(key, 'user:1'),
      { owned: false, group: '' },
      key,
    )
  }
})

test('totals all provably current-user cache while grouping only the four upstream groups', async () => {
  const stats = await currentBrowserLocalCacheStats('user:1', cacheAdapters())

  assert.equal(stats.total.files, 6)
  assert.deepEqual(
    Object.fromEntries(Object.entries(stats.groups).map(([group, value]) => [group, value.files])),
    {
      bookSourceList: 1,
      rssSources: 1,
      chapterList: 1,
      chapterContent: 1,
    },
  )
})

test('clears only the requested group owned by the scope captured by the caller', async () => {
  const removed = []
  const count = await clearBrowserLocalCacheGroup(
    'bookSourceList',
    'user:1',
    cacheAdapters(removed),
  )

  assert.equal(count, 1)
  assert.deepEqual(removed, ['localCache@bookSourceList@user:1'])
})


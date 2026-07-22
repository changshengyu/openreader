import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import { reconcileAuthoritativeShelfProgress } from '../src/utils/bookOrder.js'

const bookshelfSource = readFileSync(new URL('../src/stores/bookshelf.js', import.meta.url), 'utf8')
const homeSource = readFileSync(new URL('../src/views/Home.vue', import.meta.url), 'utf8')

test('an authoritative shelf snapshot replaces newer non-pending client progress', () => {
  const local = {
    bookId: 7,
    chapterIndex: 2,
    chapterTitle: '旧客户端章节',
    updatedAt: '2099-07-22T00:00:00Z',
  }
  const server = {
    bookId: 7,
    chapterIndex: 8,
    chapterTitle: '服务器最新章节',
    updatedAt: '2026-07-22T00:00:00Z',
  }

  assert.deepEqual(reconcileAuthoritativeShelfProgress(local, server), {
    progress: server,
    retryPending: false,
  })
})

test('an authoritative shelf snapshot clears confirmed client progress absent from the server', () => {
  const local = {
    bookId: 7,
    chapterIndex: 2,
    updatedAt: '2099-07-22T00:00:00Z',
  }

  assert.deepEqual(reconcileAuthoritativeShelfProgress(local, null), {
    progress: null,
    retryPending: false,
  })
})

test('a genuinely newer pending local read survives the snapshot and requests a CAS retry', () => {
  const local = {
    bookId: 7,
    chapterIndex: 9,
    updatedAt: '2026-07-22T00:00:02Z',
    pendingSync: true,
    baseUpdatedAt: '2026-07-22T00:00:00Z',
  }
  const server = {
    bookId: 7,
    chapterIndex: 5,
    updatedAt: '2026-07-22T00:00:01Z',
  }

  assert.deepEqual(reconcileAuthoritativeShelfProgress(local, server), {
    progress: local,
    retryPending: true,
  })
})

test('a server snapshot newer than a pending client position resolves to the server winner', () => {
  const local = {
    bookId: 7,
    chapterIndex: 4,
    updatedAt: '2026-07-22T00:00:01Z',
    pendingSync: true,
    baseUpdatedAt: '2026-07-22T00:00:00Z',
  }
  const server = {
    bookId: 7,
    chapterIndex: 10,
    updatedAt: '2026-07-22T00:00:02Z',
  }

  assert.deepEqual(reconcileAuthoritativeShelfProgress(local, server), {
    progress: server,
    retryPending: false,
  })
})

test('network shelf commits reconcile the complete server snapshot before sorting and caching', () => {
  assert.match(
    bookshelfSource,
    /const reconciledBooks = reconcileServerProgressFromBooks\(result\.value\)[\s\S]*?this\.books = sortBooks\(reconciledBooks\)/,
  )
  assert.match(bookshelfSource, /reader\.reconcileShelfProgress\(serverBooks\)/)
})

test('the visible refresh button still bypasses memory caches and requests the full shelf', () => {
  assert.match(homeSource, /bookshelf\.loadBooks\(\{ force: true, all: true \}\)/)
})

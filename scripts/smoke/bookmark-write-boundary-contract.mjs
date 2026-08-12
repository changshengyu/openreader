#!/usr/bin/env node

import { execFileSync } from 'node:child_process'
import http from 'node:http'
import https from 'node:https'

const targetURL = new URL(process.env.TARGET_URL || 'http://127.0.0.1:8080')
const databasePath = String(process.env.OPENREADER_SMOKE_DB || '').trim()
const singleLimit = 64 << 10
const batchCreateLimit = 16 << 20
const batchDeleteLimit = 16 << 10
const batchItemLimit = 2000

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

function request(path, { method = 'GET', body = '', token = '', chunked = false } = {}) {
  const url = new URL(path, targetURL)
  const transport = url.protocol === 'https:' ? https : http
  const headers = {}
  if (body) {
    headers['Content-Type'] = 'application/json'
    if (chunked) headers['Transfer-Encoding'] = 'chunked'
    else headers['Content-Length'] = Buffer.byteLength(body)
  }
  if (token) headers.Authorization = `Bearer ${token}`

  return new Promise((resolve, reject) => {
    const outgoing = transport.request(url, { method, headers }, (incoming) => {
      const chunks = []
      incoming.on('data', chunk => chunks.push(chunk))
      incoming.on('end', () => {
        const text = Buffer.concat(chunks).toString('utf8')
        let data = null
        try {
          data = text ? JSON.parse(text) : null
        } catch {
          reject(new Error(`${method} ${path}: non-JSON response ${JSON.stringify(text.slice(0, 512))}`))
          return
        }
        resolve({ status: incoming.statusCode || 0, data, text })
      })
    })
    outgoing.on('error', reject)
    if (body) {
      if (chunked) {
        const middle = Math.floor(body.length / 2)
        outgoing.write(body.slice(0, middle))
        outgoing.write(body.slice(middle))
      } else {
        outgoing.write(body)
      }
    }
    outgoing.end()
  })
}

function expectError(response, status, message) {
  assert(response.status === status, `status ${response.status}, want ${status}: ${response.text.slice(0, 512)}`)
  assert(response.data && Object.keys(response.data).length === 1, `unexpected error shape: ${response.text.slice(0, 512)}`)
  assert(response.data.error === message, `error ${JSON.stringify(response.data?.error)}, want ${JSON.stringify(message)}`)
}

function expectNotFound(response, message) {
  assert(response.status === 404, `status ${response.status}, want 404: ${response.text.slice(0, 512)}`)
  assert(response.data?.error?.code === 'NOT_FOUND', `code ${JSON.stringify(response.data?.error?.code)}, want "NOT_FOUND"`)
  assert(response.data?.error?.message === message, `message ${JSON.stringify(response.data?.error?.message)}, want ${JSON.stringify(message)}`)
}

function paddedObject(body, targetBytes) {
  assert(body.endsWith('}'), `fixture is not an object: ${body}`)
  const prefix = `${body.slice(0, -1)},"padding":"`
  const suffix = '"}'
  const padding = targetBytes - Buffer.byteLength(prefix) - Buffer.byteLength(suffix)
  assert(padding >= 0, `object fixture already exceeds ${targetBytes} bytes`)
  const result = `${prefix}${'x'.repeat(padding)}${suffix}`
  assert(Buffer.byteLength(result) === targetBytes, `object fixture is ${Buffer.byteLength(result)} bytes`)
  return result
}

function paddedArray(targetBytes) {
  const prefix = '[{"excerpt":"bounded batch","padding":"'
  const suffix = '"}]'
  const padding = targetBytes - Buffer.byteLength(prefix) - Buffer.byteLength(suffix)
  assert(padding >= 0, `array fixture already exceeds ${targetBytes} bytes`)
  const result = `${prefix}${'x'.repeat(padding)}${suffix}`
  assert(Buffer.byteLength(result) === targetBytes, `array fixture is ${Buffer.byteLength(result)} bytes`)
  return result
}

function repeatedBatch(count) {
  return `[${Array.from({ length: count }, () => '{"excerpt":"batch context"}').join(',')}]`
}

function repeatedIDs(id, count) {
  return `{"ids":[${Array.from({ length: count }, () => String(id)).join(',')}]}`
}

function sqlite(statement) {
  assert(databasePath, 'OPENREADER_SMOKE_DB is required for trigger assertions')
  return execFileSync(process.env.SQLITE3_BIN || 'sqlite3', [databasePath, statement], { encoding: 'utf8' }).trim()
}

async function register(username) {
  const response = await request('/api/auth/register', {
    method: 'POST',
    body: JSON.stringify({ username, password: 'password8' }),
  })
  assert(response.status === 200 && response.data?.token && response.data?.user?.id, `register: ${response.status} ${response.text}`)
  return response.data
}

async function createBook(token, title) {
  const response = await request('/api/books', {
    method: 'POST',
    token,
    body: JSON.stringify({ title, url: `local://${title}` }),
  })
  assert(response.status === 201 && response.data?.id, `create book: ${response.status} ${response.text}`)
  return response.data
}

async function createBookmark(token, bookID, excerpt, note = '') {
  const response = await request(`/api/books/${bookID}/bookmarks`, {
    method: 'POST',
    token,
    body: JSON.stringify({ excerpt, note }),
  })
  assert(response.status === 201 && response.data?.id, `create bookmark: ${response.status} ${response.text}`)
  return response.data
}

async function listBookmarks(token, bookID) {
  const response = await request(`/api/books/${bookID}/bookmarks`, { token })
  assert(response.status === 200 && Array.isArray(response.data), `list bookmarks: ${response.status} ${response.text}`)
  return response.data
}

async function main() {
  const suffix = `${process.pid}${Date.now().toString().slice(-7)}`
  const health = await request('/api/health')
  assert(health.status === 200, `health status ${health.status}: ${health.text}`)

  const owner = await register(`mark${suffix}`)
  const book = await createBook(owner.token, `bookmark-boundary-${suffix}`)
  const updateTarget = await createBookmark(owner.token, book.id, 'update context', 'before')
  const deleteTarget = await createBookmark(owner.token, book.id, 'delete context', 'before')

  const routes = [
    {
      name: 'create', method: 'POST', path: `/api/books/${book.id}/bookmarks`,
      body: '{"excerpt":"overflow create"}', limit: singleLimit, malformed: 'invalid bookmark payload', success: 201,
    },
    {
      name: 'batch-create', method: 'POST', path: `/api/books/${book.id}/bookmarks/batch`,
      body: '[{"excerpt":"overflow batch"}]', limit: batchCreateLimit, malformed: 'invalid bookmarks payload', success: 201,
    },
    {
      name: 'update', method: 'PUT', path: `/api/bookmarks/${updateTarget.id}`,
      body: '{"note":"overflow update"}', limit: singleLimit, malformed: 'invalid bookmark payload', success: 200,
    },
    {
      name: 'batch-delete', method: 'POST', path: `/api/books/${book.id}/bookmarks/batch-delete`,
      body: `{"ids":[${deleteTarget.id}]}`, limit: batchDeleteLimit, malformed: 'invalid bookmark ids', success: 200,
    },
  ]

  for (const route of routes) {
    for (const chunked of [false, true]) {
      const body = route.name === 'batch-create'
        ? paddedArray(route.limit + 1)
        : paddedObject(route.body, route.limit + 1)
      const overflow = await request(route.path, { method: route.method, token: owner.token, body, chunked })
      expectError(overflow, 413, 'request body too large')
    }
    const second = await request(route.path, {
      method: route.method,
      token: owner.token,
      body: `${route.body}${route.name === 'batch-create' ? '[]' : '{}'}`,
    })
    expectError(second, 400, route.malformed)
  }

  for (const route of routes) {
    const whitespace = '\r\n\t'
    const target = route.limit - Buffer.byteLength(whitespace)
    const body = (route.name === 'batch-create' ? paddedArray(target) : paddedObject(route.body, target)) + whitespace
    const exact = await request(route.path, { method: route.method, token: owner.token, body })
    assert(exact.status === route.success, `${route.name} exact limit: ${exact.status} ${exact.text.slice(0, 512)}`)
  }

  const missingPriority = await request('/api/books/999999/bookmarks', {
    method: 'POST', token: owner.token, body: paddedObject('{"excerpt":"missing"}', singleLimit + 1),
  })
  expectNotFound(missingPriority, 'book not found')

  const beforeInvalidPatch = (await listBookmarks(owner.token, book.id)).find(row => row.id === updateTarget.id)
  for (const body of ['{}', '{"ignored":true}', '{"note":null}', '{"note":7}']) {
    const invalid = await request(`/api/bookmarks/${updateTarget.id}`, { method: 'PUT', token: owner.token, body })
    expectError(invalid, 400, 'invalid bookmark payload')
  }
  const afterInvalidPatch = (await listBookmarks(owner.token, book.id)).find(row => row.id === updateTarget.id)
  assert(afterInvalidPatch.note === beforeInvalidPatch.note, 'invalid note patch changed the stored note')

  const exactBatch = await request(`/api/books/${book.id}/bookmarks/batch`, {
    method: 'POST', token: owner.token, body: repeatedBatch(batchItemLimit),
  })
  assert(exactBatch.status === 201 && exactBatch.data?.length === batchItemLimit, `exact batch item limit: ${exactBatch.status}`)
  const countBeforeOverflow = (await listBookmarks(owner.token, book.id)).length
  const overflowBatch = await request(`/api/books/${book.id}/bookmarks/batch`, {
    method: 'POST', token: owner.token, body: repeatedBatch(batchItemLimit + 1),
  })
  expectError(overflowBatch, 400, 'invalid bookmarks payload')
  assert((await listBookmarks(owner.token, book.id)).length === countBeforeOverflow, 'overflow batch created a prefix')

  const deleteCardinalityTarget = await createBookmark(owner.token, book.id, 'delete cardinality')
  const overflowDelete = await request(`/api/books/${book.id}/bookmarks/batch-delete`, {
    method: 'POST', token: owner.token, body: repeatedIDs(deleteCardinalityTarget.id, batchItemLimit + 1),
  })
  expectError(overflowDelete, 400, 'invalid bookmark ids')
  assert((await listBookmarks(owner.token, book.id)).some(row => row.id === deleteCardinalityTarget.id), 'overflow ID batch deleted its target')
  const exactDelete = await request(`/api/books/${book.id}/bookmarks/batch-delete`, {
    method: 'POST', token: owner.token, body: repeatedIDs(deleteCardinalityTarget.id, batchItemLimit),
  })
  assert(exactDelete.status === 200 && exactDelete.data?.deletedIds?.[0] === deleteCardinalityTarget.id, `exact ID batch: ${exactDelete.status} ${exactDelete.text}`)

  const contextTarget = await createBookmark(owner.token, book.id, 'context before', 'before')
  sqlite(`
    CREATE TRIGGER smoke_bookmark_note_context BEFORE UPDATE OF note ON bookmarks
    WHEN OLD.id = ${Number(contextTarget.id)} BEGIN
      UPDATE bookmarks SET excerpt = 'context after', "offset" = 91 WHERE id = OLD.id;
    END;
  `)
  const contextUpdate = await request(`/api/bookmarks/${contextTarget.id}`, {
    method: 'PUT', token: owner.token, body: '{"note":"after"}',
  })
  sqlite('DROP TRIGGER smoke_bookmark_note_context;')
  assert(
    contextUpdate.status === 200 && contextUpdate.data?.note === 'after' &&
      contextUpdate.data?.excerpt === 'context after' && contextUpdate.data?.offset === 91,
    `note update returned or persisted a stale context: ${contextUpdate.status} ${contextUpdate.text}`,
  )

  const deletedTarget = await createBookmark(owner.token, book.id, 'delete before', 'before')
  sqlite(`
    CREATE TRIGGER smoke_bookmark_note_delete BEFORE UPDATE OF note ON bookmarks
    WHEN OLD.id = ${Number(deletedTarget.id)} BEGIN
      DELETE FROM bookmarks WHERE id = OLD.id;
    END;
  `)
  const deletedUpdate = await request(`/api/bookmarks/${deletedTarget.id}`, {
    method: 'PUT', token: owner.token, body: '{"note":"must not revive"}',
  })
  sqlite('DROP TRIGGER smoke_bookmark_note_delete;')
  expectError(deletedUpdate, 404, 'bookmark not found')
  assert(!(await listBookmarks(owner.token, book.id)).some(row => row.id === deletedTarget.id), 'concurrent note update revived a deleted bookmark')

  console.log(JSON.stringify({
    status: 'ok',
    declaredAndChunkedOverflow: 413,
    exactLimits: { single: singleLimit, batchCreate: batchCreateLimit, batchDelete: batchDeleteLimit },
    cardinality: { exact: batchItemLimit, overflow: batchItemLimit + 1 },
    notePatch: 'explicit-string-only',
    concurrentContext: 'preserved',
    concurrentDelete: 'not-revived',
  }))
}

main().catch((error) => {
  console.error(error.stack || error.message)
  process.exit(1)
})

#!/usr/bin/env node

import { execFileSync } from 'node:child_process'
import { lstat, mkdir, stat, writeFile } from 'node:fs/promises'
import http from 'node:http'
import https from 'node:https'
import path from 'node:path'

const targetURL = new URL(process.env.TARGET_URL || 'http://127.0.0.1:8080')
const databasePath = String(process.env.OPENREADER_SMOKE_DB || '').trim()
const dataDir = String(process.env.OPENREADER_SMOKE_DATA || '').trim()
const libraryDir = String(process.env.OPENREADER_SMOKE_LIBRARY || '').trim()
const bodyLimit = 1 << 20

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

function request(apiPath, { method = 'GET', body = '', token = '', chunked = false } = {}) {
  const url = new URL(apiPath, targetURL)
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
          reject(new Error(`${method} ${apiPath}: non-JSON response ${JSON.stringify(text)}`))
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

function errorMessage(response) {
  return typeof response.data?.error === 'string'
    ? response.data.error
    : response.data?.error?.message
}

function expectError(response, status, message) {
  assert(response.status === status, `status ${response.status}, want ${status}: ${response.text}`)
  assert(errorMessage(response) === message, `error ${JSON.stringify(response.data?.error)}, want ${JSON.stringify(message)}`)
}

function paddedObject(body, targetBytes) {
  assert(body.endsWith('}'), `fixture is not a JSON object: ${body}`)
  const prefix = `${body.slice(0, -1)},"padding":"`
  const suffix = '"}'
  const paddingBytes = targetBytes - Buffer.byteLength(prefix) - Buffer.byteLength(suffix)
  assert(paddingBytes >= 0, `fixture already exceeds ${targetBytes} bytes`)
  const padded = `${prefix}${'x'.repeat(paddingBytes)}${suffix}`
  assert(Buffer.byteLength(padded) === targetBytes, `padded fixture is ${Buffer.byteLength(padded)} bytes`)
  return padded
}

function sqlString(value) {
  return `'${String(value).replaceAll("'", "''")}'`
}

function sqlite(sql) {
  assert(databasePath, 'OPENREADER_SMOKE_DB is required')
  return execFileSync(process.env.SQLITE3_BIN || 'sqlite3', [databasePath, sql], { encoding: 'utf8' }).trim()
}

async function register(username) {
  const response = await request('/api/auth/register', {
    method: 'POST',
    body: JSON.stringify({ username, password: 'password8' }),
  })
  assert(response.status === 200 && response.data?.token && response.data?.user?.id, `register ${username}: ${response.status} ${response.text}`)
  return response.data
}

async function createCategory(token, name) {
  const response = await request('/api/categories', {
    method: 'POST',
    token,
    body: JSON.stringify({ name }),
  })
  assert(response.status === 201 && response.data?.id, `create category ${name}: ${response.status} ${response.text}`)
  return response.data
}

async function listBooks(token) {
  const response = await request('/api/books', { token })
  assert(response.status === 200 && Array.isArray(response.data), `list books: ${response.status} ${response.text}`)
  return response.data
}

async function pathExists(target) {
  try {
    await stat(target)
    return true
  } catch (error) {
    if (error?.code === 'ENOENT') return false
    throw error
  }
}

async function insertSharedLocalBooks(user, key) {
  assert(libraryDir, 'OPENREADER_SMOKE_LIBRARY is required')
  const relative = path.join('data', user.username, `shared-${key}`)
  const alias = path.join('data', user.username, 'nested', '..', `shared-${key}`)
  const archiveRoot = path.join(libraryDir, relative)
  const sourcePath = path.join(archiveRoot, 'source.txt')
  await mkdir(archiveRoot, { recursive: true })
  await writeFile(sourcePath, `shared ${key}`, { mode: 0o644 })
  const now = new Date().toISOString()
  const insert = (title, libraryPath) => Number.parseInt(sqlite(`
    INSERT INTO books (
      user_id, source_id, type, title, library_path, original_file,
      last_check_time, can_update, created_at, updated_at
    ) VALUES (
      ${Number(user.id)}, 0, 0, ${sqlString(title)}, ${sqlString(libraryPath)},
      ${sqlString(path.join(libraryPath, 'source.txt'))}, ${Date.now()}, 1,
      ${sqlString(now)}, ${sqlString(now)}
    );
    SELECT last_insert_rowid();
  `), 10)
  return {
    archiveRoot,
    sourcePath,
    firstID: insert(`shared ${key} first`, relative),
    secondID: insert(`shared ${key} second`, alias),
  }
}

async function deleteBook(token, bookID, action) {
  if (action === 'single') {
    const response = await request(`/api/books/${bookID}`, { method: 'DELETE', token })
    assert(response.status === 204, `single delete ${bookID}: ${response.status} ${response.text}`)
    return
  }
  const response = await request('/api/books/batch', {
    method: 'POST',
    token,
    body: JSON.stringify({ action: 'delete', bookIds: [bookID] }),
  })
  assert(response.status === 200 && response.data?.deletedIds?.includes(bookID), `batch delete ${bookID}: ${response.status} ${response.text}`)
}

async function main() {
  assert(dataDir, 'OPENREADER_SMOKE_DATA is required')
  const suffix = `${process.pid}${Date.now().toString().slice(-7)}`
  const health = await request('/api/health')
  assert(health.status === 200, `health status ${health.status}: ${health.text}`)

  const owner = await register(`book${suffix}`)
  const other = await register(`other${suffix}`)
  const ownerCategory = await createCategory(owner.token, `owner-${suffix}`)
  const foreignCategory = await createCategory(other.token, `foreign-${suffix}`)

  const invalidCover = await request('/api/books', {
    method: 'POST', token: owner.token,
    body: JSON.stringify({ title: 'invalid cover', customCoverUrl: 'https://example.com/cover.jpg' }),
  })
  expectError(invalidCover, 400, 'invalid custom cover url')

  const foreignFallback = await request('/api/books', {
    method: 'POST', token: owner.token,
    body: JSON.stringify({ title: 'foreign category', categoryId: foreignCategory.id, categoryIds: [0] }),
  })
  expectError(foreignFallback, 400, 'category not found')
  assert((await listBooks(owner.token)).length === 0, 'rejected create left a book row')

  const coverURL = `/uploads/users/${owner.user.id}/covers/owned.jpg`
  const coverPath = path.join(dataDir, 'uploads', 'users', String(owner.user.id), 'covers', 'owned.jpg')
  await mkdir(path.dirname(coverPath), { recursive: true })
  await writeFile(coverPath, 'cover', { mode: 0o644 })
  const forgedID = 900123
  const created = await request('/api/books', {
    method: 'POST', token: owner.token,
    body: JSON.stringify({
      id: forgedID,
      userId: other.user.id,
      sourceId: 777,
      type: 1,
      title: '  bounded book  ',
      author: '  allowed author  ',
      coverUrl: '  https://example.com/public.jpg  ',
      customCoverUrl: `  ${coverURL}  `,
      intro: '  allowed intro  ',
      kind: '  allowed kind  ',
      wordCount: '  12万字  ',
      url: '  https://example.com/book  ',
      variable: '{"secret":"forged"}',
      libraryPath: `data/${owner.user.username}/victim`,
      originalFile: 'source.txt',
      tocFile: 'chapters.json',
      tocRule: 'forged',
      sourceFile: 'source.json',
      lastChapter: 'forged',
      chapterCount: 99,
      lastCheckTime: 123,
      createdAt: '2000-01-01T00:00:00Z',
      updatedAt: '2000-01-02T00:00:00Z',
      categoryId: ownerCategory.id,
      categoryIds: [0],
      canUpdate: false,
    }),
  })
  assert(created.status === 201 && created.data?.id, `bounded create: ${created.status} ${created.text}`)
  assert(created.data.id !== forgedID && created.data.userId === owner.user.id, `client controlled book identity: ${created.text}`)
  assert(created.data.sourceId === 0 && created.data.type === 0, `client controlled source/type: ${created.text}`)
  for (const key of ['libraryPath', 'originalFile', 'tocFile', 'tocRule', 'sourceFile', 'lastChapter']) {
    assert(created.data[key] === '', `client controlled ${key}: ${created.text}`)
  }
  assert(created.data.variable == null && created.data.chapterCount === 0 && created.data.lastCheckTime !== 123, `client controlled parser state: ${created.text}`)
  assert(created.data.canUpdate === false, `explicit false was lost: ${created.text}`)
  assert(created.data.title === 'bounded book' && created.data.author === 'allowed author' && created.data.customCoverUrl === coverURL, `allowed metadata was not normalized: ${created.text}`)
  assert(created.data.categoryIds?.length === 1 && created.data.categoryIds[0] === ownerCategory.id, `owned category fallback missing: ${created.text}`)
  const bookID = created.data.id

  for (const route of [
    { method: 'POST', path: '/api/books', body: '{"title":"overflow create"}' },
    { method: 'PUT', path: `/api/books/${bookID}`, body: '{"author":"overflow update"}' },
  ]) {
    for (const chunked of [false, true]) {
      const before = await listBooks(owner.token)
      const overflow = await request(route.path, {
        method: route.method,
        token: owner.token,
        body: paddedObject(route.body, bodyLimit + 1),
        chunked,
      })
      expectError(overflow, 413, 'request body too large')
      const after = await listBooks(owner.token)
      assert(after.length === before.length, `${route.method} overflow changed shelf cardinality`)
    }
  }

  const exactCreate = await request('/api/books', {
    method: 'POST', token: owner.token,
    body: paddedObject('{"title":"exact create"}', bodyLimit),
  })
  assert(exactCreate.status === 201, `exact create: ${exactCreate.status} ${exactCreate.text}`)
  const exactUpdate = await request(`/api/books/${bookID}`, {
    method: 'PUT', token: owner.token,
    body: paddedObject('{"author":"exact update"}', bodyLimit),
  })
  assert(exactUpdate.status === 200 && exactUpdate.data?.author === 'exact update', `exact update: ${exactUpdate.status} ${exactUpdate.text}`)

  for (const body of ['null', '[]', '{"author":"one"}{"author":"two"}']) {
    const malformed = await request(`/api/books/${bookID}`, { method: 'PUT', token: owner.token, body })
    expectError(malformed, 400, 'invalid book payload')
  }
  const overlongTitle = await request(`/api/books/${bookID}`, {
    method: 'PUT', token: owner.token,
    body: JSON.stringify({ title: '界'.repeat(81) }),
  })
  expectError(overlongTitle, 400, 'book title is too long')
  const missingTarget = await request('/api/books/999999', {
    method: 'PUT', token: owner.token,
    body: paddedObject('{"author":"missing"}', bodyLimit + 1),
  })
  expectError(missingTarget, 404, 'book not found')

  for (const action of ['single', 'batch']) {
    const shared = await insertSharedLocalBooks(owner.user, action)
    await deleteBook(owner.token, shared.firstID, action)
    assert(await pathExists(shared.sourcePath), `${action} delete removed archive with a live reference`)
    await deleteBook(owner.token, shared.secondID, action)
    assert(!(await pathExists(shared.archiveRoot)), `${action} delete left archive after its last reference`)
  }

  const coverInfo = await lstat(coverPath)
  assert(coverInfo.isFile(), 'book write cleanup touched the current user cover')
  console.log('Book write boundary smoke passed (DTO ownership, 1 MiB wire, strict JSON, owner checks, UTF-8 fields, last-reference archive cleanup)')
}

main().catch((error) => {
  console.error(error.stack || error.message)
  process.exit(1)
})

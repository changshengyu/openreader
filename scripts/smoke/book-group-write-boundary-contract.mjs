#!/usr/bin/env node

import { execFileSync } from 'node:child_process'
import http from 'node:http'
import https from 'node:https'

const targetURL = new URL(process.env.TARGET_URL || 'http://127.0.0.1:8080')
const databasePath = String(process.env.OPENREADER_SMOKE_DB || '').trim()
const bodyLimit = 16 << 10

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
          reject(new Error(`${method} ${path}: non-JSON response ${JSON.stringify(text)}`))
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
  assert(response.status === status, `status ${response.status}, want ${status}: ${response.text}`)
  assert(response.data && Object.keys(response.data).length === 1, `unexpected error shape: ${response.text}`)
  assert(response.data.error === message, `error ${JSON.stringify(response.data?.error)}, want ${JSON.stringify(message)}`)
}

function expectAPIError(response, status, message) {
  assert(response.status === status, `status ${response.status}, want ${status}: ${response.text}`)
  assert(response.data?.error?.message === message, `error ${JSON.stringify(response.data?.error)}, want message ${JSON.stringify(message)}`)
}

function errorMessage(response) {
  return typeof response.data?.error === 'string'
    ? response.data.error
    : response.data?.error?.message
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

async function register(username) {
  const response = await request('/api/auth/register', {
    method: 'POST',
    body: JSON.stringify({ username, password: 'password8' }),
  })
  assert(response.status === 200 && response.data?.token && response.data?.user?.id, `register ${username}: ${response.status} ${response.text}`)
  return response.data
}

async function createCategory(token, name, color = '') {
  const response = await request('/api/categories', {
    method: 'POST',
    token,
    body: JSON.stringify({ name, ...(color ? { color } : {}) }),
  })
  assert(response.status === 201 && response.data?.id, `create category ${name}: ${response.status} ${response.text}`)
  return response.data
}

async function currentBookGroupKeys(token) {
  const response = await request('/api/book-groups', { token })
  assert(response.status === 200 && Array.isArray(response.data), `list book groups: ${response.status} ${response.text}`)
  return response.data.map(row => row.key)
}

function preferenceCount() {
  assert(databasePath, 'OPENREADER_SMOKE_DB is required for the zero-side-effect database assertion')
  const output = execFileSync(process.env.SQLITE3_BIN || 'sqlite3', [databasePath, 'SELECT COUNT(*) FROM book_group_preferences;'], { encoding: 'utf8' })
  return Number.parseInt(output.trim(), 10)
}

async function main() {
  const suffix = `${process.pid}${Date.now().toString().slice(-7)}`
  const health = await request('/api/health')
  assert(health.status === 200, `health status ${health.status}: ${health.text}`)

  const owner = await register(`group${suffix}`)
  const other = await register(`other${suffix}`)

  const blank = await request('/api/categories', {
    method: 'POST',
    token: owner.token,
    body: '{"name":"   "}',
  })
  expectError(blank, 400, 'category name is required')
  assert(preferenceCount() === 0, 'blank category create seeded built-in preferences')

  const ownerCategory = await createCategory(owner.token, `owner-${suffix}`)
  const otherCategory = await createCategory(other.token, `other-${suffix}`)
  const createdBook = await request('/api/books', {
    method: 'POST',
    token: owner.token,
    body: JSON.stringify({ title: `boundary-${suffix}`, url: `local://boundary-${suffix}` }),
  })
  assert(createdBook.status === 201 && createdBook.data?.id, `create book: ${createdBook.status} ${createdBook.text}`)
  const bookID = createdBook.data.id

  const foreignFallback = await request(`/api/books/${bookID}/category`, {
    method: 'PUT',
    token: owner.token,
    body: JSON.stringify({ categoryId: otherCategory.id, categoryIds: [] }),
  })
  expectAPIError(foreignFallback, 400, 'category not found')
  const unchangedBook = await request(`/api/books/${bookID}`, { token: owner.token })
  assert(unchangedBook.status === 200, `read unchanged book: ${unchangedBook.status} ${unchangedBook.text}`)
  assert(!unchangedBook.data.categoryId && unchangedBook.data.categoryIds?.length === 0, `foreign category persisted: ${unchangedBook.text}`)

  const oversizedWithoutAuth = await request('/api/categories', {
    method: 'POST',
    body: paddedObject('{"name":"unauthenticated"}', bodyLimit + 1),
  })
  expectError(oversizedWithoutAuth, 401, 'missing bearer token')
  const unknownKey = await request('/api/book-groups/not-real', {
    method: 'PUT',
    token: owner.token,
    body: paddedObject('{"show":false}', bodyLimit + 1),
  })
  expectError(unknownKey, 400, 'invalid built-in book group')
  for (const path of [`/api/categories/999999`, `/api/books/999999/category`]) {
    const missingTarget = await request(path, {
      method: 'PUT',
      token: owner.token,
      body: paddedObject('{"show":false}', bodyLimit + 1),
    })
    assert(missingTarget.status === 404 && errorMessage(missingTarget)?.includes('not found'), `missing target priority ${path}: ${missingTarget.status} ${missingTarget.text}`)
  }

  const routes = async () => [
    { name: 'category-create', method: 'POST', path: '/api/categories', body: `{"name":"overflow-${suffix}"}`, malformed: 'category name is required', success: 201 },
    { name: 'category-update', method: 'PUT', path: `/api/categories/${ownerCategory.id}`, body: '{"show":false}', malformed: 'invalid category payload', success: 200 },
    { name: 'category-reorder', method: 'PUT', path: '/api/categories/reorder', body: `{"ids":[${ownerCategory.id}]}`, malformed: 'ids is required', success: 200 },
    { name: 'built-in-update', method: 'PUT', path: '/api/book-groups/all', body: '{"show":false}', malformed: 'invalid book group payload', success: 200 },
    { name: 'book-group-reorder', method: 'PUT', path: '/api/book-groups/reorder', body: JSON.stringify({ keys: await currentBookGroupKeys(owner.token) }), malformed: 'keys is required', success: 200 },
    { name: 'book-category', method: 'PUT', path: `/api/books/${bookID}/category`, body: `{"categoryId":${ownerCategory.id}}`, malformed: 'invalid category payload', success: 200 },
  ]

  for (const route of await routes()) {
    for (const chunked of [false, true]) {
      const overflow = await request(route.path, {
        method: route.method,
        token: owner.token,
        body: paddedObject(route.body, bodyLimit + 1),
        chunked,
      })
      expectError(overflow, 413, 'request body too large')
    }
    for (const suffixBody of ['{"ignored":true}', 'garbage']) {
      const malformed = await request(route.path, {
        method: route.method,
        token: owner.token,
        body: route.body + suffixBody,
      })
      expectError(malformed, 400, route.malformed)
    }
    const nullDocument = await request(route.path, {
      method: route.method,
      token: owner.token,
      body: 'null',
    })
    expectError(nullDocument, 400, route.malformed)
  }

  for (const route of await routes()) {
    const whitespace = '\r\n\t'
    const currentBody = route.name === 'book-group-reorder'
      ? JSON.stringify({ keys: await currentBookGroupKeys(owner.token) })
      : route.body
    const exactBody = paddedObject(currentBody, bodyLimit - Buffer.byteLength(whitespace)) + whitespace
    assert(Buffer.byteLength(exactBody) === bodyLimit, `${route.name} exact fixture is not ${bodyLimit} bytes`)
    const exact = await request(route.path, {
      method: route.method,
      token: owner.token,
      body: exactBody,
    })
    assert(exact.status === route.success, `${route.name} exact limit: ${exact.status} ${exact.text}`)
  }

  await createCategory(owner.token, 'n'.repeat(80), 'c'.repeat(24))
  const overlongCategory = await request('/api/categories', {
    method: 'POST',
    token: owner.token,
    body: JSON.stringify({ name: 'n'.repeat(81) }),
  })
  expectError(overlongCategory, 400, 'category name is too long')
  const overlongColor = await request(`/api/categories/${ownerCategory.id}`, {
    method: 'PUT',
    token: owner.token,
    body: JSON.stringify({ color: 'c'.repeat(25) }),
  })
  expectError(overlongColor, 400, 'category color is too long')
  const builtInAtLimit = await request('/api/book-groups/all', {
    method: 'PUT',
    token: owner.token,
    body: JSON.stringify({ name: 'b'.repeat(80) }),
  })
  assert(builtInAtLimit.status === 200, `built-in name at limit: ${builtInAtLimit.status} ${builtInAtLimit.text}`)
  const overlongBuiltIn = await request('/api/book-groups/all', {
    method: 'PUT',
    token: owner.token,
    body: JSON.stringify({ name: 'b'.repeat(81) }),
  })
  expectError(overlongBuiltIn, 400, 'book group name is too long')

  const ownedFallback = await request(`/api/books/${bookID}/category`, {
    method: 'PUT',
    token: owner.token,
    body: JSON.stringify({ categoryId: ownerCategory.id, categoryIds: [0] }),
  })
  assert(ownedFallback.status === 200, `owned fallback: ${ownedFallback.status} ${ownedFallback.text}`)
  assert(ownedFallback.data.categoryId === ownerCategory.id, `owned primary category missing: ${ownedFallback.text}`)
  assert(ownedFallback.data.categoryIds?.length === 1 && ownedFallback.data.categoryIds[0] === ownerCategory.id, `owned category membership missing: ${ownedFallback.text}`)

  console.log('BookGroup write boundary smoke passed (six-route wire limits, strict JSON, priority, field budgets, zero side effects, owner isolation)')
}

main().catch((error) => {
  console.error(error.stack || error.message)
  process.exit(1)
})

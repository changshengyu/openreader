#!/usr/bin/env node

import fs from 'node:fs'
import http from 'node:http'
import https from 'node:https'
import path from 'node:path'

const targetURL = new URL(process.env.TARGET_URL || 'http://127.0.0.1:8080')
const cacheRoot = path.resolve(String(process.env.OPENREADER_SMOKE_CACHE_DIR || ''))
const tempRoot = path.resolve(String(process.env.OPENREADER_SMOKE_TMP || ''))
const maxImportBytes = Number(process.env.OPENREADER_SMOKE_MAX_IMPORT_BYTES || 33 << 20)
const multipartEnvelopeBytes = 1 << 20
const multipartMemoryBytes = 32 << 20

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

function request(pathname, {
  method = 'GET',
  token = '',
  contentType = '',
  chunks = [],
  chunked = false,
  contentLength,
} = {}) {
  const url = new URL(pathname, targetURL)
  const transport = url.protocol === 'https:' ? https : http
  const normalizedChunks = chunks.map(chunk => Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk))
  const headers = {}
  if (contentType) headers['Content-Type'] = contentType
  if (chunked) headers['Transfer-Encoding'] = 'chunked'
  else if (contentLength !== undefined) headers['Content-Length'] = contentLength
  else if (normalizedChunks.length) headers['Content-Length'] = normalizedChunks.reduce((total, chunk) => total + chunk.length, 0)
  if (token) headers.Authorization = `Bearer ${token}`

  return new Promise((resolve, reject) => {
    const outgoing = transport.request(url, { method, headers }, incoming => {
      const responseChunks = []
      incoming.on('data', chunk => responseChunks.push(chunk))
      incoming.on('end', () => {
        const body = Buffer.concat(responseChunks)
        const text = body.toString('utf8')
        let data = null
        try {
          data = text ? JSON.parse(text) : null
        } catch {
          reject(new Error(`${method} ${pathname}: non-JSON response ${JSON.stringify(text.slice(0, 512))}`))
          return
        }
        resolve({ status: incoming.statusCode || 0, data, text })
      })
    })
    outgoing.setTimeout(120_000, () => outgoing.destroy(new Error(`${method} ${pathname}: timeout`)))
    outgoing.on('error', reject)
    for (const chunk of normalizedChunks) outgoing.write(chunk)
    outgoing.end()
  })
}

function multipartChunks(boundary, fields = [], files = []) {
  const chunks = []
  for (const [name, value] of fields) {
    chunks.push(Buffer.from(
      `--${boundary}\r\nContent-Disposition: form-data; name="${name}"\r\n\r\n${value}\r\n`,
    ))
  }
  for (const file of files) {
    chunks.push(Buffer.from(
      `--${boundary}\r\nContent-Disposition: form-data; name="${file.field}"; filename="${file.filename}"\r\n` +
      `Content-Type: ${file.contentType || 'application/octet-stream'}\r\n\r\n`,
    ))
    chunks.push(file.data)
    chunks.push(Buffer.from('\r\n'))
  }
  chunks.push(Buffer.from(`--${boundary}--\r\n`))
  return chunks
}

function multipartLength(chunks) {
  return chunks.reduce((total, chunk) => total + chunk.length, 0)
}

function multipartAtExactLength(targetBytes, boundary) {
  const empty = multipartChunks(boundary, [], [{ field: 'file', filename: 'boundary.txt', data: Buffer.alloc(0) }])
  const fixedBytes = multipartLength(empty)
  assert(fixedBytes < targetBytes, `multipart metadata is ${fixedBytes} bytes`)
  const chunks = multipartChunks(boundary, [], [{
    field: 'file',
    filename: 'boundary.txt',
    data: Buffer.alloc(targetBytes - fixedBytes, 0x78),
  }])
  assert(multipartLength(chunks) === targetBytes, `multipart fixture is ${multipartLength(chunks)} bytes`)
  return chunks
}

function multipartRequest(pathname, token, boundary, fields, files, options = {}) {
  return request(pathname, {
    method: 'POST',
    token,
    contentType: `multipart/form-data; boundary=${boundary}`,
    chunks: multipartChunks(boundary, fields, files),
    chunked: options.chunked,
  })
}

function jsonRequest(pathname, { token = '', body, method = 'POST' }) {
  return request(pathname, {
    method,
    token,
    contentType: 'application/json',
    chunks: [JSON.stringify(body)],
  })
}

function expectError(response, status, message) {
  assert(response.status === status, `status ${response.status}, want ${status}: ${response.text.slice(0, 512)}`)
  assert(response.data?.error === message, `error ${JSON.stringify(response.data?.error)}, want ${JSON.stringify(message)}`)
}

function recursiveFiles(root) {
  if (!fs.existsSync(root)) return []
  const files = []
  const visit = current => {
    for (const entry of fs.readdirSync(current, { withFileTypes: true })) {
      const entryPath = path.join(current, entry.name)
      if (entry.isDirectory()) visit(entryPath)
      else files.push(entryPath)
    }
  }
  visit(root)
  return files
}

function multipartTempFiles() {
  if (!fs.existsSync(tempRoot)) return []
  return fs.readdirSync(tempRoot).filter(name => name.startsWith('multipart-'))
}

async function main() {
  assert(process.env.OPENREADER_SMOKE_CACHE_DIR, 'OPENREADER_SMOKE_CACHE_DIR is required')
  assert(process.env.OPENREADER_SMOKE_TMP, 'OPENREADER_SMOKE_TMP is required')
  assert(Number.isSafeInteger(maxImportBytes) && maxImportBytes > multipartMemoryBytes, 'max import bytes must exceed multipart memory')

  const suffix = `${process.pid}${Date.now().toString().slice(-7)}`
  const registration = await jsonRequest('/api/auth/register', {
    body: { username: `direct${suffix}`, password: 'password8' },
  })
  assert(registration.status === 200 && registration.data?.token && registration.data?.user?.id, `register: ${registration.status} ${registration.text}`)
  const token = registration.data.token
  const userID = registration.data.user.id
  const stageRoot = path.join(cacheRoot, 'import-previews', String(userID))
  const envelopeLimit = maxImportBytes + multipartEnvelopeBytes

  const declaredBoundary = `direct-declared-${suffix}`
  const declaredChunks = multipartAtExactLength(envelopeLimit + 1, declaredBoundary)
  const declared = await request('/api/imports/books/preview', {
    method: 'POST',
    token,
    contentType: `multipart/form-data; boundary=${declaredBoundary}`,
    chunks: declaredChunks,
  })
  expectError(declared, 413, 'local import request is too large')

  const chunkedBoundary = `direct-chunked-${suffix}`
  const chunked = await request('/api/imports/books/preview', {
    method: 'POST',
    token,
    contentType: `multipart/form-data; boundary=${chunkedBoundary}`,
    chunks: multipartAtExactLength(envelopeLimit + 1, chunkedBoundary),
    chunked: true,
  })
  expectError(chunked, 413, 'local import request is too large')

  const unauthBoundary = `direct-unauth-${suffix}`
  const unauthenticated = await request('/api/imports/books/preview', {
    method: 'POST',
    contentType: `multipart/form-data; boundary=${unauthBoundary}`,
    chunks: multipartChunks(unauthBoundary, [], [{ field: 'file', filename: 'unauth.txt', data: Buffer.from('x') }]),
    contentLength: envelopeLimit + 1,
  })
  expectError(unauthenticated, 401, 'missing bearer token')
  assert(recursiveFiles(stageRoot).length === 0, 'envelope rejection created staged files')

  const exactBoundary = `direct-exact-${suffix}`
  const exact = await request('/api/imports/books/preview', {
    method: 'POST',
    token,
    contentType: `multipart/form-data; boundary=${exactBoundary}`,
    chunks: multipartAtExactLength(envelopeLimit, exactBoundary),
  })
  expectError(exact, 413, 'local book exceeds maximum import size')

  const invalidFixtures = [
    {
      fields: [['importToken', 'a'.repeat(48)]],
      files: [{ field: 'file', filename: 'mixed.txt', data: Buffer.from('第一章\n正文') }],
    },
    {
      fields: [],
      files: [
        { field: 'file', filename: 'first.txt', data: Buffer.from('第一章\n正文') },
        { field: 'file', filename: 'second.txt', data: Buffer.from('第二章\n正文') },
      ],
    },
    {
      fields: [['title', 'first'], ['title', 'second']],
      files: [{ field: 'file', filename: 'duplicate-title.txt', data: Buffer.from('第一章\n正文') }],
    },
    {
      fields: [['categoryIds', '1']],
      files: [{ field: 'file', filename: 'preview-category.txt', data: Buffer.from('第一章\n正文') }],
    },
  ]
  for (const [index, fixture] of invalidFixtures.entries()) {
    const response = await multipartRequest(
      '/api/imports/books/preview',
      token,
      `direct-invalid-${suffix}-${index}`,
      fixture.fields,
      fixture.files,
    )
    expectError(response, 400, 'invalid local import request')
  }
  assert(recursiveFiles(stageRoot).length === 0, 'invalid multipart shape created staged files')

  const diskBoundary = `direct-disk-${suffix}`
  const diskBacked = await multipartRequest('/api/imports/books/preview', token, diskBoundary, [['unexpected', 'x']], [{
    field: 'file',
    filename: 'disk-backed.txt',
    data: Buffer.alloc(multipartMemoryBytes + 1, 0x78),
  }])
  expectError(diskBacked, 400, 'invalid local import request')
  assert(multipartTempFiles().length === 0, `multipart temporary files remain: ${multipartTempFiles().join(', ')}`)

  const category = await jsonRequest('/api/categories', { token, body: { name: 'runtime group' } })
  assert(category.status === 201 && category.data?.id, `category: ${category.status} ${category.text}`)

  const previewBoundary = `direct-preview-${suffix}`
  const preview = await multipartRequest('/api/imports/books/preview', token, previewBoundary, [['tocRule', '^第一章$']], [{
    field: 'file',
    filename: 'runtime.txt',
    data: Buffer.from('第一章\n真实 HTTP 正文', 'utf8'),
  }])
  assert(preview.status === 200 && preview.data?.importToken && preview.data?.chapterCount === 1, `preview: ${preview.status} ${preview.text}`)
  const importToken = preview.data.importToken

  const reparse = await multipartRequest('/api/imports/books/preview', token, `direct-token-preview-${suffix}`, [
    ['importToken', importToken],
    ['title', 'runtime token book'],
    ['tocRule', '^第一章$'],
  ], [])
  assert(reparse.status === 200 && reparse.data?.importToken === importToken, `token reparse: ${reparse.status} ${reparse.text}`)

  const imported = await multipartRequest('/api/imports/books', token, `direct-token-import-${suffix}`, [
    ['importToken', importToken],
    ['title', 'runtime token book'],
    ['tocRule', '^第一章$'],
    ['categoryIds', String(category.data.id)],
    ['categoryIds', `${category.data.id},0`],
  ], [])
  assert(imported.status === 201 && imported.data?.title === 'runtime token book', `token import: ${imported.status} ${imported.text}`)

  const aliasPreview = await multipartRequest('/api/imports/books/preview', token, `direct-alias-preview-${suffix}`, [], [{
    field: 'file',
    filename: 'runtime-alias.txt',
    data: Buffer.from('第一章\n兼容 alias 正文', 'utf8'),
  }])
  assert(aliasPreview.status === 200 && aliasPreview.data?.importToken, `alias preview: ${aliasPreview.status} ${aliasPreview.text}`)
  const aliasImported = await multipartRequest('/api/imports/txt', token, `direct-alias-import-${suffix}`, [
    ['importToken', aliasPreview.data.importToken],
    ['title', 'runtime alias book'],
  ], [])
  assert(aliasImported.status === 201 && aliasImported.data?.title === 'runtime alias book', `alias import: ${aliasImported.status} ${aliasImported.text}`)

  const books = await request('/api/books', { token })
  assert(books.status === 200 && Array.isArray(books.data) && books.data.length === 2, `books: ${books.status} ${books.text}`)
  assert(recursiveFiles(stageRoot).length === 0, `consumed tokens left staged files: ${recursiveFiles(stageRoot).join(', ')}`)
  assert(multipartTempFiles().length === 0, `final multipart temporary files remain: ${multipartTempFiles().join(', ')}`)

  console.log(JSON.stringify({
    status: 'ok',
    envelopeLimit,
    declaredAndChunkedOverflow: 413,
    authPriority: 401,
    exactEnvelope: 'file-limit-413',
    strictShape: true,
    diskBackedTemporaryFiles: 'none',
    tokenOnlyPreviewAndImport: true,
    aliases: ['/api/imports/books', '/api/imports/txt'],
  }))
}

main().catch(error => {
  console.error(error.stack || error.message)
  process.exit(1)
})

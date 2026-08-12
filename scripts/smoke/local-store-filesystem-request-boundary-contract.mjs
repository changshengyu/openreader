#!/usr/bin/env node

import { execFileSync } from 'node:child_process'
import fs from 'node:fs'
import http from 'node:http'
import https from 'node:https'
import path from 'node:path'

const targetURL = new URL(process.env.TARGET_URL || 'http://127.0.0.1:8080')
const localStoreRoot = path.resolve(String(process.env.OPENREADER_SMOKE_LOCAL_STORE_DIR || ''))
const cacheRoot = path.resolve(String(process.env.OPENREADER_SMOKE_CACHE_DIR || ''))
const tempRoot = path.resolve(String(process.env.OPENREADER_SMOKE_TMP || ''))
const maxImportBytes = Number(process.env.OPENREADER_SMOKE_MAX_IMPORT_BYTES || 33 << 20)
const uploadEnvelopeBytes = 1 << 20
const multipartMemoryBytes = 32 << 20
const metadataBodyBytes = 16 << 10
const importBodyBytes = 1 << 20

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

function request(pathname, {
  method = 'GET',
  token = '',
  contentType = '',
  chunks = [],
  chunked = false,
  parseJSON = true,
} = {}) {
  const url = new URL(pathname, targetURL)
  const transport = url.protocol === 'https:' ? https : http
  const normalizedChunks = chunks.map(chunk => Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk))
  const headers = {}
  if (contentType) headers['Content-Type'] = contentType
  if (normalizedChunks.length > 0) {
    if (chunked) headers['Transfer-Encoding'] = 'chunked'
    else headers['Content-Length'] = normalizedChunks.reduce((total, chunk) => total + chunk.length, 0)
  }
  if (token) headers.Authorization = `Bearer ${token}`

  return new Promise((resolve, reject) => {
    const outgoing = transport.request(url, { method, headers }, (incoming) => {
      const responseChunks = []
      incoming.on('data', chunk => responseChunks.push(chunk))
      incoming.on('end', () => {
        const body = Buffer.concat(responseChunks)
        const text = body.toString('utf8')
        let data = null
        if (parseJSON) {
          try {
            data = text ? JSON.parse(text) : null
          } catch {
            reject(new Error(`${method} ${pathname}: non-JSON response ${JSON.stringify(text.slice(0, 512))}`))
            return
          }
        }
        resolve({ status: incoming.statusCode || 0, headers: incoming.headers, data, text, body })
      })
    })
    outgoing.setTimeout(120_000, () => outgoing.destroy(new Error(`${method} ${pathname}: timeout`)))
    outgoing.on('error', reject)
    for (const chunk of normalizedChunks) outgoing.write(chunk)
    outgoing.end()
  })
}

function jsonRequest(pathname, { method = 'POST', token = '', body, chunked = false }) {
  return request(pathname, {
    method,
    token,
    contentType: 'application/json',
    chunks: [body],
    chunked,
  })
}

function multipartChunks(boundary, fields, files) {
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
  const empty = multipartChunks(boundary, [['path', 'overflow-target']], [{
    field: 'file', filename: 'overflow.bin', data: Buffer.alloc(0),
  }])
  const fixedBytes = multipartLength(empty)
  assert(fixedBytes < targetBytes, `multipart metadata is ${fixedBytes} bytes`)
  const chunks = multipartChunks(boundary, [['path', 'overflow-target']], [{
    field: 'file', filename: 'overflow.bin', data: Buffer.alloc(targetBytes - fixedBytes, 0x78),
  }])
  assert(multipartLength(chunks) === targetBytes, `multipart fixture is ${multipartLength(chunks)} bytes`)
  return chunks
}

function exactPaddedJSON(targetBytes, prefix, suffix) {
  const padding = targetBytes - Buffer.byteLength(prefix) - Buffer.byteLength(suffix)
  assert(padding >= 0, `JSON fixture overhead exceeds ${targetBytes} bytes`)
  const body = `${prefix}${'x'.repeat(padding)}${suffix}`
  assert(Buffer.byteLength(body) === targetBytes, `JSON fixture is ${Buffer.byteLength(body)} bytes`)
  return body
}

function expectError(response, status, message) {
  assert(response.status === status, `status ${response.status}, want ${status}: ${response.text.slice(0, 512)}`)
  assert(response.data && Object.keys(response.data).length === 1, `unexpected error shape: ${response.text.slice(0, 512)}`)
  assert(response.data.error === message, `error ${JSON.stringify(response.data?.error)}, want ${JSON.stringify(message)}`)
}

function multipartTempFiles() {
  if (!fs.existsSync(tempRoot)) return []
  return fs.readdirSync(tempRoot).filter(name => name.startsWith('multipart-'))
}

function recursiveFiles(root) {
  if (!fs.existsSync(root)) return []
  const result = []
  const visit = current => {
    for (const entry of fs.readdirSync(current, { withFileTypes: true })) {
      const entryPath = path.join(current, entry.name)
      if (entry.isDirectory()) visit(entryPath)
      else result.push(entryPath)
    }
  }
  visit(root)
  return result
}

async function register(username) {
  const response = await jsonRequest('/api/auth/register', {
    body: JSON.stringify({ username, password: 'password8' }),
  })
  assert(response.status === 200 && response.data?.token && response.data?.user?.id, `register: ${response.status} ${response.text}`)
  return response.data
}

async function upload(token, boundary, fields, files, chunked = false) {
  return request('/api/local-store/upload', {
    method: 'POST',
    token,
    contentType: `multipart/form-data; boundary=${boundary}`,
    chunks: multipartChunks(boundary, fields, files),
    chunked,
  })
}

async function listBooks(token) {
  const response = await request('/api/books', { token })
  assert(response.status === 200 && Array.isArray(response.data), `list books: ${response.status} ${response.text}`)
  return response.data
}

async function main() {
  assert(process.env.OPENREADER_SMOKE_LOCAL_STORE_DIR, 'OPENREADER_SMOKE_LOCAL_STORE_DIR is required')
  assert(process.env.OPENREADER_SMOKE_CACHE_DIR, 'OPENREADER_SMOKE_CACHE_DIR is required')
  assert(process.env.OPENREADER_SMOKE_TMP, 'OPENREADER_SMOKE_TMP is required')
  assert(Number.isSafeInteger(maxImportBytes) && maxImportBytes > multipartMemoryBytes, 'smoke max import bytes must exceed 32 MiB')

  const suffix = `${process.pid}${Date.now().toString().slice(-7)}`
  const prefix = `boundary-${suffix}`
  const health = await request('/api/health')
  assert(health.status === 200, `health status ${health.status}: ${health.text}`)
  const owner = await register(`local${suffix}`)
  const token = owner.token
  const stageRoot = path.join(cacheRoot, 'import-previews', String(owner.user.id))

  const initialList = await request('/api/local-store', { token })
  assert(initialList.status === 200 && Array.isArray(initialList.data?.items), `initial LocalStore list: ${initialList.status} ${initialList.text}`)
  assert(fs.existsSync(localStoreRoot), `LocalStore root is missing at ${localStoreRoot}`)

  const envelopeLimit = maxImportBytes + uploadEnvelopeBytes
  for (const chunked of [false, true]) {
    const boundary = `openreader-local-overflow-${chunked ? 'chunked' : 'declared'}`
    const response = await request('/api/local-store/upload', {
      method: 'POST',
      token,
      contentType: `multipart/form-data; boundary=${boundary}`,
      chunks: multipartAtExactLength(envelopeLimit + 1, boundary),
      chunked,
    })
    expectError(response, 413, 'local store upload request is too large')
  }
  const unauthBoundary = 'openreader-local-unauthenticated'
  const unauthenticated = await request('/api/local-store/upload', {
    method: 'POST',
    contentType: `multipart/form-data; boundary=${unauthBoundary}`,
    chunks: multipartAtExactLength(envelopeLimit + 1, unauthBoundary),
  })
  expectError(unauthenticated, 401, 'missing bearer token')
  assert(!fs.existsSync(path.join(localStoreRoot, 'overflow-target')), 'rejected envelope created its target directory')

  const sixtyFive = Array.from({ length: 65 }, (_, index) => ({
    field: 'file', filename: `many-${String(index).padStart(2, '0')}.txt`, data: Buffer.from('x'),
  }))
  const invalidShapes = [
    { fields: [['path', `${prefix}-many`]], files: sixtyFive },
    {
      fields: [['path', `${prefix}-extra`]],
      files: [
        { field: 'file', filename: 'accepted.txt', data: Buffer.from('accepted') },
        { field: 'attachment', filename: 'ignored.txt', data: Buffer.from('ignored') },
      ],
    },
    {
      fields: [['path', `${prefix}-first`], ['path', `${prefix}-second`]],
      files: [{ field: 'file', filename: 'book.txt', data: Buffer.from('body') }],
    },
  ]
  for (const [index, fixture] of invalidShapes.entries()) {
    const response = await upload(token, `openreader-local-invalid-${index}`, fixture.fields, fixture.files)
    expectError(response, 400, 'invalid upload request')
  }
  for (const suffixName of ['many', 'extra', 'first', 'second']) {
    assert(!fs.existsSync(path.join(localStoreRoot, `${prefix}-${suffixName}`)), `invalid multipart wrote ${suffixName}`)
  }

  const diskBackedBytes = multipartMemoryBytes + 1
  const managedPath = `${prefix}-managed`
  const managed = await upload(token, 'openreader-local-valid', [['path', managedPath]], [
    { field: 'file', filename: 'ordinary.bin', data: Buffer.alloc(diskBackedBytes, 0x61) },
    { field: 'file', filename: 'repeat.txt', data: Buffer.from('first') },
    { field: 'file', filename: 'repeat.txt', data: Buffer.from('second') },
  ])
  assert(managed.status === 201, `valid LocalStore upload: ${managed.status} ${managed.text}`)
  assert(managed.data?.paths?.join(',') === `${managedPath}/ordinary.bin,${managedPath}/repeat.txt,${managedPath}/repeat.txt`, `upload order: ${managed.text}`)
  assert(fs.statSync(path.join(localStoreRoot, managedPath, 'ordinary.bin')).size === diskBackedBytes, 'disk-backed upload size mismatch')
  assert(fs.readFileSync(path.join(localStoreRoot, managedPath, 'repeat.txt'), 'utf8') === 'second', 'repeated target did not preserve sequential overwrite')
  assert(multipartTempFiles().length === 0, `multipart temporary files remain: ${multipartTempFiles().join(', ')}`)

  const downloaded = await request(`/api/local-store/download?path=${encodeURIComponent(`${managedPath}/repeat.txt`)}`, { token, parseJSON: false })
  assert(downloaded.status === 200 && downloaded.body.toString('utf8') === 'second', `download mismatch: ${downloaded.status}`)
  assert(String(downloaded.headers['content-disposition'] || '').startsWith('attachment;'), 'download lost attachment disposition')

  const exactDirectory = `${prefix}-exact`
  const exactMetadata = exactPaddedJSON(metadataBodyBytes, `{"path":"","name":"${exactDirectory}","padding":"`, '"}')
  const exactMetadataResponse = await jsonRequest('/api/local-store/directory', { token, body: exactMetadata })
  assert(exactMetadataResponse.status === 201, `exact metadata body: ${exactMetadataResponse.status} ${exactMetadataResponse.text}`)
  for (const body of [
    `${exactMetadata} `,
    `{"path":"","name":"${prefix}-second"}{"name":"ignored"}`,
    `{"path":"","name":"${prefix}-trailing"} trailing`,
  ]) {
    const response = await jsonRequest('/api/local-store/directory', { token, body })
    assert([400, 413].includes(response.status), `rejected metadata body: ${response.status} ${response.text}`)
  }
  assert(!fs.existsSync(path.join(localStoreRoot, `${prefix}-second`)), 'second JSON created a directory')
  assert(!fs.existsSync(path.join(localStoreRoot, `${prefix}-trailing`)), 'trailing JSON created a directory')

  const booksBefore = await listBooks(token)
  assert(booksBefore.length === 0, `fresh boundary smoke unexpectedly has ${booksBefore.length} books`)
  const exactImport = exactPaddedJSON(importBodyBytes, `{"paths":["${prefix}-missing.txt"],"padding":"`, '"}')
  const exactImportResponse = await jsonRequest('/api/local-store/import-preview', { token, body: exactImport })
  assert(exactImportResponse.status === 200 && exactImportResponse.data?.items?.length === 0, `exact import body: ${exactImportResponse.status} ${exactImportResponse.text}`)
  for (const body of [
    `${exactImport} `,
    `{"paths":["${prefix}-missing.txt"]}{"paths":["second.txt"]}`,
    `{"paths":["${prefix}-missing.txt"],"items":[{"path":"${prefix}-missing.txt"}]}`,
  ]) {
    const response = await jsonRequest('/api/local-store/import-preview', { token, body })
    assert([400, 413].includes(response.status), `rejected import body: ${response.status} ${response.text}`)
  }
  const tooManyPaths = Array.from({ length: 201 }, (_, index) => `${prefix}-missing-${index}.txt`)
  const tooMany = await jsonRequest('/api/local-store/import', { token, body: JSON.stringify({ paths: tooManyPaths }) })
  assert(tooMany.status === 400, `201 paths: ${tooMany.status} ${tooMany.text}`)

  const bulkPath = path.join(localStoreRoot, `${prefix}-bulk`)
  fs.mkdirSync(bulkPath, { recursive: true })
  for (let index = 0; index < 201; index += 1) {
    fs.writeFileSync(path.join(bulkPath, `book-${String(index).padStart(3, '0')}.txt`), '第一章 开始\n正文')
  }
  for (const endpoint of ['/api/local-store/import-preview', '/api/local-store/import']) {
    const response = await jsonRequest(endpoint, { token, body: JSON.stringify({ paths: [`${prefix}-bulk`] }) })
    assert(response.status === 400, `${endpoint} 201-file expansion: ${response.status} ${response.text.slice(0, 512)}`)
  }
  assert((await listBooks(token)).length === 0, 'rejected import cardinality created a book')
  assert(recursiveFiles(stageRoot).length === 0, 'rejected import cardinality created staged files')

  const outsideRoot = path.join(path.dirname(localStoreRoot), `${prefix}-outside`)
  const escapePath = path.join(localStoreRoot, `${prefix}-escape`)
  fs.mkdirSync(outsideRoot, { recursive: true })
  fs.writeFileSync(path.join(outsideRoot, 'secret.txt'), 'secret')
  fs.writeFileSync(path.join(outsideRoot, 'book.txt'), '第一章 开始\n根外正文')
  fs.symlinkSync(path.relative(localStoreRoot, outsideRoot), escapePath)
  const safePath = path.join(localStoreRoot, `${prefix}-safe.txt`)
  fs.writeFileSync(safePath, '第一章 开始\n根内正文')

  const rootList = await request('/api/local-store', { token })
  assert(rootList.status === 200 && rootList.data.items.some(item => item.name === `${prefix}-safe.txt`), `safe listing failed: ${rootList.status} ${rootList.text}`)
  assert(!rootList.data.items.some(item => item.name === `${prefix}-escape`), 'listing exposed symlink')
  const unsafeRequests = [
    () => request(`/api/local-store?path=${encodeURIComponent(`${prefix}-escape`)}`, { token }),
    () => request(`/api/local-store/download?path=${encodeURIComponent(`${prefix}-escape/secret.txt`)}`, { token }),
    () => jsonRequest('/api/local-store/directory', { token, body: JSON.stringify({ path: `${prefix}-escape`, name: 'created' }) }),
    () => jsonRequest('/api/local-store/rename', { method: 'PUT', token, body: JSON.stringify({ path: `${prefix}-escape/secret.txt`, name: 'renamed.txt' }) }),
    () => upload(token, 'openreader-local-symlink', [['path', `${prefix}-escape`]], [{ field: 'file', filename: 'uploaded.txt', data: Buffer.from('upload') }]),
    () => request(`/api/local-store?path=${encodeURIComponent(`${prefix}-escape/secret.txt`)}`, { method: 'DELETE', token }),
    () => jsonRequest('/api/local-store/import-preview', { token, body: JSON.stringify({ paths: [`${prefix}-escape/book.txt`] }) }),
    () => jsonRequest('/api/local-store/import', { token, body: JSON.stringify({ paths: [`${prefix}-escape/book.txt`] }) }),
  ]
  for (const run of unsafeRequests) {
    const response = await run()
    expectError(response, 400, 'invalid path')
  }
  assert(fs.readFileSync(path.join(outsideRoot, 'secret.txt'), 'utf8') === 'secret', 'symlink actions changed outside bytes')
  assert(!fs.existsSync(path.join(outsideRoot, 'created')) && !fs.existsSync(path.join(outsideRoot, 'uploaded.txt')), 'symlink actions wrote outside root')

  const fifoPath = path.join(localStoreRoot, `${prefix}-blocked.txt`)
  execFileSync('mkfifo', [fifoPath])
  const fifoUpload = await upload(token, 'openreader-local-fifo', [], [{ field: 'file', filename: `${prefix}-blocked.txt`, data: Buffer.from('replacement') }])
  expectError(fifoUpload, 400, 'invalid path')
  const fifoRename = await jsonRequest('/api/local-store/rename', {
    method: 'PUT', token, body: JSON.stringify({ path: `${prefix}-safe.txt`, name: `${prefix}-blocked.txt` }),
  })
  expectError(fifoRename, 400, 'invalid path')
  const fifoDelete = await request(`/api/local-store?path=${encodeURIComponent(`${prefix}-blocked.txt`)}`, { method: 'DELETE', token })
  expectError(fifoDelete, 400, 'invalid path')
  assert(fs.lstatSync(fifoPath).isFIFO(), 'special target was replaced or removed')
  assert(fs.readFileSync(safePath, 'utf8').includes('根内正文'), 'rename to special target changed its source')

  const snapshotPath = path.join(localStoreRoot, `${prefix}-snapshot.txt`)
  fs.writeFileSync(snapshotPath, '第一章 开始\n快照正文')
  const preview = await jsonRequest('/api/local-store/import-preview', {
    token, body: JSON.stringify({ paths: [`${prefix}-snapshot.txt`] }),
  })
  const previewItem = preview.data?.items?.[0]
  assert(preview.status === 200 && previewItem?.importToken && previewItem?.book?.chapterCount === 1, `snapshot preview: ${preview.status} ${preview.text}`)
  fs.unlinkSync(snapshotPath)
  const imported = await jsonRequest('/api/local-store/import', {
    token,
    body: JSON.stringify({ items: [{
      path: `${prefix}-snapshot.txt`,
      importToken: previewItem.importToken,
      title: `${prefix}-imported`,
    }] }),
  })
  assert(imported.status === 200 && imported.data?.imported?.[0]?.book?.title === `${prefix}-imported`, `snapshot import: ${imported.status} ${imported.text}`)
  assert(recursiveFiles(stageRoot).length === 0, 'consumed preview left staged files')
  assert((await listBooks(token)).some(book => book.title === `${prefix}-imported`), 'snapshot import is missing from the bookshelf')

  fs.rmSync(path.join(localStoreRoot, managedPath), { recursive: true, force: true })
  fs.rmSync(path.join(localStoreRoot, exactDirectory), { recursive: true, force: true })
  fs.rmSync(bulkPath, { recursive: true, force: true })
  fs.rmSync(escapePath, { force: true })
  fs.rmSync(outsideRoot, { recursive: true, force: true })
  fs.rmSync(fifoPath, { force: true })
  fs.rmSync(safePath, { force: true })

  console.log(JSON.stringify({
    status: 'ok',
    uploadEnvelope: envelopeLimit,
    declaredAndChunkedOverflow: 413,
    multipartFiles: '1..64',
    diskBackedTemporaryFiles: 'none',
    metadataBody: metadataBodyBytes,
    importBody: importBodyBytes,
    importExpansion: 200,
    symlinkAndSpecialFiles: 'rejected',
    stagedSnapshot: 'source-independent',
  }))
}

main().catch((error) => {
  console.error(error.stack || error.message)
  process.exit(1)
})

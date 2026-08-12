#!/usr/bin/env node

import fs from 'node:fs'
import http from 'node:http'
import https from 'node:https'
import path from 'node:path'

const targetURL = new URL(process.env.TARGET_URL || 'http://127.0.0.1:8080')
const multipartLimit = 33 << 20
const deleteLimit = 16 << 10
const observedTempDir = String(process.env.OPENREADER_SMOKE_TMP || '').trim()
const observedDataDir = String(process.env.OPENREADER_SMOKE_DATA_DIR || '').trim()
const tinyPNG = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=',
  'base64',
)

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
        resolve({ status: incoming.statusCode || 0, data, text, body })
      })
    })
    outgoing.setTimeout(90_000, () => outgoing.destroy(new Error(`${method} ${pathname}: timeout`)))
    outgoing.on('error', reject)
    for (const chunk of normalizedChunks) outgoing.write(chunk)
    outgoing.end()
  })
}

function jsonRequest(pathname, { method, token = '', body, chunked = false }) {
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

function multipartAtExactLength(targetBytes) {
  const boundary = 'openreader-asset-overflow'
  const empty = multipartChunks(boundary, [['type', 'font']], [{
    field: 'file',
    filename: 'reader.ttf',
    data: Buffer.alloc(0),
  }])
  const fixedBytes = multipartLength(empty)
  assert(fixedBytes < targetBytes, `multipart metadata is ${fixedBytes} bytes`)
  const data = Buffer.alloc(targetBytes - fixedBytes)
  data.set([0x00, 0x01, 0x00, 0x00])
  const chunks = multipartChunks(boundary, [['type', 'font']], [{
    field: 'file',
    filename: 'reader.ttf',
    data,
  }])
  assert(multipartLength(chunks) === targetBytes, `multipart fixture is ${multipartLength(chunks)} bytes`)
  return { boundary, chunks }
}

function expectError(response, status, message) {
  assert(response.status === status, `status ${response.status}, want ${status}: ${response.text.slice(0, 512)}`)
  assert(response.data && Object.keys(response.data).length === 1, `unexpected error shape: ${response.text.slice(0, 512)}`)
  assert(response.data.error === message, `error ${JSON.stringify(response.data?.error)}, want ${JSON.stringify(message)}`)
}

function paddedDeleteBody(url, targetBytes) {
  const prefix = `{"url":${JSON.stringify(url)},"padding":"`
  const suffix = '"}'
  const paddingBytes = targetBytes - Buffer.byteLength(prefix) - Buffer.byteLength(suffix)
  assert(paddingBytes >= 0, `delete fixture already exceeds ${targetBytes} bytes`)
  const result = prefix + 'x'.repeat(paddingBytes) + suffix
  assert(Buffer.byteLength(result) === targetBytes, `delete fixture is ${Buffer.byteLength(result)} bytes`)
  return result
}

function multipartTempFiles() {
  if (!observedTempDir || !fs.existsSync(observedTempDir)) return []
  return fs.readdirSync(observedTempDir).filter(name => name.startsWith('multipart-'))
}

function observedAssetPath(url) {
  if (!observedDataDir || !url.startsWith('/uploads/')) return ''
  return path.join(observedDataDir, 'uploads', url.slice('/uploads/'.length))
}

async function register(username) {
  const response = await jsonRequest('/api/auth/register', {
    method: 'POST',
    body: JSON.stringify({ username, password: 'password8' }),
  })
  assert(response.status === 200 && response.data?.token && response.data?.user?.id, `register: ${response.status} ${response.text}`)
  return response.data
}

async function main() {
  const suffix = `${process.pid}${Date.now().toString().slice(-7)}`
  const health = await request('/api/health')
  assert(health.status === 200, `health status ${health.status}: ${health.text}`)
  const owner = await register(`asset${suffix}`)
  const token = owner.token

  const overflow = multipartAtExactLength(multipartLimit + 1)
  for (const chunked of [false, true]) {
    const response = await request('/api/uploads', {
      method: 'POST',
      token,
      contentType: `multipart/form-data; boundary=${overflow.boundary}`,
      chunks: overflow.chunks,
      chunked,
    })
    expectError(response, 413, 'request body too large')
  }
  const unauthenticated = await request('/api/uploads', {
    method: 'POST',
    contentType: `multipart/form-data; boundary=${overflow.boundary}`,
    chunks: overflow.chunks,
  })
  expectError(unauthenticated, 401, 'missing bearer token')

  const invalidShapes = [
    {
      name: 'duplicate file',
      fields: [['type', 'cover']],
      files: [
        { field: 'file', filename: 'first.png', data: tinyPNG, contentType: 'image/png' },
        { field: 'file', filename: 'second.png', data: tinyPNG, contentType: 'image/png' },
      ],
    },
    {
      name: 'extra file field',
      fields: [['type', 'cover']],
      files: [
        { field: 'file', filename: 'cover.png', data: tinyPNG, contentType: 'image/png' },
        { field: 'ignored', filename: 'ignored.png', data: tinyPNG, contentType: 'image/png' },
      ],
    },
    {
      name: 'duplicate type',
      fields: [['type', 'cover'], ['type', 'background']],
      files: [{ field: 'file', filename: 'cover.png', data: tinyPNG, contentType: 'image/png' }],
    },
  ]
  for (const [index, fixture] of invalidShapes.entries()) {
    const boundary = `openreader-invalid-${index}`
    const response = await request('/api/uploads', {
      method: 'POST',
      token,
      contentType: `multipart/form-data; boundary=${boundary}`,
      chunks: multipartChunks(boundary, fixture.fields, fixture.files),
    })
    expectError(response, 400, 'invalid upload request')
  }

  const uploadBoundary = 'openreader-valid-upload'
  const uploaded = await request('/api/uploads', {
    method: 'POST',
    token,
    contentType: `multipart/form-data; boundary=${uploadBoundary}`,
    chunks: multipartChunks(uploadBoundary, [['type', 'cover']], [{
      field: 'file', filename: 'cover.png', data: tinyPNG, contentType: 'image/png',
    }]),
  })
  assert(uploaded.status === 201 && uploaded.data?.url && uploaded.data?.type === 'covers', `upload: ${uploaded.status} ${uploaded.text}`)
  assert(uploaded.data.size === tinyPNG.length && uploaded.data.name === 'cover.png', `upload response: ${uploaded.text}`)
  assert(uploaded.data.url.startsWith(`/uploads/users/${owner.user.id}/covers/`), `upload owner URL: ${uploaded.data.url}`)

  const publicAsset = await request(uploaded.data.url, { parseJSON: false })
  assert(publicAsset.status === 200 && publicAsset.body.equals(tinyPNG), `public asset mismatch: ${publicAsset.status}`)
  const assetPath = observedAssetPath(uploaded.data.url)
  if (assetPath) {
    assert(fs.existsSync(assetPath), `uploaded asset is missing at ${assetPath}`)
    assert(fs.readFileSync(assetPath).equals(tinyPNG), `uploaded asset bytes differ at ${assetPath}`)
  }

  const deleteOverflowBody = paddedDeleteBody(uploaded.data.url, deleteLimit + 1)
  for (const chunked of [false, true]) {
    const response = await jsonRequest('/api/uploads', {
      method: 'DELETE', token, body: deleteOverflowBody, chunked,
    })
    expectError(response, 413, 'request body too large')
  }
  for (const suffixBody of [' {"url":"ignored"}', ' trailing']) {
    const response = await jsonRequest('/api/uploads', {
      method: 'DELETE',
      token,
      body: `{"url":${JSON.stringify(uploaded.data.url)}}${suffixBody}`,
    })
    expectError(response, 400, 'url is required')
  }
  const retained = await request(uploaded.data.url, { parseJSON: false })
  assert(retained.status === 200 && retained.body.equals(tinyPNG), `rejected delete changed asset: ${retained.status}`)

  const exactDelete = await jsonRequest('/api/uploads', {
    method: 'DELETE',
    token,
    body: paddedDeleteBody(uploaded.data.url, deleteLimit),
  })
  assert(exactDelete.status === 200 && exactDelete.data?.deleted === true, `exact delete: ${exactDelete.status} ${exactDelete.text}`)
  const deleted = await request(uploaded.data.url, { parseJSON: false })
  assert(!deleted.body.equals(tinyPNG), `deleted asset remains publicly readable with status ${deleted.status}`)
  if (assetPath) assert(!fs.existsSync(assetPath), `deleted asset remains at ${assetPath}`)

  const tempFiles = multipartTempFiles()
  assert(tempFiles.length === 0, `multipart temporary files remain in ${path.resolve(observedTempDir)}: ${tempFiles.join(', ')}`)

  console.log(JSON.stringify({
    status: 'ok',
    multipartEnvelope: multipartLimit,
    declaredAndChunkedOverflow: 413,
    multipartShape: 'single-file-and-type',
    deleteBody: deleteLimit,
    deleteSingleJSON: true,
    temporaryFiles: observedTempDir ? 'none' : 'not-observed',
    finalFile: observedDataDir ? 'removed' : 'verified-over-http',
  }))
}

main().catch((error) => {
  console.error(error.stack || error.message)
  process.exit(1)
})

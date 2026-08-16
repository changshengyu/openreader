#!/usr/bin/env node

import { execFileSync } from 'node:child_process'
import fs from 'node:fs'
import http from 'node:http'
import https from 'node:https'
import path from 'node:path'

const targetURL = new URL(process.env.TARGET_URL || 'http://127.0.0.1:8080')
const webDAVRoot = path.resolve(String(process.env.OPENREADER_SMOKE_WEBDAV_DIR || ''))
const cacheRoot = path.resolve(String(process.env.OPENREADER_SMOKE_CACHE_DIR || ''))
const importBodyBytes = 1 << 20
const restoreBodyBytes = 16 << 10

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
    const outgoing = transport.request(url, { method, headers }, incoming => {
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

function exactPaddedJSON(targetBytes, prefix, suffix) {
  const padding = targetBytes - Buffer.byteLength(prefix) - Buffer.byteLength(suffix)
  assert(padding >= 0, `JSON fixture overhead exceeds ${targetBytes} bytes`)
  const body = `${prefix}${'x'.repeat(padding)}${suffix}`
  assert(Buffer.byteLength(body) === targetBytes, `JSON fixture is ${Buffer.byteLength(body)} bytes`)
  return body
}

function expectError(response, status, message = '') {
  assert(response.status === status, `status ${response.status}, want ${status}: ${response.text.slice(0, 512)}`)
  assert(response.data && Object.keys(response.data).length === 1, `unexpected error shape: ${response.text.slice(0, 512)}`)
  if (message) {
    assert(response.data.error === message, `error ${JSON.stringify(response.data?.error)}, want ${JSON.stringify(message)}`)
  }
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

async function listBooks(token) {
  const response = await request('/api/books', { token })
  assert(response.status === 200 && Array.isArray(response.data), `list books: ${response.status} ${response.text}`)
  return response.data
}

async function main() {
  assert(process.env.OPENREADER_SMOKE_WEBDAV_DIR, 'OPENREADER_SMOKE_WEBDAV_DIR is required')
  assert(process.env.OPENREADER_SMOKE_CACHE_DIR, 'OPENREADER_SMOKE_CACHE_DIR is required')

  const suffix = `${process.pid}${Date.now().toString().slice(-7)}`
  const prefix = `webdav-boundary-${suffix}`
  const health = await request('/api/health')
  assert(health.status === 200, `health status ${health.status}: ${health.text}`)
  const owner = await register(`webdav${suffix}`)
  const token = owner.token
  const stageRoot = path.join(cacheRoot, 'import-previews', String(owner.user.id))
  const backupStageRoot = path.join(cacheRoot, 'backup-uploads', String(owner.user.id))
  fs.mkdirSync(webDAVRoot, { recursive: true })

  const exactImport = exactPaddedJSON(importBodyBytes, `{"paths":["${prefix}-missing.txt"],"padding":"`, '"}')
  const exactImportResponse = await jsonRequest('/api/webdav/import-preview', { token, body: exactImport })
  assert(exactImportResponse.status === 200 && exactImportResponse.data?.items?.length === 0, `exact import body: ${exactImportResponse.status} ${exactImportResponse.text}`)
  for (const chunked of [false, true]) {
    const response = await jsonRequest('/api/webdav/import-preview', {
      token,
      body: `${exactImport} `,
      chunked,
    })
    expectError(response, 413, 'request body too large')
  }
  const unauthenticated = await jsonRequest('/api/webdav/import-preview', { body: `${exactImport} ` })
  expectError(unauthenticated, 401, 'missing bearer token')
  for (const body of [
    `{"paths":["${prefix}-missing.txt"]}{"paths":["second.txt"]}`,
    `{"paths":["${prefix}-missing.txt"]} trailing`,
    Buffer.from([0x7b, 0x22, 0x70, 0x61, 0x74, 0x68, 0x73, 0x22, 0x3a, 0x5b, 0x22, 0xff, 0x22, 0x5d, 0x7d]),
  ]) {
    const response = await jsonRequest('/api/webdav/import-preview', { token, body })
    expectError(response, 400, 'paths is required')
  }

  const tooManyPaths = Array.from({ length: 201 }, (_, index) => `${prefix}-missing-${index}.txt`)
  const tooMany = await jsonRequest('/api/webdav/import', { token, body: JSON.stringify({ paths: tooManyPaths }) })
  expectError(tooMany, 400, 'too many paths')

  const bulkPath = path.join(webDAVRoot, `${prefix}-bulk`)
  fs.mkdirSync(bulkPath, { recursive: true })
  for (let index = 0; index < 201; index += 1) {
    fs.writeFileSync(path.join(bulkPath, `book-${String(index).padStart(3, '0')}.txt`), '第一章 开始\n正文')
  }
  for (const endpoint of ['/api/webdav/import-preview', '/api/webdav/import']) {
    const response = await jsonRequest(endpoint, { token, body: JSON.stringify({ paths: [`${prefix}-bulk`] }) })
    expectError(response, 400, 'too many paths')
  }
  assert((await listBooks(token)).length === 0, 'rejected cardinality created a book')
  assert(recursiveFiles(stageRoot).length === 0, 'rejected cardinality created staged files')

  const outsideRoot = path.join(path.dirname(webDAVRoot), `${prefix}-outside`)
  const mixedRoot = path.join(webDAVRoot, `${prefix}-mixed`)
  fs.mkdirSync(outsideRoot, { recursive: true })
  fs.mkdirSync(mixedRoot, { recursive: true })
  fs.writeFileSync(path.join(outsideRoot, 'outside.txt'), '第一章 开始\n根外正文')
  fs.writeFileSync(path.join(mixedRoot, 'safe.txt'), '第一章 开始\n根内正文')
  fs.symlinkSync(path.relative(mixedRoot, outsideRoot), path.join(mixedRoot, 'escape'))
  const mixedPreview = await jsonRequest('/api/webdav/import-preview', {
    token,
    body: JSON.stringify({ paths: [`${prefix}-mixed`] }),
  })
  assert(mixedPreview.status === 200 && mixedPreview.data?.items?.length === 1, `nested symlink preview: ${mixedPreview.status} ${mixedPreview.text}`)
  assert(mixedPreview.data.items[0].path === `${prefix}-mixed/safe.txt`, `nested symlink changed planner order: ${mixedPreview.text}`)
  assert(!recursiveFiles(stageRoot).some(file => fs.readFileSync(file).includes('根外正文')), 'nested symlink staged outside bytes')
  const unrelatedToken = mixedPreview.data.items[0].importToken
  const unrelatedStageFiles = recursiveFiles(stageRoot)
    .filter(file => path.basename(file).startsWith(unrelatedToken))
    .map(file => path.basename(file))
    .sort()
  assert(unrelatedStageFiles.length > 0, 'nested safe preview did not create its stage')
  const explicitSymlink = await jsonRequest('/api/webdav/import-preview', {
    token,
    body: JSON.stringify({ paths: [`${prefix}-mixed/escape/outside.txt`] }),
  })
  expectError(explicitSymlink, 400, 'invalid path')

  const fifoPath = path.join(webDAVRoot, `${prefix}-blocked.txt`)
  execFileSync('mkfifo', [fifoPath])
  const explicitFIFO = await jsonRequest('/api/webdav/import-preview', {
    token,
    body: JSON.stringify({ paths: [`${prefix}-blocked.txt`] }),
  })
  expectError(explicitFIFO, 400, 'invalid path')
  assert(fs.lstatSync(fifoPath).isFIFO(), 'explicit special file was changed')

  const snapshotPath = path.join(webDAVRoot, `${prefix}-snapshot.txt`)
  fs.writeFileSync(snapshotPath, '第一章 开始\n快照正文')
  const preview = await jsonRequest('/api/webdav/import-preview', {
    token,
    body: JSON.stringify({ paths: [`${prefix}-snapshot.txt`] }),
  })
  const previewItem = preview.data?.items?.[0]
  assert(preview.status === 200 && previewItem?.importToken && previewItem?.book?.chapterCount === 1, `snapshot preview: ${preview.status} ${preview.text}`)

  fs.rmSync(webDAVRoot, { recursive: true, force: true })
  const replacementRoot = path.join(path.dirname(webDAVRoot), `${prefix}-replacement-root`)
  fs.mkdirSync(replacementRoot, { recursive: true })
  fs.symlinkSync(path.relative(path.dirname(webDAVRoot), replacementRoot), webDAVRoot)
  const imported = await jsonRequest('/api/webdav/import', {
    token,
    body: JSON.stringify({ items: [{
      path: `${prefix}-snapshot.txt`,
      importToken: previewItem.importToken,
      title: `${prefix}-imported`,
    }] }),
  })
  assert(imported.status === 200 && imported.data?.imported?.[0]?.book?.title === `${prefix}-imported`, `token-only import: ${imported.status} ${imported.text}`)
  assert(fs.lstatSync(webDAVRoot).isSymbolicLink(), 'token-only import replaced the mounted root')
  const remainingUnrelatedStageFiles = recursiveFiles(stageRoot)
    .filter(file => path.basename(file).startsWith(unrelatedToken))
    .map(file => path.basename(file))
    .sort()
  assert(JSON.stringify(remainingUnrelatedStageFiles) === JSON.stringify(unrelatedStageFiles), 'unrelated preview stage was changed by token-only import')
  fs.rmSync(webDAVRoot, { force: true })
  fs.mkdirSync(webDAVRoot, { recursive: true })

  const backup = await request('/api/backup/trigger', { method: 'POST', token })
  assert(backup.status === 200 && backup.data?.path?.endsWith('.zip'), `backup trigger: ${backup.status} ${backup.text}`)
  const backupPath = path.join(webDAVRoot, backup.data.path)
  const backupBytes = fs.readFileSync(backupPath)
  const exactRestore = exactPaddedJSON(restoreBodyBytes, `{"path":"${backup.data.path}","padding":"`, '"}')
  const exactRestoreResponse = await jsonRequest('/api/backup/restore-webdav', { token, body: exactRestore })
  assert(exactRestoreResponse.status === 200, `exact restore body: ${exactRestoreResponse.status} ${exactRestoreResponse.text}`)
  assert(fs.readFileSync(backupPath).equals(backupBytes), 'restore changed the mounted backup source')
  assert(recursiveFiles(backupStageRoot).length === 0, 'successful restore left a private snapshot')

  for (const chunked of [false, true]) {
    const response = await jsonRequest('/api/backup/restore-webdav', {
      token,
      body: `${exactRestore} `,
      chunked,
    })
    expectError(response, 413, 'request body too large')
  }
  const trailingRestore = await jsonRequest('/api/backup/restore-webdav', {
    token,
    body: `{"path":"${backup.data.path}"}{"path":"ignored.zip"}`,
  })
  expectError(trailingRestore, 400, 'path is required')

  const outsideBackup = path.join(path.dirname(webDAVRoot), `${prefix}-outside.zip`)
  fs.writeFileSync(outsideBackup, backupBytes)
  fs.symlinkSync(path.relative(webDAVRoot, outsideBackup), path.join(webDAVRoot, `${prefix}-symlink.zip`))
  fs.mkdirSync(path.join(webDAVRoot, `${prefix}-directory.zip`))
  const restoreFIFO = path.join(webDAVRoot, `${prefix}-fifo.zip`)
  execFileSync('mkfifo', [restoreFIFO])
  for (const unsafePath of [`${prefix}-symlink.zip`, `${prefix}-directory.zip`, `${prefix}-fifo.zip`]) {
    const response = await jsonRequest('/api/backup/restore-webdav', {
      token,
      body: JSON.stringify({ path: unsafePath }),
    })
    expectError(response, 400, 'backup file not found')
  }
  assert(fs.readFileSync(outsideBackup).equals(backupBytes), 'unsafe restore changed outside backup bytes')
  assert(recursiveFiles(backupStageRoot).length === 0, 'rejected restore created a private snapshot')
  assert((await listBooks(token)).some(book => book.title === `${prefix}-imported`), 'restored bookshelf lost the imported book')

  console.log(JSON.stringify({
    status: 'ok',
    importBody: importBodyBytes,
    restoreBody: restoreBodyBytes,
    declaredAndChunkedOverflow: 413,
    importExpansion: 200,
    symlinkAndSpecialFiles: 'rejected',
    tokenOnlyImport: 'source-independent',
    restoreSnapshot: 'private-and-cleaned',
  }))
}

main().catch(error => {
  console.error(error.stack || error.message)
  process.exit(1)
})

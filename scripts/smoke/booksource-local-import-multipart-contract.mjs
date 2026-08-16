#!/usr/bin/env node

import { execFile, spawn } from 'node:child_process'
import { mkdtemp, rm } from 'node:fs/promises'
import { createServer, request as httpRequest } from 'node:http'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { promisify } from 'node:util'

import { openSmokeBrowser } from './playwright-runtime.mjs'

const execFileAsync = promisify(execFile)
const rootDir = join(dirname(fileURLToPath(import.meta.url)), '..', '..')
const backendDir = join(rootDir, 'backend')
const publicDir = join(rootDir, 'frontend', 'dist')
const sourceFileLimit = 16 * 1024 * 1024
const multipartRequestLimit = 17 * 1024 * 1024

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

async function reserveLocalPort() {
  const server = createServer()
  await new Promise((resolve, reject) => {
    server.once('error', reject)
    server.listen(0, '127.0.0.1', resolve)
  })
  const address = server.address()
  assert(address && typeof address === 'object', 'unable to reserve an OpenReader port')
  const port = address.port
  await new Promise(resolve => server.close(resolve))
  return port
}

async function stopProcess(child) {
  if (!child || child.exitCode !== null) return
  const exited = new Promise(resolve => child.once('exit', resolve))
  child.kill('SIGTERM')
  await Promise.race([exited, new Promise(resolve => setTimeout(resolve, 5_000))])
  if (child.exitCode === null) {
    child.kill('SIGKILL')
    await Promise.race([exited, new Promise(resolve => setTimeout(resolve, 2_000))])
  }
}

async function waitForHealth(root, output) {
  const deadline = Date.now() + 60_000
  let lastError = null
  while (Date.now() < deadline) {
    try {
      const response = await fetch(`${root}/api/health`)
      if (response.ok) return
      lastError = new Error(`health returned ${response.status}`)
    } catch (error) {
      lastError = error
    }
    await new Promise(resolve => setTimeout(resolve, 250))
  }
  throw new Error(`OpenReader source-import server did not start: ${lastError?.message || 'unknown error'}\n${output()}`)
}

async function startOpenReader() {
  const tempRoot = await mkdtemp(join(tmpdir(), 'openreader-source-import-'))
  const binary = join(tempRoot, 'openreader')
  const port = await reserveLocalPort()
  await execFileAsync('go', ['build', '-o', binary, '.'], {
    cwd: backendDir,
    env: { ...process.env, GOCACHE: join(backendDir, '.gocache') },
    maxBuffer: 4 * 1024 * 1024,
  })
  let output = ''
  const child = spawn(binary, [], {
    cwd: backendDir,
    env: {
      ...process.env,
      OPENREADER_ADDR: `127.0.0.1:${port}`,
      OPENREADER_DATA_DIR: join(tempRoot, 'data'),
      OPENREADER_CACHE_DIR: join(tempRoot, 'cache'),
      OPENREADER_LIBRARY_DIR: join(tempRoot, 'library'),
      OPENREADER_LOCAL_STORE_DIR: join(tempRoot, 'library', 'localStore'),
      OPENREADER_DB: join(tempRoot, 'data', 'openreader.db'),
      OPENREADER_PUBLIC_DIR: publicDir,
      OPENREADER_JWT_SECRET: 'booksource-local-import-contract-secret',
      OPENREADER_CHECK_INTERVAL: '24h',
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  child.stdout.on('data', chunk => { output += chunk.toString() })
  child.stderr.on('data', chunk => { output += chunk.toString() })
  const root = `http://127.0.0.1:${port}`
  try {
    await waitForHealth(root, () => output)
  } catch (error) {
    await stopProcess(child)
    await rm(tempRoot, { recursive: true, force: true })
    throw error
  }
  return {
    root,
    close: async () => {
      await stopProcess(child)
      await rm(tempRoot, { recursive: true, force: true })
    },
  }
}

async function api(root, path, { token = '', method = 'GET', body } = {}) {
  const response = await fetch(`${root}/api${path}`, {
    method,
    headers: {
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const text = await response.text()
  let data = null
  try {
    data = text ? JSON.parse(text) : null
  } catch {
    data = text
  }
  return { response, data, text }
}

async function register(root, suffix) {
  const safeSuffix = String(suffix).replace(/[^a-zA-Z0-9]/g, '')
  const username = `srcimp${safeSuffix}${String(Date.now()).slice(-8)}`
  const result = await api(root, '/auth/register', {
    method: 'POST',
    body: { username, password: 'source-import-contract-123' },
  })
  assert(result.response.status === 200, `register ${suffix}: ${result.response.status} ${result.text}`)
  assert(result.data?.token, `register ${suffix}: missing token`)
  return result.data.token
}

async function multipart(root, token, files, values = {}) {
  const form = new FormData()
  for (const file of files) {
    form.append(file.field, new Blob([file.data], { type: 'application/json' }), file.name)
  }
  for (const [field, value] of Object.entries(values)) form.append(field, value)
  const response = await fetch(`${root}/api/sources/import`, {
    method: 'POST',
    headers: token ? { Authorization: `Bearer ${token}` } : {},
    body: form,
  })
  const text = await response.text()
  return { response, text, data: text ? JSON.parse(text) : null }
}

function rawHTTP(root, { token = '', contentType, contentLength, body }) {
  const target = new URL('/api/sources/import', root)
  return new Promise((resolve, reject) => {
    const request = httpRequest({
      hostname: target.hostname,
      port: target.port,
      path: target.pathname,
      method: 'POST',
      headers: {
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
        'Content-Type': contentType,
        ...(contentLength === undefined ? {} : { 'Content-Length': String(contentLength) }),
      },
    }, response => {
      const chunks = []
      response.on('data', chunk => chunks.push(chunk))
      response.on('end', () => {
        const text = Buffer.concat(chunks).toString()
        resolve({ status: response.statusCode, text, data: text ? JSON.parse(text) : null })
      })
    })
    request.once('error', reject)
    request.end(body)
  })
}

function source(identity) {
  return Buffer.from(JSON.stringify([{
    bookSourceName: identity,
    bookSourceUrl: `https://${identity}.example`,
  }]))
}

function exactJSONBytes(size) {
  const data = Buffer.alloc(size, 0x20)
  data.write('[]')
  return data
}

function assertError(result, status, message, label) {
  const actualStatus = result.response?.status ?? result.status
  assert(actualStatus === status, `${label}: ${actualStatus} ${result.text}`)
  assert(result.data?.error === message, `${label}: error ${JSON.stringify(result.data)}`)
}

async function runAPIBoundary(root) {
  const token = await register(root, 'wire')
  const declaredUnauthorized = await rawHTTP(root, {
    contentType: 'multipart/form-data; boundary=declared-unauthorized',
    contentLength: multipartRequestLimit + 1,
  })
  assertError(declaredUnauthorized, 401, 'missing bearer token', 'unauthorized declared overflow')

  const declared = await rawHTTP(root, {
    token,
    contentType: 'multipart/form-data; boundary=declared-overflow',
    contentLength: multipartRequestLimit + 1,
  })
  assertError(declared, 413, 'request body too large', 'declared overflow')

  const duplicate = await multipart(root, token, [
    { field: 'file', name: 'first.json', data: source('duplicate-first') },
    { field: 'file', name: 'second.json', data: source('duplicate-second') },
  ])
  assertError(duplicate, 400, 'invalid source import request', 'duplicate file')

  const scalar = await multipart(root, token, [
    { field: 'file', name: 'source.json', data: source('scalar-file') },
  ], { note: 'ambiguous' })
  assertError(scalar, 400, 'invalid source import request', 'scalar part')

  const foreign = await multipart(root, token, [
    { field: 'attachment', name: 'source.json', data: source('foreign-file') },
  ])
  assertError(foreign, 400, 'invalid source import request', 'foreign file')

  const broken = await rawHTTP(root, {
    token,
    contentType: 'multipart/form-data; boundary=broken',
    body: Buffer.from('--broken\r\ninvalid\r\n'),
  })
  assertError(broken, 400, 'invalid source import request', 'broken multipart')

  const exact = await multipart(root, token, [
    { field: 'file', name: 'exact.json', data: exactJSONBytes(sourceFileLimit) },
  ])
  assert(exact.response.status === 200, `exact file limit: ${exact.response.status} ${exact.text}`)

  const fileOverflow = await multipart(root, token, [
    { field: 'file', name: 'overflow.json', data: exactJSONBytes(sourceFileLimit + 1) },
  ])
  assertError(fileOverflow, 413, 'source file is too large', 'file overflow')

  const boundary = 'openreader-source-import-chunked'
  const prefix = Buffer.from(`--${boundary}\r\nContent-Disposition: form-data; name="file"; filename="source.json"\r\nContent-Type: application/json\r\n\r\n`)
  const suffix = Buffer.from(`\r\n--${boundary}--\r\n`)
  const chunked = await rawHTTP(root, {
    token,
    contentType: `multipart/form-data; boundary=${boundary}`,
    body: Buffer.concat([prefix, Buffer.alloc(multipartRequestLimit + 1, 0x78), suffix]),
  })
  assertError(chunked, 413, 'request body too large', 'chunked overflow')

  const before = await api(root, '/sources', { token })
  assert(before.response.status === 200 && before.data.length === 0, `rejected requests mutated sources: ${before.text}`)
  const valid = await multipart(root, token, [
    { field: 'file', name: 'valid.data', data: source('wire-valid') },
  ])
  assert(valid.response.status === 200 && valid.data.imported === 1, `valid multipart: ${valid.response.status} ${valid.text}`)
  return true
}

async function openMobileNavigation(page, viewport) {
  if (viewport.width > 750) return
  await page.locator('.mobile-menu-trigger').click()
  await page.waitForFunction(() => {
    const sidebar = document.querySelector('.app-sidebar')
    return sidebar && Math.abs(Number.parseFloat(getComputedStyle(sidebar).marginLeft)) < 0.5
  })
}

async function chooseLocalSource(page, file) {
  const chooserPromise = page.waitForEvent('filechooser')
  await page.getByRole('button', { name: '导入书源' }).click()
  const chooser = await chooserPromise
  await chooser.setFiles(file)
}

async function runViewport(browser, root, viewport, index) {
  const token = await register(root, `browser-${index}`)
  const context = await browser.newContext({
    viewport,
    isMobile: viewport.width <= 750,
    hasTouch: viewport.width <= 750,
  })
  const page = await context.newPage()
  const failures = []
  let importRequests = 0
  page.on('pageerror', error => failures.push(`pageerror: ${error.message}`))
  page.on('console', message => {
    if (message.type() === 'error' && !/WebSocket connection to .*\/ws\/sync/.test(message.text())) {
      failures.push(`console.error: ${message.text()}`)
    }
  })
  page.on('request', request => {
    if (request.method() === 'POST' && new URL(request.url()).pathname === '/api/sources/import') {
      importRequests += 1
    }
  })
  await page.addInitScript(value => localStorage.setItem('openreader_token', value), token)

  await page.goto(root, { waitUntil: 'networkidle' })
  await page.locator('.shelf-page').waitFor({ state: 'visible', timeout: 10_000 })
  await openMobileNavigation(page, viewport)
  await chooseLocalSource(page, {
    name: 'oversized-bookSources.json',
    mimeType: 'application/json',
    buffer: Buffer.alloc(sourceFileLimit + 1, 0x20),
  })
  await page.getByText('书源文件过大', { exact: true }).waitFor({ state: 'visible', timeout: 10_000 })
  assert(importRequests === 0, `${viewport.width}: oversized chooser reached the API`)
  assert(await page.locator('.source-import-preview-dialog').count() === 0, `${viewport.width}: oversized chooser opened preview`)

  await page.goto(root, { waitUntil: 'networkidle' })
  await openMobileNavigation(page, viewport)
  await chooseLocalSource(page, {
    name: 'bookSources.json',
    mimeType: 'application/json',
    buffer: Buffer.from(JSON.stringify([
      { bookSourceName: `浏览器安全源 ${index}`, bookSourceUrl: `https://browser-safe-${index}.example` },
      { bookSourceName: `浏览器脚本源 ${index}`, bookSourceUrl: `https://browser-script-${index}.example`, header: '<js>private</js>' },
    ])),
  })
  const preview = page.locator('.source-import-preview-dialog')
  await preview.getByText(`浏览器安全源 ${index}`, { exact: true }).waitFor({ state: 'visible', timeout: 10_000 })
  assert(await preview.locator('input[type="checkbox"]:checked').count() === 0, `${viewport.width}: preview did not start empty`)
  await preview.getByText('全选', { exact: true }).click()
  await preview.getByText('已选择 1 个', { exact: true }).waitFor({ state: 'visible' })
  await preview.getByRole('button', { name: '确定' }).click()
  await preview.waitFor({ state: 'hidden', timeout: 10_000 })
  assert(importRequests === 1, `${viewport.width}: normal chooser import requests ${importRequests}`)

  const listed = await api(root, '/sources', { token })
  assert(listed.response.status === 200, `${viewport.width}: list sources ${listed.response.status}`)
  assert(listed.data.some(item => item.name === `浏览器安全源 ${index}`), `${viewport.width}: selected source was not imported`)
  assert(!listed.data.some(item => item.name === `浏览器脚本源 ${index}`), `${viewport.width}: unselected executable source was imported`)

  const geometry = await page.evaluate(() => ({ width: document.documentElement.scrollWidth, viewport: innerWidth }))
  assert(geometry.width <= geometry.viewport + 1, `${viewport.width}: horizontal overflow ${geometry.width}`)
  assert(failures.length === 0, failures.join('\n'))
  await context.close()
  return `${viewport.width}x${viewport.height}`
}

async function run() {
  const app = await startOpenReader()
  let browser
  try {
    const apiBoundary = await runAPIBoundary(app.root)
    browser = await openSmokeBrowser()
    const viewports = []
    viewports.push(await runViewport(browser, app.root, { width: 1440, height: 900 }, 1))
    viewports.push(await runViewport(browser, app.root, { width: 390, height: 844 }, 2))
    viewports.push(await runViewport(browser, app.root, { width: 360, height: 800 }, 3))
    console.log(`booksource-local-import-multipart: ok ${viewports.join(', ')} realGo=true apiBoundary=${apiBoundary} oversizeNoRequest=true singleFile=true`)
  } finally {
    await browser?.close()
    await app.close()
  }
}

run().catch(error => {
  console.error(error.stack || error.message)
  process.exit(1)
})

#!/usr/bin/env node

import { execFile, spawn } from 'node:child_process'
import { access, mkdtemp, rm } from 'node:fs/promises'
import { createServer } from 'node:http'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { promisify } from 'node:util'

import { openSmokeBrowser } from './playwright-runtime.mjs'

const execFileAsync = promisify(execFile)
const rootDir = join(dirname(fileURLToPath(import.meta.url)), '..', '..')
const backendDir = join(rootDir, 'backend')
const publicDir = join(rootDir, 'frontend', 'dist')

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

function tinyPNG() {
  return Buffer.from(
    'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=',
    'base64',
  )
}

function minimalTTFSignature() {
  const data = Buffer.alloc(16)
  data.set([0x00, 0x01, 0x00, 0x00], 0)
  return data
}

async function reserveLocalPort() {
  const server = createServer()
  await new Promise((resolve, reject) => {
    server.once('error', reject)
    server.listen(0, '127.0.0.1', resolve)
  })
  const address = server.address()
  assert(address && typeof address === 'object', 'unable to reserve a local OpenReader test port')
  await new Promise(resolve => server.close(resolve))
  return address.port
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
    await new Promise(resolve => setTimeout(resolve, 300))
  }
  throw new Error(`OpenReader portable-assets test server did not start: ${lastError?.message || 'unknown error'}\n${output()}`)
}

async function startOpenReader() {
  await access(join(publicDir, 'index.html')).catch(() => {
    throw new Error('frontend/dist is missing; run `cd frontend && npm run build` before this smoke')
  })
  const tempRoot = await mkdtemp(join(tmpdir(), 'openreader-portable-assets-real-api-'))
  const binary = join(tempRoot, 'openreader')
  const port = await reserveLocalPort()
  await execFileAsync('go', ['build', '-o', binary, '.'], {
    cwd: backendDir,
    env: process.env,
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
      OPENREADER_JWT_SECRET: 'portable-appearance-assets-contract-secret',
      OPENREADER_CORS_ORIGIN: `http://127.0.0.1:${port}`,
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
  if (!response.ok) throw new Error(`${method} ${path} failed with ${response.status}: ${text}`)
  return data
}

async function register(root, username) {
  const result = await api(root, '/auth/register', {
    method: 'POST',
    body: { username, password: 'portable-assets-contract' },
  })
  assert(result?.token && Number(result?.user?.id) > 0, `${username}: registration returned no identity`)
  return { token: result.token, userID: Number(result.user.id) }
}

async function uploadAsset(root, token, kind, name, type, bytes) {
  const form = new FormData()
  form.append('type', kind)
  form.append('file', new Blob([bytes], { type }), name)
  const response = await fetch(`${root}/api/uploads`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
    body: form,
  })
  const text = await response.text()
  if (!response.ok) throw new Error(`asset upload failed with ${response.status}: ${text}`)
  return JSON.parse(text).url
}

async function importLocalBook(root, token, title) {
  const form = new FormData()
  form.append('file', new Blob([
    '第一章 可移植外观\n',
    '春风过处，纸页微明。\n',
    ...Array.from({ length: 24 }, (_, index) => `跨用户恢复后的正文段落 ${index + 1}。\n`),
  ], { type: 'text/plain' }), `${title}.txt`)
  form.append('title', title)
  const response = await fetch(`${root}/api/imports/books`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
    body: form,
  })
  const text = await response.text()
  if (!response.ok) throw new Error(`local import failed with ${response.status}: ${text}`)
  return JSON.parse(text)
}

async function downloadPortableBackup(root, token, name) {
  const response = await fetch(`${root}/api/backup/download/${encodeURIComponent(name)}`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!response.ok) {
    throw new Error(`portable backup download failed with ${response.status}: ${await response.text()}`)
  }
  return Buffer.from(await response.arrayBuffer())
}

async function restorePortableBackup(root, token, bytes, name) {
  const form = new FormData()
  form.append('file', new Blob([bytes], { type: 'application/zip' }), name)
  const response = await fetch(`${root}/api/backup/restore-legado`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
    body: form,
  })
  const text = await response.text()
  if (!response.ok) throw new Error(`portable backup restore failed with ${response.status}: ${text}`)
  return JSON.parse(text)
}

function managedAssetURLs(value) {
  const urls = []
  const walk = (current) => {
    if (Array.isArray(current)) {
      current.forEach(walk)
      return
    }
    if (current && typeof current === 'object') {
      Object.values(current).forEach(walk)
      return
    }
    if (typeof current === 'string' && current.startsWith('/uploads/users/')) urls.push(current)
  }
  walk(value)
  return urls
}

async function seedPortableRestore(root, viewport) {
  const suffix = `${viewport.width}${viewport.height}`
  await register(root, `bootstrap${suffix}`)
  const source = await register(root, `source${suffix}`)
  const title = `可移植外观 ${suffix}`
  const book = await importLocalBook(root, source.token, title)
  const backgroundBytes = tinyPNG()
  const coverBytes = tinyPNG()
  const fontBytes = minimalTTFSignature()
  const backgroundURL = await uploadAsset(root, source.token, 'background', 'portable-background.png', 'image/png', backgroundBytes)
  const fontURL = await uploadAsset(root, source.token, 'font', 'portable-font.ttf', 'font/ttf', fontBytes)
  const coverURL = await uploadAsset(root, source.token, 'cover', 'portable-cover.png', 'image/png', coverBytes)
  const sourcePrefix = `/uploads/users/${source.userID}/`
  for (const url of [backgroundURL, fontURL, coverURL]) {
    assert(url.startsWith(sourcePrefix), `${suffix}: source asset is not user-scoped: ${url}`)
  }

  await api(root, `/books/${book.id}`, {
    token: source.token,
    method: 'PUT',
    body: { customCoverUrl: coverURL },
  })
  await api(root, '/settings/reader', {
    token: source.token,
    method: 'PUT',
    body: {
      force: true,
      value: {
        mode: 'page',
        pageType: 'normal',
        fontFamily: 'hei',
        customFontsMap: { hei: fontURL },
        theme: 'custom',
        themeType: 'day',
        customBgColor: '#f4e9bd',
        customBgImage: backgroundURL,
        customBgImageList: [backgroundURL],
        customConfigName: '可移植外观',
        customConfigList: [{
          name: '可移植外观',
          fontFamily: 'hei',
          customFontsMap: { hei: fontURL },
          customBgImage: backgroundURL,
          customBgImageList: [backgroundURL, '/uploads/backgrounds/legacy-reference.png'],
        }],
        autoTheme: false,
        fontSize: 18,
        fontWeight: 400,
        brightness: 100,
        lineHeight: 1.8,
        paragraphSpace: 0.2,
        columnWidth: 800,
        settingsVersion: 13,
      },
    },
  })

  const trigger = await api(root, '/backup/portable/trigger', {
    token: source.token,
    method: 'POST',
  })
  assert(trigger?.format === 'openreader-portable-v2', `${suffix}: portable format ${trigger?.format}`)
  assert(Number(trigger?.localBooks) === 1, `${suffix}: portable localBooks ${trigger?.localBooks}`)
  assert(Number(trigger?.assets) === 3, `${suffix}: portable assets ${trigger?.assets}`)
  assert(Number(trigger?.legacyAssets) === 1, `${suffix}: portable legacyAssets ${trigger?.legacyAssets}`)
  const archive = await downloadPortableBackup(root, source.token, trigger.name)

  await register(root, `filler${suffix}`)
  const target = await register(root, `target${suffix}`)
  assert(target.userID !== source.userID, `${suffix}: target user ID did not change`)
  const restored = await restorePortableBackup(root, target.token, archive, trigger.name)
  assert(Number(restored?.localBooks) === 1, `${suffix}: restored localBooks ${restored?.localBooks}`)
  assert(Number(restored?.assets) === 3, `${suffix}: restored assets ${restored?.assets}`)
  assert(Number(restored?.legacyAssets) === 1, `${suffix}: restored legacyAssets ${restored?.legacyAssets}`)

  const setting = await api(root, '/settings/reader', { token: target.token })
  const books = await api(root, '/books', { token: target.token })
  const restoredBook = books.find(item => item.title === title)
  assert(restoredBook, `${suffix}: restored book is missing`)
  const targetPrefix = `/uploads/users/${target.userID}/`
  const settingText = JSON.stringify(setting?.value || {})
  assert(!settingText.includes(sourcePrefix), `${suffix}: restored setting retained source owner URL`)
  assert(!settingText.includes('openreader-asset://'), `${suffix}: restored setting retained placeholder`)
  assert(settingText.includes('/uploads/backgrounds/legacy-reference.png'), `${suffix}: legacy URL was not preserved`)
  assert(restoredBook.customCoverUrl?.startsWith(`${targetPrefix}covers/`), `${suffix}: restored cover URL ${restoredBook.customCoverUrl}`)
  assert(!restoredBook.customCoverUrl.includes(sourcePrefix), `${suffix}: restored cover retained source owner URL`)

  const restoredURLs = [...new Set([
    ...managedAssetURLs(setting.value),
    restoredBook.customCoverUrl,
  ])]
  assert(restoredURLs.length === 3, `${suffix}: restored unique asset URLs ${JSON.stringify(restoredURLs)}`)
  const backgroundRestored = restoredURLs.find(url => url.includes('/backgrounds/'))
  const fontRestored = restoredURLs.find(url => url.includes('/fonts/'))
  const coverRestored = restoredURLs.find(url => url.includes('/covers/'))
  for (const [url, expected] of [
    [backgroundRestored, backgroundBytes],
    [fontRestored, fontBytes],
    [coverRestored, coverBytes],
  ]) {
    assert(url?.startsWith(targetPrefix), `${suffix}: restored asset is not target-scoped: ${url}`)
    const response = await fetch(`${root}${url}`, { cache: 'no-store' })
    assert(response.status === 200, `${suffix}: restored asset ${url} returned ${response.status}`)
    assert(Buffer.from(await response.arrayBuffer()).equals(expected), `${suffix}: restored asset bytes changed: ${url}`)
  }

  return {
    target,
    title,
    book: restoredBook,
    backgroundURL: backgroundRestored,
    fontURL: fontRestored,
    coverURL: coverRestored,
  }
}

async function openSettings(page, viewport) {
  const button = viewport.width <= 750
    ? page.locator('.reader-mobile-top.visible .mobile-tool-button').filter({ hasText: '设置' })
    : page.locator('.reader-left-rail button[title="设置"]')
  await button.click()
  await page.locator('.settings-body').waitFor({ state: 'visible', timeout: 10_000 })
}

async function runViewport(browser, root, viewport) {
  const restored = await seedPortableRestore(root, viewport)
  const context = await browser.newContext({
    viewport,
    isMobile: viewport.width <= 750,
    hasTouch: viewport.width <= 750,
  })
  const page = await context.newPage()
  const failures = []
  page.on('pageerror', error => failures.push(`pageerror: ${error.message}`))
  page.on('console', message => {
    if (message.type() !== 'error') return
    const text = message.text()
    if (/WebSocket connection to .*\/ws\/sync/.test(text)) return
    failures.push(`console.error: ${text}`)
  })
  page.on('response', response => {
    const path = new URL(response.url()).pathname
    if (path.startsWith('/api/') && response.status() >= 400) {
      failures.push(`api ${response.status()} ${path}`)
    }
  })

  try {
    await page.addInitScript(token => localStorage.setItem('openreader_token', token), restored.target.token)
    await page.goto(root, { waitUntil: 'domcontentloaded' })
    const row = page.locator('.shelf-page .book-row').filter({ hasText: restored.title }).first()
    await row.waitFor({ state: 'visible', timeout: 15_000 })
    const shelfCover = await row.locator('.list-cover').evaluate(element => getComputedStyle(element).backgroundImage)
    assert(shelfCover.includes(restored.coverURL), `${viewport.width}: shelf cover does not use restored URL: ${shelfCover}`)

    await page.goto(`${root}/books/${restored.book.id}/read?chapter=0`, { waitUntil: 'domcontentloaded' })
    await page.locator('.reader-page').waitFor({ state: 'visible', timeout: 15_000 })
    const readerAppearance = await page.locator('.reader-page').evaluate((element) => ({
      background: getComputedStyle(element).getPropertyValue('--reader-bg-image'),
      font: getComputedStyle(element).getPropertyValue('--reader-font-family'),
      bodyWidth: document.body.scrollWidth,
      viewportWidth: window.innerWidth,
      toolbarVisible: Boolean(document.querySelector('.reader-mobile-top.visible')),
      fontFace: document.querySelector('#openreader-custom-fonts')?.textContent || '',
    }))
    assert(readerAppearance.background.includes(restored.backgroundURL), `${viewport.width}: Reader background ${readerAppearance.background}`)
    assert(readerAppearance.font.includes('OpenReaderCustomHei'), `${viewport.width}: Reader font stack ${readerAppearance.font}`)
    assert(readerAppearance.fontFace.includes(restored.fontURL), `${viewport.width}: Reader font-face retained no restored URL`)
    assert(readerAppearance.bodyWidth <= readerAppearance.viewportWidth + 1, `${viewport.width}: horizontal overflow ${readerAppearance.bodyWidth}/${readerAppearance.viewportWidth}`)
    if (viewport.width <= 750) {
      assert(readerAppearance.toolbarVisible, `${viewport.width}: mobile toolbar is not visible on entry`)
    }

    await openSettings(page, viewport)
    const preview = page.locator(`.content-bg-preview img[src="${restored.backgroundURL}"]`)
    await preview.waitFor({ state: 'visible', timeout: 10_000 })
    assert(await preview.locator('..').evaluate(element => element.classList.contains('selected')), `${viewport.width}: restored background is not selected`)
    const selectedFont = page.locator('.font-family-option.active').filter({ hasText: '黑体' })
    await selectedFont.waitFor({ state: 'visible', timeout: 10_000 })
    if (viewport.width <= 750) {
      assert(await page.locator('.reader-mobile-top.visible').isVisible(), `${viewport.width}: settings hid the mobile toolbar`)
    }
    assert(failures.length === 0, `${viewport.width}: ${failures.join('\n')}`)
    return `${viewport.width}x${viewport.height}`
  } finally {
    await context.close()
  }
}

async function run() {
  const app = await startOpenReader()
  try {
    const browser = await openSmokeBrowser()
    try {
      const completed = []
      for (const viewport of [
        { width: 1440, height: 900 },
        { width: 390, height: 844 },
        { width: 360, height: 800 },
      ]) {
        completed.push(await runViewport(browser, app.root, viewport))
      }
      console.log(`portable-appearance-assets-real-api: ok ${completed.join(', ')} realApi=true v2=true crossUser=true shelfCover=true readerBackground=true readerFont=true legacyLink=true`)
    } finally {
      await browser.close()
    }
  } finally {
    await app.close()
  }
}

run().catch(error => {
  console.error(error.stack || error.message)
  process.exit(1)
})

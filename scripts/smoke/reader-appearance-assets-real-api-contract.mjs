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
  throw new Error(`OpenReader appearance-assets test server did not start: ${lastError?.message || 'unknown error'}\n${output()}`)
}

async function startOpenReader() {
  await access(join(publicDir, 'index.html')).catch(() => {
    throw new Error('frontend/dist is missing; run `cd frontend && npm run build` before this smoke')
  })
  const tempRoot = await mkdtemp(join(tmpdir(), 'openreader-reader-assets-real-api-'))
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
      OPENREADER_JWT_SECRET: 'reader-appearance-assets-contract-secret',
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

async function importLocalBook(root, token, title) {
  const form = new FormData()
  form.append('file', new Blob([
    '第一章 开始\n',
    '春风过处，纸页微明。\n',
    ...Array.from({ length: 24 }, (_, index) => `阅读外观资产真实接口段落 ${index + 1}。\n`),
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

async function seedReader(root, viewport) {
  const suffix = `${viewport.width}${viewport.height}`
  const registered = await api(root, '/auth/register', {
    method: 'POST',
    body: { username: `assets${suffix}`, password: 'reader-assets-contract' },
  })
  assert(registered?.token && Number(registered?.user?.id) > 0, `${suffix}: registration returned no identity`)
  const book = await importLocalBook(root, registered.token, `外观资产 ${suffix}`)
  assert(Number(book?.id) > 0, `${suffix}: local import returned no book`)
  return { token: registered.token, userID: Number(registered.user.id), book }
}

async function waitForSetting(root, token, predicate, description) {
  const deadline = Date.now() + 15_000
  while (Date.now() < deadline) {
    const setting = await api(root, '/settings/reader', { token })
    if (predicate(setting?.value || {})) return setting.value
    await new Promise(resolve => setTimeout(resolve, 120))
  }
  throw new Error(`timed out: ${description}`)
}

async function waitForCondition(predicate, description, timeout = 10_000) {
  const deadline = Date.now() + timeout
  while (Date.now() < deadline) {
    if (predicate()) return
    await new Promise(resolve => setTimeout(resolve, 50))
  }
  throw new Error(`timed out: ${description}`)
}

async function openSettings(page, viewport) {
  const button = viewport.width <= 750
    ? page.locator('.reader-mobile-top.visible .mobile-tool-button').filter({ hasText: '设置' })
    : page.locator('.reader-left-rail button[title="设置"]')
  await button.click()
  await page.locator('.settings-body').waitFor({ state: 'visible', timeout: 10_000 })
}

async function uploadFromInput(page, selector, file) {
  const responsePromise = page.waitForResponse(response => (
    response.request().method() === 'POST'
    && new URL(response.url()).pathname === '/api/uploads'
  ), { timeout: 15_000 })
  await page.locator(selector).setInputFiles(file)
  const response = await responsePromise
  const text = await response.text()
  assert(response.status() === 201, `asset upload failed with ${response.status()}: ${text}`)
  return JSON.parse(text)
}

async function assertAssetGone(root, url, label) {
  const deadline = Date.now() + 10_000
  let status = 200
  while (Date.now() < deadline) {
    const response = await fetch(`${root}${url}`, { cache: 'no-store' })
    status = response.status
    if (status === 404) return
    if (status === 200 && response.headers.get('content-type')?.includes('text/html')) {
      const text = await response.text()
      if (/<!doctype html/i.test(text)) return
    }
    await new Promise(resolve => setTimeout(resolve, 100))
  }
  throw new Error(`${label} remained available as an asset, final status ${status}`)
}

async function runViewport(browser, root, viewport, fontBuffer) {
  const seeded = await seedReader(root, viewport)
  const context = await browser.newContext({
    viewport,
    isMobile: viewport.width <= 750,
    hasTouch: viewport.width <= 750,
  })
  const page = await context.newPage()
  const failures = []
  const uploadDeleteResults = []
  let expectedSettingsFailure = false
  page.on('pageerror', error => failures.push(`pageerror: ${error.message}`))
  page.on('console', message => {
    if (message.type() !== 'error') return
    const text = message.text()
    if (/WebSocket connection to .*\/ws\/sync/.test(text)) return
    if (expectedSettingsFailure && text.includes('500')) return
    failures.push(`console.error: ${text}`)
  })
  page.on('response', response => {
    const path = new URL(response.url()).pathname
    if (path === '/api/uploads' && response.request().method() === 'DELETE') {
      uploadDeleteResults.push({
        status: response.status(),
        url: response.request().postDataJSON()?.url,
      })
    }
    if (!path.startsWith('/api/') || response.status() < 400) return
    if (expectedSettingsFailure && path === '/api/settings/reader' && response.status() === 500) return
    failures.push(`api ${response.status()} ${path}`)
  })

  try {
    await page.addInitScript(token => localStorage.setItem('openreader_token', token), seeded.token)
    await page.goto(`${root}/books/${seeded.book.id}/read?chapter=0`, { waitUntil: 'domcontentloaded' })
    await page.locator('.reader-page').waitFor({ state: 'visible', timeout: 15_000 })
    await openSettings(page, viewport)
    await page.locator('.theme-custom-button').click()

    const background = await uploadFromInput(page, '.upload-bg-upload input[type="file"]', {
      name: 'reader-background.png',
      mimeType: 'image/png',
      buffer: tinyPNG(),
    })
    assert(background.url.startsWith(`/uploads/users/${seeded.userID}/backgrounds/`), `${viewport.width}: background path ${background.url}`)
    await waitForSetting(
      root,
      seeded.token,
      value => value.customBgImage === background.url && value.customBgImageList?.includes(background.url),
      `${viewport.width}: background reference persisted`,
    )
    const backgroundResponse = await fetch(`${root}${background.url}`)
    assert(backgroundResponse.status === 200, `${viewport.width}: background static response ${backgroundResponse.status}`)

    let failNextSettingSave = true
    await page.route('**/api/settings/reader', async route => {
      if (route.request().method() === 'PUT' && failNextSettingSave) {
        failNextSettingSave = false
        expectedSettingsFailure = true
        await route.fulfill({
          status: 500,
          contentType: 'application/json',
          body: JSON.stringify({ error: 'forced reader setting failure' }),
        })
        return
      }
      await route.continue()
    })
    const rejectedBackground = await uploadFromInput(page, '.upload-bg-upload input[type="file"]', {
      name: 'reader-background-rejected.png',
      mimeType: 'image/png',
      buffer: tinyPNG(),
    })
    await waitForCondition(
      () => !failNextSettingSave,
      `${viewport.width}: rejected background setting save was not attempted`,
    )
    await waitForSetting(
      root,
      seeded.token,
      value => value.customBgImage === background.url
        && value.customBgImageList?.includes(background.url)
        && !value.customBgImageList.includes(rejectedBackground.url),
      `${viewport.width}: failed upload left the previous server setting intact`,
    )
    await waitForCondition(
      () => uploadDeleteResults.some(result => result.url === rejectedBackground.url),
      `${viewport.width}: rejected background cleanup request was not sent`,
    )
    const rejectedCleanup = uploadDeleteResults.find(result => result.url === rejectedBackground.url)
    assert(
      rejectedCleanup.status === 200,
      `${viewport.width}: rejected background cleanup returned ${rejectedCleanup.status}`,
    )
    await assertAssetGone(root, rejectedBackground.url, `${viewport.width}: rejected background`)
    expectedSettingsFailure = false
    await page.unroute('**/api/settings/reader')

    const heiOption = page.locator('.font-family-option').filter({ hasText: '黑体' })
    const font = await uploadFromInput(page, '.font-family-option:has-text("黑体") input[type="file"]', {
      name: 'reader-custom-hei.ttf',
      mimeType: 'font/ttf',
      buffer: fontBuffer,
    })
    assert(font.url.startsWith(`/uploads/users/${seeded.userID}/fonts/`), `${viewport.width}: font path ${font.url}`)
    await waitForSetting(
      root,
      seeded.token,
      value => value.fontFamily === 'hei' && value.customFontsMap?.hei === font.url,
      `${viewport.width}: hei font reference persisted`,
    )
    assert(await heiOption.evaluate(element => element.classList.contains('active')), `${viewport.width}: hei option is not selected`)
    assert(await page.locator('#openreader-custom-fonts').evaluate((style, url) => (
      style.textContent.includes('OpenReaderCustomHei') && style.textContent.includes(url)
    ), font.url), `${viewport.width}: custom @font-face missing`)

    await page.reload({ waitUntil: 'domcontentloaded' })
    await page.locator('.reader-page').waitFor({ state: 'visible', timeout: 15_000 })
    await openSettings(page, viewport)
    const restoredPreview = page.locator(`.content-bg-preview img[src="${background.url}"]`)
    await restoredPreview.waitFor({ state: 'visible', timeout: 10_000 })
    assert(await restoredPreview.locator('..').evaluate(element => element.classList.contains('selected')), `${viewport.width}: restored background is not selected`)
    const restoredHei = page.locator('.font-family-option').filter({ hasText: '黑体' })
    assert(await restoredHei.evaluate(element => element.classList.contains('active')), `${viewport.width}: restored hei option is not active`)

    const fontDeletePromise = page.waitForResponse(response => (
      response.request().method() === 'DELETE'
      && new URL(response.url()).pathname === '/api/uploads'
    ), { timeout: 15_000 })
    await restoredHei.locator('button[title="恢复默认字体"]').click()
    const fontDelete = await fontDeletePromise
    assert(fontDelete.status() === 200, `${viewport.width}: font delete ${fontDelete.status()} ${await fontDelete.text()}`)
    await waitForSetting(
      root,
      seeded.token,
      value => !value.customFontsMap?.hei,
      `${viewport.width}: font reference removed`,
    )
    await assertAssetGone(root, font.url, `${viewport.width}: cleared font`)

    const backgroundDeletePromise = page.waitForResponse(response => (
      response.request().method() === 'DELETE'
      && new URL(response.url()).pathname === '/api/uploads'
    ), { timeout: 15_000 })
    await page.locator('.content-bg-preview button[title="删除背景图"]').click()
    const backgroundDelete = await backgroundDeletePromise
    assert(backgroundDelete.status() === 200, `${viewport.width}: background delete ${backgroundDelete.status()} ${await backgroundDelete.text()}`)
    await waitForSetting(
      root,
      seeded.token,
      value => value.customBgImage !== background.url && !value.customBgImageList?.includes(background.url),
      `${viewport.width}: background reference removed`,
    )
    await assertAssetGone(root, background.url, `${viewport.width}: cleared background`)

    const layout = await page.evaluate(() => ({
      bodyWidth: document.body.scrollWidth,
      viewportWidth: window.innerWidth,
      toolbarVisible: Boolean(document.querySelector('.reader-mobile-top.visible')),
      settingsVisible: Boolean(document.querySelector('.settings-body')),
    }))
    assert(layout.bodyWidth <= layout.viewportWidth + 1, `${viewport.width}: horizontal overflow ${layout.bodyWidth}/${layout.viewportWidth}`)
    if (viewport.width <= 750) {
      assert(layout.toolbarVisible, `${viewport.width}: mobile toolbar hidden while settings are open`)
    }
    assert(layout.settingsVisible, `${viewport.width}: settings closed unexpectedly`)
    assert(failures.length === 0, `${viewport.width}: ${failures.join('\n')}`)
    return `${viewport.width}x${viewport.height}`
  } finally {
    await context.close()
  }
}

async function run() {
  const app = await startOpenReader()
  const fontBuffer = minimalTTFSignature()
  try {
    const browser = await openSmokeBrowser()
    try {
      const completed = []
      for (const viewport of [
        { width: 1440, height: 900 },
        { width: 390, height: 844 },
        { width: 360, height: 800 },
      ]) {
        completed.push(await runViewport(browser, app.root, viewport, fontBuffer))
      }
      console.log(`reader-appearance-assets-real-api: ok ${completed.join(', ')} realApi=true uploadPersist=true rollbackCleanup=true fiveFontSlots=true reload=true delete=true`)
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

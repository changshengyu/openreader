#!/usr/bin/env node

import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import {
  existsSync,
  mkdtempSync,
  mkdirSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

import { openSmokeBrowser } from './playwright-runtime.mjs'

const baseURL = process.env.TARGET_URL || 'http://127.0.0.1:8080'
const outputDir = process.env.SMOKE_OUTPUT_DIR || tmpdir()

function smokeViewports() {
  return String(process.env.SMOKE_VIEWPORTS || '1440x900,390x844,360x800')
    .split(',')
    .map(value => value.trim())
    .filter(Boolean)
    .map((value) => {
      const [width, height] = value.toLowerCase().split('x').map(Number)
      if (!Number.isInteger(width) || !Number.isInteger(height) || width < 1 || height < 1) {
        throw new Error(`Invalid SMOKE_VIEWPORTS entry: ${value}`)
      }
      return { width, height }
    })
}

function comicSVG(label, color) {
  const rows = Array.from({ length: 42 }, (_, index) => (
    `<text x="80" y="${260 + index * 90}" font-size="44" fill="#fff">${label} · ${index + 1}</text>`
  )).join('')
  return `<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="4200" viewBox="0 0 1200 4200">
    <rect width="1200" height="4200" fill="${color}"/>
    <text x="80" y="150" font-size="72" fill="#fff">${label}</text>
    ${rows}
  </svg>`
}

function createCBZ() {
  const root = mkdtempSync(join(tmpdir(), 'openreader-cbz-smoke-'))
  const source = join(root, 'source')
  mkdirSync(join(source, 'pages'), { recursive: true })
  mkdirSync(join(source, 'notes'), { recursive: true })
  writeFileSync(join(source, 'ComicInfo.xml'), '<ComicInfo><Title>CBZ 浏览器契约</Title><Writer>OpenReader</Writer></ComicInfo>')
  writeFileSync(join(source, 'pages/002.svg'), comicSVG('COVER-SECOND', '#34556b'))
  writeFileSync(join(source, 'pages/001.svg'), comicSVG('FIRST-SORTED-PAGE', '#6b4534'))
  writeFileSync(join(source, 'notes/readme.txt'), 'This non-image member must never be extracted or served.')
  const archive = join(root, 'fixture.cbz')
  execFileSync(process.env.ZIP_COMMAND || 'zip', [
    '-q',
    archive,
    'ComicInfo.xml',
    'pages/002.svg',
    'pages/001.svg',
    'notes/readme.txt',
  ], { cwd: source })
  return { archive, root }
}

async function responseJSON(response) {
  const body = await response.text()
  assert.ok(response.ok, `${response.status} ${response.url}: ${body}`)
  return body ? JSON.parse(body) : null
}

async function registerAndImport(archive, mode) {
  const username = `cbzsmoke${Date.now()}${Math.random().toString(16).slice(2)}`
  const auth = await responseJSON(await fetch(`${baseURL}/api/auth/register`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password: 'test1234' }),
  }))
  assert.ok(auth.token)
  const authorization = { Authorization: `Bearer ${auth.token}` }

  const settings = await fetch(`${baseURL}/api/settings/reader`, {
    method: 'PUT',
    headers: { ...authorization, 'Content-Type': 'application/json' },
    body: JSON.stringify({
      value: {
        mode,
        pageType: 'normal',
        clickMethod: 'auto',
        fontSize: 18,
        lineHeight: 1.8,
        paragraphSpace: 0.2,
        columnWidth: 800,
        animateDuration: 0,
        settingsVersion: 12,
      },
    }),
  })
  assert.equal(settings.status, 200, await settings.text())

  const form = new FormData()
  form.append('file', new Blob([readFileSync(archive)], { type: 'application/vnd.comicbook+zip' }), 'fixture.cbz')
  const book = await responseJSON(await fetch(`${baseURL}/api/imports/books`, {
    method: 'POST',
    headers: authorization,
    body: form,
  }))
  assert.ok(book.id)
  assert.equal(book.title, 'CBZ 浏览器契约')
  assert.equal(book.author, 'OpenReader')

  const chapters = await responseJSON(await fetch(`${baseURL}/api/books/${book.id}/chapters`, { headers: authorization }))
  assert.deepEqual(chapters.map(chapter => chapter.title), ['pages/001.svg', 'pages/002.svg'])
  assert.deepEqual(chapters.map(chapter => chapter.resourcePath), ['pages/001.svg', 'pages/002.svg'])

  assert.ok(String(book.coverUrl).startsWith('/api/cbz-resource/'))
  const cover = await fetch(new URL(book.coverUrl, baseURL))
  const coverBody = await cover.text()
  assert.equal(cover.status, 200)
  assert.match(coverBody, /COVER-SECOND/)
  return { token: auth.token, book }
}

async function runViewport(browser, viewport, archive, requestedMode) {
  const { token, book } = await registerAndImport(archive, requestedMode)
  const context = await browser.newContext({ viewport })
  await context.addInitScript((value) => localStorage.setItem('openreader_token', value), token)
  const page = await context.newPage()
  const failures = []
  const resourceResponses = []
  page.on('console', (message) => {
    if (message.type() === 'error') failures.push(message.text())
  })
  page.on('pageerror', error => failures.push(error.message))
  page.on('response', (response) => {
    if (response.url().includes('/api/cbz-resource/')) {
      resourceResponses.push({ status: response.status(), url: response.url() })
    }
  })

  await page.goto(`${baseURL}/books/${book.id}/read?chapter=0`, { waitUntil: 'networkidle' })
  const image = page.locator('.reader-content-image img').first()
  await image.waitFor({ timeout: 15_000 })
  await page.waitForFunction(() => {
    const target = document.querySelector('.reader-content-image img')
    return target?.complete && target.naturalWidth > 0
  }, null, { timeout: 15_000 })

  const state = await page.evaluate(() => {
    const shell = document.querySelector('.reader-shell')
    const pageEl = document.querySelector('.reader-page')
    const chapter = document.querySelector('.chapter-content')
    const imageBox = document.querySelector('.reader-content-image')
    const body = document.querySelector('.reader-body')
    const pageRect = pageEl.getBoundingClientRect()
    const chapterRect = chapter.getBoundingClientRect()
    const imageRect = imageBox.getBoundingClientRect()
    return {
      shellClass: shell?.className || '',
      titleCount: document.querySelectorAll('.reader-body h3').length,
      chapterLeftGap: Math.round(chapterRect.left - pageRect.left),
      chapterRightGap: Math.round(pageRect.right - chapterRect.right),
      imageLeftGap: Math.round(imageRect.left - pageRect.left),
      imageRightGap: Math.round(pageRect.right - imageRect.right),
      imageWidth: Math.round(imageRect.width),
      chapterWidth: Math.round(chapterRect.width),
      bodyClientWidth: body.clientWidth,
      bodyScrollWidth: body.scrollWidth,
      bodyClientHeight: body.clientHeight,
      bodyScrollHeight: body.scrollHeight,
      imageHeight: Math.round(imageRect.height),
      bodyColumnWidth: getComputedStyle(body).columnWidth,
      bodyColumnCount: getComputedStyle(body).columnCount,
      mobileChromeVisible: Boolean(document.querySelector('.reader-mobile-top.visible')),
      autoButton: Boolean(document.querySelector('button[title="自动阅读"]')),
      ttsButton: Boolean(document.querySelector('button[title="朗读"]')),
    }
  })
  assert.equal(state.titleCount, 0, `${viewport.width}: CBZ chapter heading must stay hidden`)
  assert.ok(state.shellClass.includes(requestedMode), `${viewport.width}: mode=${state.shellClass}, want ${requestedMode}`)
  assert.equal(state.autoButton, true, `${viewport.width}: CBZ must keep auto-reading control`)
  assert.equal(state.ttsButton, true, `${viewport.width}: CBZ must keep TTS control`)

  if (viewport.width <= 750) {
    assert.equal(state.mobileChromeVisible, true, `${viewport.width}: mobile chrome must be visible on entry`)
    const leftGap = requestedMode === 'flip' ? state.imageLeftGap : state.chapterLeftGap
    const rightGap = requestedMode === 'flip' ? state.imageRightGap : state.chapterRightGap
    assert.equal(leftGap, 16, `${viewport.width}/${requestedMode}: left gap ${leftGap}`)
    assert.equal(rightGap, 16, `${viewport.width}/${requestedMode}: right gap ${rightGap}`)

    await image.click()
    await page.locator('.el-image-viewer__wrapper').waitFor({ timeout: 5000 })
    assert.equal(await page.locator('.reader-mobile-top.visible').count(), 1, 'preview must not hide reader chrome')
    await page.locator('.el-image-viewer__close').click()
    await page.locator('.el-image-viewer__wrapper').waitFor({ state: 'detached' })
  }

  if (requestedMode === 'flip') {
    assert.ok(state.imageHeight > 0 && state.bodyScrollHeight >= state.imageHeight, `flip CBZ image must remain rendered: ${JSON.stringify(state)}`)
    const autoButton = page.locator('.reader-mobile-float-right.visible button[title="自动阅读"]')
    await autoButton.click()
    await page.waitForFunction(() => document.querySelector('.reader-shell')?.classList.contains('page'))
    await page.locator('.reader-mobile-float-right.visible button[title="自动阅读"]').click()
    await page.waitForFunction(() => document.querySelector('.reader-shell')?.classList.contains('flip'))
  }

  assert.ok(resourceResponses.length >= 1, `${viewport.width}: no CBZ resource response observed`)
  assert.equal(resourceResponses.some(response => response.status >= 400), false, JSON.stringify(resourceResponses))
  assert.deepEqual(failures, [])
  const screenshot = join(outputDir, `openreader-cbz-${viewport.width}x${viewport.height}-${requestedMode}.png`)
  await page.screenshot({ path: screenshot, fullPage: false })
  assert.equal(existsSync(screenshot), true)
  await context.close()
}

async function main() {
  const fixture = createCBZ()
  try {
    const browser = await openSmokeBrowser()
    try {
      for (const viewport of smokeViewports()) {
        await runViewport(browser, viewport, fixture.archive, viewport.width <= 750 ? 'scroll' : 'page')
      }
      await runViewport(browser, { width: 390, height: 844 }, fixture.archive, 'flip')
    } finally {
      await browser.close()
    }
    console.log('reader CBZ contract smoke passed')
  } finally {
    rmSync(fixture.root, { recursive: true, force: true })
  }
}

main().catch((error) => {
  console.error(error.stack || error.message)
  process.exit(1)
})

#!/usr/bin/env node

import { openSmokeBrowser } from './playwright-runtime.mjs'

import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import {
  cpSync,
  existsSync,
  mkdtempSync,
  mkdirSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const baseURL = process.env.TARGET_URL || 'http://127.0.0.1:8080'
const outputDir = process.env.SMOKE_OUTPUT_DIR || tmpdir()

function smokeViewports() {
  const requested = String(process.env.SMOKE_VIEWPORTS || '1440x900,390x844,360x800')
    .split(',')
    .map(value => value.trim())
    .filter(Boolean)
  return requested.map((value) => {
    const [width, height] = value.toLowerCase().split('x').map(Number)
    if (!Number.isInteger(width) || !Number.isInteger(height) || width < 1 || height < 1) {
      throw new Error(`Invalid SMOKE_VIEWPORTS entry: ${value}`)
    }
    return { width, height }
  })
}

function fixtureFontPath() {
  const candidates = [
    process.env.SMOKE_FONT_PATH,
    '/System/Library/Fonts/Supplemental/Arial.ttf',
    '/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf',
  ].filter(Boolean)
  const fontPath = candidates.find(candidate => existsSync(candidate))
  if (!fontPath) {
    throw new Error('Set SMOKE_FONT_PATH to a readable TTF font for the EPUB smoke fixture.')
  }
  return fontPath
}

function createEPUB() {
  const root = mkdtempSync(join(tmpdir(), 'openreader-epub-smoke-'))
  const source = join(root, 'source')
  for (const directory of [
    'META-INF',
    'OPS/Text',
    'OPS/styles',
    'OPS/images',
    'OPS/fonts',
    'OPS/scripts',
  ]) {
    mkdirSync(join(source, directory), { recursive: true })
  }
  writeFileSync(join(source, 'mimetype'), 'application/epub+zip')
  writeFileSync(join(source, 'META-INF/container.xml'), `<?xml version="1.0"?>
<container xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OPS/content.opf"/></rootfiles>
</container>`)
  writeFileSync(join(source, 'OPS/content.opf'), `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>EPUB 浏览器契约</dc:title>
    <dc:creator>OpenReader</dc:creator>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="titlepage" href="Text/titlepage.xhtml" media-type="application/xhtml+xml"/>
    <item id="one" href="Text/one.xhtml" media-type="application/xhtml+xml"/>
    <item id="two" href="Text/two.xhtml" media-type="application/xhtml+xml"/>
    <item id="css" href="styles/book.css" media-type="text/css"/>
    <item id="cover" href="images/cover.svg" media-type="image/svg+xml"/>
    <item id="font" href="fonts/Fixture.ttf" media-type="font/ttf"/>
  </manifest>
  <spine><itemref idref="titlepage"/><itemref idref="one"/><itemref idref="two"/></spine>
</package>`)
  writeFileSync(join(source, 'OPS/nav.xhtml'), `<html xmlns="http://www.w3.org/1999/xhtml"><body>
    <nav epub:type="toc">
      <a href="Text/titlepage.xhtml">封面</a>
      <a href="Text/one.xhtml#part-a">第一章（上）</a>
      <a href="Text/one.xhtml#part-b">第一章（下）</a>
      <a href="Text/two.xhtml#opening">第二章</a>
    </nav>
  </body></html>`)
  const paragraphs = Array.from({ length: 36 }, (_, index) => (
    `<p id="p${index + 1}">第 ${index + 1} 段：春风过处，纸页微明，用于验证 EPUB iframe 高度、连续滚动与位置恢复。</p>`
  )).join('\n')
  writeFileSync(join(source, 'OPS/Text/titlepage.xhtml'), `<html xmlns="http://www.w3.org/1999/xhtml">
  <body><img id="titlepage-cover" src="../images/cover.svg" alt="封面"/></body>
</html>`)
  writeFileSync(join(source, 'OPS/Text/one.xhtml'), `<html xmlns="http://www.w3.org/1999/xhtml">
  <head>
    <link rel="stylesheet" href="../styles/book.css"/>
    <script id="epub-authored-script">window.epubAuthoredScript = true</script>
  </head>
  <body>
    <section id="part-a">
      <h1 id="start">第一章 EPUB 文档</h1>
      <p class="fixture-marker"><span class="font-probe">相对 CSS、字体和图片资源。</span></p>
      <img id="fixture-image" src="../images/cover.svg" alt="测试图片"/>
      <p><a id="hash-link" href="#p20">跳到第二十段</a></p>
      ${paragraphs}
      <p><a id="part-b-link" href="#part-b">下一节</a></p>
    </section>
    <section id="part-b">
      <h1>第一章 EPUB 第二节</h1>
      <p id="part-b-content">这是同一 XHTML 的第二个目录片段，不能在第一节 iframe 中出现。</p>
      <p><a id="next-chapter" href="two.xhtml#opening">下一章</a></p>
    </section>
  </body>
</html>`)
  writeFileSync(join(source, 'OPS/Text/two.xhtml'), `<html xmlns="http://www.w3.org/1999/xhtml">
  <head><link rel="stylesheet" href="../styles/book.css"/></head>
  <body><h1 id="opening">第二章 EPUB 文档</h1><p>跨文档链接已经更新目录状态。</p><a href="one.xhtml#part-a">上一章</a></body>
</html>`)
  writeFileSync(join(source, 'OPS/styles/book.css'), `
    @font-face { font-family: FixtureFont; src: url("../fonts/Fixture.ttf") format("truetype"); }
    .fixture-marker { border-left: 3px solid rgb(12, 34, 56); }
    .font-probe { font-family: FixtureFont !important; }
    #fixture-image { width: 48px; height: 48px; }
  `)
  writeFileSync(join(source, 'OPS/images/cover.svg'), `<svg xmlns="http://www.w3.org/2000/svg" width="48" height="48">
    <rect width="48" height="48" fill="#2f6f6d"/>
  </svg>`)
  cpSync(fixtureFontPath(), join(source, 'OPS/fonts/Fixture.ttf'))
  writeFileSync(join(source, 'OPS/scripts/evil.js'), 'window.externalEpubScript = true')

  const archive = join(root, 'fixture.epub')
  const zip = process.env.ZIP_COMMAND || 'zip'
  execFileSync(zip, ['-q', '-0', archive, 'mimetype'], { cwd: source })
  execFileSync(zip, ['-q', '-r', archive, 'META-INF', 'OPS'], { cwd: source })
  return { archive, root }
}

async function registerAndImport(archive) {
  const username = `epubsmoke${Date.now()}${Math.random().toString(16).slice(2)}`
  const register = await fetch(`${baseURL}/api/auth/register`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password: 'test1234' }),
  })
  const registerBody = await register.text()
  assert.equal(register.status, 200, registerBody)
  const auth = JSON.parse(registerBody)
  assert.ok(auth.token)

  const form = new FormData()
  form.append('file', new Blob([readFileSync(archive)], { type: 'application/epub+zip' }), 'fixture.epub')
  form.append('tocRule', 'toc')
  const imported = await fetch(`${baseURL}/api/imports/books`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${auth.token}` },
    body: form,
  })
  const importedBody = await imported.text()
  assert.equal(imported.status, 201, importedBody)
  const book = JSON.parse(importedBody)
  assert.ok(book.id)
  return { token: auth.token, book }
}

async function seedProgress(token, bookID) {
  const chaptersResponse = await fetch(`${baseURL}/api/books/${bookID}/chapters`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  const chaptersBody = await chaptersResponse.text()
  assert.equal(chaptersResponse.status, 200, chaptersBody)
  const chapters = JSON.parse(chaptersBody)
  const target = chapters[1]
  assert.ok(target?.id, 'the image-only titlepage must precede the saved first text chapter')

  const progressResponse = await fetch(`${baseURL}/api/progress`, {
    method: 'PUT',
    headers: {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      bookId: bookID,
      chapterId: target.id,
      chapterIndex: 1,
      offset: 600,
      percent: 0.1,
      chapterPercent: 0.25,
      chapterTitle: target.title,
      mode: 'page',
      clientUpdatedAt: new Date().toISOString(),
      clientId: 'epub-browser-smoke',
    }),
  })
  const progressBody = await progressResponse.text()
  assert.equal(progressResponse.status, 200, progressBody)
}

async function assertCoverFrameContract(page, resourceResponses) {
  await page.waitForSelector('iframe.epub-iframe', { timeout: 15_000 })
  const frame = page.frameLocator('iframe.epub-iframe')
  await frame.locator('#titlepage-cover').waitFor({ timeout: 10_000 })
  const state = await frame.locator('body').evaluate((body) => {
    const image = body.querySelector('#titlepage-cover')
    return {
      bridge: Boolean(document.querySelector('#openreader-epub-bridge')),
      imageLoaded: image?.complete && image.naturalWidth > 0,
    }
  })
  assert.equal(state.bridge, true)
  assert.equal(state.imageLoaded, true)
  assert.ok(resourceResponses.some(row => row.url.includes('/OPS/Text/titlepage.xhtml') && row.status === 200))
}

async function assertCurrentEpubParagraphBookmark(page, viewport) {
  if (viewport.width <= 750 && !await page.locator('.reader-mobile-top.visible').count()) {
    await page.mouse.click(Math.round(viewport.width / 2), Math.round(viewport.height / 2))
    await page.waitForTimeout(150)
  }
  const expectedParagraph = await page.locator('iframe.epub-iframe').evaluate((frame) => {
    const viewport = document.querySelector('.reader-content')?.getBoundingClientRect()
    const frameRect = frame.getBoundingClientRect()
    if (!viewport || !frame.contentDocument) return ''
    const anchor = viewport.top + Math.min(viewport.height * 0.32, 180)
    const rows = [...frame.contentDocument.querySelectorAll('p, li, blockquote')]
      .map(node => {
        const rect = node.getBoundingClientRect()
        return {
          node,
          top: frameRect.top + rect.top,
          bottom: frameRect.top + rect.bottom,
        }
      })
      .filter(row => String(row.node.textContent || '').trim() && row.bottom >= viewport.top + 8 && row.top <= viewport.bottom - 8)
    const anchored = rows.find(row => row.top <= anchor && row.bottom >= anchor)
    const selected = anchored || rows.sort((left, right) => (
      Math.abs(left.top - anchor) - Math.abs(right.top - anchor)
    ))[0]
    return String(selected?.node?.textContent || '').trim()
  })
  assert.ok(expectedParagraph, `${viewport.width}: EPUB viewport must expose a current paragraph`)

  const button = viewport.width <= 750
    ? page.locator('.reader-mobile-float-left.visible button[title="书签"]')
    : page.locator('.reader-right-rail button[title="书签"]')
  await button.click()
  const manager = page.locator('.global-bookmark-dialog')
  await manager.waitFor({ state: 'visible', timeout: 10_000 })
  await manager.getByRole('button', { name: '添加当前段落', exact: true }).click()
  const form = page.locator('.global-bookmark-form-dialog')
  await form.waitFor({ state: 'visible', timeout: 10_000 })
  assert.equal(
    await form.locator('textarea[readonly]').inputValue(),
    expectedParagraph,
    `${viewport.width}: EPUB bookmark must contain exactly one current iframe paragraph`,
  )
  await form.locator('textarea').last().fill('EPUB 当前段落')
  await form.getByRole('button', { name: '确定', exact: true }).click()
  await form.waitFor({ state: 'hidden', timeout: 10_000 })
  await manager.getByText('EPUB 当前段落', { exact: true }).waitFor({ state: 'visible', timeout: 10_000 })
  assert.equal(await manager.isVisible(), true, `${viewport.width}: saving EPUB paragraph must keep bookmark manager open`)
  await manager.getByRole('button', { name: '取消', exact: true }).click()
  await manager.waitFor({ state: 'hidden', timeout: 10_000 })
  if (viewport.width <= 750) {
    assert.equal(await page.locator('.reader-mobile-top.visible').count(), 1)
  }
}

async function assertFrameContract(page, viewport, resourceResponses) {
  console.log(`checking ${viewport.width}x${viewport.height}`)
  await page.waitForSelector('iframe.epub-iframe', { timeout: 15_000 })
  const frame = page.frameLocator('iframe.epub-iframe')
  await frame.locator('h1').waitFor({ timeout: 10_000 })
  await page.waitForTimeout(300)

  const frameState = await frame.locator('body').evaluate((body) => {
    const marker = body.querySelector('.fixture-marker')
    const image = body.querySelector('#fixture-image')
    const bodyStyle = getComputedStyle(body)
    const paragraphStyle = getComputedStyle(marker)
    return {
    text: body.innerText,
      bridge: Boolean(document.querySelector('#openreader-epub-bridge')),
      authoredScript: Boolean(document.querySelector('#epub-authored-script')),
      authoredGlobal: Boolean(window.epubAuthoredScript),
      fontSize: bodyStyle.fontSize,
      paragraphColor: paragraphStyle.color,
      borderLeftColor: paragraphStyle.borderLeftColor,
      borderLeftWidth: paragraphStyle.borderLeftWidth,
      imageWidth: Math.round(image.getBoundingClientRect().width),
      imageLoaded: image.complete && image.naturalWidth > 0,
    }
  })
  assert.match(frameState.text, /第一章 EPUB 文档/)
  assert.doesNotMatch(frameState.text, /第一章 EPUB 第二节/)
  assert.equal(frameState.bridge, true)
  assert.equal(frameState.authoredScript, false)
  assert.equal(frameState.authoredGlobal, false)
  assert.equal(frameState.fontSize, '18px')
  assert.equal(frameState.paragraphColor, 'rgb(36, 40, 44)')
  assert.equal(frameState.borderLeftColor, 'rgb(12, 34, 56)')
  assert.equal(frameState.borderLeftWidth, '3px')
  assert.equal(frameState.imageWidth, 48)
  assert.equal(frameState.imageLoaded, true)

  const contentState = await page.locator('.reader-content').evaluate((element) => ({
    scrollHeight: element.scrollHeight,
    clientHeight: element.clientHeight,
    hasTextBlocks: Boolean(document.querySelector('.reader-body [data-reader-block]')),
    frameHeight: Math.round(document.querySelector('iframe.epub-iframe').getBoundingClientRect().height),
  }))
  assert.ok(contentState.scrollHeight > contentState.clientHeight * 2)
  assert.equal(contentState.hasTextBlocks, false)
  assert.ok(contentState.frameHeight > contentState.clientHeight)
  assert.ok(resourceResponses.some(row => row.url.includes('/OPS/styles/book.css') && row.status === 200))
  assert.ok(resourceResponses.some(row => row.url.includes('/OPS/images/cover.svg') && row.status === 200))
  assert.ok(resourceResponses.some(row => row.url.includes('/OPS/fonts/Fixture.ttf') && row.status === 200))
  if (viewport.width <= 750) {
    assert.equal(await page.locator('.reader-mobile-top.visible').count(), 1)
  }

  const restoredOffset = await page.locator('.reader-content').evaluate(element => element.scrollTop)
  assert.ok(restoredOffset > 400, `saved EPUB offset was not restored: ${restoredOffset}`)
  await page.locator('.reader-content').evaluate((element) => {
    element.scrollTop = 0
    element.dispatchEvent(new Event('scroll'))
  })
  await frame.locator('body').press('ArrowDown')
  await page.waitForTimeout(350)
  const keyboardOffset = await page.locator('.reader-content').evaluate(element => element.scrollTop)
  assert.ok(keyboardOffset > 100, `EPUB ArrowDown did not page: ${keyboardOffset}`)
  await frame.locator('body').press('Home')
  await page.waitForTimeout(250)
  const homeOffset = await page.locator('.reader-content').evaluate(element => element.scrollTop)
  assert.ok(homeOffset < keyboardOffset, `EPUB Home did not move toward the top: ${homeOffset}`)

  await assertCurrentEpubParagraphBookmark(page, viewport)

  if (viewport.width <= 750) {
    if (!await page.locator('.reader-mobile-top.visible').count()) {
      await page.mouse.click(Math.round(viewport.width / 2), Math.round(viewport.height / 2))
      await page.waitForTimeout(150)
    }
    assert.equal(await page.locator('.reader-mobile-top.visible').count(), 1)
    const settingsTool = page.locator('.reader-mobile-top.visible .mobile-tool-button').filter({ hasText: '设置' })
    await settingsTool.click()
    await page.waitForSelector('.reader-mobile-workspace')
    assert.equal(await page.locator('.reader-mobile-top.visible').count(), 1)
    await page.mouse.click(Math.round(viewport.width / 2), Math.round(viewport.height / 2))
    assert.equal(await page.locator('.reader-mobile-top.visible').count(), 1)
    await settingsTool.click()
    await page.waitForFunction(() => !document.querySelector('.reader-mobile-workspace'))

    await page.mouse.click(Math.round(viewport.width / 2), Math.round(viewport.height / 2))
    await page.waitForTimeout(150)
    assert.equal(await page.locator('.reader-mobile-top.visible').count(), 0)
    await page.mouse.click(Math.round(viewport.width / 2), Math.round(viewport.height / 2))
    await page.waitForTimeout(150)
    assert.equal(await page.locator('.reader-mobile-top.visible').count(), 1)
  }

  await frame.locator('#fixture-image').click()
  await page.waitForSelector('.el-image-viewer__wrapper', { timeout: 5000 })
  await page.locator('.el-image-viewer__close').click()
  await page.waitForSelector('.el-image-viewer__wrapper', { state: 'detached' })
  assert.match(page.url(), /\/read(?:\?|$)/)

  const beforeHash = await page.locator('.reader-content').evaluate(element => element.scrollTop)
  await frame.locator('#hash-link').click()
  await page.waitForTimeout(150)
  const afterHash = await page.locator('.reader-content').evaluate(element => element.scrollTop)
  assert.ok(afterHash > beforeHash + 100)

  if (viewport.width <= 750 && await page.locator('.reader-mobile-top.visible').count()) {
    await page.mouse.click(Math.round(viewport.width / 2), Math.round(viewport.height / 2))
    await page.waitForTimeout(150)
    assert.equal(await page.locator('.reader-mobile-top.visible').count(), 0)
  }
  await frame.locator('#part-b-link').click()
  await frame.locator('#part-b-content').waitFor({ timeout: 10_000 })
  assert.equal(await frame.locator('#part-a').count(), 0)

  await frame.locator('#next-chapter').click()
  await frame.locator('h1').filter({ hasText: '第二章 EPUB 文档' }).waitFor({ timeout: 10_000 })
  if (viewport.width <= 750 && !await page.locator('.reader-mobile-top.visible').count()) {
    await page.mouse.click(Math.round(viewport.width / 2), Math.round(viewport.height / 2))
    await page.waitForTimeout(150)
  }
  await page.waitForFunction(() => document.body.textContent.includes('4 / 4'))

  await page.goBack({ waitUntil: 'domcontentloaded' })
  assert.equal(new URL(page.url()).pathname, '/', 'EPUB cross-chapter navigation must not consume browser back history')
  assert.equal(await page.locator('iframe.epub-iframe').count(), 0, 'browser back must return to the bookshelf instead of the previous EPUB chapter')
}

async function runViewport(browser, viewport, token, bookID) {
  await seedProgress(token, bookID)
  const context = await browser.newContext({ viewport })
  await context.addInitScript((value) => {
    localStorage.setItem('openreader_token', value)
  }, token)
  const page = await context.newPage()
  const failures = []
  const resourceResponses = []
  page.on('console', (message) => {
    if (message.type() === 'error') failures.push(message.text())
  })
  page.on('pageerror', error => failures.push(error.message))
  page.on('response', (response) => {
    if (response.url().includes('/api/epub-resource/')) {
      resourceResponses.push({ url: response.url(), status: response.status() })
    }
  })
  await page.goto(`${baseURL}/`, { waitUntil: 'networkidle' })
  const shelfBook = page.locator('.book-row')
  await shelfBook.waitFor({ timeout: 10_000 })
  await shelfBook.first().click()
  await page.waitForURL(new RegExp(`/books/${bookID}/read`), { waitUntil: 'domcontentloaded' })
  await assertFrameContract(page, viewport, resourceResponses)
  await page.goto(`${baseURL}/books/${bookID}/read?chapter=0`, { waitUntil: 'networkidle' })
  await assertCoverFrameContract(page, resourceResponses)
  assert.equal(resourceResponses.some(row => row.status === 401), false)
  assert.deepEqual(failures, [])
  await page.screenshot({
    path: join(outputDir, `openreader-epub-${viewport.width}x${viewport.height}.png`),
    fullPage: false,
  })
  await context.close()
}

async function main() {
  const fixture = createEPUB()
  try {
    const browser = await openSmokeBrowser()
    try {
      for (const viewport of smokeViewports()) {
        const imported = await registerAndImport(fixture.archive)
        await runViewport(browser, viewport, imported.token, imported.book.id)
      }
    } finally {
      await browser.close()
    }
    console.log('reader EPUB contract smoke passed')
  } finally {
    rmSync(fixture.root, { recursive: true, force: true })
  }
}

main().catch((error) => {
  console.error(error.stack || error.message)
  process.exit(1)
})

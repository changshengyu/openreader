#!/usr/bin/env node

import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetURL = (process.env.TARGET_URL || 'http://127.0.0.1:8080').replace(/\/$/, '')
const emptyCatalogHint = '未匹配到目录。你可以修改目录规则后重新解析，或保留空目录导入，之后再从书籍信息中刷新目录。'
const fixture = Buffer.from('== 第一章 ==\n这是正文内容。', 'utf8')
const runID = Date.now()

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

function crc32(input) {
  let crc = 0xffffffff
  for (const byte of input) {
    crc ^= byte
    for (let bit = 0; bit < 8; bit += 1) {
      crc = (crc >>> 1) ^ ((crc & 1) ? 0xedb88320 : 0)
    }
  }
  return (crc ^ 0xffffffff) >>> 0
}

function makeStoredZip(entries) {
  const localParts = []
  const centralParts = []
  let offset = 0
  for (const [name, rawContent] of entries) {
    const fileName = Buffer.from(name, 'utf8')
    const content = Buffer.from(rawContent, 'utf8')
    const checksum = crc32(content)
    const local = Buffer.alloc(30)
    local.writeUInt32LE(0x04034b50, 0)
    local.writeUInt16LE(20, 4)
    local.writeUInt16LE(0x0800, 6)
    local.writeUInt16LE(0, 8)
    local.writeUInt32LE(checksum, 14)
    local.writeUInt32LE(content.length, 18)
    local.writeUInt32LE(content.length, 22)
    local.writeUInt16LE(fileName.length, 26)
    localParts.push(local, fileName, content)

    const central = Buffer.alloc(46)
    central.writeUInt32LE(0x02014b50, 0)
    central.writeUInt16LE(20, 4)
    central.writeUInt16LE(20, 6)
    central.writeUInt16LE(0x0800, 8)
    central.writeUInt16LE(0, 10)
    central.writeUInt32LE(checksum, 16)
    central.writeUInt32LE(content.length, 20)
    central.writeUInt32LE(content.length, 24)
    central.writeUInt16LE(fileName.length, 28)
    central.writeUInt32LE(offset, 42)
    centralParts.push(central, fileName)
    offset += local.length + fileName.length + content.length
  }
  const centralDirectory = Buffer.concat(centralParts)
  const end = Buffer.alloc(22)
  end.writeUInt32LE(0x06054b50, 0)
  end.writeUInt16LE(entries.length, 8)
  end.writeUInt16LE(entries.length, 10)
  end.writeUInt32LE(centralDirectory.length, 12)
  end.writeUInt32LE(offset, 16)
  return Buffer.concat([...localParts, centralDirectory, end])
}

function makeEPUBFixture() {
  return makeStoredZip([
    ['META-INF/container.xml', `<?xml version="1.0"?>
<container><rootfiles><rootfile full-path="OPS/content.opf"/></rootfiles></container>`],
    ['OPS/content.opf', `<?xml version="1.0"?>
<package><metadata><title>导入快照 EPUB</title><creator>Smoke</creator></metadata><manifest>
  <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
  <item id="one" href="Text/one.xhtml" media-type="application/xhtml+xml"/>
  <item id="two" href="Text/two.xhtml" media-type="application/xhtml+xml"/>
</manifest><spine><itemref idref="one"/><itemref idref="two"/></spine></package>`],
    ['OPS/nav.xhtml', `<html><body><nav epub:type="toc"><ol>
  <li><a href="Text/one.xhtml">第一章</a></li>
  <li><a href="Text/two.xhtml">第二章</a></li>
</ol></nav></body></html>`],
    ['OPS/Text/one.xhtml', '<html><body><h1>第一章</h1><p>第一章正文。</p></body></html>'],
    ['OPS/Text/two.xhtml', '<html><body><h1>第二章</h1><p>第二章正文。</p></body></html>'],
  ])
}

async function signIn(page, suffix) {
  const response = await page.request.post(`${targetURL}/api/auth/register`, {
    data: { username: `txtsmoke${suffix.replace(/\D/g, '')}${runID}`, password: 'txt-smoke-password' },
  })
  assert(response.ok(), `registration failed: ${response.status()} ${await response.text()}`)
  const { token } = await response.json()
  assert(token, 'registration did not return a token')
  await page.evaluate(value => localStorage.setItem('openreader_token', value), token)
  await page.reload({ waitUntil: 'networkidle' })
}

async function openImport(page) {
  const action = page.getByText('导入书籍', { exact: true })
  const mobileTrigger = page.getByLabel('打开侧边栏', { exact: true })
  const actionIsInViewport = async () => {
    const box = await action.boundingBox()
    const viewport = page.viewportSize()
    return Boolean(box && viewport && box.x < viewport.width && box.x + box.width > 0)
  }
  if (!await actionIsInViewport() && await mobileTrigger.isVisible()) {
    await mobileTrigger.click()
    await action.waitFor({ state: 'visible' })
    await action.scrollIntoViewIfNeeded()
  }
  if (!await actionIsInViewport()) {
    throw new Error('import action is not visible')
  }
  await action.click()
  await page.getByText('导入本地书籍', { exact: true }).waitFor()
}

async function runViewport(browser, viewport) {
  const context = await browser.newContext({ viewport })
  const page = await context.newPage()
  const errors = []
  page.on('pageerror', error => errors.push(`pageerror: ${error.message}`))
  page.on('console', message => {
    if (message.type() === 'error') {
      errors.push(`console.error: ${message.text()}`)
    }
  })
  page.on('response', response => {
    if (response.status() >= 500 && response.url().includes('/api/')) {
      errors.push(`api ${response.status()}: ${response.url()}`)
    }
  })

  try {
    await page.goto(targetURL, { waitUntil: 'networkidle' })
    await signIn(page, `${viewport.width}-${viewport.height}`)
    await openImport(page)

    let releaseFirstPreview
    let markFirstPreviewSeen
    const firstPreviewSeen = new Promise(resolve => { markFirstPreviewSeen = resolve })
    const firstPreviewBlocked = new Promise(resolve => { releaseFirstPreview = resolve })
    await page.route('**/api/imports/books/preview', async route => {
      const body = route.request().postDataBuffer()
      if (body?.includes(Buffer.from('first-race.txt'))) {
        markFirstPreviewSeen()
        await firstPreviewBlocked
      }
      try {
        await route.continue()
      } catch {
        // The superseded browser request is expected to be aborted.
      }
    })
    const fileInput = page.locator('input[type="file"]')
    await fileInput.setInputFiles({
      name: 'first-race.txt',
      mimeType: 'text/plain',
      buffer: Buffer.from('第一章 旧请求\n旧正文', 'utf8'),
    })
    await firstPreviewSeen
    await fileInput.setInputFiles({
      name: 'second-race.txt',
      mimeType: 'text/plain',
      buffer: Buffer.from('第二章 新请求\n新正文', 'utf8'),
    })
    const titleInput = page.getByPlaceholder('书名（可选，不填则使用文件名）')
    await page.waitForFunction(() => {
      const input = document.querySelector('input[placeholder="书名（可选，不填则使用文件名）"]')
      return input?.value === 'second-race'
    })
    releaseFirstPreview()
    await page.waitForTimeout(150)
    assert(await titleInput.inputValue() === 'second-race', 'late file preview overwrote the latest selection')
    await page.unroute('**/api/imports/books/preview')

    await page.locator('input[type="file"]').setInputFiles({
      name: 'retry-rule.txt',
      mimeType: 'text/plain',
      buffer: fixture,
    })
    await page.getByText('已解析 1 章', { exact: true }).waitFor()

    const rule = page.getByPlaceholder('TXT目录规则（可选，留空使用默认规则，例如：^第.+章.*$）')
    await rule.fill('^不存在的目录$')
    await page.getByText('重新解析', { exact: true }).click()
    await page.locator('.direct-import-preview-empty').getByText(emptyCatalogHint, { exact: true }).waitFor()
    assert(await page.getByRole('button', { name: '导入', exact: true }).isEnabled(), 'upstream-compatible empty catalogue must remain confirmable')

    await rule.fill('^== .+ ==$')
    await page.getByText('重新解析', { exact: true }).click()
    await page.getByText('已解析 1 章', { exact: true }).waitFor()
    assert(await page.getByRole('button', { name: '导入', exact: true }).isEnabled(), 'import must re-enable after the valid staged retry')

    await page.getByRole('button', { name: '导入', exact: true }).click()
    await page.getByText('导入本地书籍', { exact: true }).waitFor({ state: 'hidden' })
    await page.getByText('retry-rule', { exact: true }).first().waitFor()

    await openImport(page)
    await page.locator('input[type="file"]').setInputFiles({
      name: 'snapshot.epub',
      mimeType: 'application/epub+zip',
      buffer: makeEPUBFixture(),
    })
    await page.getByText('已解析 2 章', { exact: true }).waitFor()
    assert(await page.getByPlaceholder('书名（可选，不填则使用文件名）').inputValue() === '导入快照 EPUB', 'EPUB metadata preview did not settle')
    await page.getByRole('button', { name: '导入', exact: true }).click()
    await page.getByText('导入本地书籍', { exact: true }).waitFor({ state: 'hidden' })
    await page.getByText('导入快照 EPUB', { exact: true }).first().waitFor()
    assert(errors.length === 0, errors.join('\n'))
    console.log(`${viewport.width}x${viewport.height}: latest-preview + TXT/EPUB staged-confirm UI ok`)
  } finally {
    await context.close()
  }
}
const browser = await openSmokeBrowser()
try {
  await runViewport(browser, { width: 1440, height: 900 })
  await runViewport(browser, { width: 390, height: 844 })
  await runViewport(browser, { width: 360, height: 800 })
} finally {
  await browser.close()
}

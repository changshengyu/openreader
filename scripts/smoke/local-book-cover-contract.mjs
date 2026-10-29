#!/usr/bin/env node

// Contract: docs/compat/local-book-cover-fixed-baseline-second-audit-p1-contract.md
// reader-dev shows an imported EPUB/UMD cover in the import dialog
// (Index.vue getCover(getBookCoverUrl(importBookInfo))), on the shelf and in
// book info. Real server and real files; no response mocks.

import { crc32, deflateSync } from 'node:zlib'

import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetURL = (process.env.TARGET_URL || 'http://127.0.0.1:8080').replace(/\/$/, '')
const runID = Date.now()

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

function pngChunk(type, data) {
  const length = Buffer.alloc(4)
  length.writeUInt32BE(data.length)
  const body = Buffer.concat([Buffer.from(type, 'ascii'), data])
  const checksum = Buffer.alloc(4)
  checksum.writeUInt32BE(crc32(body) >>> 0)
  return Buffer.concat([length, body, checksum])
}

// A decodable solid PNG; distinct sizes tell the two covers apart.
function solidPNG(width, height, [red, green, blue]) {
  const header = Buffer.alloc(13)
  header.writeUInt32BE(width, 0)
  header.writeUInt32BE(height, 4)
  header[8] = 8
  header[9] = 2
  const row = Buffer.alloc(1 + width * 3)
  for (let x = 0; x < width; x += 1) row.set([red, green, blue], 1 + x * 3)
  return Buffer.concat([
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    pngChunk('IHDR', header),
    pngChunk('IDAT', deflateSync(Buffer.concat(Array.from({ length: height }, () => row)))),
    pngChunk('IEND', Buffer.alloc(0)),
  ])
}

function storedZip(entries) {
  const locals = []
  const centrals = []
  let offset = 0
  for (const [name, data] of entries) {
    const nameBytes = Buffer.from(name, 'utf8')
    const checksum = crc32(data) >>> 0
    const local = Buffer.alloc(30)
    local.writeUInt32LE(0x04034b50, 0)
    local.writeUInt16LE(20, 4)
    local.writeUInt16LE(0x0800, 6)
    local.writeUInt16LE(0x21, 12)
    local.writeUInt32LE(checksum, 14)
    local.writeUInt32LE(data.length, 18)
    local.writeUInt32LE(data.length, 22)
    local.writeUInt16LE(nameBytes.length, 26)
    const central = Buffer.alloc(46)
    central.writeUInt32LE(0x02014b50, 0)
    central.writeUInt16LE(20, 4)
    central.writeUInt16LE(20, 6)
    central.writeUInt16LE(0x0800, 8)
    central.writeUInt16LE(0x21, 14)
    central.writeUInt32LE(checksum, 16)
    central.writeUInt32LE(data.length, 20)
    central.writeUInt32LE(data.length, 24)
    central.writeUInt16LE(nameBytes.length, 28)
    central.writeUInt32LE(offset, 42)
    locals.push(local, nameBytes, data)
    centrals.push(central, nameBytes)
    offset += local.length + nameBytes.length + data.length
  }
  const directory = Buffer.concat(centrals)
  const end = Buffer.alloc(22)
  end.writeUInt32LE(0x06054b50, 0)
  end.writeUInt16LE(entries.length, 8)
  end.writeUInt16LE(entries.length, 10)
  end.writeUInt32LE(directory.length, 12)
  end.writeUInt32LE(offset, 16)
  return Buffer.concat([...locals, directory, end])
}

// epublib PackageDocumentReader.readCover: <meta name="cover"> names the image.
function coverEPUB(title, author, cover) {
  const chapter = (heading, text) => Buffer.from(`<?xml version="1.0" encoding="utf-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><head><title>${heading}</title></head><body><h1>${heading}</h1><p>${text}</p></body></html>`)
  return storedZip([
    ['mimetype', Buffer.from('application/epub+zip')],
    ['META-INF/container.xml', Buffer.from(`<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`)],
    ['OEBPS/content.opf', Buffer.from(`<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0" unique-identifier="id">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>${title}</dc:title><dc:creator>${author}</dc:creator><dc:identifier id="id">cover-${runID}</dc:identifier><meta name="cover" content="cover-image"/></metadata>
  <manifest>
    <item id="c1" href="c1.xhtml" media-type="application/xhtml+xml"/>
    <item id="c2" href="c2.xhtml" media-type="application/xhtml+xml"/>
    <item id="cover-image" href="images/cover.png" media-type="image/png"/>
  </manifest>
  <spine><itemref idref="c1"/><itemref idref="c2"/></spine>
</package>`)],
    ['OEBPS/c1.xhtml', chapter('第一章', '封面书的第一段。')],
    ['OEBPS/c2.xhtml', chapter('第二章', '封面书的第二段。')],
    ['OEBPS/images/cover.png', cover],
  ])
}

// umdlib UmdReader: the additional section after segment 130 is the cover.
function coverUMD(title, author, cover, chapters) {
  const parts = [Buffer.from([0x89, 0x9b, 0x9a, 0xde])]
  const uint32 = value => {
    const bytes = Buffer.alloc(4)
    bytes.writeUInt32LE(value >>> 0)
    return bytes
  }
  const section = (type, flag, payload) => {
    const header = Buffer.from([0x23, 0, 0, flag, payload.length + 5])
    header.writeUInt16LE(type, 1)
    parts.push(header, payload)
  }
  const additional = (check, payload) => {
    const header = Buffer.alloc(9)
    header[0] = 0x24
    header.writeUInt32LE(check >>> 0, 1)
    header.writeUInt32LE(payload.length + 9, 5)
    parts.push(header, payload)
  }
  section(0x01, 0, Buffer.from([0x01, 0x11, 0x22]))
  section(0x02, 0, Buffer.from(title, 'utf16le'))
  section(0x03, 0, Buffer.from(author, 'utf16le'))
  section(0x82, 0x10, Buffer.concat([Buffer.from([0x01]), uint32(0x0badcafe)]))
  additional(0x0badcafe, cover)
  const offsets = []
  const titles = []
  const bodies = []
  let length = 0
  for (const chapter of chapters) {
    offsets.push(uint32(length))
    const body = Buffer.from(chapter.content, 'utf16le')
    bodies.push(body)
    length += body.length
    const encodedTitle = Buffer.from(chapter.title, 'utf16le')
    titles.push(Buffer.from([encodedTitle.length]), encodedTitle)
  }
  section(0x0b, 0, uint32(length))
  section(0x83, 0, uint32(0x11223344))
  additional(0x11223344, Buffer.concat(offsets))
  section(0x84, 1, uint32(0x55667788))
  additional(0x55667788, Buffer.concat(titles))
  additional(0x55667789, deflateSync(Buffer.concat(bodies)))
  section(0xf1, 0, Buffer.alloc(16))
  section(0x81, 1, Buffer.alloc(4))
  additional(0, uint32(0x55667789))
  return Buffer.concat(parts)
}

const fixtures = [
  {
    label: 'EPUB',
    title: '封面书EPUB',
    fileName: 'cover-book.epub',
    mimeType: 'application/epub+zip',
    width: 30,
    height: 40,
    prefix: '/api/epub-resource/',
    data: () => coverEPUB('封面书EPUB', '封面作者', solidPNG(30, 40, [200, 60, 60])),
  },
  {
    label: 'UMD',
    title: '封面书UMD',
    fileName: 'cover-book.umd',
    mimeType: 'application/octet-stream',
    width: 36,
    height: 48,
    prefix: '/api/umd-cover/',
    data: () => coverUMD('封面书UMD', '封面作者', solidPNG(36, 48, [60, 90, 200]), [
      { title: '第一章', content: '封面书的第一段。' },
      { title: '第二章', content: '封面书的第二段。' },
    ]),
  },
]

async function register(page, viewport) {
  const registration = await page.request.post(`${targetURL}/api/auth/register`, {
    data: { username: `cover${viewport.width}${runID}`, password: 'cover-import-password' },
  })
  assert(registration.ok(), `registration failed: ${registration.status()} ${await registration.text()}`)
  const { token } = await registration.json()
  assert(token, 'registration did not return a token')
  await page.evaluate(value => localStorage.setItem('openreader_token', value), token)
  await page.reload({ waitUntil: 'networkidle' })
}

async function openImport(page) {
  const action = page.getByText('导入书籍', { exact: true })
  const box = await action.boundingBox()
  const viewport = page.viewportSize()
  if (!(box && viewport && box.x < viewport.width && box.x + box.width > 0)) {
    await page.getByLabel('打开侧边栏', { exact: true }).click()
    const actionElement = await action.elementHandle()
    await page.waitForFunction(element => {
      const rect = element?.getBoundingClientRect()
      return Boolean(rect && rect.left < innerWidth && rect.right > 0)
    }, actionElement)
  }
  await action.click()
  await page.locator('.direct-import-picker-dialog').waitFor()
}

// Waits for BookCover to report a decoded image and returns what it shows.
async function loadedCover(container, label) {
  const image = container.locator('img.loaded').first()
  await image.waitFor({ timeout: 15000 }).catch(() => {
    throw new Error(`${label}: cover image did not load`)
  })
  return image.evaluate(element => ({
    src: element.getAttribute('src') || '',
    width: element.naturalWidth,
    height: element.naturalHeight,
  }))
}

async function runViewport(browser, viewport) {
  const context = await browser.newContext({
    viewport,
    isMobile: viewport.width <= 750,
    hasTouch: viewport.width <= 750,
  })
  const page = await context.newPage()
  const failures = []
  page.on('pageerror', error => failures.push(`pageerror: ${error.message}`))
  page.on('console', message => {
    if (message.type() === 'error' && !/WebSocket connection to .*\/ws\/sync/.test(message.text())) {
      failures.push(`console.error: ${message.text()}`)
    }
  })
  page.on('response', response => {
    if (response.status() >= 400 && /\/api\/(umd-cover|epub-resource)\//.test(response.url())) {
      failures.push(`cover ${response.status()}: ${new URL(response.url()).pathname.split('/').slice(0, 3).join('/')}`)
    }
    if (response.status() >= 500 && response.url().includes('/api/')) {
      failures.push(`api ${response.status()}: ${new URL(response.url()).pathname}`)
    }
  })

  try {
    await page.goto(targetURL, { waitUntil: 'networkidle' })
    await register(page, viewport)
    for (const fixture of fixtures) {
      const label = `${viewport.width} ${fixture.label}`
      await openImport(page)
      await page.locator('.direct-import-picker-dialog input[type="file"]').setInputFiles([
        { name: fixture.fileName, mimeType: fixture.mimeType, buffer: fixture.data() },
      ])
      const dialog = page.locator('.storage-import-single-dialog')
      await dialog.waitFor()
      const preview = await loadedCover(dialog.locator('.storage-import-cover'), `${label} import dialog`)
      assert(preview.src.startsWith('data:image/png;base64,'), `${label}: import dialog cover is not the preview data URL: ${preview.src.slice(0, 32)}`)
      assert(preview.width === fixture.width && preview.height === fixture.height, `${label}: import dialog shows a ${preview.width}x${preview.height} image`)
      const title = await dialog.locator('.storage-import-single-form .el-input input').first().inputValue()
      assert(title === fixture.title, `${label}: parsed title ${JSON.stringify(title)}`)
      await dialog.getByRole('button', { name: '确定导入', exact: true }).click()
      await dialog.waitFor({ state: 'hidden' })
      // The mobile sidebar opened for 导入书籍 still covers the shelf; a fresh
      // load closes it (desktop checks the shelf straight after the import).
      if (viewport.width <= 750) await page.reload({ waitUntil: 'networkidle' })

      const row = page.locator('article.book-row', { has: page.getByText(fixture.title, { exact: true }) }).first()
      await row.waitFor()
      const shelf = await loadedCover(row.locator('.cover-img'), `${label} shelf`)
      assert(shelf.src.startsWith(fixture.prefix), `${label}: shelf cover URL ${shelf.src.slice(0, 24)} is not ${fixture.prefix}`)
      assert(shelf.width === fixture.width && shelf.height === fixture.height, `${label}: shelf shows a ${shelf.width}x${shelf.height} image`)

      await row.locator('.cover-img').click()
      const info = await loadedCover(page.locator('.book-cover-shared.size-book-info'), `${label} book info`)
      assert(info.src.startsWith(fixture.prefix), `${label}: book info cover URL ${info.src.slice(0, 24)} is not ${fixture.prefix}`)
      assert(info.width === fixture.width && info.height === fixture.height, `${label}: book info shows a ${info.width}x${info.height} image`)
      await page.reload({ waitUntil: 'networkidle' })
    }
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)
    assert(overflow <= 1, `${viewport.width}: horizontal overflow ${overflow}px`)
    assert(failures.length === 0, `${viewport.width}: browser failures:\n${failures.join('\n')}`)
    console.log(`${viewport.width}x${viewport.height}: EPUB + UMD covers in import dialog, shelf and book info ok`)
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

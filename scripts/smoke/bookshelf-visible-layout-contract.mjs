#!/usr/bin/env node

import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetUrl = (process.env.TARGET_URL || 'http://127.0.0.1:4173').replace(/\/$/, '')

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

function json(data, status = 200) {
  return { status, contentType: 'application/json', body: JSON.stringify(data) }
}

function fakeToken() {
  const payload = Buffer.from(JSON.stringify({ userId: 1, sub: '1' })).toString('base64url')
  return `open.${payload}.reader`
}

function shelfBooks() {
  const checkedAt = new Date(Date.now() - (2 * 60 * 60 * 1000)).toISOString()
  return [
    {
      id: 1,
      title: 'A.B 精确标题',
      author: '甲作者',
      chapterCount: 12,
      categoryIds: [7],
      lastChapter: '第十二章',
      lastCheckTime: checkedAt,
      progress: { bookId: 1, chapterIndex: 1, chapterTitle: '第二章', percent: 0.2 },
      shelfOrderAt: '2026-08-02T05:00:00Z',
    },
    {
      id: 2,
      title: '文学分组第二本',
      author: '乙作者',
      chapterCount: 8,
      categoryIds: [7],
      lastChapter: '第八章',
      lastCheckTime: checkedAt,
      shelfOrderAt: '2026-08-02T04:00:00Z',
    },
    {
      id: 3,
      title: '本地书籍',
      author: '丙作者',
      chapterCount: 6,
      local: true,
      localPath: 'local/books/3.txt',
      shelfOrderAt: '2026-08-02T03:00:00Z',
    },
    {
      id: 4,
      title: '未分组书籍',
      author: '丁作者',
      chapterCount: 4,
      shelfOrderAt: '2026-08-02T02:00:00Z',
    },
    {
      id: 5,
      title: '普通书籍',
      author: '戊作者',
      chapterCount: 3,
      categoryIds: [8],
      shelfOrderAt: '2026-08-02T01:00:00Z',
    },
  ]
}

function bookGroups() {
  return [
    { key: 'builtin:all', name: '全部', groupName: '全部', show: true, sortOrder: 0 },
    { key: 'category:7', categoryId: 7, name: '文学', groupName: '文学', show: true, sortOrder: 1 },
    { key: 'builtin:local', name: '本地', groupName: '本地', show: true, sortOrder: 2 },
    { key: 'category:8', categoryId: 8, name: '隐藏分组', groupName: '隐藏分组', show: false, sortOrder: 3 },
    { key: 'category:9', categoryId: 9, name: '空分组', groupName: '空分组', show: true, sortOrder: 4 },
  ]
}

async function installApiMocks(page) {
  const state = {
    shelfReads: 0,
    shelfWrites: [],
  }
  const books = shelfBooks()

  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async route => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace(/^\/api/, '')
    const method = request.method()

    if (path === '/me') {
      return route.fulfill(json({ id: 1, username: 'bookshelf-layout-smoke', role: 'admin' }))
    }
    if (path === '/health') return route.fulfill(json({ version: 'smoke', commit: 'bookshelf-visible-layout' }))
    if (path === '/books' && method === 'GET') {
      state.shelfReads += 1
      await new Promise(resolve => setTimeout(resolve, state.shelfReads === 1 ? 650 : 500))
      return route.fulfill(json(books))
    }
    if (path === '/categories') {
      return route.fulfill(json([
        { id: 7, name: '文学', sortOrder: 1 },
        { id: 8, name: '隐藏分组', sortOrder: 2 },
        { id: 9, name: '空分组', sortOrder: 3 },
      ]))
    }
    if (path === '/book-groups') return route.fulfill(json(bookGroups()))
    if (path === '/sources') return route.fulfill(json([]))
    if (path === '/cache/stats') return route.fulfill(json({ files: 0, size: 0 }))
    if (path.startsWith('/cache')) return route.fulfill(json({ total: 0, books: 0, chapters: 0 }))

    if (path === '/settings/reader' && method === 'GET') {
      return route.fulfill(json({
        key: 'reader',
        value: { theme: 'parchment', mode: 'page', pageMode: 'auto' },
        updatedAt: '2026-08-02T00:00:00Z',
      }))
    }
    if (path === '/settings/shelf' && method === 'GET') {
      return route.fulfill(json({
        key: 'shelf',
        value: { view: 'list', layoutVersion: 2, groupKey: 'builtin:all' },
        updatedAt: '2026-08-02T00:00:00Z',
      }))
    }
    if (path === '/settings/search' && method === 'GET') {
      return route.fulfill(json({ key: 'search', value: {}, updatedAt: '2026-08-02T00:00:00Z' }))
    }
    if (path === '/settings/shelf' && method === 'PUT') {
      const payload = request.postDataJSON() || {}
      state.shelfWrites.push(payload)
      return route.fulfill(json({
        key: 'shelf',
        value: payload.value || {},
        updatedAt: '2026-08-02T00:00:01Z',
      }))
    }
    if (path.startsWith('/settings/') && method === 'PUT') {
      const key = path.split('/').at(-1)
      const payload = request.postDataJSON() || {}
      return route.fulfill(json({ key, value: payload.value || {}, updatedAt: '2026-08-02T00:00:01Z' }))
    }

    const detail = path.match(/^\/books\/(\d+)$/)
    if (detail) return route.fulfill(json(books.find(book => book.id === Number(detail[1])) || {}))
    const chapters = path.match(/^\/books\/(\d+)\/chapters$/)
    if (chapters) return route.fulfill(json([{ index: 0, title: '第一章' }]))
    const content = path.match(/^\/books\/(\d+)\/chapters\/0\/content$/)
    if (content) return route.fulfill(json({ title: '第一章', content: '浏览器书架契约正文。' }))
    if (/^\/books\/\d+\/bookmarks$/.test(path)) return route.fulfill(json([]))

    return route.fulfill(json({}))
  })

  return state
}

function roundedUnique(values) {
  return [...new Set(values.map(value => Math.round(value)))]
}

async function shelfGeometry(page) {
  return page.evaluate(() => {
    const title = document.querySelector('.shelf-title')
    const groups = document.querySelector('.book-group-wrapper')
    const list = document.querySelector('.book-list')
    const rows = [...document.querySelectorAll('.book-row')]
    const first = rows[0]
    const cover = first?.querySelector('.list-cover')
    const info = first?.querySelector('.info')
    const operation = first?.querySelector('.book-operation')
    const chips = [...groups.querySelectorAll('.group-chip')]
    const selectedChip = groups.querySelector('.group-chip.active')
    const titleStyle = getComputedStyle(title)
    const groupRect = groups.getBoundingClientRect()
    const firstStyle = getComputedStyle(first)
    const firstRect = first.getBoundingClientRect()
    const coverRect = cover.getBoundingClientRect()
    const infoStyle = getComputedStyle(info)
    const operationStyle = getComputedStyle(operation)
    return {
      titlePaddingLeft: Number.parseFloat(titleStyle.paddingLeft),
      titlePaddingRight: Number.parseFloat(titleStyle.paddingRight),
      groupLeft: groupRect.left,
      groupRightInset: innerWidth - groupRect.right,
      groupWidth: groupRect.width,
      groupChipWidthSum: chips.reduce((sum, chip) => sum + chip.getBoundingClientRect().width, 0),
      groupUnderlineHeight: Number.parseFloat(getComputedStyle(selectedChip, '::after').height),
      listDisplay: getComputedStyle(list).display,
      rowCssWidth: Number.parseFloat(firstStyle.width),
      rowOuterWidth: firstRect.width,
      rowPaddingLeft: Number.parseFloat(firstStyle.paddingLeft),
      rowPaddingRight: Number.parseFloat(firstStyle.paddingRight),
      rowBoxSizing: firstStyle.boxSizing,
      rowTops: rows.map(row => row.getBoundingClientRect().top),
      coverWidth: coverRect.width,
      coverHeight: coverRect.height,
      infoHeight: info.getBoundingClientRect().height,
      infoMarginLeft: Number.parseFloat(infoStyle.marginLeft),
      operationTop: Number.parseFloat(operationStyle.top),
      operationRight: Number.parseFloat(operationStyle.right),
      visibleContent: rows.map(row => {
        const rowCover = row.querySelector('.cover-img').getBoundingClientRect()
        const rowInfo = row.querySelector('.info').getBoundingClientRect()
        const rowRect = row.getBoundingClientRect()
        return { top: rowRect.top, coverLeft: rowCover.left, infoRight: rowInfo.right }
      }),
      scrollWidth: document.documentElement.scrollWidth,
      innerWidth,
    }
  })
}

async function assertGeometry(page, viewport) {
  const geometry = await shelfGeometry(page)
  const uniqueTops = roundedUnique(geometry.rowTops)
  assert(geometry.listDisplay === (viewport.width <= 750 ? 'flex' : 'grid'), `${viewport.width}: unexpected shelf display ${geometry.listDisplay}`)
  assert(Math.abs(geometry.coverWidth - 84) < 0.5, `${viewport.width}: cover width ${geometry.coverWidth}`)
  assert(Math.abs(geometry.coverHeight - 112) < 0.5, `${viewport.width}: cover height ${geometry.coverHeight}`)
  assert(Math.abs(geometry.infoHeight - 112) < 0.5, `${viewport.width}: info height ${geometry.infoHeight}`)
  assert(Math.abs(geometry.infoMarginLeft - 20) < 0.5, `${viewport.width}: info margin ${geometry.infoMarginLeft}`)
  assert(Math.abs(geometry.operationTop) < 0.5, `${viewport.width}: operation top ${geometry.operationTop}`)
  assert(Math.abs(geometry.operationRight - 5) < 0.5, `${viewport.width}: operation right ${geometry.operationRight}`)
  assert(Math.abs(geometry.groupChipWidthSum - geometry.groupWidth) < 1, `${viewport.width}: group tabs do not stretch ${geometry.groupChipWidthSum}/${geometry.groupWidth}`)
  assert(Math.abs(geometry.groupUnderlineHeight - 2) < 0.5, `${viewport.width}: selected underline ${geometry.groupUnderlineHeight}`)
  assert(geometry.scrollWidth <= geometry.innerWidth + 1, `${viewport.width}: horizontal overflow ${geometry.scrollWidth} > ${geometry.innerWidth}`)

  const visibleRows = [...geometry.visibleContent].sort((a, b) => a.top - b.top || a.coverLeft - b.coverLeft)
  for (let index = 1; index < visibleRows.length; index += 1) {
    const previous = visibleRows[index - 1]
    const current = visibleRows[index]
    if (Math.abs(previous.top - current.top) >= 1) continue
    assert(previous.infoRight <= current.coverLeft + 0.5, `${viewport.width}: visible card content overlaps ${previous.infoRight} > ${current.coverLeft}`)
  }

  if (viewport.width === 1440) {
    assert(uniqueTops.length === 3, `1440: expected two fixed tracks, got row tops ${JSON.stringify(geometry.rowTops)}`)
    assert(Math.abs(geometry.rowCssWidth - 360) < 0.5, `1440: card CSS width ${geometry.rowCssWidth}`)
    assert(Math.abs(geometry.rowOuterWidth - 408) < 0.5, `1440: card outer width ${geometry.rowOuterWidth}`)
    assert(geometry.rowBoxSizing === 'content-box', `1440: desktop box sizing ${geometry.rowBoxSizing}`)
    assert(Math.abs(geometry.rowPaddingLeft - 24) < 0.5, `1440: card left padding ${geometry.rowPaddingLeft}`)
  } else if (viewport.width === 1024) {
    assert(uniqueTops.length === 5, `1024: expected one fixed track, got row tops ${JSON.stringify(geometry.rowTops)}`)
  } else {
    assert(uniqueTops.length === 5, `${viewport.width}: expected one mobile row per book`)
    assert(geometry.rowBoxSizing === 'border-box', `${viewport.width}: mobile box sizing ${geometry.rowBoxSizing}`)
    assert(Math.abs(geometry.rowOuterWidth - viewport.width) < 0.5, `${viewport.width}: mobile row width ${geometry.rowOuterWidth}`)
    assert(Math.abs(geometry.rowPaddingLeft - 20) < 0.5, `${viewport.width}: row left padding ${geometry.rowPaddingLeft}`)
    assert(Math.abs(geometry.rowPaddingRight - 20) < 0.5, `${viewport.width}: row right padding ${geometry.rowPaddingRight}`)
    assert(Math.abs(geometry.titlePaddingLeft - 24) < 0.5, `${viewport.width}: title left inset ${geometry.titlePaddingLeft}`)
    assert(Math.abs(geometry.titlePaddingRight - 24) < 0.5, `${viewport.width}: title right inset ${geometry.titlePaddingRight}`)
    assert(Math.abs(geometry.groupLeft - 24) < 0.5, `${viewport.width}: group left inset ${geometry.groupLeft}`)
    assert(Math.abs(geometry.groupRightInset - 24) < 0.5, `${viewport.width}: group right inset ${geometry.groupRightInset}`)
  }
}

async function assertMetadata(page, viewport) {
  const row = page.locator('.book-row').filter({ hasText: 'A.B 精确标题' })
  const order = await row.evaluate(row => (
    [...row.querySelector('.info').children].map(node => node.className)
  ))
  assert(order[0] === 'book-operation', `${viewport.width}: operation is not first metadata child`)
  assert(order[1].includes('name'), `${viewport.width}: name is not second metadata child`)
  assert(order[2] === 'sub', `${viewport.width}: author/chapter sub row missing`)
  assert(order[3] === 'dur-chapter', `${viewport.width}: read chapter row missing`)
  assert(order[4] === 'last-chapter', `${viewport.width}: latest chapter row missing`)
  const sub = await row.locator('.sub').innerText()
  assert(sub.includes('甲作者') && sub.includes('共12章'), `${viewport.width}: metadata sub row ${sub}`)
}

async function assertFilteringAndPreference(page, state, viewport) {
  const title = page.locator('.shelf-title strong')
  assert((await title.textContent())?.includes('(5)'), `${viewport.width}: initial displayed count is not 5`)
  assert(await page.getByRole('tab', { name: '隐藏分组' }).count() === 0, `${viewport.width}: hidden group is visible`)
  assert(await page.getByRole('tab', { name: '空分组' }).count() === 0, `${viewport.width}: empty group is visible`)

  await page.getByRole('tab', { name: '文学' }).click()
  await page.waitForFunction(() => document.querySelector('.shelf-title strong')?.textContent?.includes('(2)'))
  await page.waitForTimeout(800)
  const write = state.shelfWrites.at(-1)?.value
  assert(write?.view === 'grid', `${viewport.width}: old list preference was not migrated: ${JSON.stringify(write)}`)
  assert(write?.layoutVersion === 3, `${viewport.width}: shelf layout version was not migrated: ${JSON.stringify(write)}`)
  assert(write?.groupKey === 'category:7', `${viewport.width}: group token was not preserved: ${JSON.stringify(write)}`)
  assert(await page.locator('.unread-num-badge').count() === 2, `${viewport.width}: non-edit badges are missing`)

  await page.locator('.shelf-title').getByRole('button', { name: '编辑', exact: true }).click()
  assert(await page.locator('.unread-num-badge').count() === 0, `${viewport.width}: badge remains visible while editing`)
  assert(await page.locator('.book-row .operation-icon').count() === 4, `${viewport.width}: edit/delete operations are incomplete`)
  const search = page.getByPlaceholder('搜索书名或作者')
  await search.fill('A.B')
  await page.waitForFunction(() => document.querySelector('.shelf-title strong')?.textContent?.includes('(1)'))
  assert(await page.getByText('A.B 精确标题', { exact: true }).count() === 1, `${viewport.width}: exact punctuation search failed`)
  await search.fill('A B')
  await page.waitForFunction(() => document.querySelectorAll('.book-row').length === 0)
  assert((await title.textContent())?.includes('(0)'), `${viewport.width}: blank result count is not 0`)
  assert(await page.locator('.shelf-page .el-empty').count() === 0, `${viewport.width}: rewritten empty state is visible`)
  assert((await page.locator('.book-list').innerText()).trim() === '', `${viewport.width}: blank result wrapper is not empty`)

  await page.locator('.shelf-title').getByRole('button', { name: '取消', exact: true }).click()
  await page.waitForFunction(() => document.querySelectorAll('.book-row').length === 2)
  await page.locator('.shelf-title').getByRole('button', { name: '编辑', exact: true }).click()
  assert(await search.inputValue() === 'A B', `${viewport.width}: edit search was unexpectedly reset`)
  await search.fill('')
  await page.locator('.shelf-title').getByRole('button', { name: '取消', exact: true }).click()
  await page.getByRole('tab', { name: '全部' }).click()
  await page.waitForFunction(() => document.querySelectorAll('.book-row').length === 5)
}

async function assertLoadingAndClickOwnership(page, state, viewport) {
  const refresh = page.locator('.shelf-title').getByRole('button', { name: '刷新', exact: true })
  await refresh.click()
  await page.getByText('正在刷新书籍信息', { exact: true }).waitFor({ state: 'visible', timeout: 3000 })
  await page.waitForFunction(() => document.querySelectorAll('.book-row').length === 5)
  await page.getByText('正在刷新书籍信息', { exact: true }).waitFor({ state: 'hidden', timeout: 3000 })
  assert(state.shelfReads >= 2, `${viewport.width}: refresh did not reload the shelf`)

  const shelfUrl = page.url()
  await page.locator('.book-row .cover-img').first().click()
  await page.waitForSelector('.book-info-dialog', { timeout: 5000 })
  assert(page.url() === shelfUrl, `${viewport.width}: cover click navigated away from shelf`)
  await page.locator('.book-info-dialog .el-dialog__headerbtn').click()
  await page.locator('.book-info-dialog').waitFor({ state: 'hidden', timeout: 5000 })

  await page.locator('.book-row .info').first().click()
  await page.waitForURL(/\/books\/\d+\/read/, { timeout: 5000 })
  await page.goto(targetUrl, { waitUntil: 'domcontentloaded' })
  await page.waitForSelector('.book-row', { timeout: 5000 })
}

async function assertNightSurface(page, viewport) {
  await page.locator('.theme-toggle').evaluate(node => node.click())
  await page.waitForFunction(() => document.documentElement.classList.contains('dark-reader'))
  await page.locator('.shelf-title').getByRole('button', { name: '编辑', exact: true }).click()
  const colors = await page.evaluate(() => ({
    shelf: getComputedStyle(document.querySelector('.shelf-page')).backgroundColor,
    title: getComputedStyle(document.querySelector('.shelf-title')).color,
    name: getComputedStyle(document.querySelector('.book-row .name')).color,
    sub: getComputedStyle(document.querySelector('.book-row .sub')).color,
    input: getComputedStyle(document.querySelector('.shelf-search-wrapper .el-input__wrapper')).backgroundColor,
  }))
  assert(colors.shelf === 'rgb(34, 34, 34)', `${viewport.width}: night shelf ${colors.shelf}`)
  assert(colors.title === 'rgb(187, 187, 187)', `${viewport.width}: night title ${colors.title}`)
  assert(colors.name === 'rgb(187, 187, 187)', `${viewport.width}: night name ${colors.name}`)
  assert(colors.sub === 'rgb(107, 107, 107)', `${viewport.width}: night sub ${colors.sub}`)
  assert(colors.input === 'rgb(68, 68, 68)', `${viewport.width}: night input ${colors.input}`)
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
    if (message.type() !== 'error') return
    if (/WebSocket connection to .*\/ws\/sync/.test(message.text())) return
    failures.push(`console.error: ${message.text()}`)
  })
  await page.addInitScript(token => localStorage.setItem('openreader_token', token), fakeToken())
  const state = await installApiMocks(page)

  await page.goto(targetUrl, { waitUntil: 'domcontentloaded' })
  await page.getByText('正在获取书籍信息', { exact: true }).waitFor({ state: 'visible', timeout: 3000 })
  assert(
    await page.locator('.shelf-page .shelf-skeleton, .shelf-page .el-skeleton, .shelf-page .el-empty').count() === 0,
    `${viewport.width}: synthetic loading or empty surface is visible`,
  )
  await page.waitForSelector('.book-row', { timeout: 5000 })
  await page.getByText('正在获取书籍信息', { exact: true }).waitFor({ state: 'hidden', timeout: 3000 })

  await assertGeometry(page, viewport)
  await assertMetadata(page, viewport)
  await assertFilteringAndPreference(page, state, viewport)
  if (viewport.width === 1440) await assertLoadingAndClickOwnership(page, state, viewport)
  await assertNightSurface(page, viewport)
  assert(failures.length === 0, failures.join('\n'))

  await context.close()
  return `${viewport.width}x${viewport.height}`
}

async function run() {
  const browser = await openSmokeBrowser()
  try {
    const results = []
    for (const viewport of [
      { width: 1440, height: 900 },
      { width: 1024, height: 1366 },
      { width: 390, height: 844 },
      { width: 360, height: 800 },
    ]) {
      results.push(await runViewport(browser, viewport))
    }
    console.log(`bookshelf-visible-layout: ok ${results.join(', ')} fixedGrid=true oldListMigrated=true loadingBlankNight=true`)
  } finally {
    await browser.close()
  }
}

run().catch(error => {
  console.error(error.stack || error.message)
  process.exit(1)
})

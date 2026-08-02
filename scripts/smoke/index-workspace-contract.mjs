#!/usr/bin/env node

import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetUrl = process.env.TARGET_URL || 'http://127.0.0.1:5173'

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

function json(data, status = 200) {
  return {
    status,
    contentType: 'application/json',
    body: JSON.stringify(data),
  }
}

function fakeToken() {
  const payload = Buffer.from(JSON.stringify({ userId: 1, sub: '1' })).toString('base64url')
  return `open.${payload}.reader`
}

function remoteBook(title = '工作台搜索结果', sourceId = 1) {
  return {
    // Search payload IDs are source-owned and may collide with a persisted
    // shelf row. BookInfo identity must remain URL-authoritative.
    id: 1,
    title,
    author: 'OpenReader',
    url: `https://source.example/${encodeURIComponent(title)}`,
    bookUrl: `https://source.example/${encodeURIComponent(title)}`,
    sourceId,
    sourceName: `工作台测试书源${sourceId}`,
    chapterCount: 3,
    lastCheckTime: Date.now() - (60 * 60 * 1000),
    latestChapter: '第一章',
    intro: '用于验证 Index 工作台搜索、书籍信息和阅读流程。',
  }
}

async function installApiMocks(page) {
  let shelfBooks = [{
    id: 1,
    title: '书架测试书',
    author: 'OpenReader',
    chapterCount: 1,
    lastChapter: '更新章节',
    lastCheckTime: Date.now() - (2 * 60 * 60 * 1000),
    shelfOrderAt: new Date().toISOString(),
    updatedAt: new Date().toISOString(),
  }]
  let remoteCreateCount = 0
  const searchRequests = []
  await page.exposeFunction('__workspaceRemoteCreateCount', () => remoteCreateCount)
  await page.exposeFunction('__workspaceSearchRequests', () => searchRequests)
  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace(/^\/api/, '')
    const method = request.method()

    if (path === '/me') return route.fulfill(json({ id: 1, username: 'workspace-smoke', role: 'admin' }))
    if (path === '/health') return route.fulfill(json({ version: 'smoke', commit: 'index-workspace' }))
    if (path === '/settings/reader' && method === 'GET') {
      return route.fulfill(json({ key: 'reader', value: { theme: 'parchment', mode: 'page', pageMode: 'auto' } }))
    }
    if (path === '/settings/reader' && method === 'PUT') return route.fulfill(json({ key: 'reader', value: {} }))
    if (path === '/settings/preferences') return route.fulfill(json({ key: 'preferences', value: {} }))
    if (path === '/categories') return route.fulfill(json([]))
    if (path === '/sources') return route.fulfill(json([
      { id: 1, name: '工作台测试书源一', enabled: true, group: '测试' },
      { id: 2, name: '工作台测试书源二', enabled: true, group: '其他' },
    ]))
    if (path === '/explore/sources') {
      return route.fulfill(json(Array.from({ length: 8 }, (_, index) => ({
        id: index + 1,
        name: `工作台探索书源${index + 1}`,
        enabled: true,
        group: index === 0 ? '测试' : (index === 1 ? '都市' : ''),
        exploreGroups: [[{
          name: index === 0 ? '热门' : `入口${index + 1}`,
          url: `https://source.example/explore/${index + 1}`,
        }]],
      }))))
    }
    if (/^\/explore\/\d+$/.test(path)) {
      const sourceId = Number(path.split('/').at(-1)) || 1
      const page = Number(url.searchParams.get('page') || 1)
      return route.fulfill(json({
        items: page > 1
          ? [remoteBook('工作台探索重复结果', sourceId), remoteBook('工作台探索续页结果', sourceId)]
          : [remoteBook('工作台探索重复结果', sourceId)],
        page,
        hasMore: page === 1,
      }))
    }
    if (path === '/search' && method === 'POST') {
      const body = request.postDataJSON() || {}
      searchRequests.push(body)
      if (body.keyword === '陈旧请求') {
        await new Promise(resolve => setTimeout(resolve, 500))
        return route.fulfill(json({ list: [remoteBook('陈旧结果')], page: 1, lastIndex: 1, hasMore: false }))
      }
      const sourceIDs = Array.isArray(body.sourceIds) ? body.sourceIds : []
      if (sourceIDs.length > 1) {
        const hasStarted = Number(body.lastIndex) >= 0
        return route.fulfill(json({
          list: hasStarted
            ? [remoteBook('交错 A1', 1), remoteBook('交错 B2', 2)]
            : [remoteBook('交错 A1', 1), remoteBook('交错 B1', 2), remoteBook('交错 A2', 1)],
          page: 1,
          lastIndex: hasStarted ? 1 : 0,
          hasMore: !hasStarted,
        }))
      }
      const page = Number(body.page || 1)
      return route.fulfill(json({
        list: page > 1
          ? [remoteBook('单源重复结果'), remoteBook('单源续页结果')]
          : [remoteBook('单源重复结果')],
        page,
        lastIndex: -1,
        hasMore: page === 1,
      }))
    }
    if (path === '/books/remote' && method === 'POST') {
      remoteCreateCount += 1
      const payload = request.postDataJSON() || {}
      const created = {
        id: 99,
        title: payload.title || '已加入的工作台书籍',
        author: payload.author || 'OpenReader',
        sourceId: payload.sourceId || 1,
        sourceName: payload.sourceName || '工作台测试书源',
        url: payload.bookUrl,
        bookUrl: payload.bookUrl,
        chapterCount: 1,
        categoryIds: payload.categoryIds || [],
      }
      shelfBooks = [created, ...shelfBooks]
      return route.fulfill(json(created))
    }
    if (path === '/books') return route.fulfill(json(shelfBooks))
    if (path === '/books/99') return route.fulfill(json(shelfBooks.find(book => book.id === 99) || {}))
    if (path === '/books/99/chapters') return route.fulfill(json([{ index: 0, title: '第一章' }]))
    if (path === '/books/99/chapters/0/content') return route.fulfill(json({ title: '第一章', content: '工作台阅读验证内容。' }))
    if (path === '/books/99/bookmarks') return route.fulfill(json([]))
    if (path === '/books/1') return route.fulfill(json(shelfBooks.find(book => book.id === 1) || {}))
    if (path.startsWith('/cache')) return route.fulfill(json({ total: 0, books: 0, chapters: 0 }))
    return route.fulfill(json({}))
  })
}

async function assertNoHorizontalOverflow(page, name) {
  const geometry = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    innerWidth: window.innerWidth,
  }))
  assert(geometry.scrollWidth <= geometry.innerWidth + 1, `${name}: horizontal overflow ${geometry.scrollWidth} > ${geometry.innerWidth}`)
}

async function openMobileNavigation(page, viewport) {
  if (viewport.width > 750) return
  if (await page.locator('.app-shell').evaluate(node => node.classList.contains('mobile-nav-open'))) return
  const trigger = page.locator('.mobile-menu-trigger')
  if (await trigger.count()) {
    await trigger.click()
  } else {
    const shell = page.locator('.app-shell')
    await shell.dispatchEvent('touchstart', {
      touches: [{ identifier: 1, clientX: 30, clientY: 180 }],
    })
    await shell.dispatchEvent('touchmove', {
      touches: [{ identifier: 1, clientX: 250, clientY: 182 }],
    })
    await shell.dispatchEvent('touchend', {
      touches: [],
      changedTouches: [{ identifier: 1, clientX: 250, clientY: 182 }],
    })
  }
  await page.waitForFunction(() => {
    const node = document.querySelector('.app-sidebar')
    return node && Math.abs(Number.parseFloat(getComputedStyle(node).marginLeft)) < 0.5
  })
}

async function closeMobileNavigation(page, viewport) {
  if (viewport.width > 750) return
  await page.locator('.app-workspace').evaluate(node => node.click())
  await page.waitForFunction(() => {
    const node = document.querySelector('.app-sidebar')
    return node && Number.parseFloat(getComputedStyle(node).marginLeft) < -1
  })
}

async function selectElementOption(page, select, label) {
  await select.click()
  const option = page.getByRole('option', { name: label, exact: true }).last()
  await option.waitFor({ state: 'visible', timeout: 10_000 })
  await option.click()
}

async function assertSidebarSearchSurface(page, viewport) {
  const settings = page.locator('.app-search-setting')
  const selects = settings.locator('.el-select')
  assert(await selects.count() === 3, `${viewport.width}: multi-source mode must expose mode, group, and concurrency`)

  await selects.nth(0).click()
  const singleModeOption = page.getByRole('option', { name: '单源搜索', exact: true }).last()
  await singleModeOption.waitFor({ state: 'visible', timeout: 10_000 })
  const modeOptions = (await page.locator('[role="option"]:visible').allTextContents())
    .map(value => value.trim())
  assert(JSON.stringify(modeOptions) === JSON.stringify(['单源搜索', '多源搜索(过滤书名/作者名)']), `${viewport.width}: visible search modes differ: ${modeOptions.join(' | ')}`)
  await page.keyboard.press('Escape')
  await singleModeOption.waitFor({ state: 'hidden', timeout: 10_000 })

  await selectElementOption(page, selects.nth(0), '单源搜索')
  await page.waitForFunction(() => document.querySelectorAll('.app-search-setting .el-select').length === 2)
  assert(await settings.getByText('全部分组', { exact: false }).count() === 0, `${viewport.width}: single-source mode must hide group selection`)

  await selectElementOption(page, settings.locator('.el-select').nth(0), '多源搜索(过滤书名/作者名)')
  await page.waitForFunction(() => document.querySelectorAll('.app-search-setting .el-select').length === 3)
  const groupSelect = settings.locator('.el-select').nth(1)
  await groupSelect.click()
  await page.getByRole('option', { name: '全部分组 (2)', exact: true }).last().waitFor({ state: 'visible', timeout: 10_000 })
  const groupOptions = (await page.locator('[role="option"]:visible').allTextContents())
    .map(value => value.trim())
    .filter(value => value === '全部分组 (2)' || value === '测试 (1)' || value === '其他 (1)')
  assert(JSON.stringify(groupOptions) === JSON.stringify(['全部分组 (2)', '测试 (1)', '其他 (1)']), `${viewport.width}: source group order/count differs: ${groupOptions.join(' | ')}`)
  await page.keyboard.press('Escape')
}

async function resultGeometry(page, selector) {
  return page.locator(selector).evaluate(node => {
    const card = node.querySelector('.book-row')
    const cover = node.querySelector('.cover-img')
    const info = node.querySelector('.info')
    const style = getComputedStyle(node)
    const cardStyle = card ? getComputedStyle(card) : null
    return {
      display: style.display,
      columns: style.gridTemplateColumns,
      cardWidth: card?.getBoundingClientRect().width || 0,
      coverWidth: cover?.getBoundingClientRect().width || 0,
      infoHeight: info?.getBoundingClientRect().height || 0,
      paddingLeft: Number.parseFloat(cardStyle?.paddingLeft || '0'),
      paddingRight: Number.parseFloat(cardStyle?.paddingRight || '0'),
    }
  })
}

async function closeShelfExploreChooser(page, viewport) {
  const chooser = page.locator('.explore-workspace-popover:visible')
  if (viewport.width <= 750) {
    await chooser.getByRole('button', { name: '关闭书海', exact: true }).click()
  } else {
    await page.locator('.shelf-title .title-actions').getByRole('button', { name: '书海', exact: true }).click()
  }
  await page.locator('.explore-workspace-popover').waitFor({ state: 'hidden', timeout: 10_000 })
}

async function assertExploreChooserContract(page, viewport) {
  const chooser = page.locator('.explore-workspace-popover:visible')
  const geometry = await chooser.evaluate(node => {
    const rect = node.getBoundingClientRect()
    return { left: rect.left, top: rect.top, width: rect.width, height: rect.height }
  })
  assert(await page.locator('.explore-popover-backdrop').count() === 0, `${viewport.width}: Explore chooser must not add a modal backdrop`)
  assert(Math.abs(geometry.top) <= 1, `${viewport.width}: Explore chooser must be fixed to top 0`)
  if (viewport.width <= 750) {
    assert(Math.abs(geometry.left) <= 1 && Math.abs(geometry.width - viewport.width) <= 1, `${viewport.width}: mobile Explore chooser must span 100vw from the origin`)
    assert(geometry.height < viewport.height - 40, `${viewport.width}: mobile Explore chooser must keep content height instead of becoming fullscreen (${geometry.height})`)
    assert(await chooser.getByRole('button', { name: '关闭书海', exact: true }).count() === 1, `${viewport.width}: mobile Explore chooser must expose its close action`)
  } else {
    assert(Math.abs(geometry.width - 600) <= 1, `${viewport.width}: desktop Explore chooser must be 600px (${geometry.width})`)
    assert(await chooser.getByRole('button', { name: '关闭书海', exact: true }).count() === 0, `${viewport.width}: desktop Explore chooser must not add an internal close action`)
  }

  const groups = (await chooser.locator('.explore-source-groups button').allTextContents()).map(value => value.trim())
  assert(JSON.stringify(groups) === JSON.stringify(['测试', '都市', '未分组']), `${viewport.width}: Explore groups must keep first-seen order plus ungrouped: ${groups.join(' | ')}`)
  assert(await chooser.locator('.el-collapse-item__header').count() === 8, `${viewport.width}: unfiltered chooser must keep source order`)
  await chooser.locator('.el-collapse-item__header').nth(0).click()
  await chooser.locator('.el-collapse-item__header').nth(1).click()
  assert(await chooser.locator('.el-collapse-item.is-active').count() === 2, `${viewport.width}: Explore collapse must allow multiple open sources`)
  await chooser.locator('.explore-source-list').evaluate(node => { node.scrollTop = 72 })
  return chooser.locator('.explore-source-list').evaluate(node => node.scrollTop)
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
  await page.addInitScript(token => window.localStorage.setItem('openreader_token', token), fakeToken())
  await installApiMocks(page)

  const root = targetUrl.replace(/\/$/, '')
  await page.goto(root, { waitUntil: 'networkidle' })
  await page.waitForSelector('.shelf-page .book-row', { timeout: 10000 })
  const shelfGeometry = await resultGeometry(page, '.shelf-page .book-list')
  const shelfCardText = await page.locator('.shelf-page .book-row').first().innerText()
  assert(shelfCardText.includes('2小时前：更新章节'), `${viewport.width}: latest chapter must use lastCheckTime instead of recent shelfOrderAt/updatedAt: ${shelfCardText}`)
  if (process.env.BOOKSHELF_TIME_ONLY === '1') {
    assert(failures.length === 0, failures.join('\n'))
    await context.close()
    return `${viewport.width}x${viewport.height}`
  }
  const shelfRoute = await page.url()
  await page.locator('.shelf-page .book-row .list-cover').first().click()
  await page.waitForSelector('.book-info-dialog', { timeout: 10000 })
  const shelfBookInfo = page.locator('.book-info-dialog')
  assert(await shelfBookInfo.getByText('书架测试书', { exact: true }).count() === 1, `${viewport.width}: shelf cover must open the shared BookInfo record`)
  assert(await shelfBookInfo.getByText('加入书架', { exact: true }).count() === 0, `${viewport.width}: shelf BookInfo must not expose an unshelved add action`)
  assert(await shelfBookInfo.getByText('开始阅读', { exact: true }).count() === 0, `${viewport.width}: shelf BookInfo must not expose a second read action`)
  await shelfBookInfo.locator('.el-dialog__headerbtn').click()
  await shelfBookInfo.waitFor({ state: 'hidden', timeout: 10000 })
  assert(await page.url() === shelfRoute, `${viewport.width}: closing shelf BookInfo must stay on the shelf route`)

  await page.evaluate(() => {
    const node = document.querySelector('.explore-workspace-popover')
    window.__exploreVisibilityTrace = []
    if (!node) return
    const record = () => window.__exploreVisibilityTrace.push({
      at: performance.now(),
      display: getComputedStyle(node).display,
      style: node.getAttribute('style') || '',
    })
    record()
    new MutationObserver(record).observe(node, { attributes: true, attributeFilter: ['style', 'class'] })
  })
  await page.locator('.shelf-title .title-actions').getByRole('button', { name: '书海', exact: true }).click()
  try {
    await page.waitForSelector('.explore-workspace-popover', { timeout: 10000 })
  } catch (error) {
    const diagnostics = await page.evaluate(() => ({
      trace: window.__exploreVisibilityTrace,
      chooserStyle: document.querySelector('.explore-workspace-popover')?.getAttribute('style') || '',
      titleActions: [...document.querySelectorAll('.shelf-title .title-actions button')].map(node => node.textContent?.trim()),
    }))
    throw new Error(`${error.message}\nExplore diagnostics: ${JSON.stringify(diagnostics)}`)
  }
  const shelfBeforeExploreSelection = await page.locator('.shelf-page').count()
  assert(shelfBeforeExploreSelection === 1, `${viewport.width}: opening 书海 must keep the shelf body visible before selecting an entry`)
  const chooserScroll = await assertExploreChooserContract(page, viewport)
  assert(chooserScroll > 0, `${viewport.width}: Explore source list must own a restorable scroll offset`)
  await closeShelfExploreChooser(page, viewport)
  await page.locator('.shelf-title .title-actions').getByRole('button', { name: '书海', exact: true }).click()
  const reopenedChooser = page.locator('.explore-workspace-popover:visible')
  await reopenedChooser.waitFor({ state: 'visible', timeout: 10_000 })
  assert(await reopenedChooser.locator('.el-collapse-item.is-active').count() === 2, `${viewport.width}: reopening Explore must preserve expanded sources`)
  const restoredChooserScroll = await reopenedChooser.locator('.explore-source-list').evaluate(node => node.scrollTop)
  assert(Math.abs(restoredChooserScroll - chooserScroll) <= 1, `${viewport.width}: reopening Explore must preserve source-list scroll (${chooserScroll} -> ${restoredChooserScroll})`)
  await closeShelfExploreChooser(page, viewport)

  await page.goto(`${root}/discover?sourceId=1&url=https%3A%2F%2Fsource.example%2Fexplore&name=%E7%83%AD%E9%97%A8`, { waitUntil: 'networkidle' })
  await page.waitForSelector('.explore-workspace-popover', { timeout: 10000 })
  const legacyExploreState = await page.evaluate(() => ({
    path: location.pathname,
    workspace: new URLSearchParams(location.search).get('workspace'),
    shelfVisible: Boolean(document.querySelector('.shelf-page')),
  }))
  assert(legacyExploreState.path === '/', `${viewport.width}: /discover must redirect to the root workspace`)
  assert(legacyExploreState.workspace === 'explore', `${viewport.width}: legacy discover intent must retain workspace=explore`)
  assert(legacyExploreState.shelfVisible, `${viewport.width}: legacy discover must hydrate the chooser before replacing the shelf`)
  await closeShelfExploreChooser(page, viewport)

  await page.goto(root, { waitUntil: 'networkidle' })
  await page.waitForSelector('.shelf-page .book-row', { timeout: 10000 })
  await openMobileNavigation(page, viewport)
  await assertSidebarSearchSurface(page, viewport)
  const freshSearchInput = page.locator('.app-shell-search input')
  await freshSearchInput.fill('默认侧栏搜索')
  await freshSearchInput.press('Enter')
  await page.waitForSelector('.result-shelf-page .remote-result-book', { timeout: 10000 })
  const freshDefaultSearch = (await page.evaluate(() => window.__workspaceSearchRequests())).at(-1)
  assert(freshDefaultSearch.concurrentCount === 24, `${viewport.width}: fresh sidebar search must use the upstream default concurrency 24`)
  assert(!Object.hasOwn(freshDefaultSearch, 'page'), `${viewport.width}: multi-source search must not drive its continuation with page`)
  assert(freshDefaultSearch.lastIndex === -1, `${viewport.width}: fresh multi-source search must start at cursor -1`)
  const interleavedTitles = await page.locator('.result-shelf-page .remote-result-book .name').allTextContents()
  assert(JSON.stringify(interleavedTitles) === JSON.stringify(['交错 A1', '交错 B1', '交错 A2']), `${viewport.width}: multi-source results must preserve interleaved service order: ${interleavedTitles.join(' | ')}`)
  assert(await page.locator('.source-result-head').count() === 0, `${viewport.width}: result scene must not regroup rows by source`)
  const searchGeometry = await resultGeometry(page, '.result-shelf-page .remote-result-list')
  assert(JSON.stringify(searchGeometry) === JSON.stringify(shelfGeometry), `${viewport.width}: result cards must reuse shelf geometry: ${JSON.stringify(searchGeometry)} vs ${JSON.stringify(shelfGeometry)}`)
  await page.getByRole('button', { name: '加载更多' }).click()
  await page.waitForFunction(() => document.querySelectorAll('.result-shelf-page .remote-result-book').length === 4)
  const multiContinuation = (await page.evaluate(() => window.__workspaceSearchRequests())).at(-1)
  assert(multiContinuation.lastIndex === 0, `${viewport.width}: multi-source continuation must use the returned cursor`)
  assert(!Object.hasOwn(multiContinuation, 'page'), `${viewport.width}: multi-source continuation must not send a page number`)
  const multiEndButton = page.getByRole('button', { name: '没有更多了' })
  assert(await multiEndButton.isDisabled(), `${viewport.width}: multi-source completion must remain visibly disabled`)
  const requestCountBeforeConfigChange = (await page.evaluate(() => window.__workspaceSearchRequests())).length
  await openMobileNavigation(page, viewport)
  await selectElementOption(page, page.locator('.app-search-setting .el-select').nth(2), '30并发线程')
  await page.waitForFunction(count => window.__workspaceSearchRequests().then(rows => rows.length > count), requestCountBeforeConfigChange)
  const rerunAfterConfigChange = (await page.evaluate(() => window.__workspaceSearchRequests())).at(-1)
  assert(rerunAfterConfigChange.keyword === '默认侧栏搜索', `${viewport.width}: search-setting changes must retain the current keyword`)
  assert(rerunAfterConfigChange.concurrentCount === 30, `${viewport.width}: search-setting changes must rerun with the new concurrency`)
  assert(rerunAfterConfigChange.lastIndex === -1, `${viewport.width}: search-setting changes must restart from the first cursor`)
  await closeMobileNavigation(page, viewport)

  await page.goto(`${root}/?workspace=search&q=单源续页&searchType=single&sourceId=1&concurrent=24`, { waitUntil: 'networkidle' })
  await page.waitForSelector('.result-shelf-page .remote-result-book', { timeout: 10000 })
  const singleFirst = (await page.evaluate(() => window.__workspaceSearchRequests())).at(-1)
  assert(singleFirst.page === 1, `${viewport.width}: single-source search must begin at page one`)
  assert(!Object.hasOwn(singleFirst, 'lastIndex'), `${viewport.width}: single-source search must not send a multi-source cursor`)
  await page.getByRole('button', { name: '加载更多' }).click()
  await page.waitForFunction(() => document.querySelectorAll('.result-shelf-page .remote-result-book').length === 2)
  const singleContinuation = (await page.evaluate(() => window.__workspaceSearchRequests())).at(-1)
  assert(singleContinuation.page === 2, `${viewport.width}: single-source continuation must advance page`)
  assert(!Object.hasOwn(singleContinuation, 'lastIndex'), `${viewport.width}: single-source continuation must keep the cursor absent`)

  await page.goto(`${root}/search?q=旧链接搜索&searchType=all&concurrent=8`, { waitUntil: 'networkidle' })
  await page.waitForSelector('.result-shelf-page .remote-result-book', { timeout: 10000 })
  const legacyState = await page.evaluate(() => ({
    path: window.location.pathname,
    workspace: new URLSearchParams(window.location.search).get('workspace'),
    heading: document.querySelector('.result-shelf-page .shelf-title strong')?.textContent || '',
  }))
  assert(legacyState.path === '/', `${viewport.width}: /search must redirect to /`)
  assert(legacyState.workspace === 'search', `${viewport.width}: redirected search must retain workspace=search`)
  assert(legacyState.heading.includes('搜索 (3)'), `${viewport.width}: search result heading is missing`)
  const legacySearch = (await page.evaluate(() => window.__workspaceSearchRequests())).at(-1)
  assert(legacySearch.concurrentCount === 8, `${viewport.width}: legacy concurrency must remain 8 until the user changes it`)
  await assertNoHorizontalOverflow(page, `${viewport.width} search`)

  assert(await page.locator('.remote-result-book .result-actions').count() === 0, `${viewport.width}: result cards must not add a non-upstream preview button`)
  const searchResultCard = page.locator('.result-shelf-page .remote-result-book').first()
  await searchResultCard.locator('.operation-icon').click()
  const resultEditor = page.locator('.remote-book-json-editor')
  await resultEditor.waitFor({ state: 'visible', timeout: 10_000 })
  await resultEditor.getByRole('button', { name: '保 存', exact: true }).click()
  const editorConfirm = page.locator('.el-message-box')
  await editorConfirm.waitFor({ state: 'visible', timeout: 10_000 })
  await editorConfirm.getByRole('button', { name: '取消', exact: true }).click()
  await editorConfirm.waitFor({ state: 'hidden', timeout: 10_000 })
  assert(await resultEditor.isVisible(), `${viewport.width}: cancelling edited-book shelf confirmation must keep the JSON editor open`)
  assert(await page.evaluate(() => window.__workspaceRemoteCreateCount()) === 0, `${viewport.width}: cancelling edited-book shelf confirmation must perform zero writes`)
  await resultEditor.getByRole('button', { name: '取 消', exact: true }).click()
  await resultEditor.waitFor({ state: 'hidden', timeout: 10_000 })
  const searchResultAdd = searchResultCard.locator('.result-add-book')
  assert(await searchResultAdd.getByText('加入书架', { exact: true }).count() === 1, `${viewport.width}: search result card must expose the upstream add action`)
  const categoryDialog = page.locator('.book-add-category-dialog')
  await searchResultAdd.click()
  await categoryDialog.waitFor({ state: 'visible', timeout: 10000 })
  assert(await categoryDialog.getByText('请选择分组：', { exact: true }).count() === 1, `${viewport.width}: result-card chooser must keep the upstream prompt`)
  await categoryDialog.getByRole('button', { name: '暂不加入', exact: true }).click()
  await categoryDialog.waitFor({ state: 'hidden', timeout: 10000 })
  assert(await page.evaluate(() => window.__workspaceRemoteCreateCount()) === 0, `${viewport.width}: cancelling result-card groups must not add a book`)

  await searchResultCard.locator('.book-cover-shared').click()
  await page.waitForSelector('.book-info-dialog', { timeout: 10000 })
  const searchBookInfo = page.locator('.book-info-dialog')
  assert(await searchBookInfo.getByText('加入书架', { exact: true }).count() === 1, `${viewport.width}: search cover must open the single unshelved BookInfo action`)
  assert(await searchBookInfo.getByText('加入并阅读', { exact: true }).count() === 0, `${viewport.width}: search BookInfo must not expose add-and-read`)
  assert(await searchBookInfo.getByText('开始阅读', { exact: true }).count() === 0, `${viewport.width}: search BookInfo must not expose a read action`)
  const searchBookInfoURL = await page.url()
  const createRequest = page.waitForRequest(request => {
    const requestURL = new URL(request.url())
    return request.method() === 'POST' && requestURL.pathname === '/api/books/remote'
  }, { timeout: 10000 })
  await searchBookInfo.getByText('加入书架', { exact: true }).click()
  const directAddRequest = await createRequest
  const directAddPayload = directAddRequest.postDataJSON() || {}
  assert(Array.isArray(directAddPayload.categoryIds) && directAddPayload.categoryIds.length === 0, `${viewport.width}: BookInfo direct add must submit no positive category selection`)
  assert(await categoryDialog.isHidden(), `${viewport.width}: BookInfo direct add must not open the result-card category chooser`)
  assert(await page.evaluate(() => window.__workspaceRemoteCreateCount()) === 1, `${viewport.width}: BookInfo direct add must persist exactly once`)
  await searchBookInfo.getByText('分组：', { exact: false }).waitFor({ state: 'visible', timeout: 10000 })
  assert(await searchBookInfo.getByText('加入书架', { exact: true }).count() === 0, `${viewport.width}: direct search BookInfo add must become the shelf state`)
  assert(await searchBookInfo.getByText('分组：', { exact: false }).count() === 1, `${viewport.width}: direct search BookInfo add must expose shelf properties`)
  assert(await page.url() === searchBookInfoURL, `${viewport.width}: BookInfo direct add must not navigate to Reader`)
  await searchBookInfo.locator('.el-dialog__headerbtn').click()
  await searchBookInfo.waitFor({ state: 'hidden', timeout: 10000 })

  await page.goto(root, { waitUntil: 'networkidle' })
  await page.waitForSelector('.shelf-page .book-row', { timeout: 10000 })
  await openMobileNavigation(page, viewport)
  const searchInput = page.locator('.app-shell-search input')
  await searchInput.fill('二次侧栏搜索')
  await searchInput.press('Enter')
  await page.waitForSelector('.result-shelf-page .remote-result-book', { timeout: 10000 })
  await closeMobileNavigation(page, viewport)
  const directSearchState = await page.evaluate(() => ({
    path: window.location.pathname,
    heading: document.querySelector('.result-shelf-page .shelf-title strong')?.textContent || '',
  }))
  assert(directSearchState.path === '/', `${viewport.width}: sidebar search must retain the root scene`)
  assert(directSearchState.heading.includes('搜索 (3)'), `${viewport.width}: second sidebar search did not refresh results`)
  const legacyPreferenceSearch = (await page.evaluate(() => window.__workspaceSearchRequests())).at(-1)
  assert(legacyPreferenceSearch.concurrentCount === 8, `${viewport.width}: sidebar search must retain the active legacy concurrency until the user changes it`)
  await assertNoHorizontalOverflow(page, `${viewport.width} second-search`)

  const secondSearchURL = await page.url()
  await page.locator('.result-shelf-page .remote-result-book').first().locator('.result-add-book').click()
  await categoryDialog.waitFor({ state: 'visible', timeout: 10000 })
  const resultCreateRequest = page.waitForRequest(request => {
    const requestURL = new URL(request.url())
    return request.method() === 'POST' && requestURL.pathname === '/api/books/remote'
  }, { timeout: 10000 })
  await categoryDialog.getByRole('button', { name: '确定', exact: true }).click()
  await resultCreateRequest
  await categoryDialog.waitFor({ state: 'hidden', timeout: 10000 })
  assert(await page.evaluate(() => window.__workspaceRemoteCreateCount()) === 2, `${viewport.width}: confirming result-card groups must add exactly once`)
  assert(await page.url() === secondSearchURL, `${viewport.width}: result-card add must preserve the Search workspace route`)

  await searchInput.fill('陈旧请求')
  await searchInput.press('Enter')
  await page.waitForTimeout(50)
  await openMobileNavigation(page, viewport)
  await page.getByRole('button', { name: '探索书源' }).click()
  const exploreChooser = page.locator('.explore-workspace-popover:visible')
  await exploreChooser.waitFor({ state: 'visible', timeout: 10000 })
  if (viewport.width <= 750) {
    await page.waitForFunction(() => {
      const sidebar = document.querySelector('.app-sidebar')
      return sidebar && Math.abs(Number.parseFloat(getComputedStyle(sidebar).marginLeft) + 260) < 0.5
    })
  }
  const chooserState = await page.evaluate(() => ({
    heading: document.querySelector('.result-shelf-page .shelf-title strong')?.textContent || '',
    sidebarMargin: Number.parseFloat(getComputedStyle(document.querySelector('.app-sidebar')).marginLeft),
    popover: (() => {
      const node = [...document.querySelectorAll('.explore-workspace-popover')]
        .find(candidate => {
          const rect = candidate.getBoundingClientRect()
          return rect.width > 0 && rect.height > 0
        })
      const rect = node?.getBoundingClientRect()
      return rect ? { left: rect.left, top: rect.top, width: rect.width, height: rect.height } : null
    })(),
  }))
  assert(chooserState.heading.includes('搜索'), `${viewport.width}: opening Explore must retain the current root result scene until an entry is selected`)
  if (viewport.width <= 750) {
    assert(Math.abs(chooserState.sidebarMargin + 260) < 0.5, `${viewport.width}: sidebar Explore trigger must close the compact navigation`)
    assert(Math.abs(chooserState.popover.left) <= 1 && Math.abs(chooserState.popover.top) <= 1, `${viewport.width}: compact Explore chooser must start at the viewport origin`)
    assert(Math.abs(chooserState.popover.width - viewport.width) <= 1, `${viewport.width}: compact Explore chooser must span the viewport width`)
    assert(chooserState.popover.height < viewport.height - 40, `${viewport.width}: compact Explore chooser must preserve content height instead of covering the viewport`)
    assert(await exploreChooser.getByRole('button', { name: '关闭书海', exact: true }).count() === 1, `${viewport.width}: compact Explore chooser must expose its close action`)
  } else {
    assert(Math.abs(chooserState.popover.width - 600) <= 1, `${viewport.width}: desktop Explore chooser must use the fixed 600px width`)
    assert(Math.abs(chooserState.popover.top) <= 1, `${viewport.width}: desktop Explore chooser must be fixed to top 0`)
    assert(await exploreChooser.getByRole('button', { name: '关闭书海', exact: true }).count() === 0, `${viewport.width}: desktop Explore chooser must not add an internal close action`)
  }
  const exploreEntry = exploreChooser.locator('.explore-entry-row button').first()
  if (!await exploreEntry.isVisible()) {
    await exploreChooser.locator('.el-collapse-item__header').first().click()
    await exploreEntry.waitFor({ state: 'visible', timeout: 10000 })
  }
  await exploreEntry.click()
  await exploreChooser.waitFor({ state: 'hidden', timeout: 10000 })
  await page.waitForSelector('.discover-page .remote-result-book', { timeout: 10000 })
  await page.waitForTimeout(550)
  const exploreState = await page.evaluate(() => ({
    path: window.location.pathname,
    heading: document.querySelector('.discover-page .shelf-title strong')?.textContent || '',
    text: document.querySelector('.result-shelf-page')?.textContent || '',
  }))
  assert(exploreState.path === '/', `${viewport.width}: Explore must remain in the root scene`)
  assert(exploreState.heading.includes('探索 (1)'), `${viewport.width}: Explore result heading is missing`)
  assert(!exploreState.text.includes('陈旧结果'), `${viewport.width}: stale search response must not overwrite Explore`)
  const exploreResultCard = page.locator('.discover-page .remote-result-book').first()
  await exploreResultCard.locator('.result-add-book').click()
  await categoryDialog.waitFor({ state: 'visible', timeout: 10000 })
  await categoryDialog.getByRole('button', { name: '暂不加入', exact: true }).click()
  await categoryDialog.waitFor({ state: 'hidden', timeout: 10000 })
  assert(await page.evaluate(() => window.__workspaceRemoteCreateCount()) === 2, `${viewport.width}: cancelling Explore result-card add must not persist`)
  await exploreResultCard.locator('.book-cover-shared').click()
  await page.waitForSelector('.book-info-dialog', { timeout: 10000 })
  const exploreBookInfo = page.locator('.book-info-dialog')
  assert(await exploreBookInfo.getByText('加入书架', { exact: true }).count() === 1, `${viewport.width}: explore cover must open the shared unshelved BookInfo action`)
  assert(await exploreBookInfo.getByText('加入并阅读', { exact: true }).count() === 0, `${viewport.width}: explore BookInfo must not expose add-and-read`)
  const exploreBookInfoURL = await page.url()
  await exploreBookInfo.locator('.el-dialog__headerbtn').click()
  await exploreBookInfo.waitFor({ state: 'hidden', timeout: 10000 })
  assert(await page.url() === exploreBookInfoURL, `${viewport.width}: closing explore BookInfo must preserve the workspace route`)
  const createsBeforeExploreEdit = await page.evaluate(() => window.__workspaceRemoteCreateCount())
  await exploreResultCard.locator('.operation-icon').click()
  await resultEditor.waitFor({ state: 'visible', timeout: 10_000 })
  const editorTextarea = resultEditor.getByRole('textbox', { name: '书籍 JSON', exact: true })
  const editedExploreBook = JSON.parse(await editorTextarea.inputValue())
  editedExploreBook.title = '工作台探索已编辑'
  await editorTextarea.fill(JSON.stringify(editedExploreBook, null, 2))
  await resultEditor.getByRole('button', { name: '保 存', exact: true }).click()
  await editorConfirm.waitFor({ state: 'visible', timeout: 10_000 })
  await editorConfirm.getByRole('button', { name: '确定', exact: true }).click()
  await resultEditor.waitFor({ state: 'hidden', timeout: 10_000 })
  assert(await page.evaluate(() => window.__workspaceRemoteCreateCount()) === createsBeforeExploreEdit + 1, `${viewport.width}: confirmed result edit must write exactly once`)
  assert(await page.url() === exploreBookInfoURL, `${viewport.width}: confirmed result edit must preserve the Explore scene`)
  await page.locator('.discover-page .title-actions').getByRole('button', { name: '加载更多', exact: true }).click()
  await page.waitForFunction(() => document.querySelectorAll('.discover-page .remote-result-book').length === 2)
  const exploreEndButton = page.locator('.discover-page .title-actions').getByRole('button', { name: '没有更多了', exact: true })
  assert(await exploreEndButton.isDisabled(), `${viewport.width}: Explore completion must remain visibly disabled`)
  await assertNoHorizontalOverflow(page, `${viewport.width} explore`)

  await page.locator('.discover-page .title-actions').getByRole('button', { name: '书架', exact: true }).click()
  await page.waitForSelector('.shelf-page .book-row', { timeout: 10000 })
  await assertNoHorizontalOverflow(page, `${viewport.width} shelf-return`)
  assert(failures.length === 0, failures.join('\n'))
  await context.close()
  return `${viewport.width}x${viewport.height}`
}

async function run() {
  const browser = await openSmokeBrowser()
  try {
    const checks = []
    checks.push(await runViewport(browser, { width: 1440, height: 900 }))
    checks.push(await runViewport(browser, { width: 1024, height: 1366 }))
    checks.push(await runViewport(browser, { width: 390, height: 844 }))
    checks.push(await runViewport(browser, { width: 360, height: 800 }))
    if (process.env.BOOKSHELF_TIME_ONLY === '1') {
      console.log(`bookshelf latest-chapter time: ok ${checks.join(', ')} source=lastCheckTime`)
    } else {
      console.log(`index-workspace: ok ${checks.join(', ')} legacyRedirects=true sidebarSearch=true canonicalBookInfo=true exploreCoverInfo=true`)
    }
  } finally {
    await browser.close()
  }
}

run().catch((error) => {
  console.error(error.stack || error.message)
  process.exit(1)
})

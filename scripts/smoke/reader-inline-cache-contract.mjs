#!/usr/bin/env node

import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetUrl = process.env.TARGET_URL || 'http://127.0.0.1:4173'
const readerUrl = `${targetUrl.replace(/\/$/, '')}/books/1/read?chapter=0`

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

async function waitFor(predicate, message, timeout = 10000) {
  const deadline = Date.now() + timeout
  while (!predicate()) {
    if (Date.now() >= deadline) throw new Error(message)
    await new Promise(resolve => setTimeout(resolve, 5))
  }
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

function localBook() {
  return {
    id: 1,
    title: 'Local Cache',
    author: 'Reader',
    sourceId: 0,
    sourceName: '',
    url: '',
    bookUrl: '',
    libraryPath: 'library/local-cache.txt',
    chapterCount: 5,
    categoryIds: [],
    progress: null,
  }
}

function chapters() {
  return Array.from({ length: 5 }, (_, index) => ({
    id: index + 11,
    index,
    title: `第${index + 1}章`,
  }))
}

function chapterResponse(index) {
  return {
    chapter: chapters()[index],
    format: 'text',
    content: `本地缓存章节 ${index}\n用于验证完整后续区间和取消状态。`,
  }
}

function deferred() {
  let resolve
  const promise = new Promise(done => {
    resolve = done
  })
  return { promise, resolve }
}

async function installMocks(page, state) {
  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace(/^\/api/, '')
    const method = request.method()

    if (path === '/me') return route.fulfill(json({ id: 1, username: 'cache-smoke', role: 'admin' }))
    if (path === '/settings/reader' && method === 'GET') {
      return route.fulfill(json({
        key: 'reader',
        updatedAt: '2026-08-11T00:00:00Z',
        value: {
          mode: 'scroll',
          pageMode: 'auto',
          autoTheme: false,
          theme: 'parchment',
          themeType: 'day',
          fontSize: 18,
          lineHeight: 1.8,
          paragraphSpace: 0.2,
          columnWidth: 800,
        },
      }))
    }
    if (path === '/settings/reader' && method === 'PUT') {
      return route.fulfill(json({ key: 'reader', updatedAt: '2026-08-11T00:00:01Z', value: {} }))
    }
    if (path === '/books/1') return route.fulfill(json(localBook()))
    if (path === '/books') return route.fulfill(json([localBook()]))
    if (path === '/books/1/chapters') return route.fulfill(json(chapters()))
    if (path === '/books/1/chapters/0/content') return route.fulfill(json(chapterResponse(0)))
    const chapterMatch = path.match(/^\/books\/1\/chapters\/(\d+)\/content$/)
    if (chapterMatch) {
      const index = Number(chapterMatch[1])
      if (state.phase === 'bootstrap') {
        return route.fulfill(json({ chapter: chapters()[index], format: 'text', content: '' }))
      }
      state.requests.push(index)
      state.active += 1
      state.peak = Math.max(state.peak, state.active)
      if (state.phase === 'cancel') await state.cancelGate.promise
      if (state.phase === 'complete' && index === 4) await state.completeGate.promise
      state.active -= 1
      return route.fulfill(json(chapterResponse(index)))
    }
    if (path === '/progress/1') return route.fulfill(json({}))
    if (path === '/progress' && method === 'PUT') return route.fulfill(json({}))
    if (path === '/sources') return route.fulfill(json([]))
    if (path === '/categories') return route.fulfill(json([]))
    if (path === '/book-groups') return route.fulfill(json([]))
    return route.fulfill(json({}))
  })
}

async function openCachePanel(page, mobile) {
  const trigger = mobile
    ? page.locator('.reader-mobile-bottom.visible .mobile-chapter-progress')
    : page.locator('.reader-page-control .progress-box')
  await trigger.click()
  await page.locator('.reader-cache-zone').waitFor({ state: 'visible' })
}

async function assertFlatSurface(page, viewport) {
  const state = await page.locator('.reader-cache-zone').evaluate((panel) => {
    const style = window.getComputedStyle(panel)
    const action = panel.querySelector('.reader-cache-actions button')
    const actionStyle = action ? window.getComputedStyle(action) : null
    return {
      borderTopWidth: style.borderTopWidth,
      boxShadow: style.boxShadow,
      actionBorderTopWidth: actionStyle?.borderTopWidth || '',
      cancelText: panel.querySelector('.reader-cache-cancel')?.textContent?.trim() || '',
      cancelCount: panel.querySelectorAll('.reader-cache-cancel').length,
      actionText: panel.querySelector('.reader-cache-actions')?.innerText || '',
      left: panel.getBoundingClientRect().left,
      right: panel.getBoundingClientRect().right,
    }
  })
  assert(state.borderTopWidth === '0px', `${viewport.width}: cache panel border ${state.borderTopWidth}`)
  assert(state.boxShadow === 'none', `${viewport.width}: cache panel shadow ${state.boxShadow}`)
  assert(state.actionBorderTopWidth === '0px', `${viewport.width}: cache action border ${state.actionBorderTopWidth}`)
  assert(state.actionText.includes('后面50章') && state.actionText.includes('后面100章') && state.actionText.includes('后面全部'), `${viewport.width}: cache actions missing`)
  assert(state.left >= 0 && state.right <= viewport.width + 1, `${viewport.width}: cache panel outside viewport ${state.left}-${state.right}`)
}

async function runViewport(browser, viewport) {
  const context = await browser.newContext({ viewport, hasTouch: viewport.width <= 1024 })
  const cachedKey = 'localCache@user:1@Local Cache_Reader@library/local-cache.txt@chapterContent-2'
  await context.addInitScript(({ token, key, value }) => {
    window.localStorage.setItem('openreader_token', token)
    window.localStorage.setItem(key, JSON.stringify(value))
  }, { token: fakeToken(), key: cachedKey, value: chapterResponse(2) })

  const page = await context.newPage()
  const failures = []
  page.on('console', (message) => {
    if (message.type() !== 'error') return
    const value = message.text()
    if (value.includes('/ws/sync') && value.includes('WebSocket connection')) return
    failures.push(value)
  })
  page.on('pageerror', error => failures.push(error.message))

  const state = {
    active: 0,
    peak: 0,
    requests: [],
    phase: 'bootstrap',
    cancelGate: deferred(),
    completeGate: deferred(),
  }
  await installMocks(page, state)
  await page.goto(readerUrl, { waitUntil: 'networkidle' })
  await page.waitForSelector('.reader-body p', { timeout: 10000 })
  state.phase = 'cancel'
  const mobile = viewport.width <= 750
  await page.waitForSelector(mobile ? '.reader-mobile-bottom.visible' : '.reader-page-control')

  await openCachePanel(page, mobile)
  await assertFlatSurface(page, viewport)
  await page.getByRole('button', { name: '后面50章', exact: true }).click()
  await page.waitForFunction(() => document.querySelector('.reader-cache-status')?.textContent?.includes('/4'))
  await waitFor(() => state.active >= 2, `${viewport.width}: cache requests did not reach concurrency 2`)
  assert(state.peak === 2, `${viewport.width}: expected two in-flight requests, peak ${state.peak}`)
  assert(!state.requests.includes(2), `${viewport.width}: browser-cached chapter 2 requested again`)

  const cancel = page.getByRole('button', { name: '取消缓存', exact: true })
  await cancel.click()
  await page.getByRole('button', { name: '后面50章', exact: true }).waitFor({ state: 'visible' })
  assert(await page.locator('.reader-cache-status').count() === 0, `${viewport.width}: cancel did not restore actions synchronously`)
  assert(await page.locator('.reader-toast').count() === 0, `${viewport.width}: cancel emitted an extra toast`)
  state.cancelGate.resolve()
  await waitFor(() => state.active === 0, `${viewport.width}: cancelled in-flight requests did not settle`)
  await page.waitForTimeout(100)
  assert(JSON.stringify([...state.requests].sort((a, b) => a - b)) === JSON.stringify([1, 3]), `${viewport.width}: cancel requests ${JSON.stringify(state.requests)}`)

  state.phase = 'complete'
  await page.getByRole('button', { name: '后面100章', exact: true }).click()
  await page.waitForFunction(() => document.querySelector('.reader-cache-status')?.textContent?.includes('/4'))
  await waitFor(() => state.requests.includes(4), `${viewport.width}: missing chapter 4 was not requested`)
  assert(state.requests.filter(index => index === 1).length === 1, `${viewport.width}: chapter 1 was fetched twice`)
  assert(state.requests.filter(index => index === 2).length === 0, `${viewport.width}: chapter 2 was fetched`)
  assert(state.requests.filter(index => index === 3).length === 1, `${viewport.width}: chapter 3 was fetched twice`)
  state.completeGate.resolve()
  await page.locator('.reader-toast').filter({ hasText: '缓存完成' }).waitFor({ state: 'visible' })
  const toast = await page.locator('.reader-toast').innerText()
  assert(toast.trim() === '缓存完成', `${viewport.width}: completion toast ${JSON.stringify(toast)}`)
  await page.getByRole('button', { name: '后面50章', exact: true }).waitFor({ state: 'visible' })
  assert(JSON.stringify([...state.requests].sort((a, b) => a - b)) === JSON.stringify([1, 3, 4]), `${viewport.width}: complete requests ${JSON.stringify(state.requests)}`)

  const finalSurface = await page.locator('.reader-cache-zone').evaluate(panel => ({
    cancelCount: panel.querySelectorAll('.reader-cache-cancel').length,
    text: panel.innerText,
  }))
  assert(finalSurface.cancelCount === 0, `${viewport.width}: idle cache panel retained cancel icon`)
  assert(!finalSurface.text.includes('取消'), `${viewport.width}: idle cache panel exposed a cancel label`)
  assert(failures.length === 0, failures.join('\n'))
  await context.close()
}

async function main() {
  const browser = await openSmokeBrowser()
  try {
    for (const viewport of [
      { width: 1440, height: 900 },
      { width: 390, height: 844 },
      { width: 360, height: 800 },
      { width: 1024, height: 1366 },
    ]) {
      await runViewport(browser, viewport)
    }
    console.log('reader inline chapter cache contract smoke passed at 1440/390/360/1024')
  } finally {
    await browser.close()
  }
}

main().catch((error) => {
  console.error(error.stack || error.message)
  process.exit(1)
})

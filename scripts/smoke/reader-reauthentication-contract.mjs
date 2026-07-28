#!/usr/bin/env node

import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetUrl = (process.env.TARGET_URL || 'http://127.0.0.1:4173').replace(/\/$/, '')
const privateTitle = '用户 A 的私有阅读书籍'
const privateParagraph = '用户 A 的私有正文在认证失效后必须立即从页面移除。'

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

function tokenFor(userId, nonce) {
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url')
  const payload = Buffer.from(JSON.stringify({ userId, sub: String(userId) })).toString('base64url')
  return `${header}.${payload}.reader-${userId}-${nonce}`
}

function json(data, status = 200) {
  return {
    status,
    contentType: 'application/json',
    body: JSON.stringify(data),
  }
}

function requestJSON(request) {
  try {
    return request.postDataJSON()
  } catch {
    return {}
  }
}

function authorizationToken(request) {
  const authorization = request.headers().authorization || ''
  return authorization.startsWith('Bearer ') ? authorization.slice(7) : ''
}

function userForToken(token, state) {
  return token === state.tokenB ? state.userB : state.userA
}

async function installApiMocks(page, state) {
  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async route => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace(/^\/api/, '')
    const method = request.method()
    const requestToken = authorizationToken(request)
    const activeUser = userForToken(requestToken, state)

    if (path === '/auth/login' && method === 'POST') {
      const payload = requestJSON(request)
      const differentAccount = payload.username === 'other-user'
      const user = differentAccount ? state.userB : state.userA
      return route.fulfill(json({
        token: differentAccount ? state.tokenB : state.tokenARenewed,
        user,
      }))
    }
    if (path === '/me') return route.fulfill(json(activeUser))
    if (path === '/health') {
      return route.fulfill(json({ version: 'reader-reauth-smoke', commit: 'reader-reauth-smoke' }))
    }
    if (path.startsWith('/settings/') && method === 'GET') {
      const key = path.slice('/settings/'.length)
      return route.fulfill(json({
        key,
        updatedAt: '2026-07-28T00:00:00Z',
        value: key === 'reader'
          ? {
              mode: 'page',
              pageMode: 'auto',
              theme: 'parchment',
              themeType: 'day',
              autoTheme: false,
              fontSize: 18,
              fontWeight: 400,
              lineHeight: 1.8,
              paragraphSpace: 0.2,
              columnWidth: 800,
              animateDuration: 0,
            }
          : {},
      }))
    }
    if (path.startsWith('/settings/') && method === 'PUT') {
      return route.fulfill(json({
        key: path.slice('/settings/'.length),
        updatedAt: '2026-07-28T00:00:01Z',
        value: requestJSON(request).value || {},
      }))
    }

    const bookA = {
      id: 1,
      title: privateTitle,
      author: '用户 A',
      sourceId: 10,
      sourceName: '用户 A 私有书源',
      url: 'https://private.example/user-a/book',
      bookUrl: 'https://private.example/user-a/book',
      chapterCount: 2,
      categoryIds: [],
      progress: {
        bookId: 1,
        chapterIndex: 0,
        chapterTitle: '第一章',
        chapterOffset: 12,
        chapterPercent: 0.12,
      },
    }
    const bookB = {
      id: 2,
      title: '用户 B 的书架书籍',
      author: '用户 B',
      sourceId: 20,
      sourceName: '用户 B 私有书源',
      url: 'https://private.example/user-b/book',
      bookUrl: 'https://private.example/user-b/book',
      chapterCount: 1,
      categoryIds: [],
      progress: null,
    }

    if (path === '/books') {
      return route.fulfill(json(activeUser.id === state.userB.id ? [bookB] : [bookA]))
    }
    if (path === '/books/1') return route.fulfill(json(bookA))
    if (path === '/books/2') return route.fulfill(json(bookB))
    if (path === '/books/1/chapters') {
      return route.fulfill(json([
        { id: 11, index: 0, title: '第一章' },
        { id: 12, index: 1, title: '第二章' },
      ]))
    }
    if (/^\/books\/1\/chapters\/\d+\/content$/.test(path)) {
      const index = Number(path.match(/chapters\/(\d+)\/content/)?.[1] || 0)
      return route.fulfill(json({
        chapter: { id: 11 + index, index, title: `第${index + 1}章` },
        content: `${privateParagraph}\n第二段私有正文用于确认 Reader 已重新初始化。`,
      }))
    }
    if (path === '/progress/1' && method === 'GET') {
      return route.fulfill(json(bookA.progress))
    }
    if (path === '/progress' && method === 'PUT') {
      state.progressWrites.push({
        token: requestToken,
        payload: requestJSON(request),
      })
      return route.fulfill(json({
        ...requestJSON(request),
        updatedAt: '2026-07-28T00:00:02Z',
      }))
    }
    if (path === '/categories' || path === '/book-groups' || path === '/sources') {
      return route.fulfill(json([]))
    }
    return route.fulfill(json({}))
  })
}

async function openReader(browser, viewport) {
  const state = {
    tokenA: tokenFor(1, 'expired'),
    tokenARenewed: tokenFor(1, 'renewed'),
    tokenB: tokenFor(2, 'renewed'),
    userA: { id: 1, username: 'same-user', role: 'admin' },
    userB: { id: 2, username: 'other-user', role: 'user' },
    progressWrites: [],
  }
  const context = await browser.newContext({
    viewport,
    hasTouch: viewport.width <= 750,
    isMobile: viewport.width <= 750,
  })
  await context.addInitScript(token => localStorage.setItem('openreader_token', token), state.tokenA)
  const page = await context.newPage()
  const errors = []
  page.on('pageerror', error => errors.push(`pageerror: ${error.message}`))
  page.on('console', message => {
    if (
      message.type() === 'error'
      && !/WebSocket connection to .*\/ws\/sync/.test(message.text())
    ) {
      errors.push(`console.error: ${message.text()}`)
    }
  })
  await installApiMocks(page, state)
  await page.goto(
    `${targetUrl}/books/1/read?chapter=0&offset=12&percent=0.12`,
    { waitUntil: 'networkidle' },
  )
  await page.waitForSelector('.reader-body [data-reader-block]', { timeout: 15_000 })
  await page.getByText(privateParagraph, { exact: true }).waitFor({ state: 'visible', timeout: 10_000 })
  return {
    context,
    errors,
    page,
    readerURL: page.url(),
    state,
  }
}

async function invalidateReader(session, viewport) {
  const { page, state } = session
  const writeBaseline = state.progressWrites.length
  await page.evaluate((rejectedToken) => {
    localStorage.removeItem('openreader_token')
    window.__openreaderAuthRequired = { reason: 'session', rejectedToken }
    window.dispatchEvent(new CustomEvent('openreader:auth-required', {
      detail: window.__openreaderAuthRequired,
    }))
  }, state.tokenA)

  await page.waitForFunction(() => (
    !document.querySelector('.reader-shell')
    && Boolean(document.querySelector('.reader-auth-blocked'))
    && Boolean(document.querySelector('.auth-dialog'))
  ))
  await page.getByText('登录状态已失效，请重新登录', { exact: true })
    .waitFor({ state: 'visible', timeout: 10_000 })
  assert(await page.locator('.reader-shell').count() === 0, `${viewport.width}: invalidated Reader remained mounted`)
  assert(await page.locator('.reader-body [data-reader-block]').count() === 0, `${viewport.width}: private paragraphs remained mounted`)
  assert(await page.getByText(privateTitle, { exact: true }).count() === 0, `${viewport.width}: private book title remained visible`)
  assert(await page.getByText(privateParagraph, { exact: true }).count() === 0, `${viewport.width}: private text remained visible`)
  assert(await page.locator('.auth-dialog .el-dialog__headerbtn').count() === 0, `${viewport.width}: invalid-session dialog exposed a close button`)

  await page.keyboard.press('Escape')
  await page.waitForTimeout(100)
  assert(await page.locator('.auth-dialog').isVisible(), `${viewport.width}: Escape dismissed invalid-session dialog`)
  await page.waitForTimeout(150)
  const oldWrites = state.progressWrites
    .slice(writeBaseline)
    .filter(write => write.token === state.tokenA)
  assert(oldWrites.length === 0, `${viewport.width}: invalidated Reader wrote old-account progress ${JSON.stringify(oldWrites)}`)
  assert(page.url() === session.readerURL, `${viewport.width}: invalidation changed Reader URL`)
}

async function submitLogin(page, username) {
  const usernameInput = page.getByPlaceholder('请输入用户名')
  const passwordInput = page.getByPlaceholder('请输入密码')
  assert(await usernameInput.count() === 1, 'reauth username input is not unique')
  assert(await passwordInput.count() === 1, 'reauth password input is not unique')
  await usernameInput.fill(username)
  await passwordInput.fill('password')
  const submit = page.locator('.auth-dialog button[type="submit"]')
  assert(await submit.count() === 1, 'reauth submit button is not unique')
  await submit.click()
  await page.locator('.auth-dialog').waitFor({ state: 'hidden', timeout: 10_000 })
}

async function assertSameAccount(browser, viewport) {
  const session = await openReader(browser, viewport)
  try {
    await invalidateReader(session, viewport)
    await submitLogin(session.page, 'same-user')
    await session.page.waitForSelector('.reader-body [data-reader-block]', { timeout: 15_000 })
    await session.page.getByText(privateParagraph, { exact: true })
      .waitFor({ state: 'visible', timeout: 10_000 })
    assert(session.page.url() === session.readerURL, `${viewport.width}: same-account login did not preserve Reader URL`)
    if (viewport.width <= 750) {
      assert(
        await session.page.locator('.reader-mobile-top.visible').count() === 1,
        `${viewport.width}: reloaded mobile Reader did not restore the default-visible tool layer`,
      )
    }
    const staleWrites = session.state.progressWrites
      .filter(write => write.token === session.state.tokenA)
    assert(staleWrites.length === 0, `${viewport.width}: same-account flow reused expired-token progress`)
    assert(session.errors.length === 0, `${viewport.width}: ${session.errors.join('\n')}`)
  } finally {
    await session.context.close()
  }
}

async function assertDifferentAccount(browser, viewport) {
  const session = await openReader(browser, viewport)
  try {
    await invalidateReader(session, viewport)
    await submitLogin(session.page, 'other-user')
    await session.page.waitForFunction(() => location.pathname === '/')
    await session.page.getByText('用户 B 的书架书籍', { exact: true })
      .waitFor({ state: 'visible', timeout: 10_000 })
    assert(new URL(session.page.url()).pathname === '/', `${viewport.width}: different account retained Reader route`)
    assert(await session.page.locator('.reader-shell').count() === 0, `${viewport.width}: different account retained Reader`)
    assert(await session.page.getByText(privateTitle, { exact: true }).count() === 0, `${viewport.width}: different account exposed old title`)
    assert(await session.page.getByText(privateParagraph, { exact: true }).count() === 0, `${viewport.width}: different account exposed old text`)
    const staleWrites = session.state.progressWrites
      .filter(write => write.token === session.state.tokenA)
    assert(staleWrites.length === 0, `${viewport.width}: different-account flow wrote old progress`)
    assert(session.errors.length === 0, `${viewport.width}: ${session.errors.join('\n')}`)
  } finally {
    await session.context.close()
  }
}

async function main() {
  const browser = await openSmokeBrowser()
  try {
    const checked = []
    for (const viewport of [
      { width: 1440, height: 900 },
      { width: 1024, height: 1366 },
      { width: 390, height: 844 },
      { width: 360, height: 800 },
    ]) {
      await assertSameAccount(browser, viewport)
      await assertDifferentAccount(browser, viewport)
      checked.push(`${viewport.width}x${viewport.height}`)
    }
    console.log(`reader-reauthentication: ok ${checked.join(', ')} sameAccount=true differentAccount=true oldProgressWrites=0`)
  } finally {
    await browser.close()
  }
}

main().catch(error => {
  console.error(error.stack || error.message)
  process.exit(1)
})

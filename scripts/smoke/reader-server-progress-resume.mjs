import assert from 'node:assert/strict'
import { openSmokeBrowser } from './playwright-runtime.mjs'
import { openContext, openShelf, waitForBookText } from './reader-shelf-switch-contract.mjs'

const base = process.env.TARGET_URL || 'http://127.0.0.1:5173'

async function checkResume(browser, viewport, scenario) {
  const switchBooks = scenario === 'switch'
  const { page, context } = await openContext(browser, viewport, 'page')
  const progress = Promise.withResolvers()
  try {
    await page.addInitScript(() => localStorage.setItem('openreader_chapter_progress@user:1@3', JSON.stringify({
      bookId: 3, chapterId: 309, chapterIndex: 9, offset: 300, chapterPercent: 0.5, updatedAt: '2026-10-01T00:00:00Z',
    })))
    await page.route('**/api/progress/3', async route => {
      await progress.promise
      await route.fulfill({ json: { bookId: 3, chapterId: 307, chapterIndex: 7, offset: 0, updatedAt: '2026-11-01T00:00:00Z' } })
    })
    await page.goto(new URL('/books/3/read', base).href, { waitUntil: 'domcontentloaded' })
    await waitForBookText(page, 3, 'reader opens at local position')
    const body = page.locator('.reader-body')
    await body.getByText('换书契约三第10章', { exact: false }).first().waitFor()
    // Pause only the server-position replace; a normal shelf navigation can then cancel it.
    await page.evaluate(() => {
      const router = document.querySelector('#app').__vue_app__.config.globalProperties.$router
      const gate = Promise.withResolvers()
      window.releaseResume = gate.resolve
      const matches = to => to.params.id === '3' && to.query.resume === '1' && String(to.query.chapter) === '7'
      router.beforeEach(to => {
        if (!matches(to) || window.resumeStarted) return
        window.resumeStarted = true
        return gate.promise
      })
      router.afterEach((to, from, failure) => {
        if (matches(to)) window.resumeResult = failure ? 'cancelled' : 'applied'
      })
    })
    progress.resolve()
    await page.waitForFunction(() => window.resumeStarted === true, undefined, { timeout: 10000 })
    if (switchBooks) {
      await openShelf(page, viewport)
      await page.locator('.reader-shelf-card[data-book-id="1"]:visible').click()
      await waitForBookText(page, 1, 'switch to book 1 before restoration completes')
      await body.getByText('换书契约一第1章', { exact: false }).first().waitFor()
    }
    if (scenario === 'same-url') {
      await page.evaluate(() => document.querySelector('#app').__vue_app__.config.globalProperties.$router.push({
        name: 'reader', params: { id: 3 }, query: { resume: '1', chapter: 7 },
      }))
      await body.getByText('换书契约三第8章', { exact: false }).first().waitFor({ timeout: 10000 })
    }
    await page.evaluate(() => window.releaseResume())
    await page.waitForFunction(expected => window.resumeResult === expected, scenario === 'normal' ? 'applied' : 'cancelled')
    // Wait through chapter rendering, including the asynchronous chapter fetch on a broken build.
    await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))))
    await page.waitForLoadState('networkidle')
    const text = await body.innerText()
    const url = new URL(page.url())
    assert.equal(url.pathname, switchBooks ? '/books/1/read' : '/books/3/read')
    assert.equal(url.searchParams.get('chapter'), switchBooks ? null : '7')
    assert.match(text, switchBooks ? /换书契约一第1章/ : /换书契约三第8章/)
    console.log(viewport.width, scenario, switchBooks ? 'book 1 chapter 1' : 'book 3 chapter 8')
  } finally {
    progress.resolve()
    await context.close()
  }
}

const browser = await openSmokeBrowser()
try {
  for (const [width, height] of [[1440, 900], [390, 844], [360, 800]]) {
    for (const scenario of ['normal', 'switch', 'same-url']) await checkResume(browser, { width, height }, scenario)
  }
} finally {
  await browser.close()
}

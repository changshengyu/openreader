import assert from 'node:assert/strict'
import { openSmokeBrowser } from './playwright-runtime.mjs'
import { openContext, openShelf, waitForBookText } from './reader-shelf-switch-contract.mjs'

const baseUrl = process.env.TARGET_URL || 'http://127.0.0.1:5173'

async function checkPendingSelection(browser, viewport, saveFails) {
  const { context, page } = await openContext(browser, viewport, 'page')
  const release = Promise.withResolvers()
  async function selectBook(id) {
    await openShelf(page, viewport)
    await page.locator(`.reader-shelf-card[data-book-id="${id}"]:visible`).click()
  }
  try {
    await page.goto(new URL('/books/3/read?chapter=0', baseUrl).href, { waitUntil: 'networkidle' })
    await waitForBookText(page, 3, 'open previous book')
    await selectBook(1)
    await waitForBookText(page, 1, 'open origin book')
    await page.waitForLoadState('networkidle')

    // Hold only the next save. Subsequent writes during history navigation must be free to finish.
    await page.route('**/api/progress', async route => {
      await release.promise
      await route.fulfill(saveFails
        ? { status: 400, json: { error: 'fixture save rejected' } }
        : { json: { ...route.request().postDataJSON(), updatedAt: new Date().toISOString() } })
    }, { times: 1 })
    const started = page.waitForRequest(request => request.method() === 'PUT' && new URL(request.url()).pathname === '/api/progress')
    await selectBook(4)
    const pending = await started
    assert.equal(Number(pending.postDataJSON().bookId), 1)
    await page.goBack({ waitUntil: 'commit' })
    await waitForBookText(page, 3, 'go back while saving')
    await page.goForward({ waitUntil: 'commit' })
    await waitForBookText(page, 1, 'return while saving')

    const url = page.url()
    const messageCount = await page.evaluate(() => window.__shelfSwitchMessages.length)
    const delivered = page.waitForResponse(response => response.request() === pending)
    release.resolve()
    await (await delivered).finished()
    await page.waitForLoadState('networkidle')
    const actual = await page.evaluate(() => ({
      url: location.href,
      booksInBody: window.__shelfSwitchSnapshot().markers,
      messageCount: window.__shelfSwitchMessages.length,
    }))
    assert.deepEqual(actual, { url, booksInBody: [1], messageCount })
    console.log(`${viewport.width}x${viewport.height} save ${saveFails ? 'rejected' : 'fulfilled'}: retained book 1 and its URL`)
  } finally {
    release.resolve()
    await context.close()
  }
}

const browser = await openSmokeBrowser()
try {
  for (const [width, height] of [[1440, 900], [390, 844], [360, 800]]) {
    for (const saveFails of [false, true]) await checkPendingSelection(browser, { width, height }, saveFails)
  }
} finally {
  await browser.close()
}

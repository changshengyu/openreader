import assert from 'node:assert/strict'
import { openSmokeBrowser } from './playwright-runtime.mjs'
import { ensureMobileChrome, openContext, openShelf, waitForBookText } from './reader-shelf-switch-contract.mjs'

const target = process.env.TARGET_URL || 'http://127.0.0.1:5173'
const catalogRoute = '**/api/books/1/chapters'
const latestTitle = 'catalog-3-0'

async function openCatalog(page, viewport) {
  if (viewport.width <= 750) {
    await ensureMobileChrome(page, viewport)
    await page.locator('.reader-mobile-top.visible .mobile-tool-button').filter({ hasText: '目录' }).click()
  } else {
    await page.getByRole('button', { name: '目录', exact: true }).first().click()
  }
  return page.locator('[data-chapter-index="0"]:visible')
}

async function readCachedTitle(page) {
  return page.evaluate(async () => {
    function read(request) {
      return new Promise((resolve, reject) => {
        request.onsuccess = () => resolve(request.result)
        request.onerror = () => reject(request.error)
      })
    }
    const db = await read(indexedDB.open('openreader-cache'))
    try {
      const records = await read(db.transaction('responses').objectStore('responses').getAll())
      return records.find(record => record.key.endsWith('reader@user:1@chapters:1'))?.value[0]?.title
    } finally {
      db.close()
    }
  })
}

async function checkCatalog(browser, viewport) {
  const { page, context } = await openContext(browser, viewport, 'page')
  const held = Promise.withResolvers()
  let requests = 0
  let offlineCatalog = false
  async function switchTo(bookId) {
    await openShelf(page, viewport)
    await page.locator(`.reader-shelf-card[data-book-id="${bookId}"]:visible`).click()
    await waitForBookText(page, bookId, `switch to book ${bookId}`)
  }
  try {
    await page.route(catalogRoute, async route => {
      if (offlineCatalog) return route.abort('internetdisconnected')
      const revision = ++requests
      if (revision === 2) await held.promise
      const chapters = [0, 1, 2].map(index => ({
        id: 100 + index, bookId: 1, index, title: `catalog-${revision}-${index}`,
      }))
      await route.fulfill({ json: chapters })
    })
    await page.goto(new URL('/books/1/read?chapter=0', target).href, { waitUntil: 'networkidle' })
    await waitForBookText(page, 1, 'warm book A')
    await switchTo(3)
    const firstRefresh = page.waitForRequest(catalogRoute)
    await switchTo(1)
    await firstRefresh
    await switchTo(3)
    await switchTo(1)
    const row = await openCatalog(page, viewport)
    await row.filter({ hasText: latestTitle }).waitFor()
    const staleResponse = page.waitForResponse(catalogRoute)
    held.resolve()
    await (await staleResponse).finished()
    await page.waitForLoadState('networkidle')
    assert.deepEqual({ displayed: await row.innerText(), cached: await readCachedTitle(page) },
      { displayed: latestTitle, cached: latestTitle })

    offlineCatalog = true
    await page.reload({ waitUntil: 'networkidle' })
    await waitForBookText(page, 1, 'reopen with catalog endpoint offline')
    const reopened = await openCatalog(page, viewport)
    assert.equal(await reopened.innerText(), latestTitle)
    console.log(`${viewport.width}x${viewport.height}: current catalog displayed, cached and reopened`)
  } finally {
    held.resolve()
    await context.close()
  }
}

const browser = await openSmokeBrowser()
try {
  for (const [width, height] of [[1440, 900], [390, 844], [360, 800]]) {
    await checkCatalog(browser, { width, height })
  }
} finally {
  await browser.close()
}

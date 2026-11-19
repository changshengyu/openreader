#!/usr/bin/env node
import assert from 'node:assert/strict'
import { openSmokeBrowser } from './playwright-runtime.mjs'
import { openContext, openShelf, waitForBookText, ensureMobileChrome } from './reader-shelf-switch-contract.mjs'
const targetUrl = process.env.TARGET_URL || 'http://127.0.0.1:5173'
async function openToc(page, viewport) {
  if (viewport.width <= 750) {
    await ensureMobileChrome(page, viewport)
    await page.locator('.reader-mobile-top.visible .mobile-tool-button').filter({ hasText: '目录' }).click()
  } else {
    await page.locator('.reader-left-rail button[aria-label="目录"]').click()
  }
}
const browser = await openSmokeBrowser()
try {
  for (const viewport of [{ width: 1440, height: 900 }, { width: 390, height: 844 }, { width: 360, height: 800 }]) {
    const { page, context, consoleErrors } = await openContext(browser, viewport, 'page')
    try {
      // Delay only a completed cursor's final callback. Iteration and real cache keys stay intact;
      // the UI must already be correct while a newly opened catalog's async refresh is pending.
      await page.addInitScript(() => {
        window.__cacheMarkListReleases = []
        const original = IDBObjectStore.prototype.openCursor
        const descriptor = Object.getOwnPropertyDescriptor(IDBRequest.prototype, 'onsuccess')
        IDBObjectStore.prototype.openCursor = function (...args) {
          const request = original.apply(this, args)
          Object.defineProperty(request, 'onsuccess', { set(callback) {
            descriptor.set.call(request, function (event) {
              if (!request.result && window.__holdCacheMarkList) {
                window.__cacheMarkListReleases.push(() => callback.call(request, event))
              } else callback.call(request, event)
            })
          } })
          return request
        }
      })
      await page.goto(`${targetUrl}/books/1/read?chapter=2`, { waitUntil: 'networkidle' })
      await waitForBookText(page, 1, 'initial book')
      await page.waitForTimeout(400)
      await openToc(page, viewport)
      await page.locator('[data-chapter-index="2"] .cached').waitFor({ state: 'visible' })
      // Selecting the current chapter dismisses the catalog on both layouts.
      await page.locator('[data-chapter-index="2"]:visible').click()
      await openShelf(page, viewport)
      await page.locator('.reader-shelf-card[data-book-id="3"]:visible').click()
      await waitForBookText(page, 3, 'switched book')
      await page.waitForTimeout(400)
      await page.evaluate(() => { window.__holdCacheMarkList = true })
      await openToc(page, viewport)
      await page.waitForFunction(() => window.__cacheMarkListReleases.length > 0)
      await page.locator('[data-chapter-index="9"]').waitFor({ state: 'visible' })
      assert.equal(await page.locator('[data-chapter-index="2"] .cached').count(), 0, 'book 1 mark leaks into book 3 before async catalog cache refresh')
      assert.equal(await page.locator('[data-chapter-index="9"] .cached').count(), 1, 'new book chapter mark missing')
      await page.evaluate(() => { window.__holdCacheMarkList = false; window.__cacheMarkListReleases.splice(0).forEach(release => release()) })
      assert.deepEqual(consoleErrors, [])
      console.log(`reader cache marks shelf contract passed ${viewport.width}x${viewport.height}`)
    } finally {
      await page.evaluate(() => {
        window.__holdCacheMarkList = false
        window.__cacheMarkListReleases?.splice(0).forEach(release => release())
      }).catch(() => {})
      await context.close()
    }
  }
} finally { await browser.close() }

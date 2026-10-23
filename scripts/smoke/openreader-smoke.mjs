#!/usr/bin/env node

import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetUrl = process.env.TARGET_URL || 'http://127.0.0.1:8080'
const readerUrl = process.env.SMOKE_READER_URL || ''

function assert(condition, message) {
  if (!condition) {
    throw new Error(message)
  }
}

async function collectPageSignals(page) {
  const failures = []
  page.on('pageerror', (error) => failures.push(`pageerror: ${error.message}`))
  page.on('console', (message) => {
    if (message.type() === 'error') {
      failures.push(`console.error: ${message.text()}`)
    }
  })
  page.on('response', (response) => {
    const status = response.status()
    const url = response.url()
    if (status >= 500 && /\/api\//.test(url)) {
      failures.push(`api ${status}: ${url}`)
    }
  })
  return failures
}

async function assertAppNotBlank(page) {
  await page.waitForLoadState('domcontentloaded')
  await page.waitForTimeout(800)
  const result = await page.evaluate(() => {
    const app = document.querySelector('#app')
    const bodyText = document.body?.innerText?.trim() || ''
    const isVisible = (node) => {
      const rect = node.getBoundingClientRect()
      const style = window.getComputedStyle(node)
      return rect.width > 0 && rect.height > 0 && style.visibility !== 'hidden' && style.display !== 'none'
    }
    return {
      hasApp: Boolean(app),
      bodyTextLength: bodyText.length,
      visibleNodes: Array.from(document.body.querySelectorAll('*')).filter(isVisible).length,
      appTextLength: app?.innerText?.trim().length || 0,
      appVisibleNodes: app ? Array.from(app.querySelectorAll('*')).filter(isVisible).length : 0,
      title: document.title,
      pathname: window.location.pathname,
    }
  })
  assert(result.hasApp, `missing #app at ${result.pathname}`)
  // Only what the app rendered counts: markup outside #app (teleported overlays,
  // static fallbacks) must not hide an app that never mounted.
  assert(result.appTextLength > 0 || result.appVisibleNodes > 3, `empty #app at ${result.pathname}`)
  return result
}

async function smokeViewport(browser, name, viewport, url) {
  const context = await browser.newContext({ viewport })
  const page = await context.newPage()
  const failures = await collectPageSignals(page)
  try {
    await page.goto(url, { waitUntil: 'networkidle' })
    const result = await assertAppNotBlank(page)
    assert(failures.length === 0, 'page reported errors')
    return result
  } catch (error) {
    // A blank page usually has a pageerror behind it; report both with the viewport.
    throw new Error([`${name} ${viewport.width}x${viewport.height}: ${error.message}`, ...failures].join('\n'))
  } finally {
    await context.close()
  }
}

async function main() {
  const targets = [
    ['desktop', { width: 1440, height: 900 }, targetUrl],
    ['mobile-390', { width: 390, height: 844 }, targetUrl],
    ['mobile-360', { width: 360, height: 800 }, targetUrl],
  ]
  if (readerUrl) {
    targets.push(['reader-mobile-390', { width: 390, height: 844 }, readerUrl])
  }

  const browser = await openSmokeBrowser()
  try {
    const checks = []
    for (const [name, viewport, url] of targets) {
      checks.push([`${name} ${viewport.width}x${viewport.height}`, await smokeViewport(browser, name, viewport, url)])
    }

    for (const [name, result] of checks) {
      console.log(`${name}: ok ${result.pathname} visibleNodes=${result.visibleNodes} text=${result.bodyTextLength}`)
    }
  } finally {
    await browser.close()
  }
}

main().catch((error) => {
  console.error(error.stack || error.message)
  process.exit(1)
})

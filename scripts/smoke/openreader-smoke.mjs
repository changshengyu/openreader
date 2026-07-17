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
    const visibleNodes = Array.from(document.body.querySelectorAll('*')).filter((node) => {
      const rect = node.getBoundingClientRect()
      const style = window.getComputedStyle(node)
      return rect.width > 0 && rect.height > 0 && style.visibility !== 'hidden' && style.display !== 'none'
    }).length
    return {
      hasApp: Boolean(app),
      bodyTextLength: bodyText.length,
      visibleNodes,
      title: document.title,
      pathname: window.location.pathname,
    }
  })
  assert(result.hasApp, `missing #app at ${result.pathname}`)
  assert(result.bodyTextLength > 0 || result.visibleNodes > 3, `blank page at ${result.pathname}`)
  return result
}

async function smokeViewport(browser, viewport, url) {
  const context = await browser.newContext({ viewport })
  const page = await context.newPage()
  const failures = await collectPageSignals(page)
  await page.goto(url, { waitUntil: 'networkidle' })
  const result = await assertAppNotBlank(page)
  assert(failures.length === 0, failures.join('\n'))
  await context.close()
  return result
}

async function main() {
  const browser = await openSmokeBrowser()
  try {
    const checks = []
    checks.push(['desktop', await smokeViewport(browser, { width: 1440, height: 900 }, targetUrl)])
    checks.push(['mobile-390', await smokeViewport(browser, { width: 390, height: 844 }, targetUrl)])
    checks.push(['mobile-360', await smokeViewport(browser, { width: 360, height: 800 }, targetUrl)])

    if (readerUrl) {
      checks.push(['reader-mobile-390', await smokeViewport(browser, { width: 390, height: 844 }, readerUrl)])
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

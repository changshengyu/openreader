#!/usr/bin/env node

import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetURL = (process.env.TARGET_URL || 'http://127.0.0.1:8080').replace(/\/$/, '')
const emptyCatalogHint = '未匹配到目录。你可以修改目录规则后重新解析，或保留空目录导入，之后再从书籍信息中刷新目录。'
const fixture = Buffer.from('== 第一章 ==\n这是正文内容。', 'utf8')
const runID = Date.now()

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

async function signIn(page, suffix) {
  const response = await page.request.post(`${targetURL}/api/auth/register`, {
    data: { username: `txt-smoke-${suffix}-${runID}`, password: 'txt-smoke-password' },
  })
  assert(response.ok(), `registration failed: ${response.status()} ${await response.text()}`)
  const { token } = await response.json()
  assert(token, 'registration did not return a token')
  await page.evaluate(value => localStorage.setItem('openreader_token', value), token)
  await page.reload({ waitUntil: 'networkidle' })
}

async function openImport(page) {
  const action = page.getByText('导入书籍', { exact: true })
  const mobileTrigger = page.getByLabel('打开侧边栏', { exact: true })
  if (await mobileTrigger.isVisible()) {
    await mobileTrigger.click()
    await action.waitFor({ state: 'visible' })
    await action.scrollIntoViewIfNeeded()
  } else if (!await action.isVisible()) {
    throw new Error('import action is not visible')
  }
  await action.click()
  await page.getByText('导入本地书籍', { exact: true }).waitFor()
}

async function runViewport(browser, viewport) {
  const context = await browser.newContext({ viewport })
  const page = await context.newPage()
  const errors = []
  page.on('pageerror', error => errors.push(`pageerror: ${error.message}`))
  page.on('console', message => {
    if (message.type() === 'error') {
      errors.push(`console.error: ${message.text()}`)
    }
  })
  page.on('response', response => {
    if (response.status() >= 500 && response.url().includes('/api/')) {
      errors.push(`api ${response.status()}: ${response.url()}`)
    }
  })

  try {
    await page.goto(targetURL, { waitUntil: 'networkidle' })
    await signIn(page, `${viewport.width}-${viewport.height}`)
    await openImport(page)
    await page.locator('input[type="file"]').setInputFiles({
      name: 'retry-rule.txt',
      mimeType: 'text/plain',
      buffer: fixture,
    })
    await page.getByText('已解析 1 章', { exact: true }).waitFor()

    const rule = page.getByPlaceholder('TXT目录规则（可选，留空使用默认规则，例如：^第.+章.*$）')
    await rule.fill('^不存在的目录$')
    await page.getByText('重新解析', { exact: true }).click()
    await page.locator('.direct-import-preview-empty').getByText(emptyCatalogHint, { exact: true }).waitFor()
    assert(await page.getByRole('button', { name: '导入', exact: true }).isEnabled(), 'upstream-compatible empty catalogue must remain confirmable')

    await rule.fill('^== .+ ==$')
    await page.getByText('重新解析', { exact: true }).click()
    await page.getByText('已解析 1 章', { exact: true }).waitFor()
    assert(await page.getByRole('button', { name: '导入', exact: true }).isEnabled(), 'import must re-enable after the valid staged retry')
    assert(errors.length === 0, errors.join('\n'))
    console.log(`${viewport.width}x${viewport.height}: direct TXT empty-catalog staged-retry UI ok`)
  } finally {
    await context.close()
  }
}
const browser = await openSmokeBrowser()
try {
  await runViewport(browser, { width: 1440, height: 900 })
  await runViewport(browser, { width: 390, height: 844 })
  await runViewport(browser, { width: 360, height: 800 })
} finally {
  await browser.close()
}

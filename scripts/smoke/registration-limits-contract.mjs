#!/usr/bin/env node
// Contract: docs/compat/registration-invite-code-user-limit-fixed-baseline-second-audit-p1-contract.md
//
// Run against a real OpenReader server on a fresh data folder started with
//   OPENREADER_INVITE_CODE=$REGISTRATION_INVITE_CODE OPENREADER_USER_LIMIT=3
// e.g. TARGET_URL=http://127.0.0.1:8080 REGISTRATION_INVITE_CODE=smoke-invite node scripts/smoke/registration-limits-contract.mjs
import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetUrl = (process.env.TARGET_URL || 'http://127.0.0.1:8080').replace(/\/$/, '')
const inviteCode = process.env.REGISTRATION_INVITE_CODE || 'smoke-invite'
const runID = `${Date.now().toString(36)}${process.pid}`.replace(/[^a-z0-9]/gi, '').slice(-10)
const password = 'password88'
const viewports = [
  ['desktop', { width: 1440, height: 900 }],
  ['mobile-390', { width: 390, height: 844 }],
  ['mobile-360', { width: 360, height: 800 }],
]

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

async function openRegisterForm(browser, viewport) {
  const context = await browser.newContext({ viewport })
  const page = await context.newPage()
  const pageErrors = []
  page.on('pageerror', error => pageErrors.push(String(error?.message || error)))
  await page.goto(`${targetUrl}/login`, { waitUntil: 'domcontentloaded' })
  await page.getByPlaceholder('请输入用户名').waitFor({ state: 'visible', timeout: 15_000 })
  return { context, page, pageErrors }
}

const inviteField = page => page.locator('.el-form-item', { hasText: '邀请码(没有则不填)' })

async function fill(page, { username, secret = password, code } = {}) {
  await page.getByPlaceholder('请输入用户名').fill(username ?? '')
  await page.getByPlaceholder('请输入密码').fill(secret)
  if (code !== undefined) await inviteField(page).locator('input').fill(code)
}

async function submitAndExpectMessage(page, expected, label) {
  const before = await page.locator('.el-message__content').count()
  await page.locator('button.auth-btn').click()
  const message = page.locator('.el-message__content', { hasText: expected })
  try {
    await message.last().waitFor({ state: 'visible', timeout: 8_000 })
  } catch {
    const shown = await page.locator('.el-message__content').allTextContents()
    throw new Error(`${label}: expected message ${JSON.stringify(expected)}, saw ${JSON.stringify(shown.slice(before))} at ${page.url()}`)
  }
  assert(new URL(page.url()).pathname === '/login', `${label}: left /login after a rejected registration`)
}

async function switchToRegister(page, name) {
  assert(!(await inviteField(page).isVisible()), `${name}: login mode must not show the invite field`)
  await page.getByRole('button', { name: '创建新账号' }).click()
  await inviteField(page).waitFor({ state: 'visible', timeout: 5_000 })
  const field = await inviteField(page).boundingBox()
  const passwordBox = await page.getByPlaceholder('请输入密码').boundingBox()
  const viewport = page.viewportSize()
  assert(field && passwordBox && field.y > passwordBox.y, `${name}: the invite field must sit below the password field`)
  assert(field.x >= 0 && field.x + field.width <= viewport.width + 1, `${name}: the invite field overflows the ${viewport.width}px viewport`)
}

async function registerFlow(browser, name, viewport, username) {
  const { context, page, pageErrors } = await openRegisterForm(browser, viewport)
  try {
    await switchToRegister(page, name)
    await fill(page, { username: '' })
    await submitAndExpectMessage(page, '请输入用户名', `${name} empty username`)
    await fill(page, { username: 'abc', code: inviteCode })
    await submitAndExpectMessage(page, '用户名不能低于5位', `${name} short username`)
    await fill(page, { username, code: '' })
    await submitAndExpectMessage(page, '请输入邀请码', `${name} missing invite code`)
    await fill(page, { username, code: `${inviteCode}-wrong` })
    await submitAndExpectMessage(page, '邀请码错误', `${name} wrong invite code`)
    await fill(page, { username, code: inviteCode })
    await page.locator('button.auth-btn').click()
    await page.waitForURL(url => new URL(url).pathname !== '/login', { timeout: 15_000 })
    assert(pageErrors.length === 0, `${name}: page errors ${pageErrors.join('; ')}`)
  } finally {
    await context.close()
  }

  const second = await openRegisterForm(browser, viewport)
  try {
    await switchToRegister(second.page, name)
    await fill(second.page, { username, code: inviteCode })
    await submitAndExpectMessage(second.page, '用户名已被占用', `${name} duplicate username`)
  } finally {
    await second.context.close()
  }
}

async function limitFlow(browser, name, viewport) {
  const { context, page, pageErrors } = await openRegisterForm(browser, viewport)
  try {
    await switchToRegister(page, name)
    await fill(page, { username: `over${name.replace(/[^a-z0-9]/g, '')}${runID}`, code: inviteCode })
    await submitAndExpectMessage(page, '超过用户数上限', `${name} user limit`)
    assert(pageErrors.length === 0, `${name}: page errors ${pageErrors.join('; ')}`)
  } finally {
    await context.close()
  }
}

async function main() {
  const health = await fetch(`${targetUrl}/api/health`, { signal: AbortSignal.timeout(10_000) })
  assert(health.ok, `health returned ${health.status}`)
  const browser = await openSmokeBrowser()
  const checks = []
  try {
    // Three accounts, one per viewport, fill OPENREADER_USER_LIMIT=3; then every viewport sees the limit.
    for (const [name, viewport] of viewports) {
      await registerFlow(browser, name, viewport, `reg${name.replace(/[^a-z0-9]/g, '')}${runID}`)
      checks.push(`${name} ${viewport.width}x${viewport.height}: invite field, empty/short/missing/wrong/duplicate messages, accepted code`)
    }
    for (const [name, viewport] of viewports) {
      await limitFlow(browser, name, viewport)
      checks.push(`${name} ${viewport.width}x${viewport.height}: 超过用户数上限`)
    }
  } finally {
    await browser.close()
  }
  console.log(JSON.stringify({ status: 'pass', targetUrl, checks }, null, 2))
}

main().catch(error => {
  console.error(error?.stack || error)
  process.exitCode = 1
})

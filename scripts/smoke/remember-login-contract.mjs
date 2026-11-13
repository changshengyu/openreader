#!/usr/bin/env node
// Contract: docs/compat/remember-login-fixed-baseline-second-audit-p2-contract.md
//
// Run against a real OpenReader server on a fresh data folder, e.g.
//   TARGET_URL=http://127.0.0.1:8080 node scripts/smoke/remember-login-contract.mjs
//
// reader-dev App.vue shows 记住登录信息, checked by default, and keeps the login
// token in localStorage only when it is checked. OpenReader keeps an unchecked
// login in this tab's sessionStorage: it survives a reload and windows the app
// opens, but not an independent tab or a browser restart.
import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetURL = (process.env.TARGET_URL || 'http://127.0.0.1:8080').replace(/\/$/, '')
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

async function register(name) {
  const username = `rem${name.replace(/[^a-z0-9]/g, '')}${runID}`.slice(0, 30)
  const response = await fetch(`${targetURL}/api/auth/register`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
    signal: AbortSignal.timeout(15_000),
  })
  assert(response.ok, `${name}: register returned ${response.status} ${await response.text()}`)
  return username
}

async function revoke(token) {
  const response = await fetch(`${targetURL}/api/auth/logout`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
    signal: AbortSignal.timeout(15_000),
  })
  assert(response.status === 204, `server-side logout returned ${response.status}`)
}

const storedTokens = page => page.evaluate(() => ({
  local: localStorage.getItem('openreader_token'),
  session: sessionStorage.getItem('openreader_token'),
}))

const pathOf = page => new URL(page.url()).pathname

function rememberBox(scope) {
  return scope.locator('.el-checkbox', { hasText: '记住登录信息' })
}

async function assertRememberLayout(page, scopeSelector, label) {
  const scope = page.locator(scopeSelector)
  const box = rememberBox(scope)
  assert(await box.count() === 1, `${label}: expected one 记住登录信息 checkbox, saw ${await box.count()}`)
  await box.waitFor({ state: 'visible', timeout: 10_000 })
  const geometry = await page.evaluate(selector => {
    const root = document.querySelector(selector)
    const checkbox = [...root.querySelectorAll('.el-checkbox')].find(element => element.textContent.includes('记住登录信息'))
    const passwordInput = root.querySelector('input[placeholder="请输入密码"]')
    const submit = root.querySelector('button[type="submit"]')
    const rect = element => element.getBoundingClientRect()
    return {
      checkbox: rect(checkbox),
      password: rect(passwordInput),
      submit: rect(submit),
      viewportWidth: innerWidth,
    }
  }, scopeSelector)
  assert(geometry.checkbox.left >= 0 && geometry.checkbox.right <= geometry.viewportWidth + 1,
    `${label}: checkbox spans ${Math.round(geometry.checkbox.left)}-${Math.round(geometry.checkbox.right)} outside ${geometry.viewportWidth}px`)
  assert(geometry.checkbox.top >= geometry.password.bottom, `${label}: the checkbox must sit below the password field`)
  assert(geometry.checkbox.bottom <= geometry.submit.top, `${label}: the checkbox must sit above the submit button`)
}

async function isChecked(scope) {
  return rememberBox(scope).evaluate(element => element.classList.contains('is-checked'))
}

async function signInOnLoginPage(page, username, { remember }) {
  await page.getByPlaceholder('请输入用户名').fill(username)
  await page.getByPlaceholder('请输入密码').fill(password)
  const card = page.locator('.auth-card')
  if (await isChecked(card) !== remember) await rememberBox(card).click()
  assert(await isChecked(card) === remember, `remember box should be ${remember ? 'checked' : 'unchecked'}`)
  await page.locator('button.auth-btn').click()
  await page.waitForURL(url => new URL(url).pathname === '/', { timeout: 15_000 })
}

async function logoutThroughSidebar(page, viewport) {
  if (viewport.width <= 750) {
    await page.locator('.mobile-menu-trigger').click()
    await page.waitForFunction(() => document.querySelector('.app-shell')?.classList.contains('mobile-nav-open'))
  }
  const account = page.locator('[data-sidebar-section="account"]')
  await account.scrollIntoViewIfNeeded()
  await account.getByText('注销', { exact: true }).click()
  await page.waitForURL(url => new URL(url).pathname === '/login', { timeout: 15_000 })
}

async function runViewport(browser, name, viewport) {
  const username = await register(name)
  const contextOptions = { viewport, isMobile: viewport.width <= 750, hasTouch: viewport.width <= 750 }
  const context = await browser.newContext(contextOptions)
  const page = await context.newPage()
  const pageErrors = []
  page.on('pageerror', error => pageErrors.push(String(error?.message || error)))
  const checks = []
  try {
    // Remembered (default): localStorage, shared by every tab.
    await page.goto(`${targetURL}/login`, { waitUntil: 'networkidle' })
    await assertRememberLayout(page, '.auth-card', `${name} /login`)
    assert(await isChecked(page.locator('.auth-card')), `${name}: 记住登录信息 must be checked by default`)
    await page.getByRole('button', { name: '创建新账号' }).click()
    assert(await rememberBox(page.locator('.auth-card')).isVisible(), `${name}: register mode must also show 记住登录信息`)
    await page.getByRole('button', { name: '已有账号，去登录' }).click()
    await signInOnLoginPage(page, username, { remember: true })
    let tokens = await storedTokens(page)
    assert(tokens.local && !tokens.session, `${name}: remembered login stored ${JSON.stringify(tokens)}`)
    await page.reload({ waitUntil: 'networkidle' })
    assert(pathOf(page) === '/', `${name}: remembered login lost on reload (${page.url()})`)
    const sibling = await context.newPage()
    await sibling.goto(targetURL, { waitUntil: 'networkidle' })
    assert(pathOf(sibling) === '/', `${name}: remembered login missing in a new tab (${sibling.url()})`)
    await sibling.close()
    await logoutThroughSidebar(page, viewport)
    tokens = await storedTokens(page)
    assert(!tokens.local && !tokens.session, `${name}: logout kept ${JSON.stringify(tokens)}`)
    checks.push('remembered: localStorage, reload, new tab, logout clears both')

    // Unremembered: this tab's sessionStorage only.
    await signInOnLoginPage(page, username, { remember: false })
    tokens = await storedTokens(page)
    assert(tokens.session && !tokens.local, `${name}: unremembered login stored ${JSON.stringify(tokens)}`)
    const revokedToken = tokens.session

    // The server ends the unremembered session while the page stays open:
    // reauthentication keeps this page's unchecked box (reader-dev keeps
    // App.vue data.remember until the page reloads).
    const refresh = page.locator('.title-actions button', { hasText: '刷新' })
    await refresh.waitFor({ state: 'visible', timeout: 15_000 })
    await page.waitForLoadState('networkidle')
    await revoke(revokedToken)
    const dialog = page.locator('.auth-dialog')
    // Any request with the revoked token gets 401; 刷新 sends one if the page
    // has not already sent one by itself.
    if (!await dialog.isVisible()) await refresh.click({ timeout: 5_000 }).catch(() => {})
    await dialog.waitFor({ state: 'visible', timeout: 15_000 })
    await page.getByText('登录状态已失效，请重新登录', { exact: true }).waitFor({ state: 'visible', timeout: 10_000 })
    await assertRememberLayout(page, '.auth-dialog', `${name} reauthentication dialog`)
    assert(!await isChecked(dialog), `${name}: the dialog must keep 记住登录信息 unchecked for this page`)
    tokens = await storedTokens(page)
    assert(!tokens.local && !tokens.session, `${name}: the rejected token was kept ${JSON.stringify(tokens)}`)
    await page.getByPlaceholder('请输入用户名').fill(username)
    await page.getByPlaceholder('请输入密码').fill(password)
    await dialog.locator('button[type="submit"]').click()
    await dialog.waitFor({ state: 'hidden', timeout: 15_000 })
    tokens = await storedTokens(page)
    assert(tokens.session && tokens.session !== revokedToken && !tokens.local,
      `${name}: reauthentication stored ${JSON.stringify({ ...tokens, sameAsRevoked: tokens.session === revokedToken })}`)
    const sessionToken = tokens.session
    checks.push('unremembered reauthentication: dialog keeps the box unchecked, new token in sessionStorage')

    await page.reload({ waitUntil: 'networkidle' })
    assert(pathOf(page) === '/', `${name}: unremembered login lost on reload (${page.url()})`)
    assert((await storedTokens(page)).session === sessionToken, `${name}: reload changed the session token`)

    const [popup] = await Promise.all([
      context.waitForEvent('page'),
      page.evaluate(() => { window.open('/', 'remember-login-probe') }),
    ])
    await popup.waitForLoadState('networkidle')
    assert(pathOf(popup) === '/', `${name}: a window opened by the app lost the session (${popup.url()})`)
    await popup.close()

    const independent = await context.newPage()
    await independent.goto(targetURL, { waitUntil: 'networkidle' })
    assert(pathOf(independent) === '/login', `${name}: an independent tab reused the unremembered login (${independent.url()})`)
    await independent.close()

    const state = await context.storageState()
    const restarted = await browser.newContext({ ...contextOptions, storageState: state })
    try {
      const reopened = await restarted.newPage()
      await reopened.goto(targetURL, { waitUntil: 'networkidle' })
      assert(pathOf(reopened) === '/login', `${name}: a restarted browser kept the unremembered login (${reopened.url()})`)
      assert(await isChecked(reopened.locator('.auth-card')), `${name}: a new page load must start with 记住登录信息 checked`)
    } finally {
      await restarted.close()
    }
    checks.push('unremembered: reload and app window keep it; new tab and restart do not; a new load starts checked')

    assert(pageErrors.length === 0, `${name}: page errors ${pageErrors.join('; ')}`)
    return checks
  } finally {
    await context.close()
  }
}

async function main() {
  const health = await fetch(`${targetURL}/api/health`, { signal: AbortSignal.timeout(10_000) })
  assert(health.ok, `health returned ${health.status}`)
  const browser = await openSmokeBrowser()
  const results = []
  try {
    for (const [name, viewport] of viewports) {
      const checks = await runViewport(browser, name, viewport)
      results.push(`${name} ${viewport.width}x${viewport.height}: ${checks.join('; ')}`)
    }
  } finally {
    await browser.close()
  }
  console.log(JSON.stringify({ status: 'pass', targetURL, checks: results }, null, 2))
}

main().catch(error => {
  console.error(error?.stack || error)
  process.exitCode = 1
})

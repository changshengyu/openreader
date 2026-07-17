#!/usr/bin/env node

import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetUrl = process.env.TARGET_URL || 'http://127.0.0.1:5173'

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

function json(data, status = 200) {
  return {
    status,
    contentType: 'application/json',
    body: JSON.stringify(data),
  }
}

function fakeToken(role) {
  const payload = Buffer.from(JSON.stringify({ userId: role === 'admin' ? 1 : 2, sub: role === 'admin' ? '1' : '2' })).toString('base64url')
  return `open.${payload}.reader`
}

async function installApiMocks(page, role, profilePermissions = {}) {
  const users = [
    {
      id: 1,
      username: 'root-admin',
      role: 'admin',
      canEditSources: true,
      canAccessWebdav: true,
      canAccessStore: true,
      bookCount: 3,
      sourceCount: 5,
      lastActiveAt: '2026-07-12T05:00:00Z',
      createdAt: '2026-07-01T05:00:00Z',
    },
    {
      id: 2,
      username: 'ordinary-user',
      role: 'user',
      canEditSources: true,
      canAccessWebdav: true,
      canAccessStore: false,
      bookCount: 1,
      sourceCount: 5,
      lastActiveAt: '',
      createdAt: '2026-07-02T05:00:00Z',
    },
  ]
  let nextUserID = 3
  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async route => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace(/^\/api/, '')
    const method = request.method()
    if (path === '/me') {
      return route.fulfill(json(role === 'admin'
        ? { id: 1, username: 'root-admin', role: 'admin', canAccessStore: true, canAccessWebdav: true }
        : { id: 2, username: 'ordinary-user', role: 'user', ...profilePermissions }))
    }
    if (path === '/settings/reader' && method === 'GET') {
      return route.fulfill(json({ key: 'reader', value: {} }))
    }
    if (path === '/admin/users') {
      if (role !== 'admin') {
        return route.fulfill(json({ error: { code: 'FORBIDDEN', message: 'admin access required' } }, 403))
      }
      if (method === 'GET') return route.fulfill(json(users))
      if (method === 'POST') {
        const payload = request.postDataJSON()
        const created = {
          id: nextUserID++,
          username: payload.username,
          role: 'user',
          canEditSources: payload.canEditSources ?? true,
          canAccessWebdav: payload.canAccessWebdav ?? true,
          canAccessStore: payload.canAccessStore ?? true,
          bookCount: 0,
          sourceCount: 5,
          lastActiveAt: '2026-07-12T05:01:00Z',
          createdAt: '2026-07-12T05:01:00Z',
        }
        users.push(created)
        return route.fulfill(json(created, 201))
      }
    }
    return route.fulfill(json({}))
  })
}

async function closeDialog(page, selector, expectedOverlay) {
  await page.locator(`${selector} .el-dialog__headerbtn`).click()
  await page.waitForFunction((overlay) => new URLSearchParams(location.search).get('overlay') !== overlay, expectedOverlay)
}

async function openAdminManager(page, root) {
  await page.goto(`${root}/settings?panel=admin&keep=user-contract`, { waitUntil: 'networkidle' })
  const dialog = page.locator('.global-user-dialog')
  await dialog.waitFor({ state: 'visible', timeout: 10000 })
  const route = await page.evaluate(() => ({
    pathname: location.pathname,
    query: Object.fromEntries(new URLSearchParams(location.search)),
  }))
  assert(route.pathname !== '/settings', 'legacy admin settings path must redirect to the workspace')
  assert(route.query.overlay === 'user-manage' && route.query.keep === 'user-contract', `legacy intent was not retained: ${JSON.stringify(route)}`)
  return dialog
}

async function assertAdminViewport(browser, viewport) {
  const context = await browser.newContext({ viewport, isMobile: viewport.width <= 750, hasTouch: viewport.width <= 750 })
  await context.addInitScript(token => localStorage.setItem('openreader_token', token), fakeToken('admin'))
  const page = await context.newPage()
  const failures = []
  page.on('pageerror', error => failures.push(`pageerror: ${error.message}`))
  page.on('console', message => {
    if (
      message.type() === 'error'
      && !/WebSocket connection to .*\/ws\/sync/.test(message.text())
      && !/Failed to load resource: the server responded with a status of 403/.test(message.text())
    ) failures.push(`console.error: ${message.text()}`)
  })
  await installApiMocks(page, 'admin')
  const root = targetUrl.replace(/\/$/, '')
  const dialog = await openAdminManager(page, root)

  const userRows = dialog.locator(viewport.width <= 750 ? '.mobile-user-card' : '.el-table__body-wrapper tbody tr')
  const rootRow = userRows.filter({ hasText: 'root-admin' }).first()
  const memberRow = userRows.filter({ hasText: 'ordinary-user' }).first()
  assert(await rootRow.locator('.el-switch').count() === 0, `${viewport.width}: protected admin must not expose permission switches`)
  assert(await rootRow.getByRole('button', { name: '重置密码', exact: true }).count() === 0, `${viewport.width}: protected admin must not expose password reset`)
  assert(await rootRow.getByText('受保护账号', { exact: true }).count() >= 1, `${viewport.width}: protected admin label missing`)
  assert(await memberRow.locator('.el-switch').count() === 3, `${viewport.width}: ordinary row must retain independent source, WebDAV, and LocalStore switches`)
  assert(await memberRow.getByRole('button', { name: '重置密码', exact: true }).count() === 1, `${viewport.width}: ordinary row must retain password reset`)
  if (viewport.width > 750) {
    assert(await dialog.getByText('最近活跃', { exact: true }).count() === 1, `${viewport.width}: activity column missing`)
    assert(await dialog.getByText('注册时间', { exact: true }).count() === 1, `${viewport.width}: registration column missing`)
  } else {
    assert((await memberRow.innerText()).includes('最近活跃：'), `${viewport.width}: mobile activity metadata missing`)
    assert((await memberRow.innerText()).includes('注册：'), `${viewport.width}: mobile registration metadata missing`)
  }
  assert((await memberRow.innerText()).includes('未登录'), `${viewport.width}: missing activity must use the deterministic empty label`)

  const geometry = await dialog.evaluate(node => {
    const rect = node.getBoundingClientRect()
    return { left: Math.round(rect.left), top: Math.round(rect.top), width: Math.round(rect.width), height: Math.round(rect.height) }
  })
  if (viewport.width <= 750) {
    assert(geometry.left === 0 && geometry.top === 0 && geometry.width === viewport.width && geometry.height === viewport.height, `${viewport.width}: admin manager must be fullscreen on mobile`)
  } else {
    assert(Math.abs(geometry.left - (viewport.width - geometry.width) / 2) <= 1, 'desktop: admin manager must remain centered')
  }

  await dialog.getByRole('button', { name: '新增', exact: true }).click()
  const createDialog = page.locator('.el-dialog').filter({ has: page.getByText('新增用户', { exact: true }) }).last()
  await createDialog.waitFor({ state: 'visible', timeout: 10000 })
  assert(await createDialog.locator('select, .el-select').count() === 0, `${viewport.width}: create-user dialog must not expose a role selector`)
  const inputs = createDialog.locator('input')
  await inputs.nth(0).fill('browsermember')
  await inputs.nth(1).fill('secret123')
  await createDialog.getByRole('button', { name: '保存', exact: true }).click()
  await createDialog.waitFor({ state: 'hidden', timeout: 10000 })
  await userRows.filter({ hasText: 'browsermember' }).first().waitFor({ state: 'visible', timeout: 10000 })
  const createdRow = userRows.filter({ hasText: 'browsermember' }).first()
  assert((await createdRow.innerText()).includes('user'), `${viewport.width}: manager-created account must be an ordinary user`)
  assert(await createdRow.locator('.el-switch').count() === 3, `${viewport.width}: manager-created user must retain the independent WebDAV switch`)

  await closeDialog(page, '.global-user-dialog', 'user-manage')
  assert(failures.length === 0, failures.join('\n'))
  await context.close()
}

async function assertNonAdminViewport(browser, viewport, profilePermissions, expectedStorage) {
  const context = await browser.newContext({ viewport, isMobile: viewport.width <= 750, hasTouch: viewport.width <= 750 })
  await context.addInitScript(token => localStorage.setItem('openreader_token', token), fakeToken('user'))
  const page = await context.newPage()
  const failures = []
  page.on('pageerror', error => failures.push(`pageerror: ${error.message}`))
  page.on('console', message => {
    if (
      message.type() === 'error'
      && !/WebSocket connection to .*\/ws\/sync/.test(message.text())
      && !/Failed to load resource: the server responded with a status of 403/.test(message.text())
    ) failures.push(`console.error: ${message.text()}`)
  })
  await installApiMocks(page, 'user', profilePermissions)
  const root = targetUrl.replace(/\/$/, '')
  await page.goto(`${root}/`, { waitUntil: 'networkidle' })
  assert(await page.getByText('管理用户空间', { exact: true }).count() === 0, `${viewport.width}: non-admin sidebar must hide user management entry`)
  assert(
    await page.getByText('浏览书仓', { exact: true }).count() === (expectedStorage.localStore ? 1 : 0),
    `${viewport.width}: LocalStore menu must follow only canAccessStore`,
  )
  assert(
    await page.getByText('文件管理', { exact: true }).count() === (expectedStorage.webdav ? 1 : 0),
    `${viewport.width}: WebDAV menu must follow only canAccessWebdav`,
  )
  assert(
    await page.getByText('保存备份', { exact: true }).count() === (expectedStorage.webdav ? 1 : 0),
    `${viewport.width}: backup menu must follow only canAccessWebdav`,
  )

  const dialog = await openAdminManager(page, root)
  await dialog.getByText('暂无用户，或当前账号无管理员权限', { exact: true }).waitFor({ state: 'visible', timeout: 10000 })
  assert(await dialog.locator('.el-table__body-wrapper tbody tr').count() === 0, `${viewport.width}: non-admin manager intent must not render stale user rows`)
  await closeDialog(page, '.global-user-dialog', 'user-manage')
  assert(failures.length === 0, failures.join('\n'))
  await context.close()
}

async function main() {
  const browser = await openSmokeBrowser()
  try {
    const checks = []
    for (const viewport of [{ width: 1440, height: 900 }, { width: 390, height: 844 }, { width: 360, height: 800 }]) {
      await assertAdminViewport(browser, viewport)
      await assertNonAdminViewport(
        browser,
        viewport,
        { canAccessStore: false, canAccessWebdav: true },
        { localStore: false, webdav: true },
      )
      await assertNonAdminViewport(
        browser,
        viewport,
        { canAccessStore: true, canAccessWebdav: false },
        { localStore: true, webdav: false },
      )
      checks.push(`${viewport.width}x${viewport.height}`)
    }
    console.log(`user-management: ok ${checks.join(', ')} adminAndNonAdmin=true`)
  } finally {
    await browser.close()
  }
}

main().catch(error => {
  console.error(error.stack || error.message)
  process.exit(1)
})

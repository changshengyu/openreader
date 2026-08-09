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
  const state = {
    permissionWrites: [],
    failNextPermissionField: '',
  }
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
      lastLoginAt: '2026-07-12T05:00:00Z',
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
      lastLoginAt: '',
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
          lastLoginAt: '2026-07-12T05:01:00Z',
          lastActiveAt: '2026-07-12T05:01:00Z',
          createdAt: '2026-07-12T05:01:00Z',
        }
        users.push(created)
        return route.fulfill(json(created, 201))
      }
    }
    const userUpdate = path.match(/^\/admin\/users\/(\d+)$/)
    if (userUpdate && method === 'PUT') {
      if (role !== 'admin') {
        return route.fulfill(json({ error: { code: 'FORBIDDEN', message: 'admin access required' } }, 403))
      }
      const payload = request.postDataJSON()
      state.permissionWrites.push(payload)
      await new Promise(resolve => setTimeout(resolve, 180))
      const changedField = Object.keys(payload)[0] || ''
      if (state.failNextPermissionField === changedField) {
        state.failNextPermissionField = ''
        return route.fulfill(json({ error: 'simulated permission update failure' }, 500))
      }
      const user = users.find(item => item.id === Number(userUpdate[1]))
      if (!user) return route.fulfill(json({ error: 'user not found' }, 404))
      Object.assign(user, payload)
      return route.fulfill(json(user))
    }
    return route.fulfill(json({}))
  })
  return state
}

async function waitUntil(check, message, timeout = 5000) {
  const deadline = Date.now() + timeout
  while (Date.now() < deadline) {
    if (await check()) return
    await new Promise(resolve => setTimeout(resolve, 20))
  }
  throw new Error(message)
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
  let expectedPermissionFailureResponses = 0
  page.on('pageerror', error => failures.push(`pageerror: ${error.message}`))
  page.on('console', message => {
    if (
      message.type() === 'error'
      && expectedPermissionFailureResponses > 0
      && /Failed to load resource: the server responded with a status of 500/.test(message.text())
    ) {
      expectedPermissionFailureResponses -= 1
      return
    }
    if (
      message.type() === 'error'
      && !/WebSocket connection to .*\/ws\/sync/.test(message.text())
      && !/Failed to load resource: the server responded with a status of 403/.test(message.text())
    ) failures.push(`console.error: ${message.text()}`)
  })
  const apiState = await installApiMocks(page, 'admin')
  const root = targetUrl.replace(/\/$/, '')
  const dialog = await openAdminManager(page, root)

  const userRows = dialog.locator('.el-table__body-wrapper tbody tr')
  const rootRow = userRows.filter({ hasText: 'root-admin' }).first()
  const memberRow = userRows.filter({ hasText: 'ordinary-user' }).first()
  assert(await rootRow.locator('.el-switch').count() === 0, `${viewport.width}: protected admin must not expose permission switches`)
  assert(await rootRow.getByRole('button', { name: '重置密码', exact: true }).count() === 0, `${viewport.width}: protected admin must not expose password reset`)
  assert(await rootRow.getByText('受保护账号', { exact: true }).count() >= 1, `${viewport.width}: protected admin label missing`)
  assert(await memberRow.locator('.el-switch').count() === 3, `${viewport.width}: ordinary row must retain independent source, WebDAV, and LocalStore switches`)
  assert(await memberRow.getByRole('button', { name: '重置密码', exact: true }).count() === 1, `${viewport.width}: ordinary row must retain password reset`)
  assert(await dialog.getByText('上次登录', { exact: true }).count() === 1, `${viewport.width}: last-login column missing`)
  assert(await dialog.getByText('注册时间', { exact: true }).count() === 1, `${viewport.width}: registration column missing`)
  assert(await dialog.getByText('WebDAV', { exact: true }).count() >= 1, `${viewport.width}: independent WebDAV column missing`)
  assert(await dialog.getByText('书仓', { exact: true }).count() >= 1, `${viewport.width}: independent LocalStore column missing`)
  assert(await dialog.locator('.mobile-user-card').count() === 0, `${viewport.width}: separate mobile card flow must be removed`)

  const permissionSwitches = memberRow.locator('.el-switch')
  const webdavSwitch = permissionSwitches.nth(0)
  const storeSwitch = permissionSwitches.nth(1)
  const sourceSwitch = permissionSwitches.nth(2)
  await webdavSwitch.click()
  await waitUntil(
    () => webdavSwitch.evaluate(node => node.classList.contains('is-loading') || Boolean(node.querySelector('.is-loading'))),
    `${viewport.width}: WebDAV switch did not expose field loading`,
  )
  assert(
    await sourceSwitch.evaluate(node => !node.classList.contains('is-disabled') && !node.querySelector('input')?.disabled),
    `${viewport.width}: WebDAV update blocked the independent source permission`,
  )
  await sourceSwitch.click()
  await waitUntil(() => apiState.permissionWrites.length === 2, `${viewport.width}: independent permission writes did not start`)
  assert(
    JSON.stringify(apiState.permissionWrites) === JSON.stringify([
      { canAccessWebdav: false },
      { canEditSources: false },
    ]),
    `${viewport.width}: permission payloads were not field-owned: ${JSON.stringify(apiState.permissionWrites)}`,
  )
  await waitUntil(
    () => Promise.all([webdavSwitch, sourceSwitch].map(locator => locator.evaluate(node => (
      !node.classList.contains('is-loading') && !node.querySelector('.is-loading')
    )))).then(values => values.every(Boolean)),
    `${viewport.width}: permission loading state did not settle`,
  )

  apiState.failNextPermissionField = 'canAccessStore'
  expectedPermissionFailureResponses += 1
  await storeSwitch.click()
  await waitUntil(() => apiState.permissionWrites.length === 3, `${viewport.width}: failed permission write did not start`)
  await waitUntil(
    () => storeSwitch.locator('input').isChecked().then(checked => !checked),
    `${viewport.width}: failed LocalStore permission did not revert`,
  )
  assert(
    JSON.stringify(apiState.permissionWrites[2]) === JSON.stringify({ canAccessStore: true }),
    `${viewport.width}: failed permission payload contained unrelated fields`,
  )
  await waitUntil(() => expectedPermissionFailureResponses === 0, `${viewport.width}: expected permission failure was not observed`)

  const geometry = await dialog.evaluate(node => {
    const rect = node.getBoundingClientRect()
    return { left: Math.round(rect.left), top: Math.round(rect.top), width: Math.round(rect.width), height: Math.round(rect.height) }
  })
  if (viewport.width <= 750) {
    assert(geometry.left === 0 && geometry.top === 0 && geometry.width === viewport.width && geometry.height === viewport.height, `${viewport.width}: admin manager must be fullscreen on mobile`)
    assert(await dialog.locator('th.el-table-fixed-column--left').count() >= 2, `${viewport.width}: selection and username columns must remain fixed while the table scrolls`)
  } else {
    assert(Math.abs(geometry.left - (viewport.width - geometry.width) / 2) <= 1, 'desktop: admin manager must remain centered')
  }
  const closeButton = await dialog.locator('.el-dialog__headerbtn').boundingBox()
  assert(
    closeButton &&
      closeButton.y >= 0 &&
      closeButton.y + closeButton.height <= viewport.height,
    `${viewport.width}: dialog close button must remain inside the viewport`,
  )

  await dialog.getByRole('button', { name: '新增', exact: true }).click()
  const createDialog = page.locator('.el-dialog').filter({ has: page.getByText('新增用户', { exact: true }) }).last()
  await createDialog.waitFor({ state: 'visible', timeout: 10000 })
  assert(await createDialog.locator('select, .el-select').count() === 0, `${viewport.width}: create-user dialog must not expose a role selector`)
  const inputs = createDialog.locator('input')
  await inputs.nth(0).fill('browsermember')
  await inputs.nth(1).fill('secret123')
  await createDialog.getByRole('button', { name: '确定', exact: true }).click()
  await createDialog.waitFor({ state: 'hidden', timeout: 10000 })
  await userRows.filter({ hasText: 'browsermember' }).first().waitFor({ state: 'visible', timeout: 10000 })
  const createdRow = userRows.filter({ hasText: 'browsermember' }).first()
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
    for (const viewport of [
      { width: 1440, height: 900 },
      { width: 1024, height: 1366 },
      { width: 390, height: 844 },
      { width: 360, height: 800 },
    ]) {
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

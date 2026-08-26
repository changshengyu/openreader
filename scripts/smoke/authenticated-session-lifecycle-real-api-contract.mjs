#!/usr/bin/env node

const targetUrl = (process.env.TARGET_URL || 'http://127.0.0.1:8080').replace(/\/$/, '')
const runID = `${Date.now().toString(36)}${process.pid}`
const initialPassword = 'password88'
const changedPassword = 'changed888'

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

async function request(path, { method = 'GET', token = '', body, expected } = {}) {
  const headers = {}
  if (token) headers.Authorization = `Bearer ${token}`
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  const response = await fetch(`${targetUrl}${path}`, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
    redirect: 'manual',
    signal: AbortSignal.timeout(12_000),
  })
  const text = await response.text()
  if (expected !== undefined && response.status !== expected) {
    throw new Error(`${method} ${path.split('?')[0]} = ${response.status}, want ${expected}: ${text.slice(0, 300)}`)
  }
  let data = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = text
    }
  }
  return { data, status: response.status }
}

async function register(username) {
  const { data } = await request('/api/auth/register', {
    method: 'POST',
    body: { username, password: initialPassword },
    expected: 200,
  })
  assert(data?.token && data?.user?.id, `register ${username} returned no session identity`)
  return data
}

async function login(username, password) {
  const { data } = await request('/api/auth/login', {
    method: 'POST',
    body: { username, password },
    expected: 200,
  })
  assert(data?.token && data?.user?.id, `login ${username} returned no session identity`)
  return data
}

async function assertStaleAcrossEntrypoints(token) {
  await request('/api/sources', { token, expected: 401 })
  await request('/api/settings/reader', {
    method: 'PUT',
    token,
    body: { value: { fontSize: 99 } },
    expected: 401,
  })
  await request('/webdav/', { token, expected: 401 })
  await request(`/ws/sync?token=${encodeURIComponent(token)}`, { expected: 401 })
}

async function main() {
  await request('/api/health', { expected: 200 })

  const administrator = await register(`sessionadmin${runID}`)
  assert(administrator.user.role === 'admin', 'fresh-volume first user is not administrator')
  const member = await register(`sessionmember${runID}`)
  const secondLogin = await login(member.user.username, initialPassword)
  assert(member.token !== secondLogin.token, 'two logins returned the same token')

  await request('/api/auth/logout', { method: 'POST', token: member.token, expected: 204 })
  await request('/api/auth/logout', { method: 'POST', token: member.token, expected: 401 })
  await request('/api/me', { token: member.token, expected: 401 })
  await request('/api/me', { token: secondLogin.token, expected: 200 })
  await request('/webdav/', { token: member.token, expected: 401 })

  await request(`/api/admin/users/${member.user.id}/password`, {
    method: 'PUT',
    token: administrator.token,
    body: { password: changedPassword },
    expected: 200,
  })
  await assertStaleAcrossEntrypoints(secondLogin.token)
  const renewed = await login(member.user.username, changedPassword)
  await request('/api/me', { token: renewed.token, expected: 200 })

  const deleted = await register(`sessiondeleted${runID}`)
  await request('/api/admin/users/batch-delete', {
    method: 'POST',
    token: administrator.token,
    body: { ids: [deleted.user.id] },
    expected: 200,
  })
  await assertStaleAcrossEntrypoints(deleted.token)

  console.log('authenticated-session-lifecycle-real-api: ok dual-login=true logout=204/401 password-reset=401 delete=401 rest=true webdav=true websocket=true')
}

main().catch((error) => {
  console.error(error.stack || error.message)
  process.exit(1)
})

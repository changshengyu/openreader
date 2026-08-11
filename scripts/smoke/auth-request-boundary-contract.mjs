#!/usr/bin/env node

import http from 'node:http'
import https from 'node:https'

const targetURL = new URL(process.env.TARGET_URL || 'http://127.0.0.1:8080')
const authLimit = 16 << 10

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

function request(path, { method = 'GET', body = '', token = '', chunked = false } = {}) {
  const url = new URL(path, targetURL)
  const transport = url.protocol === 'https:' ? https : http
  const headers = {}
  if (body) {
    headers['Content-Type'] = 'application/json'
    if (chunked) headers['Transfer-Encoding'] = 'chunked'
    else headers['Content-Length'] = Buffer.byteLength(body)
  }
  if (token) headers.Authorization = `Bearer ${token}`

  return new Promise((resolve, reject) => {
    const outgoing = transport.request(url, { method, headers }, (incoming) => {
      const chunks = []
      incoming.on('data', chunk => chunks.push(chunk))
      incoming.on('end', () => {
        const text = Buffer.concat(chunks).toString('utf8')
        let data = null
        try {
          data = text ? JSON.parse(text) : null
        } catch {
          reject(new Error(`${method} ${path}: non-JSON response ${JSON.stringify(text)}`))
          return
        }
        resolve({ status: incoming.statusCode || 0, data, text })
      })
    })
    outgoing.on('error', reject)
    if (body) {
      if (chunked) {
        const middle = Math.floor(body.length / 2)
        outgoing.write(body.slice(0, middle))
        outgoing.write(body.slice(middle))
      } else {
        outgoing.write(body)
      }
    }
    outgoing.end()
  })
}

function expectError(response, status, message, secret = '') {
  assert(response.status === status, `status ${response.status}, want ${status}: ${response.text}`)
  assert(response.data && Object.keys(response.data).length === 1, `unexpected error shape: ${response.text}`)
  assert(response.data.error === message, `error ${JSON.stringify(response.data?.error)}, want ${JSON.stringify(message)}`)
  assert(!secret || !response.text.includes(secret), 'error response leaked submitted secret')
}

function authBody(username, password, padding = '') {
  return JSON.stringify({ username, password, ...(padding ? { padding } : {}) })
}

async function main() {
  const suffix = `${process.pid}${Date.now().toString().slice(-7)}`
  const adminUsername = `auth${suffix}`
  const maxPassword = 'p'.repeat(72)

  const health = await request('/api/health')
  assert(health.status === 200, `health status ${health.status}: ${health.text}`)

  const registered = await request('/api/auth/register', {
    method: 'POST',
    body: authBody(adminUsername, maxPassword),
  })
  assert(registered.status === 200 && registered.data?.token, `registration failed: ${registered.status} ${registered.text}`)
  const token = registered.data.token

  const loggedIn = await request('/api/auth/login', {
    method: 'POST',
    body: authBody(adminUsername, maxPassword),
  })
  assert(loggedIn.status === 200 && loggedIn.data?.token, `login failed: ${loggedIn.status} ${loggedIn.text}`)

  const truncated = await request('/api/auth/login', {
    method: 'POST',
    body: authBody(adminUsername, `${maxPassword}x`),
  })
  expectError(truncated, 401, 'invalid username or password', `${maxPassword}x`)

  const oversizedNames = [`over${suffix}`, `chunk${suffix}`]
  for (const [index, chunked] of [false, true].entries()) {
    const secret = `oversized-secret-${suffix}-${index}`
    const oversized = await request('/api/auth/register', {
      method: 'POST',
      body: authBody(oversizedNames[index], secret, 'x'.repeat(authLimit)),
      chunked,
    })
    expectError(oversized, 413, 'request body too large', secret)
  }

  const multiUsername = `multi${suffix}`
  const multiSecret = `multi-secret-${suffix}`
  const multiple = await request('/api/auth/register', {
    method: 'POST',
    body: `${authBody(multiUsername, multiSecret)} ${authBody(`ignored${suffix}`, 'ignored-password')}`,
  })
  expectError(multiple, 400, 'username and password are required', multiSecret)

  const exactPrefix = '{"username":"missinguser","password":"password8","padding":"'
  const exactSuffix = '"}'
  const exactBody = exactPrefix + 'x'.repeat(authLimit - exactPrefix.length - exactSuffix.length) + exactSuffix
  assert(Buffer.byteLength(exactBody) === authLimit, `exact-limit fixture is ${Buffer.byteLength(exactBody)} bytes`)
  const exact = await request('/api/auth/login', { method: 'POST', body: exactBody })
  expectError(exact, 401, 'invalid username or password', 'password8')

  const usersAfter = await request('/api/admin/users', { token })
  assert(usersAfter.status === 200 && Array.isArray(usersAfter.data), `final user list failed: ${usersAfter.status} ${usersAfter.text}`)
  for (const forbidden of [...oversizedNames, multiUsername, `ignored${suffix}`]) {
    assert(!usersAfter.data.some(user => user.username === forbidden), `rejected request created user ${forbidden}`)
  }
  assert(usersAfter.data.some(user => user.username === adminUsername), 'registered administrator missing from user list')

  console.log('public auth request boundary smoke passed (declared/chunked 413, single JSON, exact limit, bcrypt 72-byte boundary)')
}

main().catch((error) => {
  console.error(error.stack || error.message)
  process.exit(1)
})

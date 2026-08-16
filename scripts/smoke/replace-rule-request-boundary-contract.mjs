#!/usr/bin/env node

import http from 'node:http'
import https from 'node:https'

const targetURL = new URL(process.env.TARGET_URL || 'http://127.0.0.1:8080')
const limits = {
  single: 512 << 10,
  batch: 16 << 20,
  delete: 128 << 10,
  test: 4 << 20,
}

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

function asBuffer(body) {
  if (Buffer.isBuffer(body)) return body
  return Buffer.from(body || '', 'utf8')
}

function request(path, { method = 'GET', body = '', token = '', chunked = false } = {}) {
  const url = new URL(path, targetURL)
  const transport = url.protocol === 'https:' ? https : http
  const payload = asBuffer(body)
  const headers = {}
  if (payload.length > 0) {
    headers['Content-Type'] = 'application/json'
    if (chunked) headers['Transfer-Encoding'] = 'chunked'
    else headers['Content-Length'] = payload.length
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
    if (payload.length > 0) {
      if (chunked) {
        const middle = Math.floor(payload.length / 2)
        outgoing.write(payload.subarray(0, middle))
        outgoing.write(payload.subarray(middle))
      } else {
        outgoing.write(payload)
      }
    }
    outgoing.end()
  })
}

function paddedBody(body, size) {
  const prefix = asBuffer(body)
  assert(prefix.length <= size, `cannot pad ${prefix.length}-byte body to ${size}`)
  return Buffer.concat([prefix, Buffer.alloc(size - prefix.length, 0x20)])
}

function invalidUTF8RuleBody() {
  return Buffer.concat([
    Buffer.from('{"name":"invalid-', 'utf8'),
    Buffer.from([0xff]),
    Buffer.from('","pattern":"a","scope":"*"}', 'utf8'),
  ])
}

function expectError(response, status, message) {
  assert(response.status === status, `status ${response.status}, want ${status}: ${response.text}`)
  assert(response.data && Object.keys(response.data).length === 1, `unexpected error shape: ${response.text}`)
  assert(response.data.error === message, `error ${JSON.stringify(response.data?.error)}, want ${JSON.stringify(message)}`)
}

async function listRules(token) {
  const response = await request('/api/replace-rules', { token })
  assert(response.status === 200 && Array.isArray(response.data), `list failed: ${response.status} ${response.text}`)
  return response.data
}

async function main() {
  const suffix = `${process.pid}${Date.now().toString().slice(-7)}`
  const username = `replace${suffix}`
  const password = 'replace-boundary-password'

  const health = await request('/api/health')
  assert(health.status === 200, `health failed: ${health.status} ${health.text}`)

  const registered = await request('/api/auth/register', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  })
  assert(registered.status === 200 && registered.data?.token, `register failed: ${registered.status} ${registered.text}`)
  const token = registered.data.token

  const created = await request('/api/replace-rules', {
    method: 'POST',
    token,
    body: JSON.stringify({ name: 'runtime rule', pattern: 'a', replacement: 'b', scope: '*' }),
  })
  assert(created.status === 201 && created.data?.id, `create failed: ${created.status} ${created.text}`)
  const ruleId = created.data.id

  const secondJSON = await request('/api/replace-rules', {
    method: 'POST',
    token,
    body: '{"name":"must-not-write","pattern":"a","scope":"*"} {}',
  })
  expectError(secondJSON, 400, 'pattern is required')

  const invalidUTF8 = await request('/api/replace-rules', {
    method: 'POST',
    token,
    body: invalidUTF8RuleBody(),
  })
  expectError(invalidUTF8, 400, 'pattern is required')

  const createOverflow = await request('/api/replace-rules', {
    method: 'POST',
    token,
    body: paddedBody('{"name":"overflow-create","pattern":"a","scope":"*"}', limits.single + 1),
  })
  expectError(createOverflow, 413, 'request body too large')

  const updateOverflow = await request(`/api/replace-rules/${ruleId}`, {
    method: 'PUT',
    token,
    body: paddedBody('{"name":"runtime rule","pattern":"changed","scope":"*"}', limits.single + 1),
    chunked: true,
  })
  expectError(updateOverflow, 413, 'request body too large')

  const missingTarget = await request('/api/replace-rules/99999999', {
    method: 'PUT',
    token,
    body: paddedBody('{"name":"missing","pattern":"a","scope":"*"}', limits.single + 1),
  })
  expectError(missingTarget, 404, 'replace rule not found')

  const batchOverflow = await request('/api/replace-rules/batch', {
    method: 'POST',
    token,
    body: paddedBody('[{"name":"overflow-batch","pattern":"a","scope":"*"}]', limits.batch + 1),
    chunked: true,
  })
  expectError(batchOverflow, 413, 'request body too large')

  const deleteOverflow = await request('/api/replace-rules/batch-delete', {
    method: 'POST',
    token,
    body: paddedBody(JSON.stringify({ ids: [ruleId] }), limits.delete + 1),
    chunked: true,
  })
  expectError(deleteOverflow, 413, 'request body too large')

  const testOverflow = await request('/api/replace-rules/test', {
    method: 'POST',
    token,
    body: paddedBody('{"pattern":"a","replacement":"b","text":"a"}', limits.test + 1),
  })
  expectError(testOverflow, 413, 'request body too large')

  const missingTestText = await request('/api/replace-rules/test', {
    method: 'POST',
    token,
    body: '{"pattern":"a"}',
  })
  expectError(missingTestText, 400, 'pattern and text are required')

  let rules = await listRules(token)
  assert(rules.length === 1 && rules[0].id === ruleId && rules[0].pattern === 'a', `rejected requests mutated rules: ${JSON.stringify(rules)}`)

  const updated = await request(`/api/replace-rules/${ruleId}`, {
    method: 'PUT',
    token,
    body: JSON.stringify({ name: 'runtime rule', pattern: 'after', replacement: 'done', scope: '*' }),
  })
  assert(updated.status === 200 && updated.data?.pattern === 'after', `update failed: ${updated.status} ${updated.text}`)

  const batch = await request('/api/replace-rules/batch', {
    method: 'POST',
    token,
    body: JSON.stringify([
      { name: 'batch rule', pattern: 'x', replacement: 'y', scope: '*' },
      { name: '', pattern: 'skip', scope: '*' },
    ]),
  })
  assert(batch.status === 200 && batch.data?.created === 1 && batch.data?.skipped === 1, `batch failed: ${batch.status} ${batch.text}`)
  const batchRuleId = batch.data.rules?.[0]?.id
  assert(batchRuleId, `batch response missing durable rule: ${batch.text}`)

  const tested = await request('/api/replace-rules/test', {
    method: 'POST',
    token,
    body: JSON.stringify({ pattern: 'after', replacement: 'done', text: 'before after' }),
  })
  assert(tested.status === 200 && tested.data?.output === 'before done' && tested.data?.changed === true, `test failed: ${tested.status} ${tested.text}`)

  const deleted = await request('/api/replace-rules/batch-delete', {
    method: 'POST',
    token,
    body: JSON.stringify({ ids: [batchRuleId, ruleId] }),
  })
  assert(deleted.status === 200, `delete failed: ${deleted.status} ${deleted.text}`)
  assert(JSON.stringify(deleted.data?.deletedIds) === JSON.stringify([batchRuleId, ruleId]), `delete order changed: ${deleted.text}`)

  rules = await listRules(token)
  assert(rules.length === 0, `final rule list is not empty: ${JSON.stringify(rules)}`)

  console.log('replace-rule request boundary smoke passed (five-route 413, single UTF-8 JSON, target priority, normal mutations)')
}

main().catch((error) => {
  console.error(error.stack || error.message)
  process.exit(1)
})

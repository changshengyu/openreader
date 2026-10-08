import assert from 'node:assert/strict'

// This writes disposable fixtures; refuse to run against a production host.
const base = new URL(process.env.TARGET_URL || 'http://127.0.0.1:18197')
assert.ok(['127.0.0.1', 'localhost', '[::1]'].includes(base.hostname), 'use an isolated local server')
const suffix = `${Date.now()}-${process.pid}`
const username = `dir${process.pid}${Date.now()}`
const password = 'local-directory-contract-pass'
const request = async (path, options = {}) => fetch(new URL(path, base), options)
const registration = await request('/api/auth/register', {
  method: 'POST', headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ username, password }),
})
assert.equal(registration.status, 200)
const { token } = await registration.json()
assert.ok(token)
const headers = { Authorization: `Bearer ${token}` }
const missing = `directory-smoke-${suffix}/missing/child`
for (const recursive of ['0', '1']) {
  const result = await request(`/api/local-store?path=${encodeURIComponent(missing)}&recursive=${recursive}`, { headers })
  assert.equal(result.status, 404)
  assert.deepEqual(await result.json(), { error: 'local store path not found' })
}
const root = await request('/api/local-store', { headers })
assert.equal(root.status, 200)
assert.ok(!(await root.json()).items.some(item => item.name === missing.split('/')[0]))
const parent = `directory-smoke-${suffix}/parent/nested`
try {
  const created = await request('/api/local-store/directory', {
    method: 'POST', headers: { ...headers, 'Content-Type': 'application/json' },
    body: JSON.stringify({ path: parent, name: ' 子 目录 ' }),
  })
  assert.equal(created.status, 201)
  // Legacy metadata names trim outer whitespace; raw MKCOL names do not.
  const path = `${parent}/子 目录`
  assert.deepEqual(await created.json(), { path })
  const duplicate = await request('/api/local-store/directory', {
    method: 'POST', headers: { ...headers, 'Content-Type': 'application/json' },
    body: JSON.stringify({ path: parent, name: ' 子 目录 ' }),
  })
  assert.equal(duplicate.status, 409)
  const listing = await request(`/api/local-store?path=${encodeURIComponent(path)}`, { headers })
  assert.equal(listing.status, 200)
  assert.deepEqual((await listing.json()).items, [])
  const form = new FormData()
  form.set('path', `${parent}/upload/child`)
  form.set('file', new Blob(['directory smoke bytes']), 'book.txt')
  const upload = await request('/api/local-store/upload', { method: 'POST', headers, body: form })
  assert.equal(upload.status, 201)
  const saved = await upload.json()
  assert.equal(saved.path, `${parent}/upload/child/book.txt`)
  const download = await request(`/api/local-store/download?path=${encodeURIComponent(saved.path)}`, { headers })
  assert.equal(download.status, 200)
  assert.equal(await download.text(), 'directory smoke bytes')
  console.log('PASS LocalStore missing404/no-create, lazy root, recursive directory201/duplicate409, space/Unicode, upload-parent and exact download bytes')
} finally {
  const cleanup = await request(`/api/local-store?path=${encodeURIComponent(missing.split('/')[0])}`, { method: 'DELETE', headers })
  assert.equal(cleanup.status, 204)
}

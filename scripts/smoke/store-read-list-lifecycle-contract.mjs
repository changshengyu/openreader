import assert from 'node:assert/strict'
import { execFile, spawn } from 'node:child_process'
import { chmod, mkdir, mkdtemp, readFile, rm, stat, symlink, writeFile } from 'node:fs/promises'
import { createServer } from 'node:net'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { promisify } from 'node:util'

// Own the process and all fixtures. No TARGET_URL/CDP/production-volume mode.
const exec = promisify(execFile)
const repo = join(dirname(fileURLToPath(import.meta.url)), '..', '..')
const owned = await mkdtemp(join(tmpdir(), 'openreader-read-list-http-'))
const data = join(owned, 'data')
const local = join(owned, 'library', 'localStore')
let child
let output = ''
try {
  const reservation = createServer()
  await new Promise(resolve => reservation.listen(0, '127.0.0.1', resolve))
  const port = reservation.address().port
  await new Promise(resolve => reservation.close(resolve))
  const base = `http://127.0.0.1:${port}`
  const binary = join(owned, 'openreader')
  await exec('go', ['build', '-o', binary, '.'], { cwd: join(repo, 'backend') })
  child = spawn(binary, [], { cwd: join(repo, 'backend'), env: {
    ...process.env, OPENREADER_ADDR: `127.0.0.1:${port}`,
    OPENREADER_DATA_DIR: data, OPENREADER_DB: join(data, 'openreader.db'),
    OPENREADER_CACHE_DIR: join(owned, 'cache'), OPENREADER_LIBRARY_DIR: join(owned, 'library'),
    OPENREADER_LOCAL_STORE_DIR: local, OPENREADER_PUBLIC_DIR: join(repo, 'frontend', 'dist'),
    OPENREADER_JWT_SECRET: 'isolated-read-list-http-contract-secret', OPENREADER_CHECK_INTERVAL: '24h',
  }, stdio: ['ignore', 'pipe', 'pipe'] })
  child.stdout.on('data', chunk => { output += chunk })
  child.stderr.on('data', chunk => { output += chunk })
  const deadline = Date.now() + 30_000
  while (true) {
    try { if ((await fetch(`${base}/api/health`)).ok) break } catch {}
    assert.ok(Date.now() < deadline, `local server did not start: ${output}`)
    await new Promise(resolve => setTimeout(resolve, 100))
  }
  const req = (path, options = {}) => fetch(`${base}${path}`, { ...options, signal: AbortSignal.timeout(15_000) })
  const accounts = []
  for (const username of ['readadmin', 'readmembera', 'readmemberb']) {
    const response = await req('/api/auth/register', { method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password: 'disposable-read-contract-pass' }) })
    assert.equal(response.status, 200)
    const { token } = await response.json()
    assert.ok(token)
    accounts.push({ username, headers: { Authorization: `Bearer ${token}` }, basic: {
      Authorization: `Basic ${Buffer.from(`${username}:disposable-read-contract-pass`).toString('base64')}`,
    } })
  }
  for (const [index, user] of accounts.entries()) {
    const davRoot = index === 0 ? join(data, 'webdav') : join(data, 'webdav', 'users', user.username)
    const localRoot = index === 0 ? local : join(local, 'users', user.username)
    // REST trims the whole path's outer whitespace; raw DAV preserves it.
    const localName = ' 中文 file.txt'
    const payload = `0123456789-${user.username}`
    for (const [prefix, headers] of [['/webdav', user.headers], ['/reader3/webdav', user.basic]]) {
      const root = await req(`${prefix}/`, { method: 'PROPFIND', headers: { ...headers, Depth: '0' } })
      assert.equal(root.status, 207)
    }
    const lazyLocal = await req('/api/local-store', { headers: user.headers })
    assert.equal(lazyLocal.status, 200)
    for (const root of [davRoot, localRoot]) {
      await mkdir(join(root, 'readcase', 'A-dir', 'deep'), { recursive: true })
      await mkdir(join(root, 'readcase', 'empty'))
      await mkdir(join(root, 'readcase', '.hidden'))
      await writeFile(join(root, 'readcase', 'A-dir', 'deep', 'child.txt'), payload)
      await writeFile(join(root, 'readcase', '.hidden', 'secret.txt'), 'hidden')
      await writeFile(join(root, 'readcase', root === localRoot ? localName : ' 中文 file.txt '), payload)
      await writeFile(join(root, 'readcase', 'z.txt'), payload)
      await writeFile(join(root, 'readcase', '000.txt'), payload)
      await chmod(join(root, 'readcase', '000.txt'), 0)
    }
    for (const prefix of ['/webdav', '/reader3/webdav']) {
      for (const headers of [user.headers, user.basic]) {
        for (const depth of ['0', '1', 'infinity']) {
          const response = await req(`${prefix}/readcase`, { method: 'PROPFIND', headers: { ...headers, Depth: depth } })
          assert.equal(response.status, 207)
          const xml = await response.text()
          assert.ok(xml.includes('DAV:'))
          assert.equal(xml.includes('z.txt'), depth !== '0')
          assert.ok(!xml.includes('child.txt'), 'DAV must not recurse')
          if (depth !== '0') assert.ok(xml.includes('000.txt'), '000 stat must not read content')
        }
        const directory = await req(`${prefix}/readcase`, { headers })
        assert.equal(directory.status, prefix === '/webdav' ? 207 : 405)
        const encoded = encodeURIComponent(' 中文 file.txt ')
        const file = `${prefix}/readcase/${encoded}`
        const normal = await req(file, { headers })
        assert.equal(normal.status, 200)
        assert.equal(await normal.text(), payload)
        const range = await req(file, { headers: { ...headers, Range: 'bytes=1-4' } })
        assert.equal(range.status, 206)
        assert.equal(await range.text(), '1234')
        const conditional = await req(file, { headers: { ...headers, 'If-Modified-Since': normal.headers.get('last-modified') } })
        assert.equal(conditional.status, 304)
        const invalid = await req(file, { headers: { ...headers, Range: 'bytes=10000-' } })
        assert.equal(invalid.status, 416)
        const missing = await req(`${prefix}/readcase/missing`, { headers })
        assert.equal(missing.status, 404)
      }
    }
    for (const recursive of ['0', '1', 'true']) {
      const response = await req(`/api/local-store?path=readcase&recursive=${recursive}`, { headers: user.headers })
      assert.equal(response.status, 200)
      const body = await response.json()
      assert.equal(body.recursive, recursive !== '0')
      const paths = body.items.map(item => item.path)
      assert.ok(!paths.some(path => path.includes('.hidden')))
      assert.equal(paths.includes('readcase/A-dir/deep/child.txt'), recursive !== '0')
      assert.ok(paths.includes(`readcase/${localName}`))
      const sorted = [...body.items].sort((a, b) => Number(b.isDir) - Number(a.isDir) ||
        (a.path.toLowerCase() < b.path.toLowerCase() ? -1 : a.path.toLowerCase() > b.path.toLowerCase() ? 1 : 0))
      assert.deepEqual(body.items, sorted)
    }
    const download = `/api/local-store/download?path=${encodeURIComponent(`readcase/${localName}`)}`
    for (const [extra, status, bytes] of [[{}, 200, payload], [{ Range: 'bytes=1-4' }, 206, '1234']]) {
      const response = await req(download, { headers: { ...user.headers, ...extra } })
      assert.equal(response.status, status)
      assert.ok(response.headers.get('content-disposition').includes('attachment'))
      assert.equal(await response.text(), bytes)
    }
    const missing = await req('/api/local-store?path=readcase/missing/child&recursive=1', { headers: user.headers })
    assert.equal(missing.status, 404)
    assert.deepEqual(await missing.json(), { error: 'local store path not found' })
    await assert.rejects(stat(join(localRoot, 'readcase', 'missing')), { code: 'ENOENT' })
    for (const root of [davRoot, localRoot]) {
      const outside = join(owned, `outside-${index}-${root === localRoot ? 'local' : 'dav'}.txt`)
      await writeFile(outside, 'foreign-secret-bytes')
      await symlink(outside, join(root, 'readcase', 'unsafe-link'))
      assert.equal((await stat(join(root, 'readcase', '000.txt'))).mode & 0o777, 0)
      assert.equal(await readFile(outside, 'utf8'), 'foreign-secret-bytes')
    }
    for (const prefix of ['/webdav', '/reader3/webdav']) {
      const response = await req(`${prefix}/readcase`, { method: 'PROPFIND', headers: user.basic })
      assert.equal(response.status, 403)
      assert.equal(await response.text(), '')
    }
    const safeList = await req('/api/local-store?path=readcase&recursive=1', { headers: user.headers })
    assert.equal(safeList.status, 200)
    assert.ok(!(await safeList.json()).items.some(item => item.name === 'unsafe-link'))
    console.log(`PASS ${user.username}: dual-prefix Basic/Bearer, Depth, directory GET, Unicode/space, Range/304/416, 000 metadata, recursive/hidden/sorted, missing404/no-write, unsafe policies and private bytes`)
  }
  // Existing protocol and LocalStore adjacent workflows on this owned fresh instance.
  await exec('sh', ['scripts/smoke/webdav-protocol-contract.sh'], { cwd: repo, env: { ...process.env, TARGET_URL: base } })
  await exec('node', ['scripts/smoke/local-store-directory-lifecycle-contract.mjs'], { cwd: repo, env: { ...process.env, TARGET_URL: base } })
  console.log('PASS actual Basic/curl DAV protocol and LocalStore directory/upload HTTP adjacency')
} catch (error) {
  error.message += `\nOwned diagnostic server output:\n${output}`
  throw error
} finally {
  if (child && child.exitCode === null) {
    const exited = new Promise(resolve => child.once('exit', resolve))
    child.kill('SIGTERM')
    await Promise.race([exited, new Promise(resolve => setTimeout(resolve, 5000))])
    if (child.exitCode === null) { child.kill('SIGKILL'); await exited }
  }
  await rm(owned, { recursive: true, force: true })
}

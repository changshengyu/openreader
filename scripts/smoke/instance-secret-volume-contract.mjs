import assert from 'node:assert/strict'
import { execFile, spawn } from 'node:child_process'
import { createCipheriv, createDecipheriv, createHash, createHmac, randomBytes } from 'node:crypto'
import { chmod, lstat, mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { request as httpRequest } from 'node:http'
import { createServer } from 'node:net'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { promisify } from 'node:util'
import { crc32, inflateRawSync } from 'node:zlib'

// Real-startup contract for the instance signing secret (S2): the deployed
// form with OPENREADER_JWT_SECRET unset, across one volume that an older
// release creates and the candidate then upgrades, restarts, rolls back from
// and upgrades again; plus fresh, published-default, custom and malformed
// volumes. Run it with either
//   CANDIDATE_IMAGE + PREVIOUS_IMAGE   Docker images (the release workflow), or
//   CANDIDATE_BIN + PREVIOUS_BIN       binaries built from this tree and from
//                                      a release before the instance secret.
// The previous release must still sign with the published default (92c98a9
// or earlier). Every process, container and directory used here is owned.

const exec = promisify(execFile)
const docker = Boolean(process.env.CANDIDATE_IMAGE || process.env.PREVIOUS_IMAGE)
const runtimes = docker
  ? { candidate: process.env.CANDIDATE_IMAGE, previous: process.env.PREVIOUS_IMAGE }
  : { candidate: process.env.CANDIDATE_BIN, previous: process.env.PREVIOUS_BIN }
assert.ok(runtimes.candidate && runtimes.previous,
  'set CANDIDATE_IMAGE and PREVIOUS_IMAGE, or CANDIDATE_BIN and PREVIOUS_BIN')

const published = ['change-this-before-deploy', 'change-me-in-production']
const secretName = 'instance-secret'
const secretFormat = /^[0-9a-f]{64}\n$/
const cbzPurpose = 'openreader:cbz-resource:v1'
const coverPurpose = 'openreader:cover-image:v1'
const password = 'instance-secret-contract-pass'
const owned = await mkdtemp(join(tmpdir(), 'openreader-instance-secret-'))
const running = new Set()
let containers = 0

async function freePort() {
  const reservation = createServer()
  await new Promise(resolve => reservation.listen(0, '127.0.0.1', resolve))
  const { port } = reservation.address()
  await new Promise(resolve => reservation.close(resolve))
  return port
}

async function newVolume(label) {
  const root = join(owned, label)
  const volume = {
    label, root, data: join(root, 'data'), cache: join(root, 'cache'),
    library: join(root, 'library'), publicDir: join(root, 'public'),
  }
  for (const directory of [volume.data, volume.cache, volume.library, volume.publicDir]) {
    await mkdir(directory, { recursive: true })
  }
  return volume
}

function binaryEnv(volume, port, secret) {
  const env = Object.fromEntries(Object.entries(process.env).filter(([key]) => !key.startsWith('OPENREADER_')))
  Object.assign(env, {
    OPENREADER_ADDR: `127.0.0.1:${port}`, OPENREADER_DATA_DIR: volume.data,
    OPENREADER_DB: join(volume.data, 'openreader.db'), OPENREADER_CACHE_DIR: volume.cache,
    OPENREADER_LIBRARY_DIR: volume.library, OPENREADER_LOCAL_STORE_DIR: join(volume.library, 'localStore'),
    OPENREADER_PUBLIC_DIR: volume.publicDir, OPENREADER_CHECK_INTERVAL: '24h',
  })
  if (secret !== undefined) env.OPENREADER_JWT_SECRET = secret
  return env
}

function dockerRunArgs(name, volume, port, secret, image) {
  const args = [
    'run', '--name', name, '-p', `127.0.0.1:${port}:8080`,
    '-e', 'OPENREADER_ADDR=:8080', '-e', 'OPENREADER_DATA_DIR=/app/data',
    '-e', 'OPENREADER_CACHE_DIR=/app/cache', '-e', 'OPENREADER_LIBRARY_DIR=/app/library',
    '-v', `${volume.data}:/app/data`, '-v', `${volume.cache}:/app/cache`, '-v', `${volume.library}:/app/library`,
  ]
  if (secret !== undefined) args.push('-e', `OPENREADER_JWT_SECRET=${secret}`)
  args.push(image)
  return args
}

async function waitHealthy(instance) {
  const deadline = Date.now() + 60_000
  while (true) {
    try {
      if ((await fetch(`${instance.base}/api/health`, { signal: AbortSignal.timeout(2_000) })).ok) return
    } catch {}
    if (await instance.exitedEarly()) {
      throw new Error(`${instance.kind} on ${instance.volume.label} exited before it was healthy:\n${await instance.logs()}`)
    }
    assert.ok(Date.now() < deadline, `${instance.kind} on ${instance.volume.label} did not become healthy:\n${await instance.logs()}`)
    await new Promise(resolve => setTimeout(resolve, 200))
  }
}

async function start(kind, volume, { secret } = {}) {
  const port = await freePort()
  const instance = { kind, volume, port, base: `http://127.0.0.1:${port}`, output: '' }
  if (docker) {
    instance.name = `openreader-instance-secret-${process.pid}-${++containers}`
    await exec('docker', ['run', '-d', ...dockerRunArgs(instance.name, volume, port, secret, runtimes[kind]).slice(1)])
    instance.logs = async () => {
      try {
        const { stdout, stderr } = await exec('docker', ['logs', instance.name], { maxBuffer: 16 << 20 })
        instance.output = stdout + stderr
      } catch {}
      return instance.output
    }
    instance.exitedEarly = async () => {
      const { stdout } = await exec('docker', ['inspect', '-f', '{{.State.Running}}', instance.name])
      return stdout.trim() !== 'true'
    }
    instance.stop = async () => {
      running.delete(instance)
      await exec('docker', ['stop', '-t', '20', instance.name]).catch(() => {})
      await instance.logs()
      await exec('docker', ['rm', '-f', instance.name]).catch(() => {})
    }
  } else {
    const child = spawn(runtimes[kind], [], { env: binaryEnv(volume, port, secret), stdio: ['ignore', 'pipe', 'pipe'] })
    child.stdout.on('data', chunk => { instance.output += chunk })
    child.stderr.on('data', chunk => { instance.output += chunk })
    const exited = new Promise(resolve => child.once('exit', code => resolve(code)))
    instance.logs = async () => instance.output
    instance.exitedEarly = async () => child.exitCode !== null || child.signalCode !== null
    instance.stop = async () => {
      running.delete(instance)
      if (child.exitCode === null && child.signalCode === null) child.kill('SIGTERM')
      await exited
    }
  }
  running.add(instance)
  await waitHealthy(instance)
  return instance
}

// startFailure runs a server that must refuse to start and returns its exit
// code and output.
async function startFailure(kind, volume) {
  const port = await freePort()
  if (docker) {
    const name = `openreader-instance-secret-${process.pid}-${++containers}`
    try {
      await exec('docker', dockerRunArgs(name, volume, port, undefined, runtimes[kind]), { timeout: 60_000, maxBuffer: 16 << 20 })
      return { code: 0, output: '' }
    } catch (error) {
      return { code: error.killed ? null : error.code, output: `${error.stdout || ''}${error.stderr || ''}` }
    } finally {
      await exec('docker', ['rm', '-f', name]).catch(() => {})
    }
  }
  const child = spawn(runtimes[kind], [], { env: binaryEnv(volume, port), stdio: ['ignore', 'pipe', 'pipe'] })
  let output = ''
  child.stdout.on('data', chunk => { output += chunk })
  child.stderr.on('data', chunk => { output += chunk })
  const code = await Promise.race([
    new Promise(resolve => child.once('exit', code => resolve(code))),
    new Promise(resolve => setTimeout(() => resolve(null), 30_000)),
  ])
  if (code === null) child.kill('SIGKILL')
  return { code, output }
}

async function secretInfo(volume) {
  try {
    return await lstat(join(volume.data, secretName))
  } catch (error) {
    if (error.code === 'ENOENT') return null
    throw error
  }
}

// Files written by a container are root-owned, so Docker mode reads through
// the candidate image instead of the host.
async function secretContent(volume) {
  if (!docker) return readFile(join(volume.data, secretName), 'utf8')
  const { stdout } = await exec('docker', [
    'run', '--rm', '--entrypoint', '/bin/cat', '-v', `${volume.data}:/check:ro`, runtimes.candidate, `/check/${secretName}`,
  ])
  return stdout
}

async function requireInstanceSecret(volume, label) {
  const info = await secretInfo(volume)
  assert.ok(info, `${label}: data/${secretName} was not created`)
  assert.ok(info.isFile(), `${label}: data/${secretName} is not a regular file`)
  assert.equal(info.mode & 0o777, 0o600, `${label}: data/${secretName} mode`)
  const content = await secretContent(volume)
  // Never let an assertion print the secret.
  assert.ok(secretFormat.test(content), `${label}: data/${secretName} is not 64 lowercase hex characters and a newline`)
  return { secret: content.trim(), ino: info.ino, mtimeMs: info.mtimeMs }
}

async function requireSameInstanceSecret(volume, expected, label) {
  const current = await requireInstanceSecret(volume, label)
  assert.ok(current.secret === expected.secret, `${label}: data/${secretName} changed`)
  assert.equal(current.ino, expected.ino, `${label}: data/${secretName} was replaced`)
  assert.equal(current.mtimeMs, expected.mtimeMs, `${label}: data/${secretName} was rewritten`)
}

async function requireLogsWithout(instance, secret, label) {
  const output = await instance.logs()
  assert.ok(!output.includes(secret), `${label}: server output contains the instance secret`)
}

async function call(instance, path, { method = 'GET', token, json, form, headers = {} } = {}) {
  const init = { method, headers: { ...headers }, redirect: 'manual', signal: AbortSignal.timeout(30_000) }
  if (token) init.headers.Authorization = `Bearer ${token}`
  if (json !== undefined) {
    init.headers['Content-Type'] = 'application/json'
    init.body = JSON.stringify(json)
  }
  if (form) init.body = form
  const response = await fetch(`${instance.base}${path}`, init)
  const body = Buffer.from(await response.arrayBuffer())
  return { status: response.status, body, text: body.toString('utf8'), json: () => JSON.parse(body.toString('utf8')) }
}

// raw sends path exactly as given; fetch would resolve dot segments first.
function raw(instance, method, path, headers = {}) {
  return new Promise((resolve, reject) => {
    const request = httpRequest({ host: '127.0.0.1', port: instance.port, method, path, headers, timeout: 30_000 }, response => {
      const chunks = []
      response.on('data', chunk => chunks.push(chunk))
      response.on('end', () => resolve({ status: response.statusCode, text: Buffer.concat(chunks).toString('utf8') }))
    })
    request.on('timeout', () => request.destroy(new Error(`${method} ${path} timed out`)))
    request.on('error', reject)
    request.end()
  })
}

function syncHandshake(instance, token) {
  return new Promise((resolve, reject) => {
    const request = httpRequest({
      host: '127.0.0.1', port: instance.port, path: `/ws/sync?token=${encodeURIComponent(token)}`, timeout: 15_000,
      headers: {
        Connection: 'Upgrade', Upgrade: 'websocket', 'Sec-WebSocket-Version': '13',
        'Sec-WebSocket-Key': randomBytes(16).toString('base64'),
      },
    })
    request.on('upgrade', (_, socket) => {
      socket.destroy()
      resolve(101)
    })
    request.on('response', response => {
      response.resume()
      resolve(response.statusCode)
    })
    request.on('timeout', () => request.destroy(new Error('/ws/sync handshake timed out')))
    request.on('error', reject)
    request.end()
  })
}

async function requireSessionAccepted(instance, token, userId, label) {
  const me = await call(instance, '/api/me', { token })
  assert.equal(me.status, 200, `${label}: GET /api/me`)
  assert.equal(me.json().id, userId, `${label}: GET /api/me user`)
  for (const prefix of ['/webdav/', '/reader3/webdav/']) {
    const response = await call(instance, prefix, { method: 'PROPFIND', token, headers: { Depth: '0' } })
    assert.equal(response.status, 207, `${label}: PROPFIND ${prefix}`)
  }
  assert.equal(await syncHandshake(instance, token), 101, `${label}: /ws/sync handshake`)
}

async function requireSessionRejected(instance, token, label) {
  assert.equal((await call(instance, '/api/me', { token })).status, 401, `${label}: GET /api/me`)
  for (const prefix of ['/webdav/', '/reader3/webdav/']) {
    const response = await call(instance, prefix, { method: 'PROPFIND', token, headers: { Depth: '0' } })
    assert.equal(response.status, 401, `${label}: PROPFIND ${prefix}`)
  }
  assert.equal(await syncHandshake(instance, token), 401, `${label}: /ws/sync handshake`)
}

async function register(instance, username) {
  const response = await call(instance, '/api/auth/register', { method: 'POST', json: { username, password } })
  assert.equal(response.status, 200, `register ${username}: ${response.text}`)
  const { token, user } = response.json()
  assert.ok(token && user?.id, `register ${username} returned no session`)
  return { token, id: user.id }
}

async function login(instance, username) {
  const response = await call(instance, '/api/auth/login', { method: 'POST', json: { username, password } })
  assert.equal(response.status, 200, `login ${username}: ${response.text}`)
  return response.json().token
}

function jwtSignedWith(token, secret) {
  const [header, payload, signature] = token.split('.')
  return createHmac('sha256', secret).update(`${header}.${payload}`).digest('base64url') === signature
}

function signJWT(claims, secret) {
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url')
  const payload = Buffer.from(JSON.stringify(claims)).toString('base64url')
  return `${header}.${payload}.${createHmac('sha256', secret).update(`${header}.${payload}`).digest('base64url')}`
}

function requireSignedOnlyByInstance(token, instance, label) {
  for (const value of published) assert.ok(!jwtSignedWith(token, value), `${label} verifies with ${value}`)
  assert.ok(jwtSignedWith(token, instance), `${label} is not signed with the instance secret`)
}

function capabilitySignature(secret, purpose, payload) {
  const key = createHmac('sha256', secret).update(purpose).digest()
  return createHmac('sha256', key).update(payload).digest('base64url')
}

function cbzCapability(resourceUrl) {
  const match = /^\/api\/cbz-resource\/([^/]+)\//.exec(resourceUrl)
  assert.ok(match, `unexpected CBZ resource URL ${resourceUrl}`)
  return decodeURIComponent(match[1])
}

function resignCBZLink(resourceUrl, secret) {
  const capability = cbzCapability(resourceUrl)
  const payload = capability.split('.')[0]
  return resourceUrl.replace(capability, `${payload}.${capabilitySignature(secret, cbzPurpose, payload)}`)
}

function coverKey(secret) {
  return createHash('sha256').update(`${coverPurpose}\0${secret}`).digest()
}

function sealCover(secret, claims) {
  const nonce = randomBytes(12)
  const cipher = createCipheriv('aes-256-gcm', coverKey(secret), nonce)
  cipher.setAAD(Buffer.from(coverPurpose))
  const sealed = Buffer.concat([cipher.update(JSON.stringify(claims)), cipher.final(), cipher.getAuthTag()])
  return `v1.${Buffer.concat([nonce, sealed]).toString('base64url')}`
}

function coverOpens(token, secret) {
  const sealed = Buffer.from(token.replace(/^v1\./, ''), 'base64url')
  try {
    const decipher = createDecipheriv('aes-256-gcm', coverKey(secret), sealed.subarray(0, 12))
    decipher.setAAD(Buffer.from(coverPurpose))
    decipher.setAuthTag(sealed.subarray(sealed.length - 16))
    decipher.update(sealed.subarray(12, sealed.length - 16))
    decipher.final()
    return true
  } catch {
    return false
  }
}

function storedZip(entries) {
  const locals = []
  const centrals = []
  let offset = 0
  for (const [name, content] of entries) {
    const nameBytes = Buffer.from(name)
    const data = Buffer.from(content)
    const checksum = crc32(data)
    const local = Buffer.alloc(30)
    local.writeUInt32LE(0x04034b50, 0)
    local.writeUInt16LE(20, 4)
    local.writeUInt16LE(0x21, 12)
    local.writeUInt32LE(checksum, 14)
    local.writeUInt32LE(data.length, 18)
    local.writeUInt32LE(data.length, 22)
    local.writeUInt16LE(nameBytes.length, 26)
    const central = Buffer.alloc(46)
    central.writeUInt32LE(0x02014b50, 0)
    central.writeUInt16LE(20, 4)
    central.writeUInt16LE(20, 6)
    central.writeUInt16LE(0x21, 14)
    central.writeUInt32LE(checksum, 16)
    central.writeUInt32LE(data.length, 20)
    central.writeUInt32LE(data.length, 24)
    central.writeUInt16LE(nameBytes.length, 28)
    central.writeUInt32LE(offset, 42)
    locals.push(local, nameBytes, data)
    centrals.push(central, nameBytes)
    offset += local.length + nameBytes.length + data.length
  }
  const directory = Buffer.concat(centrals)
  const end = Buffer.alloc(22)
  end.writeUInt32LE(0x06054b50, 0)
  end.writeUInt16LE(entries.length, 8)
  end.writeUInt16LE(entries.length, 10)
  end.writeUInt32LE(directory.length, 12)
  end.writeUInt32LE(offset, 16)
  return Buffer.concat([...locals, directory, end])
}

function zipEntries(archive) {
  let end = archive.length - 22
  while (end >= 0 && archive.readUInt32LE(end) !== 0x06054b50) end--
  assert.ok(end >= 0, 'backup is not a zip archive')
  const count = archive.readUInt16LE(end + 10)
  let cursor = archive.readUInt32LE(end + 16)
  const entries = new Map()
  for (let index = 0; index < count; index++) {
    assert.equal(archive.readUInt32LE(cursor), 0x02014b50, 'backup central directory is corrupt')
    const method = archive.readUInt16LE(cursor + 10)
    const size = archive.readUInt32LE(cursor + 20)
    const nameLength = archive.readUInt16LE(cursor + 28)
    const extraLength = archive.readUInt16LE(cursor + 30)
    const commentLength = archive.readUInt16LE(cursor + 32)
    const localOffset = archive.readUInt32LE(cursor + 42)
    const name = archive.subarray(cursor + 46, cursor + 46 + nameLength).toString('utf8')
    const dataStart = localOffset + 30 + archive.readUInt16LE(localOffset + 26) + archive.readUInt16LE(localOffset + 28)
    const data = archive.subarray(dataStart, dataStart + size)
    assert.ok(method === 0 || method === 8, `backup entry ${name} uses zip method ${method}`)
    entries.set(name, method === 8 ? inflateRawSync(data) : data)
    cursor += 46 + nameLength + extraLength + commentLength
  }
  return entries
}

async function importCBZ(instance, token) {
  const form = new FormData()
  const archive = storedZip([
    ['ComicInfo.xml', '<ComicInfo><Title>实例密钥漫画</Title></ComicInfo>'],
    ['pages/001.jpg', 'instance-secret-first-page'],
  ])
  form.append('file', new Blob([archive], { type: 'application/zip' }), 'instance-secret.cbz')
  form.append('title', '实例密钥漫画')
  const response = await call(instance, '/api/imports/books', { method: 'POST', token, form })
  assert.equal(response.status, 201, `CBZ import: ${response.text}`)
  return response.json().id
}

async function cbzLink(instance, token, bookId) {
  const response = await call(instance, `/api/books/${bookId}/chapters/0/content`, { token })
  assert.equal(response.status, 200, `CBZ chapter content: ${response.text}`)
  const { format, resourceUrl } = response.json()
  assert.equal(format, 'cbz')
  return resourceUrl
}

async function requireLinkServed(instance, link, label) {
  const response = await call(instance, link)
  assert.equal(response.status, 200, `${label}: status`)
  assert.equal(response.text, 'instance-secret-first-page', `${label}: body`)
}

async function requireNotExposed(instance, token, secret) {
  const authorization = { Authorization: `Bearer ${token}` }
  for (const prefix of ['/webdav', '/reader3/webdav']) {
    for (const depth of ['1', 'infinity']) {
      const listing = await call(instance, `${prefix}/`, { method: 'PROPFIND', token, headers: { Depth: depth } })
      assert.equal(listing.status, 207, `PROPFIND ${prefix}/ Depth ${depth}`)
      assert.ok(!listing.text.includes(secretName) && !listing.text.includes(secret), `PROPFIND ${prefix}/ lists data/${secretName}`)
    }
    for (const path of ['/../instance-secret', '/..%2finstance-secret', '/%2e%2e/instance-secret', '/../../data/instance-secret']) {
      for (const method of ['GET', 'PROPFIND']) {
        const response = await raw(instance, method, `${prefix}${path}`, { ...authorization, Depth: '0' })
        assert.ok(!response.text.includes(secret), `${method} ${prefix}${path} returned the instance secret`)
      }
    }
  }
  for (const path of ['/uploads/../instance-secret', '/uploads/%2e%2e/instance-secret', '/uploads/..%2finstance-secret',
    '/uploads/users/../../instance-secret']) {
    const response = await raw(instance, 'GET', path)
    assert.ok(!response.text.includes(secret), `GET ${path} returned the instance secret`)
  }
  const dataPath = docker ? '/app/data/instance-secret' : join(instance.volume.data, secretName)
  for (const path of ['../../data/instance-secret', '../data/instance-secret', dataPath]) {
    const download = await call(instance, `/api/local-store/download?path=${encodeURIComponent(path)}`, { token })
    assert.ok(!download.text.includes(secret), `local store download ${path} returned the instance secret`)
  }
  for (const name of ['..%2finstance-secret', '%2e%2e%2finstance-secret', '..%2f..%2finstance-secret']) {
    const response = await raw(instance, 'GET', `/api/backup/download/${name}`, authorization)
    assert.ok(!response.text.includes(secret), `backup download ${name} returned the instance secret`)
  }
  for (const trigger of ['/api/backup/trigger', '/api/backup/portable/trigger']) {
    const created = await call(instance, trigger, { method: 'POST', token })
    assert.equal(created.status, 200, `${trigger}: ${created.text}`)
    const { name } = created.json()
    const download = await call(instance, `/api/backup/download/${encodeURIComponent(name)}`, { token })
    assert.equal(download.status, 200, `download ${name}`)
    for (const [entry, content] of zipEntries(download.body)) {
      assert.ok(!entry.includes(secretName) && !content.includes(secret), `backup ${name} entry ${entry} carries the instance secret`)
    }
  }
}

try {
  if (docker) await exec('docker', ['pull', '--quiet', runtimes.previous], { maxBuffer: 16 << 20 })

  // historical: a release from before the instance secret signs with the
  // published default.
  const volume = await newVolume('lifecycle')
  let server = await start('previous', volume)
  assert.equal(await secretInfo(volume), null, 'the previous release created data/instance-secret')
  const owner = await register(server, 'instanceowner')
  assert.ok(jwtSignedWith(owner.token, published[0]), 'the previous release does not sign with the published default')
  const previousSession = owner.token
  const bookId = await importCBZ(server, previousSession)
  const previousLink = await cbzLink(server, previousSession, bookId)
  await requireLinkServed(server, previousLink, 'previous release CBZ link on the previous release')
  await server.stop()

  // upgrade
  server = await start('candidate', volume)
  const created = await requireInstanceSecret(volume, 'upgrade')
  const instanceSecret = created.secret
  assert.ok((await server.logs()).includes(secretName), 'upgrade: no startup log names the instance secret file')
  await requireSessionAccepted(server, previousSession, owner.id, 'session from the previous release after the upgrade')
  assert.equal((await call(server, previousLink)).status, 403, 'CBZ link from the previous release after the upgrade')
  const candidateSession = await login(server, 'instanceowner')
  requireSignedOnlyByInstance(candidateSession, instanceSecret, 'candidate session')
  const candidateLink = await cbzLink(server, candidateSession, bookId)
  await requireLinkServed(server, candidateLink, 'candidate CBZ link')
  const candidateCapability = cbzCapability(candidateLink)
  for (const value of published) {
    const payload = candidateCapability.split('.')[0]
    assert.notEqual(capabilitySignature(value, cbzPurpose, payload), candidateCapability.split('.')[1], `candidate CBZ link is signed with ${value}`)
    assert.equal((await call(server, resignCBZLink(candidateLink, value))).status, 403, `CBZ link re-signed with ${value}`)
    const forgedCover = sealCover(value, { v: 1, p: coverPurpose, u: 987654, r: 'https://cover.invalid/forged.png', e: Math.floor(Date.now() / 1000) + 3600 })
    assert.equal((await call(server, `/api/cover/${encodeURIComponent(forgedCover)}`)).status, 403, `cover link sealed with ${value}`)
    const legacy = signJWT({ userId: owner.id, iat: Date.UTC(2020, 0, 1) / 1000 }, value)
    await requireSessionRejected(server, legacy, `legacy token signed with ${value}`)
  }
  // exposure (before the cover book below: portable backups refuse a local
  // shelf entry that has no archive)
  await requireNotExposed(server, candidateSession, instanceSecret)

  const coverBook = await call(server, '/api/books', {
    method: 'POST', token: candidateSession, json: { title: '实例密钥封面', coverUrl: 'https://cover.invalid/instance-secret.png' },
  })
  assert.equal(coverBook.status, 201, `create cover book: ${coverBook.text}`)
  const shelf = (await call(server, '/api/books', { token: candidateSession })).json()
  const coverLink = shelf.find(book => book.title === '实例密钥封面')?.coverResourceUrl || ''
  assert.match(coverLink, /^\/api\/cover\/v1\./, 'candidate cover link')
  const coverToken = decodeURIComponent(coverLink.slice('/api/cover/'.length))
  for (const value of published) assert.ok(!coverOpens(coverToken, value), `candidate cover link opens with ${value}`)
  assert.ok(coverOpens(coverToken, instanceSecret), 'candidate cover link is not sealed with the instance secret')
  await server.stop()
  await requireLogsWithout(server, instanceSecret, 'upgrade')

  // restart
  server = await start('candidate', volume)
  await requireSameInstanceSecret(volume, created, 'restart')
  await requireSessionAccepted(server, candidateSession, owner.id, 'candidate session after a restart')
  await requireSessionAccepted(server, previousSession, owner.id, 'previous-release session after a restart')
  await requireLinkServed(server, candidateLink, 'candidate CBZ link after a restart')
  await server.stop()
  await requireLogsWithout(server, instanceSecret, 'restart')

  // rollback: the previous release neither knows nor touches the file.
  server = await start('previous', volume)
  await requireSameInstanceSecret(volume, created, 'rollback')
  await requireSessionRejected(server, candidateSession, 'candidate session on the previous release')
  await requireSessionAccepted(server, previousSession, owner.id, 'previous-release session after the rollback')
  assert.equal((await call(server, candidateLink)).status, 403, 'candidate CBZ link on the previous release')
  const rollbackSession = await login(server, 'instanceowner')
  assert.ok(jwtSignedWith(rollbackSession, published[0]), 'the previous release signed a rollback session without the published default')
  await server.stop()

  // re-upgrade
  server = await start('candidate', volume)
  await requireSameInstanceSecret(volume, created, 're-upgrade')
  await requireSessionAccepted(server, candidateSession, owner.id, 'candidate session after the re-upgrade')
  await requireSessionAccepted(server, previousSession, owner.id, 'previous-release session after the re-upgrade')
  await requireSessionAccepted(server, rollbackSession, owner.id, 'rollback session after the re-upgrade')
  requireSignedOnlyByInstance(await login(server, 'instanceowner'), instanceSecret, 'session after the re-upgrade')
  await server.stop()
  await requireLogsWithout(server, instanceSecret, 're-upgrade')

  // fresh install, and the other published default configured explicitly
  for (const [label, secret] of [['fresh', undefined], ['previous-default', published[1]]]) {
    const freshVolume = await newVolume(label)
    const fresh = await start('candidate', freshVolume, { secret })
    const { secret: freshSecret } = await requireInstanceSecret(freshVolume, label)
    assert.notEqual(freshSecret, instanceSecret, `${label}: two volumes share an instance secret`)
    const admin = await register(fresh, 'freshadmin')
    assert.equal((await call(fresh, '/api/me', { token: admin.token })).json().role, 'admin', `${label}: first account role`)
    requireSignedOnlyByInstance(admin.token, freshSecret, `${label} session`)
    for (const value of published) {
      await requireSessionRejected(fresh, signJWT({ userId: admin.id, iat: Date.UTC(2020, 0, 1) / 1000 }, value), `${label}: legacy token signed with ${value}`)
    }
    await fresh.stop()
    await requireLogsWithout(fresh, freshSecret, label)
  }

  // a custom secret keeps today's behaviour and creates nothing
  const customVolume = await newVolume('custom')
  const customSecret = 'instance-secret-contract-custom-value'
  const custom = await start('candidate', customVolume, { secret: customSecret })
  assert.equal(await secretInfo(customVolume), null, 'a custom secret created data/instance-secret')
  const customUser = await register(custom, 'customowner')
  assert.ok(jwtSignedWith(customUser.token, customSecret), 'custom session is not signed with the custom secret')
  await custom.stop()

  // a malformed file stops the start without being changed or printed
  const malformedVolume = await newVolume('malformed')
  const malformedPath = join(malformedVolume.data, secretName)
  await writeFile(malformedPath, 'not-an-instance-secret\n', { mode: 0o600 })
  await chmod(malformedPath, 0o600)
  const failure = await startFailure('candidate', malformedVolume)
  assert.ok(failure.code !== 0 && failure.code !== null, `malformed data/${secretName}: exit code ${failure.code}`)
  assert.match(failure.output, /instance-secret/, 'malformed data/instance-secret: the error does not name the file')
  assert.ok(!failure.output.includes('not-an-instance-secret'), 'malformed data/instance-secret: the error prints its content')
  assert.equal(await readFile(malformedPath, 'utf8'), 'not-an-instance-secret\n', 'malformed data/instance-secret was changed')
  await assert.rejects(lstat(join(malformedVolume.data, 'openreader.db')), { code: 'ENOENT' },
    'malformed data/instance-secret: the database was opened before the start was refused')

  console.log(`instance secret volume contract passed (${docker ? 'docker' : 'binary'}: historical, upgrade, exposure, restart, rollback, re-upgrade, fresh, previous-default, custom, malformed)`)
} finally {
  for (const instance of [...running]) await instance.stop().catch(() => {})
  if (docker) {
    await exec('docker', ['run', '--rm', '--entrypoint', '/bin/sh', '-v', `${owned}:/cleanup`, runtimes.candidate,
      '-c', 'chmod -R a+rwX /cleanup']).catch(() => {})
  }
  await rm(owned, { recursive: true, force: true })
}

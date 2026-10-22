import assert from 'node:assert/strict'
import { execFile } from 'node:child_process'
import { readFile } from 'node:fs/promises'
import { createServer } from 'node:http'
import test, { after } from 'node:test'
import { fileURLToPath } from 'node:url'

// Wiring of the release browser gate, plus the precision of the checker it runs
// (openreader-smoke.mjs as a child process, exactly as the gate does) on owned
// loopback pages.
const read = path => readFile(new URL(path, import.meta.url), 'utf8')
const checker = fileURLToPath(new URL('./openreader-smoke.mjs', import.meta.url))

test('release workflow installs the pinned browser and loads the native validation image before publish', async () => {
  const source = await read('../../.github/workflows/docker-publish.yml')
  const triggers = source.slice(0, source.indexOf('workflow_dispatch:'))
  for (const path of ['package.json', 'package-lock.json', 'scripts/smoke/openreader-smoke.mjs', 'scripts/smoke/playwright-runtime.mjs',
    'scripts/smoke/release-browser-smoke.sh', 'scripts/smoke/release-browser-smoke.test.mjs'])
    assert.ok(triggers.includes(`- ${path}\n`), path)
  const at = text => {const index = source.indexOf(text); assert.ok(index > 0, text); return index}
  const install = at('run: npm ci && npm run smoke:install-browser')
  const tests = at('run: node --test scripts/smoke/release-browser-smoke.test.mjs')
  const amd64 = at('- name: Build native validation image'), gate = at('run: ./scripts/smoke/release-browser-smoke.sh')
  const arm64 = at('- name: Build arm64 validation image')
  const login = at('- name: Log in to GHCR'), publish = at('- name: Build and publish multi-platform image')
  assert.ok(install < tests && tests < amd64 && amd64 < gate && gate < arm64 && arm64 < login && login < publish)
  const gateStep = source.slice(source.lastIndexOf('\n      - name:', gate), source.indexOf('\n      - name:', gate))
  assert.ok(gateStep.includes('IMAGE: openreader-ci:${{ github.sha }}'), gateStep)
  assert.ok(!source.includes('SMOKE_PUBLIC_DIR'))
  assert.ok(!/continue-on-error|\n\s+if:/.test(source.slice(install, publish)))
})

test('smoke browser is pinned by the root lockfile', async () => {
  const manifest = JSON.parse(await read('../../package.json'))
  const lock = JSON.parse(await read('../../package-lock.json'))
  const version = manifest.devDependencies.playwright
  assert.match(version, /^\d+\.\d+\.\d+$/)
  assert.equal(lock.packages[''].devDependencies.playwright, version)
  for (const name of ['playwright', 'playwright-core']) {
    const entry = lock.packages[`node_modules/${name}`]
    assert.equal(entry.version, version, name)
    assert.match(entry.integrity, /^sha512-/, name)
  }
  assert.equal(manifest.scripts['smoke:install-browser'], 'playwright install chromium-headless-shell')
})

test('gate script loads only a fresh loopback container in the pinned headless shell', async () => {
  const script = await read('./release-browser-smoke.sh')
  assert.ok(script.includes('docker run -d --name "$NAME" -p 127.0.0.1::8080'))
  assert.ok(script.includes('env -u CDP_URL -u CHROME_PATH -u SMOKE_READER_URL TARGET_URL="$BASE_URL"'))
  const mounts = script.split('\n').filter(line => /\s-v\s|--volume|--mount/.test(line))
  assert.deepEqual(mounts, ['  set -- "$@" -v "$(cd "$SMOKE_PUBLIC_DIR" && pwd):/app/public:ro"'])
})

const rendered = '<div id="app"><main><h1>Owned page</h1><p>rendered by the app</p></main></div>'
const pages = {
  '/rendered/': rendered,
  '/throws/': '<div id="app"></div><script>throw new TypeError("e is not a function")</script>',
  '/outside/': '<div id="app"></div><p>static text outside the app root</p>',
  '/console/': `${rendered}<script>console.error("owned console failure")</script>`,
  '/api-5xx/': `${rendered}<script>fetch("/api/owned-failure")</script>`,
  '/no-app/': '<main><h1>404 page not found</h1></main>',
}
const server = createServer((request, response) => {
  if (request.url.startsWith('/api/')) return response.writeHead(500).end('owned failure')
  if (!pages[request.url]) return response.writeHead(404).end()
  response.writeHead(200, {'Content-Type': 'text/html; charset=utf-8'}).end('<!doctype html><html><head><meta charset="utf-8">' +
    `<link rel="icon" href="data:,"><title>owned</title></head><body>${pages[request.url]}</body></html>`)
})
const listening = new Promise(resolve => server.listen(0, '127.0.0.1', resolve))
after(() => {server.closeAllConnections(); return new Promise(resolve => server.close(resolve))})

async function runChecker(path) {
  await listening
  const env = {...process.env, TARGET_URL: `http://127.0.0.1:${server.address().port}${path}`}
  for (const name of ['CDP_URL', 'CHROME_PATH', 'SMOKE_READER_URL']) delete env[name]
  return new Promise(resolve => execFile(process.execPath, [checker], {env, timeout: 120_000},
    (error, stdout, stderr) => resolve({code: error ? error.code : 0, stdout, stderr})))
}
let renderedRun
const renderedResult = () => (renderedRun ??= runChecker('/rendered/'))

test('checker passes a rendered app at the three release viewports', async () => {
  const {code, stdout, stderr} = await renderedResult()
  assert.equal(code, 0, stderr)
  for (const label of ['desktop', 'mobile-390', 'mobile-360'])
    assert.match(stdout, new RegExp(`^${label}\\b[^:]*: ok /rendered/ `, 'm'))
})

test('checker reports the size of each viewport it loaded', async () => {
  const {stdout} = await renderedResult()
  for (const label of ['desktop 1440x900', 'mobile-390 390x844', 'mobile-360 360x800'])
    assert.match(stdout, new RegExp(`^${label}: ok /rendered/ `, 'm'))
})

test('checker fails an app that never mounts and names the page error', async () => {
  const {code, stderr} = await runChecker('/throws/')
  assert.equal(code, 1)
  assert.match(stderr, /desktop 1440x900: empty #app at \/throws\//)
  assert.match(stderr, /pageerror: e is not a function/)
})

test('checker fails an empty #app even when markup outside it is visible', async () => {
  const {code, stdout, stderr} = await runChecker('/outside/')
  assert.equal(code, 1, stdout)
  assert.match(stderr, /desktop 1440x900: empty #app at \/outside\//)
})

for (const [path, signal] of [
  ['/console/', /console\.error: owned console failure/],
  ['/api-5xx/', /api 500: http:\/\/127\.0\.0\.1:\d+\/api\/owned-failure/],
  ['/no-app/', /missing #app at \/no-app\//],
]) test(`checker fails closed on ${path}`, async () => {
  const {code, stderr} = await runChecker(path)
  assert.equal(code, 1)
  assert.match(stderr, signal)
})

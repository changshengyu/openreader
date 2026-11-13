import assert from 'node:assert/strict'
import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'
import vm from 'node:vm'

// PWA service worker contract W2/W5/W14/W21 (docs/compat/pwa-service-worker-fixed-baseline-second-audit-p2-contract.md):
// `npm run build` turns the Vite output into a same-origin precache manifest and a
// build-scoped worker script, with nothing imported from another origin.

async function loadBuilder() {
  try {
    return await import('../pwa/build-service-worker.mjs')
  } catch (error) {
    assert.fail(`frontend/pwa/build-service-worker.mjs must generate the offline-shell service worker: ${error.message}`)
  }
}

async function writeDistFixture() {
  const root = await mkdtemp(join(tmpdir(), 'openreader-sw-build-'))
  const files = {
    'index.html': '<!doctype html><script type="module" src="/assets/index-AAA.js"></script>',
    'assets/index-AAA.js': 'import("./Reader-BBB.js")',
    'assets/Reader-BBB.js': 'export default {}',
    'assets/index-AAA.js.map': '{"version":3}',
    'bg/山水画.jpg': 'jpeg bytes',
    'themes/content_0.png': 'png bytes',
    'manifest.webmanifest': '{"name":"OpenReader"}',
    'openreader.svg': '<svg xmlns="http://www.w3.org/2000/svg"></svg>',
    'sw.js': 'stale worker from a previous build',
    '.DS_Store': 'finder metadata',
  }
  for (const [path, body] of Object.entries(files)) {
    const target = join(root, ...path.split('/'))
    await mkdir(join(target, '..'), { recursive: true })
    await writeFile(target, body)
  }
  return root
}

test('collects every build file except the worker, source maps and dotfiles, in URL form', async t => {
  const builder = await loadBuilder()
  const dist = await writeDistFixture()
  t.after(() => rm(dist, { recursive: true, force: true }))

  const files = await builder.collectPrecacheFiles(dist)
  assert.deepEqual(files, [
    'assets/Reader-BBB.js',
    'assets/index-AAA.js',
    'bg/山水画.jpg',
    'index.html',
    'manifest.webmanifest',
    'openreader.svg',
    'themes/content_0.png',
  ])
  assert.deepEqual(files.map(builder.precacheURL), [
    '/assets/Reader-BBB.js',
    '/assets/index-AAA.js',
    '/bg/%E5%B1%B1%E6%B0%B4%E7%94%BB.jpg',
    '/index.html',
    '/manifest.webmanifest',
    '/openreader.svg',
    '/themes/content_0.png',
  ])
})

test('writes dist/sw.js with a build id derived from the build contents', async t => {
  const builder = await loadBuilder()
  const dist = await writeDistFixture()
  t.after(() => rm(dist, { recursive: true, force: true }))

  const first = await builder.writeServiceWorker(dist)
  assert.match(first.buildId, /^[0-9a-f]{16}$/)
  assert.ok(first.precache.includes('/index.html'), 'the offline shell document must be precached')
  assert.ok(first.precache.includes('/assets/Reader-BBB.js'), 'lazy route chunks must be precached (W21)')
  assert.ok(!first.precache.includes('/sw.js'), 'the worker must not precache itself')

  const source = await readFile(join(dist, 'sw.js'), 'utf8')
  assert.ok(source.includes(JSON.stringify(first.buildId)), 'the build id must change the worker bytes so browsers see the update')
  assert.ok(source.includes('"/assets/index-AAA.js"'))

  const second = await builder.writeServiceWorker(dist)
  assert.equal(second.buildId, first.buildId, 'regenerating over its own output must be stable')

  await writeFile(join(dist, 'bg', '山水画.jpg'), 'changed jpeg bytes')
  const third = await builder.writeServiceWorker(dist)
  assert.notEqual(third.buildId, first.buildId, 'any changed build file must produce a new build id')
})

test('refuses to generate a worker for a build without index.html', async t => {
  const builder = await loadBuilder()
  const dist = await mkdtemp(join(tmpdir(), 'openreader-sw-empty-'))
  t.after(() => rm(dist, { recursive: true, force: true }))
  await writeFile(join(dist, 'openreader.svg'), '<svg></svg>')
  await assert.rejects(builder.writeServiceWorker(dist), /index\.html/)
})

test('the generated worker compiles and imports nothing from another origin', async () => {
  const builder = await loadBuilder()
  const source = await builder.renderServiceWorker({
    buildId: '0123456789abcdef',
    precache: ['/index.html', '/assets/index-AAA.js', '/assets/$&-literal.js'],
  })
  assert.doesNotThrow(() => new vm.Script(source, { filename: 'sw.js' }))
  assert.doesNotMatch(source, /importScripts/, 'no third-party Workbox runtime (W5)')
  assert.doesNotMatch(source, /https?:\/\//, 'the worker must not reference absolute URLs')
  assert.doesNotMatch(source, /__OPENREADER_SW_CONFIG__/, 'the config placeholder must be replaced')
  assert.ok(source.includes('"/assets/$&-literal.js"'), 'file names must be embedded literally')
  assert.ok(source.includes("'openreader-'"), 'every cache the worker owns carries the openreader- prefix')
})

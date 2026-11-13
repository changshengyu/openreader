import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

// PWA service worker contract T1–T3 wiring (docs/compat/pwa-service-worker-fixed-baseline-second-audit-p2-contract.md §6, §10).

const read = path => readFileSync(fileURLToPath(new URL(path, import.meta.url)), 'utf8')

test('npm run build generates the service worker from the finished Vite output', () => {
  const pkg = JSON.parse(read('../package.json'))
  assert.equal(pkg.scripts.build, 'vite build && node pwa/build-service-worker.mjs')
})

test('main.js sets up the service worker after mounting, gated on production builds', () => {
  const main = read('../src/main.js')
  assert.match(main, /import \{ setupServiceWorker \} from '\.\/utils\/serviceWorker'/)
  const mount = main.indexOf("app.mount('#app')")
  const setup = main.indexOf('setupServiceWorker({ isProduction: import.meta.env.PROD })')
  assert.ok(mount !== -1 && setup > mount, 'registration must not delay the first render')
})

test('the version button shows build info first, then forces the service worker update', () => {
  const layout = read('../src/layouts/AppLayout.vue')
  assert.match(layout, /import \{ forceServiceWorkerUpdate \} from '\.\.\/utils\/serviceWorker'/)
  assert.match(layout, /<button class="app-version-text" type="button" @click\.stop="updateAppVersion">\{\{ appVersionLabel \}\}<\/button>/)
  assert.match(
    layout,
    /async function updateAppVersion\(\) \{\n {2}await refreshHealthInfo\(true\)\n {2}await forceServiceWorkerUpdate\(\)\n\}/,
  )
  assert.match(
    layout,
    /\{ key: 'backendStatus', label: [^\n]*action: \(\) => refreshHealthInfo\(true\) \}/,
    'the backend status action stays a health check only (settings contract §2.2)',
  )
})

test('existing browser smokes keep service workers blocked unless a contract opts in', () => {
  const runtime = read('../../scripts/smoke/playwright-runtime.mjs')
  assert.match(runtime, /newContext\(\{ serviceWorkers: 'block', \.\.\.options \}\)/)
  assert.match(runtime, /newPage\(\{ serviceWorkers: 'block', \.\.\.options \}\)/)
  const contract = read('../../scripts/smoke/pwa-service-worker-contract.mjs')
  assert.match(contract, /serviceWorkers: 'allow'/)
})

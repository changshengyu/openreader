// Generates dist/sw.js after `vite build` (run by `npm run build`). The worker
// precaches every file of this build except itself, source maps and dotfiles, and
// embeds a content-derived build id so browsers detect each release as an update.
// See docs/compat/pwa-service-worker-fixed-baseline-second-audit-p2-contract.md.

import { createHash } from 'node:crypto'
import { readdir, readFile, writeFile } from 'node:fs/promises'
import { join, relative, resolve, sep } from 'node:path'
import { fileURLToPath } from 'node:url'

export const SERVICE_WORKER_FILE = 'sw.js'
const CONFIG_PLACEHOLDER = '__OPENREADER_SW_CONFIG__'
const TEMPLATE_URL = new URL('./service-worker.js', import.meta.url)
const URL_BASE = 'https://openreader.invalid/'

function isPrecacheable(path) {
  const name = path.split('/').pop()
  return path !== SERVICE_WORKER_FILE && !name.startsWith('.') && !name.endsWith('.map')
}

async function listFiles(directory) {
  const files = []
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const path = join(directory, entry.name)
    if (entry.isDirectory()) files.push(...await listFiles(path))
    else if (entry.isFile()) files.push(path)
  }
  return files
}

export async function collectPrecacheFiles(distDir) {
  const root = resolve(distDir)
  return (await listFiles(root))
    .map(path => relative(root, path).split(sep).join('/'))
    .filter(isPrecacheable)
    .sort()
}

// The pathname a browser reports for the file, e.g. bg/山水画.jpg → /bg/%E5%B1%B1….jpg.
export function precacheURL(path) {
  return new URL(path, URL_BASE).pathname
}

export async function computeBuildId(distDir, files) {
  const build = createHash('sha256')
  for (const path of files) {
    const content = await readFile(join(resolve(distDir), ...path.split('/')))
    build.update(path).update('\0').update(createHash('sha256').update(content).digest('hex')).update('\n')
  }
  return build.digest('hex').slice(0, 16)
}

export async function renderServiceWorker({ buildId, precache }) {
  const template = await readFile(TEMPLATE_URL, 'utf8')
  if (!template.includes(CONFIG_PLACEHOLDER)) throw new Error('service worker template lost its config placeholder')
  const config = JSON.stringify({ buildId, precache })
  return template.replace(CONFIG_PLACEHOLDER, () => config)
}

export async function writeServiceWorker(distDir) {
  const files = await collectPrecacheFiles(distDir)
  if (!files.includes('index.html')) {
    throw new Error(`${distDir} has no index.html; run vite build before generating the service worker`)
  }
  const buildId = await computeBuildId(distDir, files)
  const precache = files.map(precacheURL)
  await writeFile(join(resolve(distDir), SERVICE_WORKER_FILE), await renderServiceWorker({ buildId, precache }))
  return { buildId, precache }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const distDir = process.argv[2] || fileURLToPath(new URL('../dist/', import.meta.url))
  const { buildId, precache } = await writeServiceWorker(distDir)
  console.log(`service worker ${buildId}: ${precache.length} precached files`)
}

import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const routerPath = resolve(__dirname, '../src/router/index.js')
const homePath = resolve(__dirname, '../src/views/Home.vue')
const layoutPath = resolve(__dirname, '../src/layouts/AppLayout.vue')

test('makes the root route the canonical shelf, search, and explore workspace', () => {
  const router = readFileSync(routerPath, 'utf8')
  const home = readFileSync(homePath, 'utf8')

  assert.doesNotMatch(router, /const Search\s*=\s*\(\)\s*=>/)
  assert.doesNotMatch(router, /const Discover\s*=\s*\(\)\s*=>/)
  assert.match(router, /path:\s*'\/search',[\s\S]*?redirect:\s*to\s*=>[\s\S]*?workspace:\s*'search'/)
  assert.match(router, /path:\s*'\/discover',[\s\S]*?redirect:\s*to\s*=>[\s\S]*?workspace:\s*'explore'/)
  assert.match(home, /<SearchWorkspace\s+v-if="workspace\.isSearchResult"\s+embedded/)
  assert.match(home, /<DiscoverWorkspace\s+v-else-if="workspace\.isExploreResult"\s+embedded/)
  assert.match(home, /workspace\.beginSearch\(/)
  assert.match(home, /workspace\.beginExplore\(/)
  assert.match(home, /workspace\.backToShelf\(\)/)
})

test('sends sidebar search and explore actions into the root workspace without an implicit mobile close', () => {
  const layout = readFileSync(layoutPath, 'utf8')

  assert.match(layout, /useIndexWorkspaceStore/)
  assert.match(layout, /onWorkspaceSearch:\s*beginWorkspaceSearch/)
  assert.match(layout, /function beginWorkspaceSearch\(/)
  assert.match(layout, /function beginWorkspaceExplore\(/)
  assert.match(layout, /\{ key: 'discover',[\s\S]*?action:\s*beginWorkspaceExplore/)
  assert.match(layout, /if \(isMobileShell\.value && item\.closeMobile\)/)
  assert.match(layout, /function withoutWorkspaceQuery\(/)
  assert.match(layout, /router\.replace\(\{ name: 'home', query: withoutWorkspaceQuery\(route\.query\) \}\)/)
})

import assert from 'node:assert/strict'
import { existsSync, readFileSync } from 'node:fs'
import test from 'node:test'

const obsoleteActions = new URL('../src/utils/bookInfoOverlayActions.js', import.meta.url)
const entrySources = [
  '../src/views/Home.vue',
  '../src/views/Search.vue',
  '../src/views/Discover.vue',
  '../src/views/Reader.vue',
  '../src/layouts/AppLayout.vue',
].map(path => [path, readFileSync(new URL(path, import.meta.url), 'utf8')])

test('removes the unreachable contextual BookInfo action strategy', () => {
  assert.equal(
    existsSync(obsoleteActions),
    false,
    'the zero-reference bookInfoOverlayActions module must not survive the shared-overlay convergence',
  )
})

test('keeps every product entry on the one shared BookInfo overlay', () => {
  const obsoleteBuilders = /build(?:BookInfo(?:Read|StartRead)|Search(?:Existing|Add)Book)Actions/
  for (const [path, source] of entrySources) {
    assert.doesNotMatch(source, /bookInfoOverlayActions/, `${path} must not import the retired action strategy`)
    assert.doesNotMatch(source, obsoleteBuilders, `${path} must not rebuild contextual BookInfo action arrays`)
  }
  for (const [path, source] of entrySources.slice(0, 4)) {
    assert.match(source, /overlay\.openBookInfo\(/, `${path} must continue opening the shared BookInfo overlay`)
  }
  assert.match(entrySources.at(-1)[1], /overlay\.openBookInfo\(mergedBook/, 'legacy book links must hydrate the same shared overlay')
})

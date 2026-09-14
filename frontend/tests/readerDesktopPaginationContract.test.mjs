import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const zones = readFileSync(
  new URL('../src/components/reader/ReaderClickZones.vue', import.meta.url),
  'utf8',
)

test('desktop reader click zones keep the upstream 30/40/30 geometry', () => {
  assert.match(zones, /\.tap-left\s*\{[^}]*width:\s*30%;/s)
  assert.match(zones, /\.tap-right\s*\{[^}]*width:\s*30%;/s)
  assert.match(zones, /\.tap-center\s*\{[^}]*top:\s*30%;[^}]*right:\s*30%;[^}]*bottom:\s*30%;[^}]*left:\s*30%;/s)
  assert.match(zones, /\.tap-upper\s*\{[^}]*height:\s*30%;/s)
  assert.match(zones, /\.tap-lower\s*\{[^}]*height:\s*30%;/s)
  assert.match(zones, /grid-template-rows:\s*30% 40% 30%;/)
  assert.match(zones, /grid-template-columns:\s*30% 40% 30%;/)
})

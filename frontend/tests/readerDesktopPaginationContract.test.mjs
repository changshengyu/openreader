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
  // 显示翻页区域 draws reader-dev's halves under a 30%–70% menu box (reader-menu contract D1).
  assert.match(zones, /\.click-zone-menu\s*\{[^}]*top:\s*30%;[^}]*right:\s*30%;[^}]*bottom:\s*30%;[^}]*left:\s*30%;/s)
  assert.match(zones, /\.click-zone-prev\s*\{[^}]*bottom:\s*50%;/s)
  assert.match(zones, /\.click-zone-next\s*\{[^}]*top:\s*50%;/s)
})

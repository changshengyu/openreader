// Contract: docs/compat/browser-document-title-fixed-baseline-second-audit-p0-contract.md section 16 (W19).
// reader-dev's built manifest.json uses display "fullscreen" with no orientation; the user chose to
// match it on 2026-11-13. The name, colours, icons and the iOS head tags stay as they are.
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const read = path => readFileSync(resolve(root, path), 'utf8')
const manifest = () => JSON.parse(read('public/manifest.webmanifest'))

test('M1/M2: the home-screen app opens fullscreen without an orientation lock, like reader-dev', () => {
  const current = manifest()
  assert.equal(current.display, 'fullscreen', 'manifest display')
  assert.equal(Object.hasOwn(current, 'orientation'), false, `manifest must not lock the orientation (found ${current.orientation})`)
})

test('M3/M4: the name, scope and iOS head tags are unchanged', () => {
  const current = manifest()
  assert.equal(current.name, '闲读')
  assert.equal(current.short_name, '闲读')
  assert.equal(current.start_url, '/')
  assert.equal(current.scope, '/')
  const html = read('index.html')
  assert.match(html, /<link rel="manifest" href="\/manifest\.webmanifest" \/>/)
  assert.match(html, /<meta name="apple-mobile-web-app-capable" content="yes" \/>/)
})

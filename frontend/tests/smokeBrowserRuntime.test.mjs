import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import test from 'node:test'

const smokeDirectory = resolve(process.cwd(), '../scripts/smoke')

test('browser smoke scripts use the shared crash-safe Playwright runtime', () => {
  const directLaunchers = readdirSync(smokeDirectory)
    .filter(name => name.endsWith('.mjs') && name !== 'playwright-runtime.mjs')
    .filter(name => /chromium\.launch\s*\(/.test(
      readFileSync(resolve(smokeDirectory, name), 'utf8'),
    ))

  assert.deepEqual(
    directLaunchers,
    [],
    `smoke scripts must not repeatedly launch macOS Google Chrome: ${directLaunchers.join(', ')}`,
  )
})

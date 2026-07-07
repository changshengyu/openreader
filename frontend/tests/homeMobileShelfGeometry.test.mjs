import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

const __dirname = dirname(fileURLToPath(import.meta.url))
const homePath = resolve(__dirname, '../src/views/Home.vue')
const source = readFileSync(homePath, 'utf8')

test('locks mobile shelf title and group geometry to upstream Index spacing', () => {
  assert.match(source, /\.shelf-page\.mobile-shelf \.shelf-title\s*\{[\s\S]*padding:\s*20px 24px 0;/)
  assert.match(source, /\.shelf-page\.mobile-shelf \.shelf-title strong\s*\{[\s\S]*font-size:\s*20px;/)
  assert.match(source, /\.shelf-page\.mobile-shelf \.book-group-wrapper\s*\{[\s\S]*margin-right:\s*24px;[\s\S]*margin-left:\s*24px;/)
})

test('locks mobile shelf row and cover geometry to upstream Index spacing', () => {
  assert.match(source, /\.shelf-page\.mobile-shelf \.book-row\s*\{[\s\S]*grid-template-columns:\s*84px minmax\(0, 1fr\);[\s\S]*gap:\s*20px;[\s\S]*padding:\s*10px 20px;/)
  assert.match(source, /\.shelf-page\.mobile-shelf \.list-cover\s*\{[\s\S]*width:\s*84px;[\s\S]*height:\s*112px;/)
  assert.match(source, /\.shelf-page\.mobile-shelf \.list-main\s*\{[\s\S]*min-height:\s*112px;/)
})

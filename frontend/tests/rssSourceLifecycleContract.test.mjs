import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const manager = readFileSync(new URL('../src/components/RSSManager.vue', import.meta.url), 'utf8')

test('RSS source editor requires both the upstream name and URL identities', () => {
  assert.match(
    manager,
    /async function saveSource\(\)\s*\{\s*if \(!draft\.value\.title\.trim\(\)\)\s*\{\s*ElMessage\.warning\('RSS 源名称不能为空'\)\s*return\s*\}\s*if \(!draft\.value\.url\.trim\(\)\)/,
  )
})

test('RSS manual and imported singleUrl defaults remain distinct like upstream', () => {
  assert.match(
    manager,
    /function openEditor\(source = null\)\s*\{\s*const normalizedSource = source \|\| \{\}[\s\S]*?\.\.\.pickRSSAdvancedFields\(normalizedSource\)/,
    'manual create must normalize the upstream falsy new-source sentinel before reading advanced fields',
  )
  assert.match(manager, /pickRSSAdvancedFields\(source, \{ singleURLDefault: false \}\)/)
  assert.match(manager, /function pickRSSAdvancedFields\(source = \{\}, \{ singleURLDefault = true \} = \{\}\)/)
})

test('RSS list and load-more responses are committed through request ownership gates', () => {
  assert.match(manager, /const articleListRequestGate = createRSSArticleRequestGate\(\)/)
  assert.match(manager, /const articleLoadMoreRequestGate = createRSSArticleRequestGate\(\)/)
  assert.match(manager, /function resetSourceArticleState[\s\S]*?invalidateArticleRequests\(\)/)
  assert.match(manager, /async function loadArticles\(parentOperation = null\)[\s\S]*?operations\.canCommit\([\s\S]*?articleListRequestGate\.begin\([\s\S]*?articleListRequestGate\.isCurrent\(/)
  assert.match(manager, /async function loadMoreArticles\(\)[\s\S]*?articleLoadMoreRequestGate\.begin\([\s\S]*?articleLoadMoreRequestGate\.isCurrent\(/)
  assert.match(manager, /async function handleFilterChange\(\)[\s\S]*?resetSourceArticleState\(\)[\s\S]*?await loadArticles\(operation\)/)
  assert.match(manager, /async function selectSource\(sourceId\)[\s\S]*?const query = articleRequestQuery\(1\)[\s\S]*?await loadArticles\(operation\)[\s\S]*?!operations\.canCommit\(operation\)[\s\S]*?!isArticleRequestQueryCurrent\(query\)[\s\S]*?await refreshSelectedSource\(operation\)/)
  assert.match(manager, /async function handleSortChange\(\)[\s\S]*?const query = articleRequestQuery\(1\)[\s\S]*?!operations\.canCommit\(operation\)[\s\S]*?!isArticleRequestQueryCurrent\(query\)[\s\S]*?await refreshSelectedSource\(operation\)/)
})

import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const manager = readFileSync(new URL('../src/components/RSSManager.vue', import.meta.url), 'utf8')
const overlay = readFileSync(new URL('../src/components/overlays/OverlayRSS.vue', import.meta.url), 'utf8')

test('aligns RSS source-open and close transitions with the upstream modal chain', () => {
  assert.match(overlay, /<el-dialog/, 'the upstream RSS source scene must use a centred dialog')
  assert.doesNotMatch(overlay, /<el-drawer/, 'the root RSS source scene must not remain a drawer')
  assert.match(overlay, /class="global-rss-dialog"/, 'the root source dialog needs a stable browser contract hook')
  assert.match(overlay, /:fullscreen="isMobile"/, 'the root source dialog must be fullscreen on compact screens')
  assert.match(overlay, /destroy-on-close/, 'closing the root RSS dialog must recreate its source state')
  assert.match(overlay, /<RSSManager\s+:is-mobile="isMobile"\s+:visible="overlay\.rssVisible"\s*\/>/, 'the canonical RSS manager must receive root overlay visibility')
  assert.match(manager, /visible:\s*\{[\s\S]*?type:\s*Boolean/, 'RSSManager must own an explicit root-visible lifecycle')
  assert.match(manager, /const articleListDialogVisible\s*=\s*ref\(false\)/, 'the article list needs an independent dialog state')
  assert.match(manager, /<el-dialog\s+v-model="articleListDialogVisible"[\s\S]*?class="rss-article-list-dialog"/, 'source selection must open a distinct upstream-style article-list dialog')
  assert.match(manager, /<el-dialog\s+v-model="articleDialogVisible"[\s\S]*?class="rss-article-content-dialog"/, 'article content must remain a distinct dialog from the article list')
  assert.match(manager, /watch\(\(\) => props\.visible,[\s\S]*?resetRSSWorkspace\(\)/, 'root close must reset transient RSS scene state')
  assert.doesNotMatch(manager, /async function openRSSWorkspace\(\)\s*\{[\s\S]*?await selectSource\(/, 'opening the source dialog must not skip straight to the article-list dialog')
  assert.match(manager, /watch\(articleListDialogVisible,[\s\S]*?resetSourceArticleState\(\{ resetSort: true \}\)/, 'closing the article-list dialog must clear only article-list transient state')
  assert.match(manager, /async function selectSource\(sourceId\)\s*\{[\s\S]*?resetSourceArticleState\([^)]*\)[\s\S]*?articleListDialogVisible\.value = true[\s\S]*?await refreshSelectedSource\(\)/, 'source selection must reset stale article state, open the list dialog, and refresh the selected source like upstream')
  assert.match(manager, /async function handleSortChange\(\)\s*\{[\s\S]*?resetSourceArticleState\(\)[\s\S]*?await loadArticles\(\)[\s\S]*?await refreshSelectedSource\(\)/, 'sort changes must reset the old page, show scoped cache, then refresh page one')
  assert.match(manager, /async function refreshSource\(source\)[\s\S]*?await loadArticles\(\)/, 'a selected-source refresh must render the refreshed/cached rows')
  assert.match(manager, /function resetRSSWorkspace\(\)[\s\S]*?articleListDialogVisible\.value = false[\s\S]*?resetSourceArticleState\(\)/, 'root reset must close the list dialog and delegate article-state cleanup')
  assert.match(manager, /function resetSourceArticleState\([^)]*\)[\s\S]*?articleDialogVisible\.value = false[\s\S]*?articleImagePreviewVisible\.value = false/, 'source cleanup must close article and image overlays')
  assert.match(manager, /function applyRSSSources\(data\)\s*\{\s*if \(!props\.visible\) return/, 'a source request finishing after root close must not restore stale state')
  assert.match(manager, /async function loadArticles\(\)[\s\S]*?if \(!props\.visible\) return[\s\S]*?articles\.value = result\.items/, 'an article request finishing after root close must not restore stale rows')
})

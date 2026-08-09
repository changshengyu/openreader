import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const manager = readFileSync(new URL('../src/components/RSSManager.vue', import.meta.url), 'utf8')
const overlay = readFileSync(new URL('../src/components/overlays/OverlayRSS.vue', import.meta.url), 'utf8')

test('RSS root is the fixed-baseline 500px/fullscreen source dialog', () => {
  assert.doesNotMatch(overlay, /<el-dialog|<el-drawer/, 'OverlayRSS must not add a second shell around the sibling RSS dialogs')
  assert.match(manager, /class="global-rss-dialog"/)
  assert.match(manager, /width="500px"/)
  assert.match(manager, /:fullscreen="isMobile"/)
  assert.match(manager, /RSS订阅\(\{\{ sources\.length \}\}\)/)
  assert.match(manager, />新增</)
  assert.match(manager, />导入</)
  assert.match(manager, /\{\{ rssEditMode \? '取消' : '编辑' \}\}/)
  assert.doesNotMatch(manager, />刷新</)
})

test('RSS source grid is icon/name-only and edit mode uses overlay actions', () => {
  assert.match(manager, /<RSSSourceGrid/)
  assert.doesNotMatch(manager, /source\.group/)
  assert.doesNotMatch(manager, /source\.enabled/)
  assert.doesNotMatch(manager, /rss-source-card/)
  assert.doesNotMatch(manager, /暂无 RSS 源/)
})

test('RSS article list and content remain independent fixed-baseline dialogs', () => {
  assert.match(manager, /<RSSArticleListDialog/)
  assert.match(manager, /<RSSArticleDialog/)
  assert.match(manager, /const articleListDialogVisible\s*=\s*ref\(false\)/)
  assert.match(manager, /const articleDialogVisible\s*=\s*ref\(false\)/)
  assert.match(manager, /watch\(\(\) => props\.visible,[\s\S]*?resetRSSWorkspace\(\)/)
  assert.match(manager, /watch\(articleListDialogVisible,[\s\S]*?resetSourceArticleState/)
  assert.match(manager, /async function openArticle\(article\)[\s\S]*?await getRSSArticleContent[\s\S]*?articleDialogVisible\.value = true/,
    'content must finish before the article dialog opens')
})

test('the fixed-baseline visible scene does not expose OpenReader-only article controls', () => {
  for (const text of ['全部', '未读', '收藏', '标已读', '标未读', '刷新文章', '打开原文', '未知作者', '无摘要']) {
    assert.doesNotMatch(manager, new RegExp(text), `unexpected visible RSS control: ${text}`)
  }
})

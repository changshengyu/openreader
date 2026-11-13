import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

// Contract: docs/compat/browser-document-title-fixed-baseline-second-audit-p0-contract.md section 14
// (D1: the user named the product 闲读 on 2026-11-12).
const PRODUCT_NAME = '闲读'
const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const read = path => readFileSync(resolve(root, path), 'utf8')

async function loadDocumentTitle() {
  return import('../src/utils/documentTitle.js')
}

test('the app names itself 闲读 in the tab, home-screen, icon, sidebar and login heading', () => {
  const html = read('index.html')
  assert.equal(html.match(/<title>([^<]*)<\/title>/i)?.[1]?.trim(), PRODUCT_NAME, 'index.html <title>')
  assert.match(html, /<meta name="apple-mobile-web-app-title" content="闲读" \/>/, 'apple-mobile-web-app-title')
  const manifest = JSON.parse(read('public/manifest.webmanifest'))
  assert.equal(manifest.name, PRODUCT_NAME, 'manifest name')
  assert.equal(manifest.short_name, PRODUCT_NAME, 'manifest short_name')
  assert.match(read('public/openreader.svg'), /aria-label="闲读"/, 'icon aria-label')
  assert.match(read('src/layouts/AppLayout.vue'), /<div class="app-brand-title">闲读<\/div>/, 'sidebar brand')
  assert.match(read('src/views/Login.vue'), /<h1 class="auth-title">闲读<\/h1>/, 'login heading')
})

test('technical names keep OpenReader', () => {
  // Storage keys, font families and export prefixes are identifiers, not the display name.
  // The 记住登录信息 slice moved the login token key into utils/authToken.js, its only reader and writer.
  assert.match(read('src/utils/authToken.js'), /export const AUTH_TOKEN_KEY = 'openreader_token'/, 'login token storage key')
  assert.match(read('src/utils/readerFonts.js'), /customFamily: 'OpenReaderCustomSystem'/)
  assert.match(read('src/views/SourceDebug.vue'), /OpenReader-source-debug-/)
})

test('the shell and source-debug titles follow the product name', async () => {
  const titles = await loadDocumentTitle()
  assert.equal(titles.SHELL_DOCUMENT_TITLE, PRODUCT_NAME)
  assert.equal(titles.SOURCE_DEBUG_DOCUMENT_TITLE, '闲读书源编辑器', 'reader-dev titles its source editor <产品名>书源编辑器')
  assert.equal(typeof titles.restoreShellDocumentTitle, 'function', 'restoreShellDocumentTitle must be exported')
  const previous = globalThis.document
  globalThis.document = { title: '某书 - 第一章' }
  try {
    titles.restoreShellDocumentTitle()
    assert.equal(globalThis.document.title, PRODUCT_NAME)
  } finally {
    if (previous === undefined) delete globalThis.document
    else globalThis.document = previous
  }
})

test('home, login and source debug set their tab titles through the single writer on mount', () => {
  const layout = read('src/layouts/AppLayout.vue')
  assert.match(layout, /import \{ restoreShellDocumentTitle \} from '\.\.\/utils\/documentTitle(\.js)?'/)
  assert.match(layout, /onMounted\(restoreShellDocumentTitle\)/, 'the home workspace sets the shell title when it mounts (S2)')

  const login = read('src/views/Login.vue')
  assert.match(login, /import \{ restoreShellDocumentTitle \} from '\.\.\/utils\/documentTitle(\.js)?'/)
  assert.match(login, /onMounted\(restoreShellDocumentTitle\)/, 'the login page sets the shell title when it mounts (S3)')

  const sourceDebug = read('src/views/SourceDebug.vue')
  assert.match(sourceDebug, /import \{ SOURCE_DEBUG_DOCUMENT_TITLE, restoreShellDocumentTitle, setDocumentTitle \} from '\.\.\/utils\/documentTitle(\.js)?'/)
  assert.match(sourceDebug, /onMounted\(\(\) => setDocumentTitle\(SOURCE_DEBUG_DOCUMENT_TITLE\)\)/, 'source debug sets its own title (T1)')
  assert.match(sourceDebug, /onBeforeUnmount\(restoreShellDocumentTitle\)/, 'source debug restores the shell title when it unmounts (T1)')
})
